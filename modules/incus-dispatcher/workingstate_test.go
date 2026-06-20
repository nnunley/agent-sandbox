package main

import (
	"testing"
	"time"
)

// inline events.jsonl covering the required scenarios:
// 1. a ctx_shell (MCP-prefixed) line
// 2. a ctx_read line
// 3. a later Bash line (ctx_shell must win)
// 4. a plain Read line
// 5. a malformed line (should be skipped, not counted)
// 6. a blank line (should be skipped, not counted)
const sampleEvents = `{"type":"system","subtype":"init","session_id":"abc"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"mcp__lean-ctx__ctx_shell","input":{"command":"go build ./..."}}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"mcp__lean-ctx__ctx_read","input":{"file_path":"/repo/pkg/ir/ir.go"}}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"go test ./..."}}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t4","name":"Read","input":{"file_path":"/repo/main.go"}}]}}
not valid json at all
`

func baseTime() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) }

func TestProjectWorkingState_EventCount(t *testing.T) {
	now := baseTime()
	lastMod := now.Add(-10 * time.Second)
	ws := ProjectWorkingState([]byte(sampleEvents), lastMod, now, time.Minute)
	// 5 valid JSON lines (system + 4 assistant); blank and malformed are skipped/not counted
	if ws.EventCount != 5 {
		t.Errorf("EventCount = %d, want 5", ws.EventCount)
	}
}

func TestProjectWorkingState_CtxShellWinsOverBash(t *testing.T) {
	now := baseTime()
	lastMod := now.Add(-5 * time.Second)
	ws := ProjectWorkingState([]byte(sampleEvents), lastMod, now, time.Minute)

	if ws.LastShellTool != "ctx_shell" {
		t.Errorf("LastShellTool = %q, want %q", ws.LastShellTool, "ctx_shell")
	}
	if ws.LastShellCmd != "go build ./..." {
		t.Errorf("LastShellCmd = %q, want %q", ws.LastShellCmd, "go build ./...")
	}
}

func TestProjectWorkingState_CtxReadWinsOverRead(t *testing.T) {
	now := baseTime()
	lastMod := now.Add(-5 * time.Second)
	ws := ProjectWorkingState([]byte(sampleEvents), lastMod, now, time.Minute)

	if ws.LastRead != "/repo/pkg/ir/ir.go" {
		t.Errorf("LastRead = %q, want %q", ws.LastRead, "/repo/pkg/ir/ir.go")
	}
}

func TestProjectWorkingState_FallbackToBash(t *testing.T) {
	// events with only a Bash tool_use (no ctx_shell)
	events := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}
`
	now := baseTime()
	lastMod := now.Add(-5 * time.Second)
	ws := ProjectWorkingState([]byte(events), lastMod, now, time.Minute)

	if ws.LastShellTool != "Bash" {
		t.Errorf("LastShellTool = %q, want %q", ws.LastShellTool, "Bash")
	}
	if ws.LastShellCmd != "go test ./..." {
		t.Errorf("LastShellCmd = %q, want %q", ws.LastShellCmd, "go test ./...")
	}
}

func TestProjectWorkingState_NoTools(t *testing.T) {
	events := `{"type":"system","subtype":"init"}
