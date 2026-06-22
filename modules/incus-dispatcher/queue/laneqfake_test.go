package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/agent-sandbox/incus-dispatcher/queue/laneqpb"
)

// fakeLaneqServer implements laneqpb.LaneqServer with faithful state machine semantics.
// Used by integration tests (SCENARIO-0091) to validate the LaneqQueue adapter.
type fakeLaneqServer struct {
	laneqpb.UnimplementedLaneqServer

	mu         sync.Mutex
	now        func() time.Time              // Controllable clock for testing
	directives map[string]*fakeDirective    // by ID
	seq        int                           // for auto-ID generation
	parked     map[string]bool               // parked directives (excluded from claim/peek/reap)
}

// fakeDirective models the laneq directive state machine.
type fakeDirective struct {
	Id              string
	Priority        laneqpb.Priority
	Body            string
	Status          laneqpb.Status
	Lane            string
	CreatedAtUnix   int64
	TakenAtUnix     *int64 // optional
	DoneAtUnix      *int64 // optional
	TakenBy         string
	LeaseUntilUnix  *int64 // optional; presence indicates active lease
	RequeueCount    int32
	ParentId        string
	NotBeforeUnix   *int64    // optional; nil = always eligible
	BlockedBy       []string  // dependency IDs; empty = no blocking
}

// newFakeLaneqServer creates a fake server with the given clock.
func newFakeLaneqServer(now func() time.Time) *fakeLaneqServer {
	return &fakeLaneqServer{
		now:         now,
		directives:  make(map[string]*fakeDirective),
		parked:      make(map[string]bool),
	}
}

// newID generates a unique directive ID.
func (s *fakeLaneqServer) newID() string {
	s.seq++
	return fmt.Sprintf("d-%d", s.seq)
}

// Push enqueues a directive.
func (s *fakeLaneqServer) Push(ctx context.Context, req *laneqpb.PushRequest) (*laneqpb.PushResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.newID()
	now := s.now()

	fd := &fakeDirective{
		Id:            id,
		Priority:      req.Priority,
		Body:          req.Body,
		Status:        laneqpb.Status_STATUS_PENDING,
		Lane:          req.Lane,
		CreatedAtUnix: now.Unix(),
		TakenBy:       "",
		RequeueCount:  0,
		ParentId:      req.ParentId,
		BlockedBy:     []string{},
	}
	if fd.Lane == "" {
		fd.Lane = "default"
	}
	if fd.Priority == laneqpb.Priority_PRIORITY_UNSPECIFIED {
		fd.Priority = laneqpb.Priority_PRIORITY_P1
	}

	s.directives[id] = fd

	return &laneqpb.PushResponse{
		Id:       id,
		Priority: fd.Priority,
		Lane:     fd.Lane,
		ParentId: req.ParentId,
		Status:   laneqpb.Status_STATUS_PENDING,
		Summary:  fmt.Sprintf("enqueued in lane '%s'", fd.Lane),
	}, nil
}

// reclaimExpiredAndPromoteDeferred performs the shared state maintenance for Take and Peek:
// 1. Reclaim expired leases (status=pending, requeue_count++).
// 2. Promote deferred directives whose not_before has passed AND all blocked_by deps are terminal.
// This helper ensures Take and Peek both see the same queue state.
func (s *fakeLaneqServer) reclaimExpiredAndPromoteDeferred(lane string, now time.Time) {
	// Step 1: Reclaim expired leases.
	for _, fd := range s.directives {
		if fd.Status == laneqpb.Status_STATUS_TAKEN && fd.LeaseUntilUnix != nil && *fd.LeaseUntilUnix <= now.Unix() {
			fd.Status = laneqpb.Status_STATUS_PENDING
			fd.TakenBy = ""
			fd.LeaseUntilUnix = nil
			fd.RequeueCount++
		}
	}

	// Step 2: Promote deferred directives whose not_before has passed AND all blocked_by deps are terminal.
	for _, fd := range s.directives {
		if fd.Status == laneqpb.Status_STATUS_DEFERRED && fd.Lane == lane {
			// Check not_before eligibility.
			eligible := true
			if fd.NotBeforeUnix != nil && *fd.NotBeforeUnix > now.Unix() {
				eligible = false
			}

			// Check blocked_by dependencies.
			if eligible {
				for _, depID := range fd.BlockedBy {
					depFd, ok := s.directives[depID]
					if !ok {
						// Blocking dependency doesn't exist; treat as unblocked.
						continue
					}
					if depFd.Status != laneqpb.Status_STATUS_DONE && depFd.Status != laneqpb.Status_STATUS_DROPPED {
						eligible = false
						break
					}
				}
			}

			if eligible {
				fd.Status = laneqpb.Status_STATUS_PENDING
				fd.NotBeforeUnix = nil
				fd.BlockedBy = []string{}
			}
		}
	}
}

