package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/agent-sandbox/incus-dispatcher/queue/laneqpb"
)

// LaneqQueue is a gRPC adapter that implements Queue over the laneq client.
// It is a drop-in replacement for MemoryQueue.
//
// Storage split:
//   - Scheduling fields (priority, not_before_unix, lease_until_unix, requeue_count, etc.)
//     are laneq columns.
//   - The rich Directive (Intent, Template, Origin, Repo, Ref, Task, Grade, HandoffIn,
//     Deadline, MaxAttempts) is JSON-marshaled into laneq's opaque body field.
//
// Lane policy: LaneqQueue is configured with a single lane at construction (default="default").
// All Claim/Peek operations use this lane. Multi-lane fan-out is an ITER-0008 extension point
// (per-lane LaneqQueue instances or a lane-aware extension).
type LaneqQueue struct {
	client laneqpb.LaneqClient
	conn   *grpc.ClientConn // owned by LaneqQueue if non-nil; must be closed on shutdown
	lane   string
}

// Compile-time check: LaneqQueue must implement the Queue interface.
var _ Queue = (*LaneqQueue)(nil)

// NewLaneqQueue returns a new LaneqQueue using the provided gRPC client and lane.
// If lane is empty, "default" is used.
// Deprecated: use NewLaneqQueueWithConn for production code (enables proper conn cleanup).
// This constructor is retained for testing with mock clients.
func NewLaneqQueue(client laneqpb.LaneqClient, lane string) *LaneqQueue {
	if lane == "" {
		lane = "default"
	}
	return &LaneqQueue{
		client: client,
		conn:   nil,
		lane:   lane,
	}
}

// NewLaneqQueueWithConn returns a new LaneqQueue using the provided gRPC connection and lane.
// If lane is empty, "default" is used.
// The LaneqQueue takes ownership of the connection; the caller must not close it.
// On daemon shutdown, Close() must be called to release the connection.
func NewLaneqQueueWithConn(conn *grpc.ClientConn, lane string) *LaneqQueue {
	if lane == "" {
		lane = "default"
	}
	return &LaneqQueue{
		client: laneqpb.NewLaneqClient(conn),
		conn:   conn,
		lane:   lane,
	}
}

// importanceToProto converts queue.Importance to laneqpb.Priority.
func importanceToProto(imp Importance) laneqpb.Priority {
	switch imp {
	case ImportanceHigh:
		return laneqpb.Priority_PRIORITY_P0
	case ImportanceLow:
		return laneqpb.Priority_PRIORITY_P2
	default:
		return laneqpb.Priority_PRIORITY_P1
	}
}

// protoToImportance converts laneqpb.Priority back to queue.Importance.
func protoToImportance(p laneqpb.Priority) Importance {
	switch p {
	case laneqpb.Priority_PRIORITY_P0:
		return ImportanceHigh
	case laneqpb.Priority_PRIORITY_P2:
		return ImportanceLow
	default:
		return ImportanceNormal
	}
}

// Push enqueues a directive, assigning an ID if empty. Returns the ID.
func (q *LaneqQueue) Push(d Directive) (string, error) {
	if d.ID == "" {
		// Let laneq assign the ID.
		d.ID = ""
	}
	if d.Importance == "" {
		d.Importance = ImportanceNormal
	}

	// Marshal the full directive to JSON for the opaque body.
	body, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("push: marshal directive: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := q.client.Push(ctx, &laneqpb.PushRequest{
		Body:     string(body),
		Priority: importanceToProto(d.Importance),
		Lane:     q.lane,
	})
	if err != nil {
		return "", fmt.Errorf("push: gRPC error: %w", err)
	}

	return resp.Id, nil
}

// Claim atomically reserves the highest-priority ELIGIBLE (NotBefore <= now)
// pending directive and returns it with a Lease. Returns ErrEmpty if none.
func (q *LaneqQueue) Claim(consumer string, leaseDur time.Duration) (Directive, Lease, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := q.client.Take(ctx, &laneqpb.TakeRequest{
		Consumer:         consumer,
		Lane:             q.lane,
		LeaseDurationMs:  int64(leaseDur.Milliseconds()),
		ReapStaleSeconds: 0, // No auto-reap here.
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return Directive{}, Lease{}, ErrEmpty
		}
		return Directive{}, Lease{}, fmt.Errorf("claim: gRPC error: %w", err)
	}

	// If the response directive is empty/nil, return ErrEmpty.
	if resp.Directive == nil || resp.Directive.Id == "" {
		return Directive{}, Lease{}, ErrEmpty
	}

	// Unmarshal the body back to a rich Directive and overlay laneq columns.
	d, lease, err := q.directiveFromProto(resp.Directive, consumer)
	if err != nil {
		return Directive{}, Lease{}, fmt.Errorf("claim: unmarshal body: %w", err)
	}

	return d, lease, nil
}

