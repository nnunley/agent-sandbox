package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/agent-sandbox/usage"
)

func usageSourceInteractive() usage.Source { return usage.SourceInteractive }
func usageSourceFleet() usage.Source       { return usage.SourceFleet }

func TestUsageScanner_NonStreaming(t *testing.T) {
	body := `{"id":"msg_x","model":"claude-opus-4-8","usage":{"input_tokens":120,"cache_creation_input_tokens":5,"cache_read_input_tokens":7,"output_tokens":33}}`
	sc := newUsageScanner("anthropic")
	// Drive it the way the proxy does: copy through a TeeReader.
	var sink strings.Builder
	if _, err := io.Copy(&sink, io.TeeReader(strings.NewReader(body), sc)); err != nil {
		t.Fatal(err)
	}
	if sink.String() != body {
		t.Fatal("scanner altered the passed-through bytes")
	}
	now := time.Unix(1000, 0).UTC()
	ev, ok := sc.result(now)
	if !ok {
		t.Fatal("result ok=false, want a usage event")
	}
	if ev.Provider != "anthropic" || ev.Model != "claude-opus-4-8" {
		t.Fatalf("header fields wrong: %+v", ev)
	}
	if ev.InputTokens != 120 || ev.CacheCreationTokens != 5 || ev.CacheReadTokens != 7 || ev.OutputTokens != 33 {
		t.Fatalf("tokens wrong: %+v", ev)
	}
	if ev.TurnID != "msg_x" {
		t.Fatalf("turn_id=%q want msg_x", ev.TurnID)
	}
	if !ev.Ts.Equal(now) {
		t.Fatalf("ts=%v want now", ev.Ts)
	}
}

func TestUsageScanner_StreamingSSE(t *testing.T) {
	// message_start carries input/cache under message.usage; message_delta carries
	// cumulative output_tokens at the top level. Interleaved with other events.
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-opus-4-8","usage":{"input_tokens":200,"cache_creation_input_tokens":10,"cache_read_input_tokens":20,"output_tokens":1}}}`,
		``,
		`event: ping`,
		`data: {"type":"ping"}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","usage":{"output_tokens":77}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	sc := newUsageScanner("anthropic")
	var sink strings.Builder
	_, _ = io.Copy(&sink, io.TeeReader(strings.NewReader(stream), sc))
	if sink.String() != stream {
		t.Fatal("scanner altered streamed bytes")
	}
	ev, ok := sc.result(time.Unix(2000, 0).UTC())
	if !ok {
		t.Fatal("result ok=false")
	}
	if ev.InputTokens != 200 || ev.CacheCreationTokens != 10 || ev.CacheReadTokens != 20 {
		t.Fatalf("input/cache wrong: %+v", ev)
	}
	if ev.OutputTokens != 77 {
		t.Fatalf("output=%d want 77 (cumulative from message_delta)", ev.OutputTokens)
	}
	if ev.Model != "claude-opus-4-8" {
		t.Fatalf("model=%q", ev.Model)
	}
	if ev.TurnID != "msg_1" {
		t.Fatalf("turn_id=%q want msg_1", ev.TurnID)
	}
}

func TestUsageScanner_StreamingSSEChunkedWrite(t *testing.T) {
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_chunk","model":"claude-opus-4-8","usage":{"input_tokens":200,"cache_creation_input_tokens":10,"cache_read_input_tokens":20,"output_tokens":1}}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","usage":{"output_tokens":77}}`,
		``,
	}, "\n")
	sc := newUsageScanner("anthropic")
	split := len(stream) / 2
	if _, err := sc.Write([]byte(stream[:split])); err != nil {
		t.Fatal(err)
	}
	if _, err := sc.Write([]byte(stream[split:])); err != nil {
		t.Fatal(err)
	}
	ev, ok := sc.result(time.Unix(2100, 0).UTC())
	if !ok {
		t.Fatal("result ok=false")
	}
	if ev.InputTokens != 200 || ev.CacheCreationTokens != 10 || ev.CacheReadTokens != 20 || ev.OutputTokens != 77 {
		t.Fatalf("tokens wrong after chunked writes: %+v", ev)
	}
	if ev.TurnID != "msg_chunk" {
		t.Fatalf("turn_id=%q want msg_chunk", ev.TurnID)
	}
}

func TestUsageScanner_NoUsage(t *testing.T) {
	sc := newUsageScanner("anthropic")
	_, _ = io.Copy(io.Discard, io.TeeReader(strings.NewReader(`{"error":"x"}`), sc))
	if _, ok := sc.result(time.Now()); ok {
		t.Fatal("a body with no usage must yield ok=false")
	}
}

func TestSourceForClass(t *testing.T) {
	if sourceForClass(classInteractive) != usageSourceInteractive() {
		t.Fatal("interactive class must map to SourceInteractive")
	}
	if sourceForClass(classFleet) != usageSourceFleet() {
		t.Fatal("fleet class must map to SourceFleet")
	}
}