// Take atomically claims the highest-priority eligible pending directive.
// Semantics:
// 1. Reclaim expired leases (status=pending, requeue_count++).
// 2. Promote deferred directives whose not_before has passed AND all blocked_by deps are terminal.
// 3. Pick status=pending, lowest priority value, lowest id (FIFO).
// 4. Set taken/taken_by/lease_until.
// 5. Return directive or (nil, ErrEmpty).
func (s *fakeLaneqServer) Take(ctx context.Context, req *laneqpb.TakeRequest) (*laneqpb.TakeResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	lane := req.Lane
	if lane == "" {
		lane = "default"
	}

	// Perform shared state maintenance.
	s.reclaimExpiredAndPromoteDeferred(lane, now)

	// Step 3: Find best pending directive in this lane (not parked).
	var best *fakeDirective
	var bestID string

	for id, fd := range s.directives {
		if fd.Lane != lane || fd.Status != laneqpb.Status_STATUS_PENDING || s.parked[id] {
			continue
		}

		// Select: lowest priority value (P0 < P1 < P2), ties broken by ID (FIFO).
		if best == nil || fd.Priority < best.Priority || (fd.Priority == best.Priority && id < bestID) {
			best = fd
			bestID = id
		}
	}

	if best == nil {
		return &laneqpb.TakeResponse{Directive: nil, Consumer: req.Consumer, Lane: lane}, nil
	}

	// Step 4: Claim the directive.
	best.Status = laneqpb.Status_STATUS_TAKEN
	best.TakenAtUnix = &[]int64{now.Unix()}[0]
	best.TakenBy = req.Consumer
	leaseUntil := now.Add(time.Duration(req.LeaseDurationMs) * time.Millisecond).Unix()
	best.LeaseUntilUnix = &leaseUntil

	return &laneqpb.TakeResponse{
		Directive: s.fdToProto(best),
		Consumer:  req.Consumer,
		Lane:      lane,
	}, nil
}

// Peek returns the directive Take would return next, without claiming or mutating it.
// Peek performs the same reclaim-expired-leases and promote-deferred maintenance as Take
// to ensure faithful state observation. The only difference from Take is that Peek does
// NOT claim/lease the selected directive—it returns it read-only.
func (s *fakeLaneqServer) Peek(ctx context.Context, req *laneqpb.PeekRequest) (*laneqpb.PeekResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	lane := req.Lane
	if lane == "" {
		lane = "default"
	}

	// Perform shared state maintenance (same as Take).
	s.reclaimExpiredAndPromoteDeferred(lane, now)

	// Find best pending directive in this lane (not parked).
	var best *fakeDirective
	var bestID string

	for id, fd := range s.directives {
		if fd.Lane != lane || fd.Status != laneqpb.Status_STATUS_PENDING || s.parked[id] {
			continue
		}

		// Select: lowest priority value, ties broken by ID (FIFO).
		if best == nil || fd.Priority < best.Priority || (fd.Priority == best.Priority && id < bestID) {
			best = fd
			bestID = id
		}
	}

	if best == nil {
		return &laneqpb.PeekResponse{Directive: nil}, nil
	}

	// Return the directive read-only (no consumer token assigned; Peek does not lease).
	return &laneqpb.PeekResponse{Directive: s.fdToProto(best)}, nil
}

