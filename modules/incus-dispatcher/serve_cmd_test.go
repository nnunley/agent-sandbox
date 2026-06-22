package main

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// TestBuildQueue validates the queue backend selection.
func TestBuildQueue(t *testing.T) {
	tests := []struct {
		name      string
		queueType string
		laneqAddr string
		wantErr   bool
		checkType func(q queue.Queue) bool
		cleanup   func(q queue.Queue) error
	}{
		{
			name:      "memory backend (default)",
			queueType: "memory",
			laneqAddr: "",
			wantErr:   false,
			checkType: func(q queue.Queue) bool {
				_, ok := q.(*queue.MemoryQueue)
				return ok
			},
			cleanup: nil,
		},
		{
			name:      "memory backend (explicit)",
			queueType: "memory",
			laneqAddr: "localhost:50051",
			wantErr:   false,
			checkType: func(q queue.Queue) bool {
				_, ok := q.(*queue.MemoryQueue)
				return ok
			},
			cleanup: nil,
		},
		{
			name:      "laneq backend with address",
			queueType: "laneq",
			laneqAddr: "localhost:50051",
			wantErr:   false, // grpc.NewClient does lazy dial, so this succeeds; actual dial errors occur on first RPC
			checkType: func(q queue.Queue) bool {
				_, ok := q.(*queue.LaneqQueue)
				return ok
			},
			cleanup: func(q queue.Queue) error {
				if c, ok := q.(interface{ Close() error }); ok {
					return c.Close()
				}
				return nil
			},
		},
		{
			name:      "laneq backend missing address",
			queueType: "laneq",
			laneqAddr: "",
			wantErr:   true, // laneq requires an address
			checkType: nil,
			cleanup:   nil,
		},
		{
			name:      "unknown backend",
			queueType: "unknown",
			laneqAddr: "localhost:50051",
			wantErr:   true,
			checkType: nil,
			cleanup:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := buildQueue(tt.queueType, tt.laneqAddr)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildQueue(%q, %q): got err=%v, want err=%v", tt.queueType, tt.laneqAddr, err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkType != nil {
				if !tt.checkType(q) {
					t.Errorf("buildQueue(%q, %q): wrong type, got %T", tt.queueType, tt.laneqAddr, q)
				}
				// Clean up if needed (e.g., close gRPC connection).
				if tt.cleanup != nil {
					if err := tt.cleanup(q); err != nil {
						t.Errorf("buildQueue(%q, %q): cleanup failed: %v", tt.queueType, tt.laneqAddr, err)
					}
				}
			}
		})
	}
}

// TestLaneqQueueClose verifies that LaneqQueue.Close() properly closes the gRPC connection.
func TestLaneqQueueClose(t *testing.T) {
	// Create a dummy gRPC connection (won't actually connect due to lazy dial).
	conn, err := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.Dial failed: %v", err)
	}

	q := queue.NewLaneqQueueWithConn(conn, "test-lane")

	// Close should succeed.
	if err := q.Close(); err != nil {
		t.Errorf("LaneqQueue.Close() failed: %v", err)
	}

	// Double-close is safe (gRPC conn.Close returns nil on already-closed connections in newer versions).
	// This verifies the Close() method is idempotent or at least doesn't panic.
	err = q.Close()
	if err != nil && err.Error() != "rpc error: code = Canceled desc = context canceled" {
		// Some gRPC versions return an error on double-close; that's acceptable.
		// We just verify it doesn't panic.
		t.Logf("LaneqQueue.Close() (second call) returned: %v (acceptable)", err)
	}
}
