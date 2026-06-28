package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/agent-sandbox/usage"
)

func TestMetering_AppendsFleetUsage(t *testing.T) {
	t0 := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	ledger := filepath.Join(dir, "usage.jsonl")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"model":"claude-opus-4-8","usage":{"input_tokens":120,"output_tokens":33}}`)
	}))
	defer upstream.Close()

	s, _ := newTestServer(t, ledger, upstream.URL, t0)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/anthropic/v1/messages", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	l, _ := usage.OpenLedger(ledger)
	evs := l.Events()
	if len(evs) != 1 {
		t.Fatalf("ledger has %d events, want 1", len(evs))
	}
	if evs[0].Source != usage.SourceFleet || evs[0].InputTokens != 120 || evs[0].OutputTokens != 33 {
		t.Fatalf("metered event wrong: %+v", evs[0])
	}
}

func TestMetering_InteractiveSource(t *testing.T) {
	t0 := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	ledger := filepath.Join(dir, "usage.jsonl")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"model":"claude-opus-4-8","usage":{"input_tokens":5,"output_tokens":7}}`)
	}))
	defer upstream.Close()
	s, _ := newTestServer(t, ledger, upstream.URL, t0)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/interactive/anthropic/v1/messages", "application/json", nil)
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	l, _ := usage.OpenLedger(ledger)
	evs := l.Events()
	if len(evs) != 1 || evs[0].Source != usage.SourceInteractive {
		t.Fatalf("want 1 interactive-source event, got %+v", evs)
	}
}

func TestMetering_CalibratesOnUpstream429(t *testing.T) {
	t0 := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	ledger := filepath.Join(dir, "usage.jsonl")
	// Some fleet usage already this window, no ceiling yet (uncalibrated -> gate fails open).
	l0, _ := usage.OpenLedger(ledger)
	_ = l0.Append(usage.UsageEvent{Provider: "anthropic", OutputTokens: 123_000, Ts: t0, Source: usage.SourceFleet})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer upstream.Close()
	s, _ := newTestServer(t, ledger, upstream.URL, t0.Add(time.Minute))
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, _ := http.Post(srv.URL+"/anthropic/v1/messages", "application/json", nil)
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	l, _ := usage.OpenLedger(ledger)
	lims := l.Limits()
	if len(lims) != 1 {
		t.Fatalf("limits=%d want 1 (calibration on upstream 429)", len(lims))
	}
	if lims[0].Provider != "anthropic" || lims[0].WindowClass != "5h" || lims[0].UsedAt != 123_000 {
		t.Fatalf("calibration limit wrong: %+v", lims[0])
	}
}