`
	now := baseTime()
	lastMod := now.Add(-5 * time.Second)
	ws := ProjectWorkingState([]byte(events), lastMod, now, time.Minute)

	if ws.LastShellTool != "" {
		t.Errorf("LastShellTool = %q, want \"\"", ws.LastShellTool)
	}
	if ws.LastShellCmd != "" {
		t.Errorf("LastShellCmd = %q, want \"\"", ws.LastShellCmd)
	}
	if ws.LastRead != "" {
		t.Errorf("LastRead = %q, want \"\"", ws.LastRead)
	}
}

func TestProjectWorkingState_AliveAndSinceLast(t *testing.T) {
	now := baseTime()
	window := 30 * time.Second

	t.Run("alive when within window", func(t *testing.T) {
		lastMod := now.Add(-10 * time.Second)
		ws := ProjectWorkingState([]byte(`{"type":"system"}`+"\n"), lastMod, now, window)
		if !ws.Alive {
			t.Error("Alive = false, want true")
		}
		if ws.SinceLast != 10*time.Second {
			t.Errorf("SinceLast = %v, want 10s", ws.SinceLast)
		}
	})

	t.Run("alive when exactly at window boundary", func(t *testing.T) {
		lastMod := now.Add(-30 * time.Second)
		ws := ProjectWorkingState([]byte(`{"type":"system"}`+"\n"), lastMod, now, window)
		if !ws.Alive {
			t.Error("Alive = false, want true (boundary is inclusive)")
		}
	})

	t.Run("dead when past window", func(t *testing.T) {
		lastMod := now.Add(-31 * time.Second)
		ws := ProjectWorkingState([]byte(`{"type":"system"}`+"\n"), lastMod, now, window)
		if ws.Alive {
			t.Error("Alive = true, want false")
		}
	})
}

func TestProjectWorkingState_PhaseGuess(t *testing.T) {
	cases := []struct {
		name    string
		cmd     string
		want    string
	}{
		{"go build", "go build ./...", "compile"},
		{"go test with pkg/ir", "go test ./pkg/ir/... -v", "oracle"},
		{"go test ./... without ir", "go test ./...", "regress"},
		{"make check-generated", "make check-generated", "regress"},
		{"unknown", "ls -la", ""},
		{"empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build a minimal events.jsonl with the right shell command
			var events string
			if tc.cmd != "" {
				events = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"` + tc.cmd + `"}}]}}` + "\n"
			} else {
				events = `{"type":"system","subtype":"init"}` + "\n"
			}
			now := baseTime()
			lastMod := now.Add(-5 * time.Second)
			ws := ProjectWorkingState([]byte(events), lastMod, now, time.Minute)
			if ws.PhaseGuess != tc.want {
				t.Errorf("PhaseGuess for cmd %q = %q, want %q", tc.cmd, ws.PhaseGuess, tc.want)
			}
		})
	}
}

func TestProjectWorkingState_OraclePrecedenceOverRegress(t *testing.T) {
	// "go test" with "pkg/ir" should be oracle, not regress — oracle is checked first
	cmd := "go test ./pkg/ir/..."
	events := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"` + cmd + `"}}]}}` + "\n"
	now := baseTime()
	lastMod := now.Add(-5 * time.Second)
	ws := ProjectWorkingState([]byte(events), lastMod, now, time.Minute)
	if ws.PhaseGuess != "oracle" {
		t.Errorf("PhaseGuess = %q, want %q", ws.PhaseGuess, "oracle")
	}
}

func TestProjectWorkingState_MostRecentCtxShell(t *testing.T) {
	// two ctx_shell calls — the LAST one by file order wins
	events := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"mcp__lean-ctx__ctx_shell","input":{"command":"go build ./..."}}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"mcp__lean-ctx__ctx_shell","input":{"command":"go test ./pkg/ir/..."}}]}}
`
	now := baseTime()
	lastMod := now.Add(-5 * time.Second)
	ws := ProjectWorkingState([]byte(events), lastMod, now, time.Minute)
	if ws.LastShellCmd != "go test ./pkg/ir/..." {
		t.Errorf("LastShellCmd = %q, want last ctx_shell cmd", ws.LastShellCmd)
	}
}

func TestProjectWorkingState_MostRecentRead(t *testing.T) {
	// two ctx_read calls — last one wins; a later plain Read does NOT override
	events := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"mcp__lean-ctx__ctx_read","input":{"file_path":"/repo/a.go"}}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"mcp__lean-ctx__ctx_read","input":{"file_path":"/repo/b.go"}}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t3","name":"Read","input":{"file_path":"/repo/c.go"}}]}}
`
	now := baseTime()
	lastMod := now.Add(-5 * time.Second)
	ws := ProjectWorkingState([]byte(events), lastMod, now, time.Minute)
	if ws.LastRead != "/repo/b.go" {
		t.Errorf("LastRead = %q, want /repo/b.go (last ctx_read)", ws.LastRead)
	}
}

func TestProjectWorkingState_EmptyInput(t *testing.T) {
	now := baseTime()
	lastMod := now.Add(-5 * time.Second)
	ws := ProjectWorkingState([]byte{}, lastMod, now, time.Minute)
	if ws.EventCount != 0 {
		t.Errorf("EventCount = %d, want 0", ws.EventCount)
	}
	if ws.LastShellTool != "" || ws.LastShellCmd != "" || ws.LastRead != "" || ws.PhaseGuess != "" {
		t.Errorf("unexpected non-empty fields on empty input: %+v", ws)
	}
}
