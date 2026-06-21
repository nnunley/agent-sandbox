package main

import (
	"strings"
	"testing"
	"time"
)

// STORY-0071 AC-2: the heartbeat must surface real worker activity — when the
// worker is actively running ctx_shell commands the line reports the last shell
// command (and its age), never the misleading "(no shell yet)". RenderHeartbeat
// is the app-level surface; the projector (AC-1) feeds it.

func ctxShellEvents() []byte {
	return []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__lean-ctx__ctx_shell","input":{"command":"go build ./..."}}]}}
`)
}

func TestRenderHeartbeat_ShowsCtxShellNotNoShellYet(t *testing.T) {
	now := time.Now()
	ws := ProjectWorkingState(ctxShellEvents(), now.Add(-3*time.Second), now, 60*time.Second)
	line := RenderHeartbeat(ws)
	if strings.Contains(line, "no shell yet") {
		t.Errorf("heartbeat falsely reports '(no shell yet)' while ctx_shell active: %q", line)
	}
	if !strings.Contains(line, "go build ./...") {
		t.Errorf("heartbeat does not report the last ctx_shell command: %q", line)
	}
	if !strings.Contains(line, "ctx_shell") {
		t.Errorf("heartbeat does not name the ctx_shell tool: %q", line)
	}
}

func TestRenderHeartbeat_NoShellYetWhenIdle(t *testing.T) {
	now := time.Now()
	ws := ProjectWorkingState([]byte(""), now, now, 60*time.Second)
	line := RenderHeartbeat(ws)
	if !strings.Contains(line, "no shell yet") {
		t.Errorf("expected '(no shell yet)' when no shell command has run: %q", line)
	}
}

func TestRenderHeartbeat_ReportsAge(t *testing.T) {
	now := time.Now()
	ws := ProjectWorkingState(ctxShellEvents(), now.Add(-12*time.Second), now, 60*time.Second)
	line := RenderHeartbeat(ws)
	// Age of last activity must appear so a stalled worker is visible.
	if !strings.Contains(line, "12s") && !strings.Contains(line, "s ago") {
		t.Errorf("heartbeat does not report activity age: %q", line)
	}
}
