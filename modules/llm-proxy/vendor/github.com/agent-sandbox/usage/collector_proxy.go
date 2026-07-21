package usage

import (
	"encoding/json"
	"time"
)

type anthropicBody struct {
	Model string `json:"model"`
	Usage *struct {
		InputTokens         int64 `json:"input_tokens"`
		CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
		CacheReadTokens     int64 `json:"cache_read_input_tokens"`
		OutputTokens        int64 `json:"output_tokens"`
	} `json:"usage"`
}

// ParseAnthropicUsage parses a fleet Anthropic response body into a UsageEvent.
// Returns ok=false when the body carries no usage block. now stamps the event.
func ParseAnthropicUsage(provider string, body []byte, now time.Time) (UsageEvent, bool) {
	var b anthropicBody
	if err := json.Unmarshal(body, &b); err != nil || b.Usage == nil {
		return UsageEvent{}, false
	}
	u := b.Usage
	if u.InputTokens == 0 && u.CacheCreationTokens == 0 && u.CacheReadTokens == 0 && u.OutputTokens == 0 {
		return UsageEvent{}, false
	}
	return UsageEvent{
		Provider:            provider,
		Model:               b.Model,
		InputTokens:         u.InputTokens,
		CacheCreationTokens: u.CacheCreationTokens,
		CacheReadTokens:     u.CacheReadTokens,
		OutputTokens:        u.OutputTokens,
		Ts:                  now,
		Source:              SourceFleet,
	}, true
}
