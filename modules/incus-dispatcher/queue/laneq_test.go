package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/agent-sandbox/incus-dispatcher/queue/laneqpb"
)

// mockLaneqClient is a hand-written fake client for testing contract mapping.
type mockLaneqClient struct {
	// Controllable behavior for each method.
	pushResp      *laneqpb.PushResponse
	pushErr       error
	takeResp      *laneqpb.TakeResponse
	takeErr       error
	peekResp      *laneqpb.PeekResponse
	peekErr       error
	touchResp     *laneqpb.TouchResponse
	touchErr      error
	setStatusResp *laneqpb.SetStatusResponse
	setStatusErr  error
	deferResp     *laneqpb.DeferResponse
	deferErr      error
	reapResp      *laneqpb.ReapResponse
	reapErr       error
	parkResp      *laneqpb.ParkResponse
	parkErr       error

	// Track calls for assertion.
	pushCalls      []pushCall
	takeCalls      []takeCall
	peekCalls      []peekCall
	touchCalls     []touchCall
	setStatusCalls []setStatusCall
	deferCalls     []deferCall
	reapCalls      []reapCall
	parkCalls      []parkCall
}

type pushCall struct {
	req *laneqpb.PushRequest
}

type takeCall struct {
	req *laneqpb.TakeRequest
}

type peekCall struct {
	req *laneqpb.PeekRequest
}

type touchCall struct {
	req *laneqpb.TouchRequest
}

type setStatusCall struct {
	req *laneqpb.SetStatusRequest
}

type deferCall struct {
	req *laneqpb.DeferRequest
}

type reapCall struct {
	req *laneqpb.ReapRequest
}

type parkCall struct {
	req *laneqpb.ParkRequest
}

func (m *mockLaneqClient) Push(ctx context.Context, in *laneqpb.PushRequest, opts ...grpc.CallOption) (*laneqpb.PushResponse, error) {
	m.pushCalls = append(m.pushCalls, pushCall{in})
	return m.pushResp, m.pushErr
}

func (m *mockLaneqClient) Take(ctx context.Context, in *laneqpb.TakeRequest, opts ...grpc.CallOption) (*laneqpb.TakeResponse, error) {
	m.takeCalls = append(m.takeCalls, takeCall{in})
	return m.takeResp, m.takeErr
}

func (m *mockLaneqClient) Peek(ctx context.Context, in *laneqpb.PeekRequest, opts ...grpc.CallOption) (*laneqpb.PeekResponse, error) {
	m.peekCalls = append(m.peekCalls, peekCall{in})
	return m.peekResp, m.peekErr
}

func (m *mockLaneqClient) Touch(ctx context.Context, in *laneqpb.TouchRequest, opts ...grpc.CallOption) (*laneqpb.TouchResponse, error) {
	m.touchCalls = append(m.touchCalls, touchCall{in})
	return m.touchResp, m.touchErr
}

func (m *mockLaneqClient) SetStatus(ctx context.Context, in *laneqpb.SetStatusRequest, opts ...grpc.CallOption) (*laneqpb.SetStatusResponse, error) {
	m.setStatusCalls = append(m.setStatusCalls, setStatusCall{in})
	return m.setStatusResp, m.setStatusErr
}

func (m *mockLaneqClient) Defer(ctx context.Context, in *laneqpb.DeferRequest, opts ...grpc.CallOption) (*laneqpb.DeferResponse, error) {
	m.deferCalls = append(m.deferCalls, deferCall{in})
	return m.deferResp, m.deferErr
}

func (m *mockLaneqClient) Reap(ctx context.Context, in *laneqpb.ReapRequest, opts ...grpc.CallOption) (*laneqpb.ReapResponse, error) {
	m.reapCalls = append(m.reapCalls, reapCall{in})
	return m.reapResp, m.reapErr
}

func (m *mockLaneqClient) Park(ctx context.Context, in *laneqpb.ParkRequest, opts ...grpc.CallOption) (*laneqpb.ParkResponse, error) {
	m.parkCalls = append(m.parkCalls, parkCall{in})
	return m.parkResp, m.parkErr
}

