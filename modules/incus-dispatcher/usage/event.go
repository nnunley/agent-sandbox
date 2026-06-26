// Package usage measures per-provider LLM token usage and estimates remaining
// per-window budget. Measurement only — no enforcement (that is sub-project 2).
package usage

import "time"

// Source identifies where a usage event was captured.
type Source string

const (
	SourceFleet       Source = "fleet"       // a fleet worker call brokered by the llm-proxy
	SourceInteractive Source = "interactive" // an interactive Claude Code turn (bypasses the proxy)
)

// UsageEvent is one captured unit of provider token usage. Token categories are kept
// separate (Anthropic reports input, cache-creation, cache-read, and output distinctly);
// Total sums them so the estimator learns a ceiling in consistent units.
type UsageEvent struct {
	Provider            string    `json:"provider"`
	Model               string    `json:"model,omitempty"`
	InputTokens         int64     `json:"input_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int64     `json:"cache_read_tokens,omitempty"`
	OutputTokens        int64     `json:"output_tokens"`
	Ts                  time.Time `json:"ts"`
	Source              Source    `json:"source"`
	TurnID              string    `json:"turn_id,omitempty"`  // provider message id; used to de-dup streaming vs transcript
	Estimated           bool      `json:"estimated,omitempty"` // true when counts are a local fallback, not provider-reported
}

// Total is the sum of all token categories — the figure the estimator meters against.
func (e UsageEvent) Total() int64 {
	return e.InputTokens + e.CacheCreationTokens + e.CacheReadTokens + e.OutputTokens
}

// LimitEvent records an observed exhaustion/throttle: the cumulative window usage at the
// moment the provider reported "limit reached" (HTTP 429 / rate-limit notice). UsedAt is
// the realized effective ceiling for that (provider, window-class) window.
type LimitEvent struct {
	Provider    string    `json:"provider"`
	WindowClass string    `json:"window_class"` // "5h" | "weekly"
	UsedAt      int64     `json:"used_at"`
	Ts          time.Time `json:"ts"`
}
