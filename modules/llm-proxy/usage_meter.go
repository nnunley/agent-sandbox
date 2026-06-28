package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/agent-sandbox/usage"
)

// usageLedgerPath mirrors the dispatcher's defaultLedgerPath: FLEET_USAGE_LEDGER
// overrides; otherwise ~/.fleet/usage.jsonl.
func usageLedgerPath() string {
	if p := os.Getenv("FLEET_USAGE_LEDGER"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fleet", "usage.jsonl")
}

// sourceForClass maps a route class to the ledger Source it produces.
func sourceForClass(c routeClass) usage.Source {
	if c == classInteractive {
		return usage.SourceInteractive
	}
	return usage.SourceFleet
}

// usageScannerLineCap bounds the partial-line buffer so a pathological body
// without newlines cannot grow memory without limit. A real non-streaming
// Anthropic body is a few KB; SSE data lines are small.
const usageScannerLineCap = 1 << 20 // 1 MiB

// tokenBlock is the usage shape Anthropic emits, both as a streaming SSE field
// and in a non-streaming body.
type tokenBlock struct {
	Input       int64 `json:"input_tokens"`
	CacheCreate int64 `json:"cache_creation_input_tokens"`
	CacheRead   int64 `json:"cache_read_input_tokens"`
	Output      int64 `json:"output_tokens"`
}

// wireUsage covers both layouts: top-level usage (non-streaming + message_delta)
// and usage nested under message (message_start).
type wireUsage struct {
	Model string      `json:"model"`
	Usage *tokenBlock `json:"usage"`
	Message struct {
		Model string      `json:"model"`
		Usage *tokenBlock `json:"usage"`
	} `json:"message"`
}

// usageScanner observes a passing response stream and extracts token usage
// without buffering the whole body, reordering, or delaying client bytes. It is
// fed via io.TeeReader, so its Write receives exactly the bytes sent to the client.
type usageScanner struct {
	provider                         string
	tail                             []byte
	in, cacheCreate, cacheRead, out int64
	model                            string
	got                              bool
}

func newUsageScanner(provider string) *usageScanner {
	return &usageScanner{provider: provider}
}

func (s *usageScanner) Write(p []byte) (int, error) {
	s.tail = append(s.tail, p...)
	for {
		i := bytes.IndexByte(s.tail, '\n')
		if i < 0 {
			if len(s.tail) > usageScannerLineCap {
				s.scanLine(s.tail)
				s.tail = s.tail[:0]
			}
			break
		}
		s.scanLine(s.tail[:i])
		s.tail = append([]byte(nil), s.tail[i+1:]...)
	}
	return len(p), nil
}

// scanLine inspects one line (SSE "data:" lines or a whole non-streaming body),
// merging any usage fields it finds. Non-usage / unparseable lines are ignored.
func (s *usageScanner) scanLine(line []byte) {
	line = bytes.TrimSpace(line)
	line = bytes.TrimPrefix(line, []byte("data:"))
	line = bytes.TrimSpace(line)
	if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) || line[0] != '{' {
		return
	}
	var w wireUsage
	if err := json.Unmarshal(line, &w); err != nil {
		return
	}
	blk := w.Usage
	if blk == nil {
		blk = w.Message.Usage
	}
	if blk == nil {
		return
	}
	if blk.Input > 0 {
		s.in = blk.Input
	}
	if blk.CacheCreate > 0 {
		s.cacheCreate = blk.CacheCreate
	}
	if blk.CacheRead > 0 {
		s.cacheRead = blk.CacheRead
	}
	if blk.Output > 0 {
		s.out = blk.Output // cumulative; latest wins
	}
	if m := w.Model; m != "" {
		s.model = m
	} else if m := w.Message.Model; m != "" {
		s.model = m
	}
	s.got = true
}

// result flushes any trailing partial line and returns the merged usage event.
// ok is false when no usage was seen.
func (s *usageScanner) result(now time.Time) (usage.UsageEvent, bool) {
	if len(s.tail) > 0 {
		s.scanLine(s.tail)
		s.tail = s.tail[:0]
	}
	if !s.got {
		return usage.UsageEvent{}, false
	}
	return usage.UsageEvent{
		Provider:            s.provider,
		Model:               s.model,
		InputTokens:         s.in,
		CacheCreationTokens: s.cacheCreate,
		CacheReadTokens:     s.cacheRead,
		OutputTokens:        s.out,
		Ts:                  now,
	}, true
}