// SetStatus changes a directive's status (pending, done, dropped).
// For SetStatus(PENDING), increment requeue_count (T1 fork requirement).
// SetStatus to DONE or DROPPED requires the directive to be in TAKEN status.
func (s *fakeLaneqServer) SetStatus(ctx context.Context, req *laneqpb.SetStatusRequest) (*laneqpb.SetStatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fd, ok := s.directives[req.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "directive not found")
	}

	now := s.now()

	switch req.Status {
	case laneqpb.Status_STATUS_DONE:
		// Only allow DONE if currently TAKEN (has an active lease).
		if fd.Status != laneqpb.Status_STATUS_TAKEN {
			return nil, status.Error(codes.FailedPrecondition, "directive is not taken")
		}
		fd.Status = laneqpb.Status_STATUS_DONE
		fd.DoneAtUnix = &[]int64{now.Unix()}[0]
		fd.TakenBy = ""
		fd.LeaseUntilUnix = nil
	case laneqpb.Status_STATUS_PENDING:
		// Requeue: must be TAKEN to requeue.
		if fd.Status != laneqpb.Status_STATUS_TAKEN {
			return nil, status.Error(codes.FailedPrecondition, "directive is not taken")
		}
		// Clear taken state and increment requeue_count (T1 fork requirement).
		fd.Status = laneqpb.Status_STATUS_PENDING
		fd.TakenBy = ""
		fd.LeaseUntilUnix = nil
		fd.NotBeforeUnix = nil
		fd.BlockedBy = []string{}
		fd.RequeueCount++
	case laneqpb.Status_STATUS_DROPPED:
		// Only allow DROPPED if currently TAKEN.
		if fd.Status != laneqpb.Status_STATUS_TAKEN {
			return nil, status.Error(codes.FailedPrecondition, "directive is not taken")
		}
		fd.Status = laneqpb.Status_STATUS_DROPPED
		fd.DoneAtUnix = &[]int64{now.Unix()}[0]
		fd.TakenBy = ""
		fd.LeaseUntilUnix = nil
	default:
		return nil, status.Error(codes.InvalidArgument, "SetStatus only supports PENDING, DONE, DROPPED")
	}

	return &laneqpb.SetStatusResponse{Id: req.Id, Status: fd.Status}, nil
}

// Defer defers a directive until a specified time, optionally with blocking dependencies.
func (s *fakeLaneqServer) Defer(ctx context.Context, req *laneqpb.DeferRequest) (*laneqpb.DeferResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fd, ok := s.directives[req.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "directive not found")
	}

	fd.Status = laneqpb.Status_STATUS_DEFERRED

	// Set not_before based on until_unix or delay_ms.
	now := s.now()
	if req.UntilUnix != nil {
		fd.NotBeforeUnix = req.UntilUnix
	} else if req.DelayMs > 0 {
		notBefore := now.Add(time.Duration(req.DelayMs) * time.Millisecond).Unix()
		fd.NotBeforeUnix = &notBefore
	}

	// Set blocked_by.
	fd.BlockedBy = req.BlockedBy

	return &laneqpb.DeferResponse{
		Id:              req.Id,
		Status:          laneqpb.Status_STATUS_DEFERRED,
		NotBeforeUnix:   fd.NotBeforeUnix,
		BlockedBy:       fd.BlockedBy,
	}, nil
}

// Touch renews the lease on a claimed directive.
func (s *fakeLaneqServer) Touch(ctx context.Context, req *laneqpb.TouchRequest) (*laneqpb.TouchResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fd, ok := s.directives[req.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "directive not found")
	}

	// Verify the directive is taken and the consumer matches.
	if fd.Status != laneqpb.Status_STATUS_TAKEN || fd.TakenBy != req.Consumer {
		return nil, status.Error(codes.FailedPrecondition, "lease not held by this consumer")
	}

	now := s.now()
	leaseUntil := now.Add(time.Duration(req.LeaseDurationMs) * time.Millisecond).Unix()
	fd.LeaseUntilUnix = &leaseUntil

	return &laneqpb.TouchResponse{
		Id:             req.Id,
		LeaseUntilUnix: fd.LeaseUntilUnix,
	}, nil
}