// Unimplemented methods (not needed for T2).
func (m *mockLaneqClient) Show(ctx context.Context, in *laneqpb.ShowRequest, opts ...grpc.CallOption) (*laneqpb.ShowResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClient) Listing(ctx context.Context, in *laneqpb.ListingRequest, opts ...grpc.CallOption) (*laneqpb.ListingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClient) Reprioritize(ctx context.Context, in *laneqpb.ReprioritizeRequest, opts ...grpc.CallOption) (*laneqpb.ReprioritizeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClient) Stats(ctx context.Context, in *laneqpb.StatsRequest, opts ...grpc.CallOption) (*laneqpb.StatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClient) ThreadStatus(ctx context.Context, in *laneqpb.ThreadStatusRequest, opts ...grpc.CallOption) (*laneqpb.ThreadStatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClient) Unpark(ctx context.Context, in *laneqpb.UnparkRequest, opts ...grpc.CallOption) (*laneqpb.UnparkResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// Tests begin here.

func TestLaneqPush(t *testing.T) {
	mock := &mockLaneqClient{
		pushResp: &laneqpb.PushResponse{
			Id:       "d-123",
			Priority: laneqpb.Priority_PRIORITY_P1,
			Lane:     "default",
			Status:   laneqpb.Status_STATUS_PENDING,
		},
	}
	q := NewLaneqQueue(mock, "default")

	d := Directive{
		Intent:     "test",
		Template:   "example",
		Importance: ImportanceNormal,
		Origin:     "orchestrator",
		Repo:       "repo",
		Ref:        "main",
		Task:       "task-1",
		Attempts:   0,
	}

	id, err := q.Push(d)
	if err != nil {
		t.Fatalf("Push error: %v", err)
	}
	if id != "d-123" {
		t.Errorf("Push returned id=%q, want %q", id, "d-123")
	}

	// Verify the gRPC call.
	if len(mock.pushCalls) != 1 {
		t.Fatalf("expected 1 Push call, got %d", len(mock.pushCalls))
	}

	call := mock.pushCalls[0]
	if call.req.Lane != "default" {
		t.Errorf("Push lane=%q, want %q", call.req.Lane, "default")
	}
	if call.req.Priority != laneqpb.Priority_PRIORITY_P1 {
		t.Errorf("Push priority=%v, want %v", call.req.Priority, laneqpb.Priority_PRIORITY_P1)
	}

	// Verify JSON body round-trip.
	var retrieved Directive
	if err := json.Unmarshal([]byte(call.req.Body), &retrieved); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if retrieved.Intent != "test" || retrieved.Template != "example" {
		t.Errorf("Body round-trip failed: %+v", retrieved)
	}
}

func TestLaneqPushImportanceMapping(t *testing.T) {
	tests := []struct {
		name       string
		importance Importance
		want       laneqpb.Priority
	}{
		{"high", ImportanceHigh, laneqpb.Priority_PRIORITY_P0},
		{"normal", ImportanceNormal, laneqpb.Priority_PRIORITY_P1},
		{"low", ImportanceLow, laneqpb.Priority_PRIORITY_P2},
		{"empty defaults to normal", "", laneqpb.Priority_PRIORITY_P1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockLaneqClient{
				pushResp: &laneqpb.PushResponse{
					Id:     "d-test",
					Status: laneqpb.Status_STATUS_PENDING,
				},
			}
			q := NewLaneqQueue(mock, "default")

			d := Directive{Intent: "test", Importance: tt.importance}
			_, _ = q.Push(d)

			if len(mock.pushCalls) != 1 {
				t.Fatalf("expected 1 call")
			}
			if mock.pushCalls[0].req.Priority != tt.want {
				t.Errorf("importance=%v mapped to %v, want %v", tt.importance, mock.pushCalls[0].req.Priority, tt.want)
			}
		})
	}
}

