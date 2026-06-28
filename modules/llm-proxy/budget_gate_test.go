package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-sandbox/usage"
)

// newTestServer builds a Server whose single fleet route points at a counting
// upstream, with a ledger at ledgerPath and a fixed clock.
func newTestServer(t *testing.T, ledgerPath string, upstreamURL string, now time.Time) (*Server, *int64) {
	t.Helper()
	var upstreamHits int64
	// (the caller wires the upstream; here we only assemble the Server)
	rt, err := newRoute("/anthropic", upstreamURL, "k", "anthropic", true, classFleet)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := newRoute("/interactive/anthropic", upstreamURL, "k", "anthropic", true, classInteractive)
	if err != nil {
		t.Fatal(err)
	}
	s := newServer([]route{rt, ir}, &nopLogSink{})
	s.ledgerPath = ledgerPath
	s.reservePct = 0.30
	s.priors = map[string]int64{}
	s.now = func() time.Time { return now }
	return s, &upstreamHits
}

type nopLogSink struct{}

func (nopLogSink) Log(logEntry) {}

func TestBudgetGate_DefersOverCapFleet_NoUpstream(t *testing.T) {
	t0 := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	ledger := filepath.Join(dir, "usage.jsonl")
	// Calibrate a ceiling of 1,000,000 and record 800,000 already used this window.
	l, _ := usage.OpenLedger(ledger)
	_ = l.AppendLimit(usage.LimitEvent{Provider: "anthropic", WindowClass: "5h", UsedAt: 1_000_000, Ts: t0.Add(-time.Hour)})
	_ = l.Append(usage.UsageEvent{Provider: "anthropic", OutputTokens: 800_000, Ts: t0, Source: usage.SourceFleet})

	var hits int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	s, _ := newTestServer(t, ledger, upstream.URL, t0.Add(time.Hour))
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Fleet call: cap = 1,000,000 * 0.70 = 700,000; used 800,000 -> deferred.
	resp, err := http.Post(srv.URL+"/anthropic/v1/messages", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status=%d want 429", resp.StatusCode)
	}
	if resp.Header.Get("X-Budget-Deferred") != "1" {
		t.Fatal("missing X-Budget-Deferred marker")
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	if got := atomic.LoadInt64(&hits); got != 0 {
		t.Fatalf("upstream hit %d times, want 0 (deferred before forwarding)", got)
	}
}

func TestBudgetGate_InteractiveNeverCapped(t *testing.T) {
	t0 := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	ledger := filepath.Join(dir, "usage.jsonl")
	l, _ := usage.OpenLedger(ledger)
	_ = l.AppendLimit(usage.LimitEvent{Provider: "anthropic", WindowClass: "5h", UsedAt: 1_000_000, Ts: t0.Add(-time.Hour)})
	_ = l.Append(usage.UsageEvent{Provider: "anthropic", OutputTokens: 800_000, Ts: t0, Source: usage.SourceInteractive})

	var hits int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	s, _ := newTestServer(t, ledger, upstream.URL, t0.Add(time.Hour))
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Same over-cap usage, but via the interactive route -> always forwarded.
	resp, err := http.Post(srv.URL+"/interactive/anthropic/v1/messages", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d want 200 (interactive never capped)", resp.StatusCode)
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Fatalf("upstream hit %d times, want 1", got)
	}
}

func TestBudgetGate_FailsOpenOnMissingLedger(t *testing.T) {
	t0 := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	s, _ := newTestServer(t, "/nonexistent/dir/usage.jsonl", "https://example.com", t0)
	allow, _ := s.budgetAllows("anthropic", t0)
	if !allow {
		t.Fatal("missing ledger must fail open (allow)")
	}
}