// Reap reclaims expired leases and requeues them. Returns the count reclaimed.
func (s *fakeLaneqServer) Reap(ctx context.Context, req *laneqpb.ReapRequest) (*laneqpb.ReapResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	var reclaimed int32

	for _, fd := range s.directives {
		// Reclaim expired leases.
		if req.ExpiredLeases && fd.Status == laneqpb.Status_STATUS_TAKEN && fd.LeaseUntilUnix != nil && *fd.LeaseUntilUnix <= now.Unix() {
			fd.Status = laneqpb.Status_STATUS_PENDING
			fd.TakenBy = ""
			fd.LeaseUntilUnix = nil
			fd.RequeueCount++
			reclaimed++
		}

		// Reclaim stale deferred directives.
		if req.StaleSeconds > 0 && fd.Status == laneqpb.Status_STATUS_DEFERRED && fd.CreatedAtUnix > 0 {
			if now.Unix()-fd.CreatedAtUnix > int64(req.StaleSeconds) {
				fd.Status = laneqpb.Status_STATUS_PENDING
				fd.NotBeforeUnix = nil
				fd.BlockedBy = []string{}
				reclaimed++
			}
		}
	}

	var mode string
	if req.ExpiredLeases && req.StaleSeconds > 0 {
		mode = "both"
	} else if req.ExpiredLeases {
		mode = "expired_leases"
	} else if req.StaleSeconds > 0 {
		mode = "stale_deferred"
	}

	return &laneqpb.ReapResponse{
		Mode:      mode,
		Reclaimed: reclaimed,
		Detail:    fmt.Sprintf("reclaimed %d directives", reclaimed),
	}, nil
}

// Park moves a claimed directive into durable hold (parked status).
// Parked directives are excluded from Take/Peek/Reap and never auto-promote.
func (s *fakeLaneqServer) Park(ctx context.Context, req *laneqpb.ParkRequest) (*laneqpb.ParkResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fd, ok := s.directives[req.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "directive not found")
	}

	// Verify the directive is taken and the consumer matches.
	if fd.Status != laneqpb.Status_STATUS_TAKEN || fd.TakenBy != req.Consumer {
		return nil, status.Error(codes.FailedPrecondition, "lease not held by this consumer")
	}

	fd.Status = laneqpb.Status_STATUS_PARKED
	s.parked[req.Id] = true

	return &laneqpb.ParkResponse{
		Id:     req.Id,
		Status: laneqpb.Status_STATUS_PARKED,
	}, nil
}

// Unpark removes a directive from parked status (returns to pending).
func (s *fakeLaneqServer) Unpark(ctx context.Context, req *laneqpb.UnparkRequest) (*laneqpb.UnparkResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fd, ok := s.directives[req.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "directive not found")
	}

	if fd.Status != laneqpb.Status_STATUS_PARKED {
		return nil, status.Error(codes.FailedPrecondition, "directive is not parked")
	}

	fd.Status = laneqpb.Status_STATUS_PENDING
	delete(s.parked, req.Id)

	return &laneqpb.UnparkResponse{
		Id:     req.Id,
		Status: laneqpb.Status_STATUS_PENDING,
	}, nil
}

// ThreadStatus queries the status of a directive thread (parent + children).
func (s *fakeLaneqServer) ThreadStatus(ctx context.Context, req *laneqpb.ThreadStatusRequest) (*laneqpb.ThreadStatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootFd, ok := s.directives[req.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "directive not found")
	}

	// Collect all directives in this thread (root + descendants by parent_id).
	// Use multiple passes to handle arbitrary depth (though proto suggests only 1 level).
	threadMembers := map[string]*fakeDirective{req.Id: rootFd}
	changed := true
	for changed {
		changed = false
		for id, fd := range s.directives {
			if _, alreadyIncluded := threadMembers[id]; alreadyIncluded {
				continue
			}
			// Check if this directive's parent is already in the thread.
			if _, parentIsInThread := threadMembers[fd.ParentId]; fd.ParentId != "" && parentIsInThread {
				threadMembers[id] = fd
				changed = true
			}
		}
	}

	// Determine thread status: open if any non-terminal, else done.
	var openItems []*laneqpb.ThreadItem
	var openCount int32
	var totalCount int32

	for id, fd := range threadMembers {
		totalCount++
		if fd.Status != laneqpb.Status_STATUS_DONE && fd.Status != laneqpb.Status_STATUS_DROPPED {
			openCount++
			openItems = append(openItems, &laneqpb.ThreadItem{
				Id:             id,
				Status:         fd.Status,
				CreatedAtUnix:  fd.CreatedAtUnix,
			})
		}
	}

	threadStatus := laneqpb.Status_STATUS_DONE
	if openCount > 0 {
		threadStatus = laneqpb.Status_STATUS_PENDING // "open" is represented as pending (non-terminal)
	}

	return &laneqpb.ThreadStatusResponse{
		Root:      req.Id,
		Status:    threadStatus,
		Total:     totalCount,
		Open:      openCount,
		OpenItems: openItems,
	}, nil
}