func TestLaneqClaimSuccess(t *testing.T) {
	d := Directive{
		Intent:     "test",
		Template:   "example",
		Importance: ImportanceHigh,
		Origin:     "orchestrator",
		Repo:       "repo",
		Ref:        "main",
		Task:       "task-1",
		Attempts:   2,         // Should be updated from requeue_count.
		Lane:       "default", // Body lane (will be overridden by proto).
	}

	body, _ := json.Marshal(d)
	leaseUntil := time.Now().Add(30 * time.Second).Unix()

	mock := &mockLaneqClient{
		takeResp: &laneqpb.TakeResponse{
			Directive: &laneqpb.Directive{
				Id:             "d-456",
				Priority:       laneqpb.Priority_PRIORITY_P0,
				Body:           string(body),
				Status:         laneqpb.Status_STATUS_TAKEN,
				Lane:           "urgent", // Proto lane (authoritative column).
				TakenBy:        "consumer-1",
				LeaseUntilUnix: &leaseUntil,
				RequeueCount:   5,
			},
			Consumer: "consumer-1",
			Lane:     "urgent",
		},
	}
	q := NewLaneqQueue(mock, "default")

	claimed, lease, err := q.Claim("consumer-1", 30*time.Second)
	if err != nil {
		t.Fatalf("Claim error: %v", err)
	}

	// Verify directive fields.
	if claimed.Intent != "test" || claimed.Template != "example" {
		t.Errorf("Claim directive body: got %+v", claimed)
	}

	// Verify Importance is mapped from priority.
	if claimed.Importance != ImportanceHigh {
		t.Errorf("Claim importance=%v, want %v", claimed.Importance, ImportanceHigh)
	}

	// Verify Attempts is mapped from requeue_count.
	if claimed.Attempts != 5 {
		t.Errorf("Claim attempts=%d, want %d", claimed.Attempts, 5)
	}

	// Verify Lane column is authoritative: proto lane="urgent" overrides body lane="default".
	if claimed.Lane != "urgent" {
		t.Errorf("Claim lane=%q, want %q (column should override body JSON)", claimed.Lane, "urgent")
	}

	// Verify lease.
	if lease.DirectiveID != "d-456" {
		t.Errorf("Lease DirectiveID=%q, want %q", lease.DirectiveID, "d-456")
	}
	if lease.Token != "consumer-1" {
		t.Errorf("Lease Token=%q, want %q", lease.Token, "consumer-1")
	}

	// Verify Expiry is correctly constructed from Unix seconds (not milliseconds).
	expectedExpiry := time.Unix(leaseUntil, 0)
	if lease.Expiry != expectedExpiry {
		t.Errorf("Lease Expiry=%v, want %v", lease.Expiry, expectedExpiry)
	}

	// Verify the gRPC call.
	if len(mock.takeCalls) != 1 {
		t.Fatalf("expected 1 Take call")
	}
	call := mock.takeCalls[0]
	if call.req.Consumer != "consumer-1" || call.req.Lane != "default" {
		t.Errorf("Take call: consumer=%q lane=%q", call.req.Consumer, call.req.Lane)
	}
	if call.req.LeaseDurationMs != 30000 {
		t.Errorf("Take LeaseDurationMs=%d, want 30000", call.req.LeaseDurationMs)
	}
}

func TestLaneqClaimEmpty(t *testing.T) {
	mock := &mockLaneqClient{
		takeErr: status.Error(codes.NotFound, "no eligible directive"),
	}
	q := NewLaneqQueue(mock, "default")

	_, _, err := q.Claim("consumer-1", 30*time.Second)
	if err != ErrEmpty {
		t.Errorf("Claim error: got %v, want ErrEmpty", err)
	}
}

func TestLaneqClaimEmptyDirective(t *testing.T) {
	// Server returns a TakeResponse with an empty/nil Directive.
	mock := &mockLaneqClient{
		takeResp: &laneqpb.TakeResponse{
			Directive: nil,
			Consumer:  "consumer-1",
			Lane:      "default",
		},
	}
	q := NewLaneqQueue(mock, "default")

	_, _, err := q.Claim("consumer-1", 30*time.Second)
	if err != ErrEmpty {
		t.Errorf("Claim with nil Directive: got %v, want ErrEmpty", err)
	}
}

