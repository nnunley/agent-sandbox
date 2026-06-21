package main

import (
	"fmt"
	"strings"
	"time"
)

// RenderHeartbeat renders a one-line worker heartbeat from a projected
// WorkingState (STORY-0071 AC-2). It reports the last shell command (ctx_shell
// preferred, Bash fallback) and the age of the last activity, so an active
// worker never falsely reads as "(no shell yet)". The "(no shell yet)" marker
// appears only when no shell command has been observed.
func RenderHeartbeat(ws WorkingState) string {
	var b strings.Builder

	if ws.Alive {
		b.WriteString("alive")
	} else {
		b.WriteString("STALE")
	}
	fmt.Fprintf(&b, " | %s ago | events=%d", ws.SinceLast.Round(time.Second), ws.EventCount)

	if ws.LastShellTool != "" {
		fmt.Fprintf(&b, " | %s: %s", ws.LastShellTool, truncateCmd(ws.LastShellCmd))
	} else {
		b.WriteString(" | (no shell yet)")
	}

	if ws.PhaseGuess != "" {
		fmt.Fprintf(&b, " | phase=%s", ws.PhaseGuess)
	}
	if ws.LastRead != "" {
		fmt.Fprintf(&b, " | read=%s", ws.LastRead)
	}
	return b.String()
}

// truncateCmd keeps heartbeat lines to one screen row.
func truncateCmd(cmd string) string {
	const max = 80
	cmd = strings.TrimSpace(cmd)
	if len(cmd) > max {
		return cmd[:max-1] + "…"
	}
	return cmd
}
