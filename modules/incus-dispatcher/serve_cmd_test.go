package main

import (
	"testing"

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
		},
		{
			name:      "laneq backend missing address",
			queueType: "laneq",
			laneqAddr: "",
			wantErr:   true, // laneq requires an address
			checkType: nil,
		},
		{
			name:      "unknown backend",
			queueType: "unknown",
			laneqAddr: "localhost:50051",
			wantErr:   true,
			checkType: nil,
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
			}
		})
	}
}