func TestLaneqPeekSuccess(t *testing.T) {
	d := Directive{
		Intent:     "test",
		Template:   "example",
		Importance: ImportanceLow,
		Attempts:   1,
		Lane:       "default", // Body lane (will be overridden by proto).
	}

	body, _ := json.Marshal(d)

	mock := &mockLaneqClient{
		peekResp: &laneqpb.PeekResponse{
			Directive: &laneqpb.Directive{
				Id:           "d-789",
				Priority:     laneqpb.Priority_PRIORITY_P2,
				Body:         string(body),
				Status:       laneqpb.Status_STATUS_PENDING,
				Lane:         "priority", // Proto lane (authoritative column).
				RequeueCount: 1,
			},
		},
	}
	q := NewLaneqQueue(mock, "default")

	peeked, err := q.Peek()
	if err != nil {
		t.Fatalf("Peek error: %v", err)
	}

	if peeked.Intent != "test" {
		t.Errorf("Peek directive: got %+v", peeked)
	}
	if peeked.Importance != ImportanceLow {
		t.Errorf("Peek importance=%v, want %v", peeked.Importance, ImportanceLow)
	}
	if peeked.Attempts != 1 {
		t.Errorf("Peek attempts=%d, want %d", peeked.Attempts, 1)
	}

	// Verify Lane column is authoritative: proto lane="priority" overrides body lane="default".
	if peeked.Lane != "priority" {
		t.Errorf("Peek lane=%q, want %q (column should override body JSON)", peeked.Lane, "priority")
	}

	// Verify the gRPC call.
	if len(mock.peekCalls) != 1 {
		t.Fatalf("expected 1 Peek call")
	}
	if mock.peekCalls[0].req.Lane != "default" {
		t.Errorf("Peek lane=%q, want %q", mock.peekCalls[0].req.Lane, "default")
	}
}

func TestLaneqPeekEmpty(t *testing.T) {
	mock := &mockLaneqClient{
		peekErr: status.Error(codes.NotFound, "no eligible directive"),
	}
	q := NewLaneqQueue(mock, "default")

	_, err := q.Peek()
	if err != ErrEmpty {
		t.Errorf("Peek error: got %v, want ErrEmpty", err)
	}
}

func TestLaneqNotBeforeMapping(t *testing.T) {
	// Test that NotBefore round-trips correctly.
	now := time.Now()
	notBefore := now.Add(1 * time.Hour)

	d := Directive{
		Intent:     "test",
		NotBefore:  notBefore,
		Importance: ImportanceNormal,
	}

	body, _ := json.Marshal(d)
	notBeforeUnix := notBefore.Unix()

	mock := &mockLaneqClient{
		peekResp: &laneqpb.PeekResponse{
			Directive: &laneqpb.Directive{
				Id:            "d-nb",
				Body:          string(body),
				Priority:      laneqpb.Priority_PRIORITY_P1,
				Status:        laneqpb.Status_STATUS_DEFERRED,
				NotBeforeUnix: &notBeforeUnix,
			},
		},
	}
	q := NewLaneqQueue(mock, "default")

	peeked, _ := q.Peek()

	// The NotBefore should be reconstructed from the proto.
	// Allow 1-second tolerance for time serialization.
	if peeked.NotBefore.Unix() != notBefore.Unix() {
		t.Errorf("NotBefore: got %v, want %v", peeked.NotBefore, notBefore)
	}
}

func TestLaneqTouchSuccess(t *testing.T) {
	expectedExpiry := time.Now().Add(60 * time.Second).Unix()

	mock := &mockLaneqClient{
		touchResp: &laneqpb.TouchResponse{
			Id:             "d-456",
			LeaseUntilUnix: &expectedExpiry,
		},
	}
	q := NewLaneqQueue(mock, "default")

	originalLease := Lease{
		DirectiveID: "d-456",
		Token:       "consumer-1",
		Expiry:      time.Now(),
	}

	newLease, err := q.Touch(originalLease, 60*time.Second)
	if err != nil {
		t.Fatalf("Touch error: %v", err)
	}

	if newLease.DirectiveID != "d-456" {
		t.Errorf("Touch lease ID=%q, want %q", newLease.DirectiveID, "d-456")
	}
	if newLease.Token != "consumer-1" {
		t.Errorf("Touch lease token=%q, want %q", newLease.Token, "consumer-1")
	}

	// Verify Expiry is correctly constructed from Unix seconds (not milliseconds).
	expectedTime := time.Unix(expectedExpiry, 0)
	if newLease.Expiry != expectedTime {
		t.Errorf("Touch lease Expiry=%v, want %v", newLease.Expiry, expectedTime)
	}

	// Verify gRPC call.
	if len(mock.touchCalls) != 1 {
		t.Fatalf("expected 1 Touch call")
	}
	call := mock.touchCalls[0]
	if call.req.Id != "d-456" || call.req.Consumer != "consumer-1" {
		t.Errorf("Touch call: id=%q consumer=%q", call.req.Id, call.req.Consumer)
	}
	if call.req.LeaseDurationMs != 60000 {
		t.Errorf("Touch LeaseDurationMs=%d, want 60000", call.req.LeaseDurationMs)
	}
}