// directiveFromProto reconstructs a Directive from a proto message,
// overlaying laneq's scheduling columns. Uses strict JSON decoding to reject
// unknown fields and ensure body integrity.
func (q *LaneqQueue) directiveFromProto(pb *laneqpb.Directive, consumer string) (Directive, Lease, error) {
	// Unmarshal the opaque body JSON with strict field checking.
	// This rejects any unknown fields, ensuring the body is a valid Directive.
	d, err := ParseDirective([]byte(pb.Body))
	if err != nil {
		return Directive{}, Lease{}, fmt.Errorf("unmarshal body: %w", err)
	}

	// Overlay laneq column values.
	d.ID = pb.Id
	d.Importance = protoToImportance(pb.Priority)
	d.Attempts = int(pb.RequeueCount)
	d.Lane = pb.Lane

	// Unpack optional not_before_unix (seconds -> time.Time).
	if pb.NotBeforeUnix != nil {
		d.NotBefore = time.Unix(*pb.NotBeforeUnix, 0)
	} else {
		d.NotBefore = time.Time{}
	}

	// Build the lease from the proto's taken_by and lease_until_unix.
	var lease Lease
	lease.DirectiveID = pb.Id
	lease.Token = pb.TakenBy // Use consumer ID as the token (no separate opaque token in laneq).
	// DIVERGENCE (real-wire, SCENARIO-0092): the real laneq server does NOT enforce
	// per-consumer token ownership on Touch/Done — leases are keyed by directive id, so a
	// different consumer can operate on a held directive (the in-process fake is stricter).
	// Leases are therefore NOT consumer-exclusive on the real substrate. ITER-0008
	// (multi-consumer / recursive delegation / work-stealing) MUST NOT assume exclusivity;
	// add an opaque per-claim token upstream in laneq if exclusivity is required.

	if pb.LeaseUntilUnix != nil {
		lease.Expiry = time.Unix(*pb.LeaseUntilUnix, 0)
	} else {
		// This shouldn't happen for a claimed directive, but be defensive.
		lease.Expiry = time.Now()
	}

	return d, lease, nil
}

// Touch renews a lease. Returns ErrLeaseLost if expired/unknown.
func (q *LaneqQueue) Touch(lease Lease, leaseDur time.Duration) (Lease, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := q.client.Touch(ctx, &laneqpb.TouchRequest{
		Id:              lease.DirectiveID,
		Consumer:        lease.Token,
		LeaseDurationMs: int64(leaseDur.Milliseconds()),
	})
	if err != nil {
		if status.Code(err) == codes.NotFound || status.Code(err) == codes.FailedPrecondition {
			return Lease{}, ErrLeaseLost
		}
		return Lease{}, fmt.Errorf("touch: gRPC error: %w", err)
	}

	// Unpack the new lease_until_unix (seconds, not milliseconds).
	var newExpiry time.Time
	if resp.LeaseUntilUnix != nil {
		newExpiry = time.Unix(*resp.LeaseUntilUnix, 0)
	} else {
		// Server returned no lease; directive is no longer claimed.
		return Lease{}, ErrLeaseLost
	}

	return Lease{
		DirectiveID: lease.DirectiveID,
		Token:       lease.Token,
		Expiry:      newExpiry,
	}, nil
}

// Done marks a claimed directive complete and removes it.
func (q *LaneqQueue) Done(lease Lease) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := q.client.SetStatus(ctx, &laneqpb.SetStatusRequest{
		Id:     lease.DirectiveID,
		Status: laneqpb.Status_STATUS_DONE,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound || status.Code(err) == codes.FailedPrecondition {
			return ErrLeaseLost
		}
		return fmt.Errorf("done: gRPC error: %w", err)
	}

	return nil
}

// Requeue returns a claimed directive to pending, incrementing Attempts and
// setting its NotBefore (zero = immediately eligible).
//
// Note on requeue_count semantics (T1 fork requirement):
// The laneq gRPC server MUST increment requeue_count on the requeue path
// (SetStatus→PENDING) and on reap, so that Attempts tracking matches the
// MemoryQueue stub. Stock laneq only increments on reap. This adapter assumes
// the nnunley/laneq fork implements the required increment behavior; see
// SCENARIO-0092 (T1 verification).
func (q *LaneqQueue) Requeue(lease Lease, notBefore time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if notBefore.IsZero() {
		// Immediately eligible: SetStatus(PENDING).
		_, err := q.client.SetStatus(ctx, &laneqpb.SetStatusRequest{
			Id:     lease.DirectiveID,
			Status: laneqpb.Status_STATUS_PENDING,
		})
		if err != nil {
			if status.Code(err) == codes.NotFound || status.Code(err) == codes.FailedPrecondition {
				return ErrLeaseLost
			}
			return fmt.Errorf("requeue: SetStatus error: %w", err)
		}
	} else {
		// Deferred: Defer(id, until=notBefore).
		_, err := q.client.Defer(ctx, &laneqpb.DeferRequest{
			Id:        lease.DirectiveID,
			UntilUnix: ptrInt64(notBefore.Unix()),
		})
		if err != nil {
			if status.Code(err) == codes.NotFound || status.Code(err) == codes.FailedPrecondition {
				return ErrLeaseLost
			}
			return fmt.Errorf("requeue: Defer error: %w", err)
		}
	}

	return nil
}