// Show retrieves full details of a directive by ID, including its thread.
func (s *fakeLaneqServer) Show(ctx context.Context, req *laneqpb.ShowRequest) (*laneqpb.ShowResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fd, ok := s.directives[req.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "directive not found")
	}

	// Collect thread members (children by parent_id).
	var threadItems []*laneqpb.ThreadItem
	for id, member := range s.directives {
		if member.ParentId == req.Id {
			threadItems = append(threadItems, &laneqpb.ThreadItem{
				Id:            id,
				Status:        member.Status,
				CreatedAtUnix: member.CreatedAtUnix,
			})
		}
	}

	return &laneqpb.ShowResponse{
		Directive: s.fdToProto(fd),
		Thread:    threadItems,
	}, nil
}

// Listing queries directives with optional filters.
func (s *fakeLaneqServer) Listing(ctx context.Context, req *laneqpb.ListingRequest) (*laneqpb.ListingResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var results []*laneqpb.Directive

	for _, fd := range s.directives {
		// Filter by status if not all_statuses.
		if !req.AllStatuses && fd.Status != laneqpb.Status_STATUS_PENDING {
			continue
		}

		// Filter by lane if specified.
		if req.Lane != "" && fd.Lane != req.Lane {
			continue
		}

		// Filter by thread (parent_id) if specified.
		if req.Thread != "" && fd.ParentId != req.Thread {
			continue
		}

		results = append(results, s.fdToProto(fd))
	}

	return &laneqpb.ListingResponse{Directives: results}, nil
}

// Stats returns queue statistics.
func (s *fakeLaneqServer) Stats(ctx context.Context, req *laneqpb.StatsRequest) (*laneqpb.StatsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	byStatus := make(map[string]int32)
	consumerMap := make(map[string]*laneqpb.ConsumerStats)

	for _, fd := range s.directives {
		statusName := fd.Status.String()
		byStatus[statusName]++

		if fd.TakenBy != "" {
			if _, ok := consumerMap[fd.TakenBy]; !ok {
				consumerMap[fd.TakenBy] = &laneqpb.ConsumerStats{Consumer: fd.TakenBy}
			}
			consumerMap[fd.TakenBy].ActiveLeases++
		}
	}

	var consumers []*laneqpb.ConsumerStats
	for _, cs := range consumerMap {
		consumers = append(consumers, cs)
	}

	return &laneqpb.StatsResponse{
		ByStatus:  byStatus,
		Consumers: consumers,
	}, nil
}

// Reprioritize changes a directive's priority.
func (s *fakeLaneqServer) Reprioritize(ctx context.Context, req *laneqpb.ReprioritizeRequest) (*laneqpb.ReprioritizeResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fd, ok := s.directives[req.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "directive not found")
	}

	fd.Priority = req.Priority

	return &laneqpb.ReprioritizeResponse{
		Id:       req.Id,
		Priority: fd.Priority,
	}, nil
}

// fdToProto converts a fakeDirective to a proto Directive.
func (s *fakeLaneqServer) fdToProto(fd *fakeDirective) *laneqpb.Directive {
	return &laneqpb.Directive{
		Id:             fd.Id,
		Priority:       fd.Priority,
		Body:           fd.Body,
		Status:         fd.Status,
		Lane:           fd.Lane,
		CreatedAtUnix:  fd.CreatedAtUnix,
		TakenAtUnix:    fd.TakenAtUnix,
		DoneAtUnix:     fd.DoneAtUnix,
		TakenBy:        fd.TakenBy,
		LeaseUntilUnix: fd.LeaseUntilUnix,
		RequeueCount:   fd.RequeueCount,
		ParentId:       fd.ParentId,
		NotBeforeUnix:  fd.NotBeforeUnix,
		BlockedBy:      fd.BlockedBy,
	}
}