func TestLaneqTouchLeaseLost(t *testing.T) {
	mock := &mockLaneqClient{
		touchErr: status.Error(codes.NotFound, "lease not found"),
	}
	q := NewLaneqQueue(mock, "default")

	lease := Lease{DirectiveID: "d-456", Token: "consumer-1"}

	_, err := q.Touch(lease, 60*time.Second)
	if err != ErrLeaseLost {
		t.Errorf("Touch error: got %v, want ErrLeaseLost", err)
	}
}

func TestLaneqDone(t *testing.T) {
	mock := &mockLaneqClient{
		setStatusResp: &laneqpb.SetStatusResponse{
			Id:     "d-456",
			Status: laneqpb.Status_STATUS_DONE,
		},
	}
	q := NewLaneqQueue(mock, "default")

	lease := Lease{DirectiveID: "d-456", Token: "consumer-1"}

	err := q.Done(lease)
	if err != nil {
		t.Fatalf("Done error: %v", err)
	}

	// Verify gRPC call.
	if len(mock.setStatusCalls) != 1 {
		t.Fatalf("expected 1 SetStatus call")
	}
	call := mock.setStatusCalls[0]
	if call.req.Id != "d-456" || call.req.Status != laneqpb.Status_STATUS_DONE {
		t.Errorf("SetStatus call: id=%q status=%v", call.req.Id, call.req.Status)
	}
}

func TestLaneqDoneLeaseLost(t *testing.T) {
	mock := &mockLaneqClient{
		setStatusErr: status.Error(codes.NotFound, "directive not found"),
	}
	q := NewLaneqQueue(mock, "default")

	lease := Lease{DirectiveID: "d-456", Token: "consumer-1"}

	err := q.Done(lease)
	if err != ErrLeaseLost {
		t.Errorf("Done error: got %v, want ErrLeaseLost", err)
	}
}

func TestLaneqRequeueImmediate(t *testing.T) {
	mock := &mockLaneqClient{
		setStatusResp: &laneqpb.SetStatusResponse{
			Id:     "d-456",
			Status: laneqpb.Status_STATUS_PENDING,
		},
	}
	q := NewLaneqQueue(mock, "default")

	lease := Lease{DirectiveID: "d-456", Token: "consumer-1"}

	// Requeue with zero NotBefore = immediately eligible.
	err := q.Requeue(lease, time.Time{})
	if err != nil {
		t.Fatalf("Requeue error: %v", err)
	}

	// Should call SetStatus(PENDING), not Defer.
	if len(mock.setStatusCalls) != 1 {
		t.Fatalf("expected 1 SetStatus call, got %d", len(mock.setStatusCalls))
	}
	if mock.setStatusCalls[0].req.Status != laneqpb.Status_STATUS_PENDING {
		t.Errorf("SetStatus status=%v, want STATUS_PENDING", mock.setStatusCalls[0].req.Status)
	}
}