// ptrInt64 is a helper to create a pointer to an int64.
func ptrInt64(v int64) *int64 {
	return &v
}

// Peek returns the directive Claim would return next — the highest-priority
// eligible (NotBefore <= now) pending directive — without claiming it.
// No lease is created and the queue is not mutated. Returns ErrEmpty if
// no eligible pending directive exists.
func (q *LaneqQueue) Peek() (Directive, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := q.client.Peek(ctx, &laneqpb.PeekRequest{
		Lane: q.lane,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return Directive{}, ErrEmpty
		}
		return Directive{}, fmt.Errorf("peek: gRPC error: %w", err)
	}

	// If the response directive is empty/nil, return ErrEmpty.
	if resp.Directive == nil || resp.Directive.Id == "" {
		return Directive{}, ErrEmpty
	}

	// Unmarshal the body and overlay columns (no lease for Peek).
	d, _, err := q.directiveFromProto(resp.Directive, "")
	if err != nil {
		return Directive{}, fmt.Errorf("peek: unmarshal body: %w", err)
	}

	return d, nil
}

// Park moves a CLAIMED directive (held by lease) into a DURABLE parked hold state.
// Returns ErrLeaseLost if the lease is expired/unknown.
func (q *LaneqQueue) Park(lease Lease) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := q.client.Park(ctx, &laneqpb.ParkRequest{
		Id:       lease.DirectiveID,
		Consumer: lease.Token,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound || status.Code(err) == codes.FailedPrecondition {
			return ErrLeaseLost
		}
		return fmt.Errorf("park: gRPC error: %w", err)
	}

	return nil
}

// Reprioritize changes the priority of a directive by ID.
// Used by Temporal workflows to update directive priority based on urgency changes.
// Returns an error if the directive is not found or if the RPC fails.
func (q *LaneqQueue) Reprioritize(id string, importance Importance) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := q.client.Reprioritize(ctx, &laneqpb.ReprioritizeRequest{
		Id:       id,
		Priority: importanceToProto(importance),
	})
	if err != nil {
		return fmt.Errorf("reprioritize: gRPC error: %w", err)
	}

	return nil
}

// Defer sets the not-before eligibility time for a directive by ID (lease-free).
// Used by Temporal workflows to make a directive eligible (by setting notBefore to now
// or earlier) or defer it until a future time.
// Unlike Requeue, this does NOT require a lease (no consumer context).
// Returns an error if the directive is not found or if the RPC fails.
//
// This is the SOLE public path for Temporal to update not-before; it mirrors the
// Defer RPC call inside Requeue (laneq.go:278-289) but is exposed as a lease-free
// public method for the ReprojectActivity sole-writer seam (STORY-0044 AC-3).
// DeferDirective (queue.Queue interface) returns a claimed directive to pending with Attempts PRESERVED.
// Uses the underlying Defer(id, notBefore) from the Reprojector interface.
func (q *LaneqQueue) DeferDirective(lease Lease, notBefore time.Time) error {
	return q.Defer(lease.DirectiveID, notBefore)
}

// Defer is the laneq Reprojector method that defers a directive by ID.
func (q *LaneqQueue) Defer(id string, notBefore time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := q.client.Defer(ctx, &laneqpb.DeferRequest{
		Id:        id,
		UntilUnix: ptrInt64(notBefore.Unix()),
	})
	if err != nil {
		return fmt.Errorf("defer: gRPC error: %w", err)
	}

	return nil
}

// Reap reclaims expired leases (requeues them). Returns the count reclaimed.
func (q *LaneqQueue) Reap() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := q.client.Reap(ctx, &laneqpb.ReapRequest{
		ExpiredLeases: true,
		StaleSeconds:  0,
	})
	if err != nil {
		return 0, fmt.Errorf("reap: gRPC error: %w", err)
	}

	return int(resp.Reclaimed), nil
}

// Len reports pending + claimed directive counts (for tests/observability).
// This stub implementation always returns (0, 0) as laneq doesn't expose
// a direct count endpoint. A full implementation would use Stats or similar.
//
// TODO(backlog): Implement via Stats() RPC when needed for observability. NOT a committed
// ITER-0008b-scope AC and not required by any iteration gate (the live time-plane proofs in
// ITER-0007b E1 don't depend on it); a non-blocking observability enhancement deferred beyond
// ITER-0008b. Current Len() is a documented stub, not load-bearing.
func (q *LaneqQueue) Len() (pending, claimed int) {
	return 0, 0
}

// Close gracefully closes the underlying gRPC connection.
// Called during daemon shutdown to ensure clean resource cleanup.
// Safe to call multiple times (grpc.ClientConn.Close is idempotent).
func (q *LaneqQueue) Close() error {
	if q.conn != nil {
		return q.conn.Close()
	}
	return nil
}
