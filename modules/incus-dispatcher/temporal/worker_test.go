package temporal

import (
	"context"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// TestNewWorker_FailFastInvalidQueue verifies that NewWorker fails fast if a non-nil
// queue is provided that does not implement Reprojector, instead of silently leaving
// Queue=nil (which would fail at activity runtime on the live cluster).
func TestNewWorker_FailFastInvalidQueue(t *testing.T) {
	ctx := context.Background()
	cfg := WorkerConfig{
		TemporalAddress: "127.0.0.1:7233",
		TaskQueue:       "test-queue",
		Namespace:       "default",
	}

	// Create an invalid queue that does not implement Reprojector.
	// Using a simple type that satisfies queue.Queue but not Reprojector.
	invalidQueue := &fakeQueueNonReprojector{}

	// NewWorker should fail immediately.
	w, err := NewWorker(ctx, cfg, invalidQueue)

	if err == nil {
		t.Fatalf("NewWorker with non-Reprojector queue should return error, got nil")
	}
	if w != nil {
		t.Fatalf("NewWorker with non-Reprojector queue should return nil worker, got %+v", w)
	}
	if err.Error() != "queue *temporal.fakeQueueNonReprojector does not implement Reprojector (expected *queue.LaneqQueue)" {
		t.Errorf("unexpected error message: %v", err)
	}
	t.Logf("✓ Fail-fast verified: NewWorker rejects non-Reprojector queue with explicit error")
}

// fakeQueueNonReprojector is a fake queue that does NOT implement Reprojector.
// Used to test the fail-fast path in NewWorker.
type fakeQueueNonReprojector struct{}

func (f *fakeQueueNonReprojector) Push(d queue.Directive) (string, error) {
	return "", nil
}
func (f *fakeQueueNonReprojector) Claim(consumer string, leaseDur time.Duration) (queue.Directive, queue.Lease, error) {
	return queue.Directive{}, queue.Lease{}, nil
}
func (f *fakeQueueNonReprojector) Touch(lease queue.Lease, leaseDur time.Duration) (queue.Lease, error) {
	return queue.Lease{}, nil
}
func (f *fakeQueueNonReprojector) Done(lease queue.Lease) error {
	return nil
}
func (f *fakeQueueNonReprojector) Requeue(lease queue.Lease, notBefore time.Time) error {
	return nil
}
func (f *fakeQueueNonReprojector) Peek() (queue.Directive, error) {
	return queue.Directive{}, nil
}
func (f *fakeQueueNonReprojector) Park(lease queue.Lease) error {
	return nil
}
func (f *fakeQueueNonReprojector) Reap() (int, error) {
	return 0, nil
}
func (f *fakeQueueNonReprojector) Len() (pending, claimed int) {
	return 0, 0
}