func TestLaneqRequeueDeferred(t *testing.T) {
	notBefore := time.Now().Add(2 * time.Hour)
	notBeforeUnix := notBefore.Unix()

	mock := &mockLaneqClient{
		deferResp: &laneqpb.DeferResponse{
			Id:            "d-456",
			Status:        laneqpb.Status_STATUS_DEFERRED,
			NotBeforeUnix: &notBeforeUnix,
		},
	}
	q := NewLaneqQueue(mock, "default")

	lease := Lease{DirectiveID: "d-456", Token: "consumer-1"}

	err := q.Requeue(lease, notBefore)
	if err != nil {
		t.Fatalf("Requeue error: %v", err)
	}

	// Should call Defer with the notBefore timestamp.
	if len(mock.deferCalls) != 1 {
		t.Fatalf("expected 1 Defer call, got %d", len(mock.deferCalls))
	}
	call := mock.deferCalls[0]
	if call.req.Id != "d-456" {
		t.Errorf("Defer id=%q, want %q", call.req.Id, "d-456")
	}
	if call.req.UntilUnix == nil || *call.req.UntilUnix != notBeforeUnix {
		t.Errorf("Defer UntilUnix=%v, want %d", call.req.UntilUnix, notBeforeUnix)
	}
}

func TestLaneqRequeueLeaseLost(t *testing.T) {
	mock := &mockLaneqClient{
		setStatusErr: status.Error(codes.FailedPrecondition, "lease expired"),
	}
	q := NewLaneqQueue(mock, "default")

	lease := Lease{DirectiveID: "d-456", Token: "consumer-1"}

	err := q.Requeue(lease, time.Time{})
	if err != ErrLeaseLost {
		t.Errorf("Requeue error: got %v, want ErrLeaseLost", err)
	}
}

func TestLaneqReap(t *testing.T) {
	mock := &mockLaneqClient{
		reapResp: &laneqpb.ReapResponse{
			Mode:      "expired_leases",
			Reclaimed: 3,
			Detail:    "reclaimed 3 expired leases",
		},
	}
	q := NewLaneqQueue(mock, "default")

	count, err := q.Reap()
	if err != nil {
		t.Fatalf("Reap error: %v", err)
	}
	if count != 3 {
		t.Errorf("Reap count=%d, want 3", count)
	}

	// Verify gRPC call.
	if len(mock.reapCalls) != 1 {
		t.Fatalf("expected 1 Reap call")
	}
	call := mock.reapCalls[0]
	if !call.req.ExpiredLeases {
		t.Errorf("Reap ExpiredLeases=%v, want true", call.req.ExpiredLeases)
	}
}

func TestLaneqPark(t *testing.T) {
	mock := &mockLaneqClient{
		parkResp: &laneqpb.ParkResponse{
			Id:     "d-456",
			Status: laneqpb.Status_STATUS_PARKED,
		},
	}
	q := NewLaneqQueue(mock, "default")

	lease := Lease{DirectiveID: "d-456", Token: "consumer-1"}

	err := q.Park(lease)
	if err != nil {
		t.Fatalf("Park error: %v", err)
	}

	// Verify gRPC call.
	if len(mock.parkCalls) != 1 {
		t.Fatalf("expected 1 Park call")
	}
	call := mock.parkCalls[0]
	if call.req.Id != "d-456" || call.req.Consumer != "consumer-1" {
		t.Errorf("Park call: id=%q consumer=%q", call.req.Id, call.req.Consumer)
	}
}

func TestLaneqParkLeaseLost(t *testing.T) {
	mock := &mockLaneqClient{
		parkErr: status.Error(codes.NotFound, "directive not found"),
	}
	q := NewLaneqQueue(mock, "default")

	lease := Lease{DirectiveID: "d-456", Token: "consumer-1"}

	err := q.Park(lease)
	if err != ErrLeaseLost {
		t.Errorf("Park error: got %v, want ErrLeaseLost", err)
	}
}

func TestLaneqLaneDefault(t *testing.T) {
	// If no lane is specified, it should default to "default".
	mock := &mockLaneqClient{
		peekResp: &laneqpb.PeekResponse{
			Directive: &laneqpb.Directive{
				Id:     "d-test",
				Body:   `{"intent":"test"}`,
				Lane:   "default",
				Status: laneqpb.Status_STATUS_PENDING,
			},
		},
	}
	q := NewLaneqQueue(mock, "")

	_, _ = q.Peek()

	if len(mock.peekCalls) != 1 {
		t.Fatalf("expected 1 Peek call")
	}
	if mock.peekCalls[0].req.Lane != "default" {
		t.Errorf("Lane=%q, want %q", mock.peekCalls[0].req.Lane, "default")
	}
}

