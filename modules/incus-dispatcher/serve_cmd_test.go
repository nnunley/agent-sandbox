package main

import (
	"strings"
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
			// ITER-0007c T2 AC-3: Pass empty strings for auth flags (legacy passthrough, no interceptor).
			q, err := buildQueue(tt.queueType, tt.laneqAddr, "", "", "")
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

// TestBuildQueueAuthPartialConfig verifies that partial auth flag configuration fails loudly.
// ITER-0007c PAR fix: misconfiguration must not silently disable auth.
func TestBuildQueueAuthPartialConfig(t *testing.T) {
	tests := []struct {
		name          string
		grantFile     string
		clientKeyPath string
		aud           string
		wantErr       bool
		errContains   string
	}{
		{
			name:          "all three set (happy path, but key files don't exist)",
			grantFile:     "/tmp/nonexistent_grant",
			clientKeyPath: "/tmp/nonexistent_key",
			aud:           "laneq://agent-host:9999",
			wantErr:       true, // Fails on file load, but not on partial-config
			errContains:   "load", // Expected error path: file load (grant or key), not config validation
		},
		{
			name:          "none set (legacy passthrough)",
			grantFile:     "",
			clientKeyPath: "",
			aud:           "",
			wantErr:       false, // Should succeed with legacy passthrough
		},
		{
			name:          "only grantFile set",
			grantFile:     "/tmp/grant",
			clientKeyPath: "",
			aud:           "",
			wantErr:       true,
			errContains:   "partially configured",
		},
		{
			name:          "only clientKeyPath set",
			grantFile:     "",
			clientKeyPath: "/tmp/key",
			aud:           "",
			wantErr:       true,
			errContains:   "partially configured",
		},
		{
			name:          "only aud set",
			grantFile:     "",
			clientKeyPath: "",
			aud:           "laneq://agent-host:9999",
			wantErr:       true,
			errContains:   "partially configured",
		},
		{
			name:          "grantFile and clientKeyPath set, aud missing",
			grantFile:     "/tmp/grant",
			clientKeyPath: "/tmp/key",
			aud:           "",
			wantErr:       true,
			errContains:   "partially configured",
		},
		{
			name:          "grantFile and aud set, clientKeyPath missing",
			grantFile:     "/tmp/grant",
			clientKeyPath: "",
			aud:           "laneq://agent-host:9999",
			wantErr:       true,
			errContains:   "partially configured",
		},
		{
			name:          "clientKeyPath and aud set, grantFile missing",
			grantFile:     "",
			clientKeyPath: "/tmp/key",
			aud:           "laneq://agent-host:9999",
			wantErr:       true,
			errContains:   "partially configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := buildQueue("laneq", "localhost:50051", tt.grantFile, tt.clientKeyPath, tt.aud)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildQueue: got err=%v, want err=%v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("buildQueue: error %q does not contain %q", err, tt.errContains)
				}
			}
			if !tt.wantErr && q != nil {
				// Clean up the queue if it was created.
				if c, ok := q.(interface{ Close() error }); ok {
					c.Close()
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