func TestLaneqLenStub(t *testing.T) {
	// Len() is a stub that returns (0, 0) until implemented via Stats RPC.
	// This test pins the documented stub behavior in CI.
	mock := &mockLaneqClient{}
	q := NewLaneqQueue(mock, "default")

	pending, claimed := q.Len()
	if pending != 0 || claimed != 0 {
		t.Errorf("Len()=(%d, %d), want (0, 0); stub unimplemented per TODO(ITER-0008b)", pending, claimed)
	}
}

func TestLaneqDirectiveBodyRoundTrip(t *testing.T) {
	// Comprehensive test: a full Directive with all fields.
	deadline := time.Now().Add(24 * time.Hour)
	d := Directive{
		Intent:      "complex-test",
		Template:    "advanced-template",
		Importance:  ImportanceHigh,
		Origin:      "orchestrator",
		Repo:        "https://example.com/repo",
		Ref:         "feature/branch",
		Task:        "multi-step-task",
		HandoffIn:   "some-bundle",
		Deadline:    &deadline,
		MaxAttempts: 10,
		Attempts:    3,
		Grade: &GradeSpec{
			OracleRef: "oracle-v1",
			Cmd:       "make test",
			Expect:    map[string]any{"passed": true},
		},
	}

	body, _ := json.Marshal(d)

	mock := &mockLaneqClient{
		takeResp: &laneqpb.TakeResponse{
			Directive: &laneqpb.Directive{
				Id:             "d-complex",
				Body:           string(body),
				Priority:       laneqpb.Priority_PRIORITY_P0,
				Status:         laneqpb.Status_STATUS_TAKEN,
				RequeueCount:   3,
				TakenBy:        "consumer-advanced",
				LeaseUntilUnix: ptrInt64(time.Now().Add(30 * time.Second).Unix()),
			},
		},
	}
	q := NewLaneqQueue(mock, "default")

	claimed, _, _ := q.Claim("consumer-advanced", 30*time.Second)

	// Verify all fields round-trip.
	if claimed.Intent != "complex-test" {
		t.Errorf("Intent=%q, want %q", claimed.Intent, "complex-test")
	}
	if claimed.Template != "advanced-template" {
		t.Errorf("Template=%q, want %q", claimed.Template, "advanced-template")
	}
	if claimed.Importance != ImportanceHigh {
		t.Errorf("Importance=%v, want %v", claimed.Importance, ImportanceHigh)
	}
	if claimed.Origin != "orchestrator" {
		t.Errorf("Origin=%q, want %q", claimed.Origin, "orchestrator")
	}
	if claimed.Repo != "https://example.com/repo" {
		t.Errorf("Repo=%q, want %q", claimed.Repo, "https://example.com/repo")
	}
	if claimed.Ref != "feature/branch" {
		t.Errorf("Ref=%q, want %q", claimed.Ref, "feature/branch")
	}
	if claimed.Task != "multi-step-task" {
		t.Errorf("Task=%q, want %q", claimed.Task, "multi-step-task")
	}
	if claimed.HandoffIn != "some-bundle" {
		t.Errorf("HandoffIn=%q, want %q", claimed.HandoffIn, "some-bundle")
	}
	if claimed.MaxAttempts != 10 {
		t.Errorf("MaxAttempts=%d, want %d", claimed.MaxAttempts, 10)
	}
	if claimed.Attempts != 3 {
		t.Errorf("Attempts=%d, want %d", claimed.Attempts, 3)
	}
	if claimed.Grade == nil || claimed.Grade.OracleRef != "oracle-v1" {
		t.Errorf("Grade=%v, want oracle-v1", claimed.Grade)
	}
}

// TestReprioritize verifies that Reprioritize maps to the laneq gRPC RPC correctly.
func TestReprioritize(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		importance Importance
		resp       *laneqpb.ReprioritizeResponse
		err        error
		wantErr    bool
	}{
		{
			name:       "success",
			id:         "d-123",
			importance: ImportanceHigh,
			resp: &laneqpb.ReprioritizeResponse{
				Id:       "d-123",
				Priority: laneqpb.Priority_PRIORITY_P0,
			},
			err:     nil,
			wantErr: false,
		},
		{
			name:       "not found",
			id:         "d-missing",
			importance: ImportanceLow,
			resp:       nil,
			err:        status.Error(codes.NotFound, "directive not found"),
			wantErr:    true,
		},
		{
			name:       "rpc error",
			id:         "d-456",
			importance: ImportanceNormal,
			resp:       nil,
			err:        status.Error(codes.Internal, "database error"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockLaneqClientWithReprioritize{
				reprioritizeResp: tt.resp,
				reprioritizeErr:  tt.err,
			}
			q := NewLaneqQueue(mock, "default")

			err := q.Reprioritize(tt.id, tt.importance)

			if (err != nil) != tt.wantErr {
				t.Errorf("Reprioritize() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(mock.reprioritizeCalls) != 1 {
				t.Fatalf("expected 1 call to Reprioritize, got %d", len(mock.reprioritizeCalls))
			}

			call := mock.reprioritizeCalls[0]
			if call.req.Id != tt.id {
				t.Errorf("Reprioritize call: Id = %q, want %q", call.req.Id, tt.id)
			}
			if call.req.Priority != importanceToProto(tt.importance) {
				t.Errorf("Reprioritize call: Priority = %v, want %v", call.req.Priority, importanceToProto(tt.importance))
			}
		})
	}
}

// mockLaneqClientWithReprioritize extends mockLaneqClient with Reprioritize support.
type mockLaneqClientWithReprioritize struct {
	reprioritizeResp  *laneqpb.ReprioritizeResponse
	reprioritizeErr   error
	reprioritizeCalls []reprioritizeCall
}

type reprioritizeCall struct {
	req *laneqpb.ReprioritizeRequest
}

func (m *mockLaneqClientWithReprioritize) Reprioritize(ctx context.Context, in *laneqpb.ReprioritizeRequest, opts ...grpc.CallOption) (*laneqpb.ReprioritizeResponse, error) {
	m.reprioritizeCalls = append(m.reprioritizeCalls, reprioritizeCall{in})
	return m.reprioritizeResp, m.reprioritizeErr
}

// Stub out all other methods so it implements laneqpb.LaneqClient.
func (m *mockLaneqClientWithReprioritize) Push(ctx context.Context, in *laneqpb.PushRequest, opts ...grpc.CallOption) (*laneqpb.PushResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) Take(ctx context.Context, in *laneqpb.TakeRequest, opts ...grpc.CallOption) (*laneqpb.TakeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) Peek(ctx context.Context, in *laneqpb.PeekRequest, opts ...grpc.CallOption) (*laneqpb.PeekResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) Show(ctx context.Context, in *laneqpb.ShowRequest, opts ...grpc.CallOption) (*laneqpb.ShowResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) Listing(ctx context.Context, in *laneqpb.ListingRequest, opts ...grpc.CallOption) (*laneqpb.ListingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) SetStatus(ctx context.Context, in *laneqpb.SetStatusRequest, opts ...grpc.CallOption) (*laneqpb.SetStatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) Defer(ctx context.Context, in *laneqpb.DeferRequest, opts ...grpc.CallOption) (*laneqpb.DeferResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) Touch(ctx context.Context, in *laneqpb.TouchRequest, opts ...grpc.CallOption) (*laneqpb.TouchResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) Reap(ctx context.Context, in *laneqpb.ReapRequest, opts ...grpc.CallOption) (*laneqpb.ReapResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) Stats(ctx context.Context, in *laneqpb.StatsRequest, opts ...grpc.CallOption) (*laneqpb.StatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) ThreadStatus(ctx context.Context, in *laneqpb.ThreadStatusRequest, opts ...grpc.CallOption) (*laneqpb.ThreadStatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) Park(ctx context.Context, in *laneqpb.ParkRequest, opts ...grpc.CallOption) (*laneqpb.ParkResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (m *mockLaneqClientWithReprioritize) Unpark(ctx context.Context, in *laneqpb.UnparkRequest, opts ...grpc.CallOption) (*laneqpb.UnparkResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
