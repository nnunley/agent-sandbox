# Cross-Provider Usage Meter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a durable, per-provider usage ledger + remaining-budget estimator (sub-project 1 of the budget-aware worklist orchestrator) that measures real Claude Code + fleet token usage and learns each provider's arbitrary per-window ceiling from observed exhaustion.

**Architecture:** A new stdlib-only Go package `usage` under `modules/incus-dispatcher/` with four units — typed events, an append-only JSONL ledger, an anchored-window estimator, and Claude-Code/proxy collectors — plus a `dispatcher usage` CLI readout. Measurement only; no enforcement (that is sub-project 2). The estimator is a pure function over a ledger snapshot with an injected clock.

**Tech Stack:** Go 1.25.6, standard library only (encoding/json, os, bufio, sync, time). No third-party deps.

## Global Constraints

- Go module: `modules/incus-dispatcher/` (nested go.mod; run all `go` commands from that dir). New package import path: `github.com/agent-sandbox/incus-dispatcher/usage`.
- **Standard library only** — no new third-party dependencies (matches `modules/llm-proxy`).
- **Injected clock:** pure logic (window math, estimator) takes `now time.Time` as a parameter; never call `time.Now()` inside it. Collectors/CLI may read the real clock at the edge.
- **Durability pattern:** the ledger mirrors the existing `FileEscalationLane` (`modules/incus-dispatcher/fileescalationlane.go`) — append a JSON line + `file.Sync()` before returning; reconstruct in-memory state on open; skip a corrupt line with a logged warning, never fatal.
- **Tests run under `-race`:** `cd modules/incus-dispatcher && go test -race ./usage/` (and `./...` for the full suite). `go vet ./...` must stay clean.
- **Commit messages must NOT contain** `Claude`, `Generated with`, `Co-Authored-By: Claude`, or `claude.com/claude-code` (a pre-commit hook blocks them). Use plain human-style messages.
- **Defensive copies:** any method returning a slice of ledger state returns a copy, not the backing array (codebase convention, see `audit.go`).

---

### Task 1: Usage + limit event types

**Files:**
- Create: `modules/incus-dispatcher/usage/event.go`
- Test: `modules/incus-dispatcher/usage/event_test.go`

**Interfaces:**
- Produces: `UsageEvent` struct, `LimitEvent` struct, `Source` consts (`SourceFleet`, `SourceInteractive`), `func (UsageEvent) Total() int64`.

- [ ] **Step 1: Write the failing test**

```go
package usage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUsageEvent_TotalAndJSON(t *testing.T) {
	ts := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	e := UsageEvent{
		Provider: "anthropic", Model: "claude-opus-4-8",
		InputTokens: 20260, CacheCreationTokens: 14744, CacheReadTokens: 15874, OutputTokens: 383,
		Ts: ts, Source: SourceInteractive, TurnID: "msg_01Wk",
	}
	if got, want := e.Total(), int64(20260+14744+15874+383); got != want {
		t.Fatalf("Total()=%d want %d", got, want)
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back UsageEvent
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Total() != e.Total() || back.TurnID != e.TurnID || back.Source != SourceInteractive {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
}

func TestLimitEvent_JSON(t *testing.T) {
	le := LimitEvent{Provider: "anthropic", WindowClass: "5h", UsedAt: 1_200_000, Ts: time.Unix(100, 0).UTC()}
	b, _ := json.Marshal(le)
	var back LimitEvent
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.UsedAt != le.UsedAt || back.WindowClass != "5h" {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestUsageEvent -v`
Expected: FAIL — package/identifiers undefined (`undefined: UsageEvent`).

- [ ] **Step 3: Write minimal implementation**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run 'TestUsageEvent|TestLimitEvent' -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add modules/incus-dispatcher/usage/event.go modules/incus-dispatcher/usage/event_test.go
git commit -m "usage: UsageEvent + LimitEvent types"
```

---

### Task 2: Durable JSONL ledger

**Files:**
- Create: `modules/incus-dispatcher/usage/ledger.go`
- Test: `modules/incus-dispatcher/usage/ledger_test.go`

**Interfaces:**
- Consumes: `UsageEvent`, `LimitEvent` (Task 1).
- Produces: `func OpenLedger(path string) (*Ledger, error)`, `func (*Ledger) Append(UsageEvent) error`, `func (*Ledger) AppendLimit(LimitEvent) error`, `func (*Ledger) Events() []UsageEvent`, `func (*Ledger) Limits() []LimitEvent`.

- [ ] **Step 1: Write the failing test**

```go
package usage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLedger_AppendReopenDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ts := time.Unix(1000, 0).UTC()
	if err := l.Append(UsageEvent{Provider: "anthropic", OutputTokens: 5, Ts: ts, Source: SourceInteractive, TurnID: "a"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := l.AppendLimit(LimitEvent{Provider: "anthropic", WindowClass: "5h", UsedAt: 999, Ts: ts}); err != nil {
		t.Fatalf("appendLimit: %v", err)
	}

	// Reopen a SECOND ledger over the same file: it must reconstruct both records (durability).
	l2, err := OpenLedger(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if ev := l2.Events(); len(ev) != 1 || ev[0].TurnID != "a" {
		t.Fatalf("events after reopen = %+v, want 1 with TurnID a", ev)
	}
	if lim := l2.Limits(); len(lim) != 1 || lim[0].UsedAt != 999 {
		t.Fatalf("limits after reopen = %+v, want 1 with UsedAt 999", lim)
	}
}

func TestLedger_SkipsCorruptLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	l, _ := OpenLedger(path)
	_ = l.Append(UsageEvent{Provider: "anthropic", OutputTokens: 1, Ts: time.Unix(1, 0).UTC()})
	// Append a garbage line directly, then a good one.
	appendRawLine(t, path, "{not json")
	l3, _ := OpenLedger(path)
	if err := l3.Append(UsageEvent{Provider: "openai", OutputTokens: 2, Ts: time.Unix(2, 0).UTC()}); err != nil {
		t.Fatalf("append after corrupt: %v", err)
	}
	l4, err := OpenLedger(path)
	if err != nil {
		t.Fatalf("reopen after corrupt: %v", err) // corrupt line must NOT be fatal
	}
	if got := len(l4.Events()); got != 2 {
		t.Fatalf("events = %d, want 2 (corrupt line skipped)", got)
	}
}
```

Add this helper to the same test file:

```go
import "os"

func appendRawLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("write raw: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestLedger -v`
Expected: FAIL — `undefined: OpenLedger`.

- [ ] **Step 3: Write minimal implementation**

```go
package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ledgerLine is the discriminated JSONL record: exactly one of Usage/Limit is set.
type ledgerLine struct {
	Kind  string      `json:"kind"` // "usage" | "limit"
	Usage *UsageEvent `json:"usage,omitempty"`
	Limit *LimitEvent `json:"limit,omitempty"`
}

// Ledger is an append-only, fsync-durable JSONL store of usage and limit events.
// It reconstructs in-memory state on open and is safe for concurrent use.
type Ledger struct {
	mu     sync.Mutex
	path   string
	events []UsageEvent
	limits []LimitEvent
}

// OpenLedger opens (creating if absent) the ledger at path and reconstructs prior records.
// A malformed line is skipped with a stderr warning, never fatal.
func OpenLedger(path string) (*Ledger, error) {
	l := &Ledger{path: path}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var rec ledgerLine
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			fmt.Fprintf(os.Stderr, "usage ledger: skipping malformed line: %v\n", err)
			continue
		}
		switch rec.Kind {
		case "usage":
			if rec.Usage != nil {
				l.events = append(l.events, *rec.Usage)
			}
		case "limit":
			if rec.Limit != nil {
				l.limits = append(l.limits, *rec.Limit)
			}
		}
	}
	return l, sc.Err()
}

func (l *Ledger) appendLine(rec ledgerLine) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync() // durability: the line must reach disk before we return
}

// Append records a usage event durably and in memory.
func (l *Ledger) Append(e UsageEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.appendLine(ledgerLine{Kind: "usage", Usage: &e}); err != nil {
		return err
	}
	l.events = append(l.events, e)
	return nil
}

// AppendLimit records an exhaustion/limit event durably and in memory.
func (l *Ledger) AppendLimit(le LimitEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.appendLine(ledgerLine{Kind: "limit", Limit: &le}); err != nil {
		return err
	}
	l.limits = append(l.limits, le)
	return nil
}

// Events returns a defensive copy of all usage events in append order.
func (l *Ledger) Events() []UsageEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]UsageEvent, len(l.events))
	copy(out, l.events)
	return out
}

// Limits returns a defensive copy of all limit events in append order.
func (l *Ledger) Limits() []LimitEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]LimitEvent, len(l.limits))
	copy(out, l.limits)
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./usage/ -run TestLedger -v`
Expected: PASS (both tests), race-clean.

- [ ] **Step 5: Commit**

```bash
git add modules/incus-dispatcher/usage/ledger.go modules/incus-dispatcher/usage/ledger_test.go
git commit -m "usage: durable fsync JSONL ledger with corrupt-line skip"
```

---

### Task 3: Anchored window model

**Files:**
- Create: `modules/incus-dispatcher/usage/window.go`
- Test: `modules/incus-dispatcher/usage/window_test.go`

**Interfaces:**
- Consumes: `UsageEvent` (Task 1).
- Produces: `WindowClass` struct, `Window5h`, `WindowWeekly` vars, `func CurrentWindow(events []UsageEvent, wc WindowClass, now time.Time) (anchor, reset time.Time, active bool)`.

**Behavior:** the window is anchored on first-use-after-idle (NOT fixed clock). Walking provider events in time order, the anchor starts at the earliest event and re-anchors to any event at/after `anchor+Length`. The active window is the last anchored window; it is `active` iff `now < anchor+Length`. If `now >= anchor+Length` the window has expired (idle past expiry) and `active` is false — the next event would re-anchor.

- [ ] **Step 1: Write the failing test**

```go
package usage

import (
	"testing"
	"time"
)

func ev(min int) UsageEvent { // event at T0 + min minutes
	return UsageEvent{Provider: "anthropic", OutputTokens: 1, Ts: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC).Add(time.Duration(min) * time.Minute)}
}

func TestCurrentWindow_ContinuousStaysAnchored(t *testing.T) {
	evs := []UsageEvent{ev(0), ev(60), ev(120)} // all within 5h of T0
	now := ev(150).Ts
	anchor, reset, active := CurrentWindow(evs, Window5h, now)
	if !anchor.Equal(ev(0).Ts) {
		t.Fatalf("anchor=%v want %v", anchor, ev(0).Ts)
	}
	if !reset.Equal(ev(0).Ts.Add(5 * time.Hour)) {
		t.Fatalf("reset=%v", reset)
	}
	if !active {
		t.Fatal("want active (now within window)")
	}
}

func TestCurrentWindow_IdlePastExpiryReanchors(t *testing.T) {
	// ev(0), then a gap > 5h: ev(360) is 6h after T0 → re-anchors at ev(360).
	evs := []UsageEvent{ev(0), ev(360)}
	now := ev(370).Ts
	anchor, _, active := CurrentWindow(evs, Window5h, now)
	if !anchor.Equal(ev(360).Ts) {
		t.Fatalf("anchor=%v want re-anchored %v", anchor, ev(360).Ts)
	}
	if !active {
		t.Fatal("want active (now within re-anchored window)")
	}
}

func TestCurrentWindow_SubExpiryGapDoesNotReanchor(t *testing.T) {
	// gap of 4h (< 5h) keeps the original anchor.
	evs := []UsageEvent{ev(0), ev(240)}
	now := ev(250).Ts
	anchor, _, active := CurrentWindow(evs, Window5h, now)
	if !anchor.Equal(ev(0).Ts) {
		t.Fatalf("anchor=%v want %v (no re-anchor)", anchor, ev(0).Ts)
	}
	if !active {
		t.Fatal("want active")
	}
}

func TestCurrentWindow_ExpiredWhenIdleNow(t *testing.T) {
	evs := []UsageEvent{ev(0)}
	now := ev(301).Ts // 5h1m after anchor → expired, no new event yet
	_, _, active := CurrentWindow(evs, Window5h, now)
	if active {
		t.Fatal("want NOT active (window expired, idle)")
	}
}

func TestCurrentWindow_NoEvents(t *testing.T) {
	_, _, active := CurrentWindow(nil, Window5h, ev(0).Ts)
	if active {
		t.Fatal("want NOT active with no events")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestCurrentWindow -v`
Expected: FAIL — `undefined: CurrentWindow`.

- [ ] **Step 3: Write minimal implementation**

```go
package usage

import (
	"sort"
	"time"
)

// WindowClass is a usage window of a fixed length, anchored on first-use-after-idle.
type WindowClass struct {
	Name   string
	Length time.Duration
}

var (
	// Window5h is the Claude Code Max ~5-hour session window (anchored on first use).
	Window5h = WindowClass{Name: "5h", Length: 5 * time.Hour}
	// WindowWeekly is the longer weekly window, tracked the same way.
	WindowWeekly = WindowClass{Name: "weekly", Length: 7 * 24 * time.Hour}
)

// CurrentWindow derives the active window's anchor and reset for class wc from events,
// at time now. Anchor floats: it starts at the earliest event and re-anchors to any event
// at/after anchor+Length. The window is active iff now is before anchor+Length.
func CurrentWindow(events []UsageEvent, wc WindowClass, now time.Time) (anchor, reset time.Time, active bool) {
	if len(events) == 0 {
		return time.Time{}, time.Time{}, false
	}
	ts := make([]time.Time, 0, len(events))
	for _, e := range events {
		if !e.Ts.After(now) { // only events up to now
			ts = append(ts, e.Ts)
		}
	}
	if len(ts) == 0 {
		return time.Time{}, time.Time{}, false
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].Before(ts[j]) })
	anchor = ts[0]
	for _, t := range ts[1:] {
		if !t.Before(anchor.Add(wc.Length)) { // t >= anchor+Length → re-anchor
			anchor = t
		}
	}
	reset = anchor.Add(wc.Length)
	active = now.Before(reset)
	return anchor, reset, active
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestCurrentWindow -v`
Expected: PASS (all five).

- [ ] **Step 5: Commit**

```bash
git add modules/incus-dispatcher/usage/window.go modules/incus-dispatcher/usage/window_test.go
git commit -m "usage: anchored-on-first-use window model"
```

---

### Task 4: Estimator — used + window facts (visibility-sooner)

**Files:**
- Create: `modules/incus-dispatcher/usage/estimator.go`
- Test: `modules/incus-dispatcher/usage/estimator_test.go`

**Interfaces:**
- Consumes: `UsageEvent`, `LimitEvent`, `CurrentWindow`, `WindowClass` (Tasks 1, 3).
- Produces: `Confidence` consts, `Estimate` struct, `Estimator` struct, `func (Estimator) Estimate(events []UsageEvent, limits []LimitEvent, provider string, wc WindowClass, now time.Time) Estimate`.

This task delivers the **visibility-sooner** requirement: with zero calibration the estimate still returns correct `Used`, `WindowAnchor`, `WindowReset`, `WindowActive`, and `Confidence: ConfUncalibrated`. Ceiling learning is Task 5.

- [ ] **Step 1: Write the failing test**

```go
package usage

import (
	"testing"
	"time"
)

func TestEstimate_UsedAndWindowVisibleUncalibrated(t *testing.T) {
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	evs := []UsageEvent{
		{Provider: "anthropic", OutputTokens: 100, Ts: t0, Source: SourceInteractive},
		{Provider: "anthropic", InputTokens: 50, Ts: t0.Add(30 * time.Minute), Source: SourceFleet},
		{Provider: "openai", OutputTokens: 999, Ts: t0, Source: SourceFleet}, // other provider ignored
	}
	now := t0.Add(time.Hour)
	est := Estimator{}.Estimate(evs, nil, "anthropic", Window5h, now)

	if est.Used != 150 {
		t.Fatalf("Used=%d want 150 (only anthropic in window)", est.Used)
	}
	if !est.WindowAnchor.Equal(t0) || !est.WindowReset.Equal(t0.Add(5*time.Hour)) || !est.WindowActive {
		t.Fatalf("window facts wrong: %+v", est)
	}
	if est.Confidence != ConfUncalibrated {
		t.Fatalf("Confidence=%s want uncalibrated (no limits)", est.Confidence)
	}
	if est.CeilingEst != 0 {
		t.Fatalf("CeilingEst=%d want 0 when uncalibrated", est.CeilingEst)
	}
}

func TestEstimate_ExpiredWindowUsedZero(t *testing.T) {
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	evs := []UsageEvent{{Provider: "anthropic", OutputTokens: 100, Ts: t0}}
	now := t0.Add(6 * time.Hour) // idle past expiry
	est := Estimator{}.Estimate(evs, nil, "anthropic", Window5h, now)
	if est.WindowActive {
		t.Fatal("want window not active")
	}
	if est.Used != 0 {
		t.Fatalf("Used=%d want 0 (no active window)", est.Used)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestEstimate -v`
Expected: FAIL — `undefined: Estimator`.

- [ ] **Step 3: Write minimal implementation**

```go
package usage

import "time"

// Confidence labels how trustworthy a remaining estimate is.
type Confidence string

const (
	ConfUncalibrated Confidence = "uncalibrated" // no exhaustion observed yet
	ConfLow          Confidence = "low"
	ConfMed          Confidence = "med"
	ConfHigh         Confidence = "high"
)

// Estimate is the per-(provider,window) usage picture at a point in time.
type Estimate struct {
	Provider     string
	WindowClass  string
	Used         int64 // tokens used in the active window (0 if no active window)
	CeilingEst   int64 // learned effective ceiling; 0 when uncalibrated
	RemainingEst int64 // max(0, CeilingEst-Used) when calibrated; 0 otherwise
	Confidence   Confidence
	WindowAnchor time.Time
	WindowReset  time.Time
	WindowActive bool
}

// Estimator turns ledger snapshots into per-provider estimates. PublishedPrior is an
// optional weak per-provider ceiling prior (0 = unknown); it does not by itself calibrate.
type Estimator struct {
	PublishedPrior map[string]int64
}

// Estimate computes the active window's usage and (Task 5) the learned ceiling.
func (e Estimator) Estimate(events []UsageEvent, limits []LimitEvent, provider string, wc WindowClass, now time.Time) Estimate {
	// Filter this provider's events.
	var pev []UsageEvent
	for _, ev := range events {
		if ev.Provider == provider {
			pev = append(pev, ev)
		}
	}
	anchor, reset, active := CurrentWindow(pev, wc, now)

	var used int64
	if active {
		for _, ev := range pev {
			if !ev.Ts.Before(anchor) && ev.Ts.Before(reset) {
				used += ev.Total()
			}
		}
	}

	return Estimate{
		Provider:     provider,
		WindowClass:  wc.Name,
		Used:         used,
		Confidence:   ConfUncalibrated,
		WindowAnchor: anchor,
		WindowReset:  reset,
		WindowActive: active,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestEstimate -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add modules/incus-dispatcher/usage/estimator.go modules/incus-dispatcher/usage/estimator_test.go
git commit -m "usage: estimator window usage + visibility-sooner facts"
```

---

### Task 5: Estimator — ceiling learning from exhaustion

**Files:**
- Modify: `modules/incus-dispatcher/usage/estimator.go`
- Test: `modules/incus-dispatcher/usage/estimator_test.go`

**Interfaces:**
- Extends `Estimator.Estimate` to fill `CeilingEst`, `RemainingEst`, and a graded `Confidence` from `limits`.

**Behavior:** for this `(provider, wc.Name)`, gather matching `LimitEvent.UsedAt` calibration points. `CeilingEst` = the most recent calibration point (rolling — tracks plan/behavior changes). `Confidence`: 0 points → `ConfUncalibrated` (or `ConfLow` if a non-zero `PublishedPrior` exists, using the prior as `CeilingEst`); 1–2 → `ConfLow`; 3–5 → `ConfMed`; >5 → `ConfHigh`. `RemainingEst = max(0, CeilingEst-Used)` whenever `CeilingEst > 0`.

- [ ] **Step 1: Write the failing test**

```go
func TestEstimate_CalibratesFromExhaustion(t *testing.T) {
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	evs := []UsageEvent{{Provider: "anthropic", OutputTokens: 800_000, Ts: t0}}
	// One exhaustion at 1.2M → that IS the realized ceiling for the 5h window.
	limits := []LimitEvent{{Provider: "anthropic", WindowClass: "5h", UsedAt: 1_200_000, Ts: t0.Add(-time.Hour)}}
	now := t0.Add(time.Hour)
	est := Estimator{}.Estimate(evs, limits, "anthropic", Window5h, now)

	if est.CeilingEst != 1_200_000 {
		t.Fatalf("CeilingEst=%d want 1200000", est.CeilingEst)
	}
	if est.RemainingEst != 1_200_000-800_000 {
		t.Fatalf("RemainingEst=%d want 400000", est.RemainingEst)
	}
	if est.Confidence != ConfLow {
		t.Fatalf("Confidence=%s want low (1 point)", est.Confidence)
	}
}

func TestEstimate_MostRecentCalibrationWins(t *testing.T) {
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	limits := []LimitEvent{
		{Provider: "anthropic", WindowClass: "5h", UsedAt: 1_000_000, Ts: t0.Add(-48 * time.Hour)},
		{Provider: "anthropic", WindowClass: "5h", UsedAt: 1_500_000, Ts: t0.Add(-24 * time.Hour)}, // newer
	}
	est := Estimator{}.Estimate(nil, limits, "anthropic", Window5h, t0)
	if est.CeilingEst != 1_500_000 {
		t.Fatalf("CeilingEst=%d want most-recent 1500000", est.CeilingEst)
	}
	if est.Confidence != ConfLow { // 2 points → still low
		t.Fatalf("Confidence=%s want low", est.Confidence)
	}
}

func TestEstimate_PublishedPriorWhenNoCalibration(t *testing.T) {
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	evs := []UsageEvent{{Provider: "anthropic", OutputTokens: 100, Ts: t0}}
	e := Estimator{PublishedPrior: map[string]int64{"anthropic": 2_000_000}}
	est := e.Estimate(evs, nil, "anthropic", Window5h, t0.Add(time.Minute))
	if est.CeilingEst != 2_000_000 || est.RemainingEst != 2_000_000-100 {
		t.Fatalf("prior not applied: %+v", est)
	}
	if est.Confidence != ConfLow { // prior → low, not uncalibrated
		t.Fatalf("Confidence=%s want low (prior)", est.Confidence)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestEstimate_Calibrates -v`
Expected: FAIL — `CeilingEst=0 want 1200000` (learning not implemented yet).

- [ ] **Step 3: Write minimal implementation**

Replace the `return Estimate{...}` block at the end of `Estimate` with ceiling learning:

```go
	// Ceiling learning: gather calibration points for this provider + window class,
	// newest last (append order is roughly chronological; sort by Ts to be safe).
	var points []LimitEvent
	for _, le := range limits {
		if le.Provider == provider && le.WindowClass == wc.Name {
			points = append(points, le)
		}
	}
	sortLimitsByTs(points)

	var ceiling int64
	conf := ConfUncalibrated
	switch {
	case len(points) == 0:
		if e.PublishedPrior != nil {
			if p := e.PublishedPrior[provider]; p > 0 {
				ceiling = p
				conf = ConfLow // weak prior, not observed
			}
		}
	case len(points) <= 2:
		ceiling = points[len(points)-1].UsedAt // most recent (rolling)
		conf = ConfLow
	case len(points) <= 5:
		ceiling = points[len(points)-1].UsedAt
		conf = ConfMed
	default:
		ceiling = points[len(points)-1].UsedAt
		conf = ConfHigh
	}

	var remaining int64
	if ceiling > 0 {
		remaining = ceiling - used
		if remaining < 0 {
			remaining = 0
		}
	}

	return Estimate{
		Provider:     provider,
		WindowClass:  wc.Name,
		Used:         used,
		CeilingEst:   ceiling,
		RemainingEst: remaining,
		Confidence:   conf,
		WindowAnchor: anchor,
		WindowReset:  reset,
		WindowActive: active,
	}
```

Add the sort helper to `estimator.go` (and `import "sort"`):

```go
func sortLimitsByTs(ls []LimitEvent) {
	sort.Slice(ls, func(i, j int) bool { return ls[i].Ts.Before(ls[j].Ts) })
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./usage/ -run TestEstimate -v`
Expected: PASS (all estimator tests, including Task 4's).

- [ ] **Step 5: Commit**

```bash
git add modules/incus-dispatcher/usage/estimator.go modules/incus-dispatcher/usage/estimator_test.go
git commit -m "usage: estimator learns per-window ceiling from exhaustion + published prior"
```

---

### Task 6: Claude Code usage collector

**Files:**
- Create: `modules/incus-dispatcher/usage/collector_claude.go`
- Test: `modules/incus-dispatcher/usage/collector_claude_test.go`

**Interfaces:**
- Consumes: `UsageEvent`, `Source` (Task 1).
- Produces: `func ParseClaudeTranscriptLine(line []byte) (UsageEvent, bool)`, `func ParseClaudeStreamLine(line []byte, now time.Time) (UsageEvent, bool)`.

**Schema (verified against a real transcript 2026-06-26):** an assistant record has top-level `type:"assistant"`, `timestamp` (RFC3339), and `message:{ id, model, usage:{ input_tokens, cache_creation_input_tokens, cache_read_input_tokens, output_tokens } }`. The streaming JSON shares the same `message` shape but may lack a top-level timestamp (use `now`). Lines that are not assistant-with-usage return `ok=false`.

- [ ] **Step 1: Write the failing test**

```go
package usage

import (
	"testing"
	"time"
)

const sampleTranscriptLine = `{"type":"assistant","timestamp":"2026-06-25T22:44:04.198Z","uuid":"u1","message":{"id":"msg_01Wk","model":"claude-opus-4-8","usage":{"input_tokens":20260,"cache_creation_input_tokens":14744,"cache_read_input_tokens":15874,"output_tokens":383}}}`

func TestParseClaudeTranscriptLine(t *testing.T) {
	ev, ok := ParseClaudeTranscriptLine([]byte(sampleTranscriptLine))
	if !ok {
		t.Fatal("ok=false, want a usage event")
	}
	if ev.Provider != "anthropic" || ev.Source != SourceInteractive {
		t.Fatalf("provider/source wrong: %+v", ev)
	}
	if ev.TurnID != "msg_01Wk" || ev.Model != "claude-opus-4-8" {
		t.Fatalf("id/model wrong: %+v", ev)
	}
	if ev.InputTokens != 20260 || ev.CacheCreationTokens != 14744 || ev.CacheReadTokens != 15874 || ev.OutputTokens != 383 {
		t.Fatalf("tokens wrong: %+v", ev)
	}
	want := time.Date(2026, 6, 25, 22, 44, 4, 198000000, time.UTC)
	if !ev.Ts.Equal(want) {
		t.Fatalf("ts=%v want %v", ev.Ts, want)
	}
}

func TestParseClaude_NonAssistantIgnored(t *testing.T) {
	if _, ok := ParseClaudeTranscriptLine([]byte(`{"type":"user","message":{"content":"hi"}}`)); ok {
		t.Fatal("user line should be ignored")
	}
	if _, ok := ParseClaudeTranscriptLine([]byte(`{bad json`)); ok {
		t.Fatal("garbage line should be ignored, not panic")
	}
}

func TestParseClaudeStreamLine_UsesNow(t *testing.T) {
	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	line := `{"type":"assistant","message":{"id":"msg_stream","model":"claude-opus-4-8","usage":{"input_tokens":5,"output_tokens":7}}}`
	ev, ok := ParseClaudeStreamLine([]byte(line), now)
	if !ok {
		t.Fatal("ok=false")
	}
	if !ev.Ts.Equal(now) || ev.TurnID != "msg_stream" || ev.OutputTokens != 7 {
		t.Fatalf("stream parse wrong: %+v", ev)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestParseClaude -v`
Expected: FAIL — `undefined: ParseClaudeTranscriptLine`.

- [ ] **Step 3: Write minimal implementation**

```go
package usage

import (
	"encoding/json"
	"time"
)

// claudeRecord matches the subset of a Claude Code assistant record (transcript or stream-json)
// the meter needs. Unknown fields are ignored.
type claudeRecord struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens         int64 `json:"input_tokens"`
			CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadTokens     int64 `json:"cache_read_input_tokens"`
			OutputTokens        int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// parseClaude builds a UsageEvent from a Claude record. ts is used when the record has no
// usable timestamp (streaming). Returns ok=false for non-assistant or usage-less records.
func parseClaude(line []byte, ts time.Time) (UsageEvent, bool) {
	var r claudeRecord
	if err := json.Unmarshal(line, &r); err != nil {
		return UsageEvent{}, false
	}
	if r.Type != "assistant" || r.Message.ID == "" {
		return UsageEvent{}, false
	}
	u := r.Message.Usage
	if u.InputTokens == 0 && u.CacheCreationTokens == 0 && u.CacheReadTokens == 0 && u.OutputTokens == 0 {
		return UsageEvent{}, false
	}
	when := ts
	if r.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339, r.Timestamp); err == nil {
			when = parsed.UTC()
		}
	}
	return UsageEvent{
		Provider:            "anthropic",
		Model:               r.Message.Model,
		InputTokens:         u.InputTokens,
		CacheCreationTokens: u.CacheCreationTokens,
		CacheReadTokens:     u.CacheReadTokens,
		OutputTokens:        u.OutputTokens,
		Ts:                  when,
		Source:              SourceInteractive,
		TurnID:              r.Message.ID,
	}, true
}

// ParseClaudeTranscriptLine parses one ~/.claude/projects/**/*.jsonl line (durable record).
func ParseClaudeTranscriptLine(line []byte) (UsageEvent, bool) {
	return parseClaude(line, time.Time{})
}

// ParseClaudeStreamLine parses one Claude Code stream-json output line; now stamps it.
func ParseClaudeStreamLine(line []byte, now time.Time) (UsageEvent, bool) {
	return parseClaude(line, now)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestParseClaude -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add modules/incus-dispatcher/usage/collector_claude.go modules/incus-dispatcher/usage/collector_claude_test.go
git commit -m "usage: Claude Code transcript + stream-json usage collector"
```

---

### Task 7: Ingest command — Claude Code transcripts → ledger (de-duped)

**Files:**
- Create: `modules/incus-dispatcher/usage/ingest.go`
- Test: `modules/incus-dispatcher/usage/ingest_test.go`

**Interfaces:**
- Consumes: `Ledger`, `ParseClaudeTranscriptLine`, `UsageEvent` (Tasks 2, 6).
- Produces: `func IngestTranscript(l *Ledger, transcriptPath string) (added, skipped int, err error)`.

**Behavior:** read each line of a transcript file, parse usage events, and append those whose `(Source,TurnID)` is not already in the ledger (de-dup — re-ingesting the same transcript, or a streaming event already captured, must not double-count). Returns counts.

- [ ] **Step 1: Write the failing test**

```go
package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIngestTranscript_DedupsByTurnID(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "session.jsonl")
	// two assistant lines (distinct ids) + a noise line
	content := sampleTranscriptLine + "\n" +
		`{"type":"user","message":{"content":"hi"}}` + "\n" +
		`{"type":"assistant","timestamp":"2026-06-25T23:00:00.000Z","message":{"id":"msg_two","model":"claude-opus-4-8","usage":{"input_tokens":10,"output_tokens":20}}}` + "\n"
	if err := os.WriteFile(tr, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	l, _ := OpenLedger(filepath.Join(dir, "usage.jsonl"))

	added, skipped, err := IngestTranscript(l, tr)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if added != 2 || skipped != 0 {
		t.Fatalf("first ingest added=%d skipped=%d, want 2/0", added, skipped)
	}
	// Re-ingest the SAME file: everything is a duplicate now.
	added2, skipped2, _ := IngestTranscript(l, tr)
	if added2 != 0 || skipped2 != 2 {
		t.Fatalf("re-ingest added=%d skipped=%d, want 0/2 (de-dup)", added2, skipped2)
	}
	if len(l.Events()) != 2 {
		t.Fatalf("ledger has %d events, want 2", len(l.Events()))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestIngestTranscript -v`
Expected: FAIL — `undefined: IngestTranscript`.

- [ ] **Step 3: Write minimal implementation**

```go
package usage

import (
	"bufio"
	"os"
)

// IngestTranscript appends usage events from a Claude Code transcript file to the ledger,
// skipping any whose (Source,TurnID) is already present (idempotent re-ingest).
func IngestTranscript(l *Ledger, transcriptPath string) (added, skipped int, err error) {
	seen := make(map[string]bool)
	for _, e := range l.Events() {
		if e.TurnID != "" {
			seen[string(e.Source)+"\x00"+e.TurnID] = true
		}
	}
	f, err := os.Open(transcriptPath)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		ev, ok := ParseClaudeTranscriptLine(sc.Bytes())
		if !ok {
			continue
		}
		key := string(ev.Source) + "\x00" + ev.TurnID
		if ev.TurnID != "" && seen[key] {
			skipped++
			continue
		}
		if err := l.Append(ev); err != nil {
			return added, skipped, err
		}
		seen[key] = true
		added++
	}
	return added, skipped, sc.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./usage/ -run TestIngestTranscript -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/incus-dispatcher/usage/ingest.go modules/incus-dispatcher/usage/ingest_test.go
git commit -m "usage: idempotent transcript ingest with TurnID de-dup"
```

---

### Task 8: Reporter + `dispatcher usage` CLI readout

**Files:**
- Create: `modules/incus-dispatcher/usage/report.go`
- Test: `modules/incus-dispatcher/usage/report_test.go`
- Create: `modules/incus-dispatcher/usage_cmd.go` (package main — CLI wiring)
- Modify: `modules/incus-dispatcher/main.go` (register the `usage` subcommand — see Step 3b)

**Interfaces:**
- Consumes: `Ledger`, `Estimator`, `Estimate`, `Window5h` (Tasks 2, 4, 5).
- Produces: `func Report(l *Ledger, est Estimator, now time.Time, w io.Writer)` — prints one line per provider present in the ledger.

**Behavior:** for each distinct provider in the ledger, compute the `Window5h` estimate and print a human line. Visibility-sooner: always show anchor/reset/time-left/used; show remaining as a number when calibrated, else the uncalibrated label.

- [ ] **Step 1: Write the failing test**

```go
package usage

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestReport_ShowsUsedAndUncalibrated(t *testing.T) {
	dir := t.TempDir()
	l, _ := OpenLedger(dir + "/usage.jsonl")
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	_ = l.Append(UsageEvent{Provider: "anthropic", OutputTokens: 100, Ts: t0, Source: SourceInteractive})

	var buf bytes.Buffer
	Report(l, Estimator{}, t0.Add(time.Hour), &buf)
	out := buf.String()
	if !strings.Contains(out, "anthropic") || !strings.Contains(out, "uncalibrated") {
		t.Fatalf("missing provider/uncalibrated label:\n%s", out)
	}
	if !strings.Contains(out, "100") { // used tokens visible immediately
		t.Fatalf("used tokens not shown:\n%s", out)
	}
}

func TestReport_ShowsRemainingWhenCalibrated(t *testing.T) {
	dir := t.TempDir()
	l, _ := OpenLedger(dir + "/usage.jsonl")
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	_ = l.Append(UsageEvent{Provider: "anthropic", OutputTokens: 800000, Ts: t0})
	_ = l.AppendLimit(LimitEvent{Provider: "anthropic", WindowClass: "5h", UsedAt: 1200000, Ts: t0.Add(-time.Hour)})

	var buf bytes.Buffer
	Report(l, Estimator{}, t0.Add(time.Hour), &buf)
	if !strings.Contains(buf.String(), "400000") { // 1.2M - 800k remaining
		t.Fatalf("remaining not shown:\n%s", buf.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestReport -v`
Expected: FAIL — `undefined: Report`.

- [ ] **Step 3a: Write the reporter (`usage/report.go`)**

```go
package usage

import (
	"fmt"
	"io"
	"sort"
	"time"
)

// Report prints a per-provider usage line for every provider present in the ledger,
// computed for the 5h window at now. Visibility-sooner: window facts + used are always
// shown; remaining is a number when calibrated, else an uncalibrated label.
func Report(l *Ledger, est Estimator, now time.Time, w io.Writer) {
	events := l.Events()
	limits := l.Limits()
	seen := map[string]bool{}
	var providers []string
	for _, e := range events {
		if !seen[e.Provider] {
			seen[e.Provider] = true
			providers = append(providers, e.Provider)
		}
	}
	sort.Strings(providers)
	if len(providers) == 0 {
		fmt.Fprintln(w, "no usage recorded yet")
		return
	}
	for _, p := range providers {
		e := est.Estimate(events, limits, p, Window5h, now)
		left := "expired/idle"
		if e.WindowActive {
			left = e.WindowReset.Sub(now).Round(time.Minute).String() + " left"
		}
		var remaining string
		if e.CeilingEst > 0 {
			remaining = fmt.Sprintf("est ~%d remaining (%s)", e.RemainingEst, e.Confidence)
		} else {
			remaining = "est remaining: uncalibrated (learns ceiling after first limit-hit)"
		}
		fmt.Fprintf(w, "%s: %d used this window (resets %s, %s) · %s\n",
			p, e.Used, e.WindowReset.Format("15:04"), left, remaining)
	}
}
```

- [ ] **Step 3b: Write the CLI wiring (`usage_cmd.go`) and register it**

Create `modules/incus-dispatcher/usage_cmd.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/usage"
)

// defaultLedgerPath is where the usage ledger lives unless overridden by FLEET_USAGE_LEDGER.
func defaultLedgerPath() string {
	if p := os.Getenv("FLEET_USAGE_LEDGER"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fleet", "usage.jsonl")
}

// runUsageCommand implements `dispatcher usage [ingest <transcript>]`.
func runUsageCommand(args []string) int {
	path := defaultLedgerPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "usage:", err)
		return 1
	}
	l, err := usage.OpenLedger(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage:", err)
		return 1
	}
	if len(args) >= 2 && args[0] == "ingest" {
		added, skipped, err := usage.IngestTranscript(l, args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "usage ingest:", err)
			return 1
		}
		fmt.Printf("ingested %d new events (%d duplicates skipped)\n", added, skipped)
		return 0
	}
	usage.Report(l, usage.Estimator{}, time.Now(), os.Stdout)
	return 0
}
```

Then register it in `main.go`'s subcommand dispatch. Find the existing `switch` over subcommands (where `tui`, `serve`, `grade`, etc. are dispatched — search for `case "tui"`) and add:

```go
	case "usage":
		os.Exit(runUsageCommand(os.Args[2:]))
```

- [ ] **Step 4: Run tests + build to verify**

Run: `cd modules/incus-dispatcher && go test -race ./usage/ -run TestReport -v && go build ./... && go vet ./...`
Expected: tests PASS; build succeeds; vet clean. Manual smoke (optional): `go run . usage` prints `no usage recorded yet` (or a real line if `~/.fleet/usage.jsonl` exists).

- [ ] **Step 5: Commit**

```bash
git add modules/incus-dispatcher/usage/report.go modules/incus-dispatcher/usage/report_test.go modules/incus-dispatcher/usage_cmd.go modules/incus-dispatcher/main.go
git commit -m "usage: dispatcher usage readout + ingest CLI"
```

---

### Task 9: Proxy collector — fleet usage into the ledger

**Files:**
- Create: `modules/incus-dispatcher/usage/collector_proxy.go`
- Test: `modules/incus-dispatcher/usage/collector_proxy_test.go`
- Modify: `modules/llm-proxy/` (the response path — append a ledger line; exact file located in Step 3b)

**Interfaces:**
- Consumes: `UsageEvent`, `Source` (Task 1).
- Produces: `func ParseAnthropicUsage(provider string, body []byte, now time.Time) (UsageEvent, bool)` (and the same for OpenAI/ollama shapes), parsing a provider response body's `usage` block into a fleet `UsageEvent`.

**Boundary note:** sub-project 2 modifies the llm-proxy for enforcement; if wiring the proxy here proves entangled, this task may move to sub-project 2 — but the parser (Step 3a) and its tests stay in `usage` regardless. The parser is the contract; the proxy appends to the same ledger via `usage.Ledger.Append` (the proxy already imports nothing from incus-dispatcher today — see Step 3b for the import/replace decision).

- [ ] **Step 1: Write the failing test**

```go
package usage

import (
	"testing"
	"time"
)

func TestParseAnthropicUsage(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-8","usage":{"input_tokens":120,"cache_creation_input_tokens":5,"cache_read_input_tokens":7,"output_tokens":33}}`)
	now := time.Date(2026, 6, 26, 11, 0, 0, 0, time.UTC)
	ev, ok := ParseAnthropicUsage("anthropic", body, now)
	if !ok {
		t.Fatal("ok=false")
	}
	if ev.Source != SourceFleet || ev.Provider != "anthropic" || ev.Model != "claude-opus-4-8" {
		t.Fatalf("header fields wrong: %+v", ev)
	}
	if ev.InputTokens != 120 || ev.CacheCreationTokens != 5 || ev.CacheReadTokens != 7 || ev.OutputTokens != 33 {
		t.Fatalf("tokens wrong: %+v", ev)
	}
	if !ev.Ts.Equal(now) {
		t.Fatalf("ts=%v want now", ev.Ts)
	}
}

func TestParseAnthropicUsage_NoUsageIgnored(t *testing.T) {
	if _, ok := ParseAnthropicUsage("anthropic", []byte(`{"error":"x"}`), time.Now()); ok {
		t.Fatal("response without usage should be ignored")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./usage/ -run TestParseAnthropicUsage -v`
Expected: FAIL — `undefined: ParseAnthropicUsage`.

- [ ] **Step 3a: Write the parser (`usage/collector_proxy.go`)**

```go
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
```

- [ ] **Step 3b: Wire the proxy to append fleet usage**

Locate the llm-proxy response handler: `cd modules/llm-proxy && grep -rln "ResponseWriter\|httputil\|io.Copy\|resp.Body" *.go`. In the handler that receives the upstream provider response, after the body is read, append a usage line to the ledger.

Decision for cross-module use: `modules/llm-proxy` has its own go.mod. Add the incus-dispatcher module as a dependency via a `replace` directive so the proxy can call `usage.OpenLedger`/`Append`:

In `modules/llm-proxy/go.mod` add:
```
require github.com/agent-sandbox/incus-dispatcher v0.0.0
replace github.com/agent-sandbox/incus-dispatcher => ../incus-dispatcher
```
Then run `cd modules/llm-proxy && go mod tidy`.

In the proxy response path (guarded so a usage-write failure never breaks brokering):
```go
import "github.com/agent-sandbox/incus-dispatcher/usage"

// after reading the upstream response body into `respBody` and knowing `providerName`:
if ev, ok := usage.ParseAnthropicUsage(providerName, respBody, time.Now()); ok {
	if l, err := usage.OpenLedger(usageLedgerPath()); err == nil {
		_ = l.Append(ev) // best-effort: metering must never block brokering
	}
}
```
where `usageLedgerPath()` reads `FLEET_USAGE_LEDGER` or defaults to `~/.fleet/usage.jsonl` (mirror `defaultLedgerPath` from Task 8; a tiny local copy in the proxy is fine — it is stdlib-only).

If the `replace`-based cross-module wiring proves heavy, STOP and defer Step 3b to sub-project 2 (which already modifies the proxy), keeping Steps 1/3a (the parser + tests) here. Log that decision in the commit message.

- [ ] **Step 4: Run tests + build to verify**

Run: `cd modules/incus-dispatcher && go test -race ./usage/ && go vet ./...`
Then: `cd modules/llm-proxy && go build ./... && go vet ./... && go test -race ./...`
Expected: all PASS; both modules build; vet clean.

- [ ] **Step 5: Commit**

```bash
git add modules/incus-dispatcher/usage/collector_proxy.go modules/incus-dispatcher/usage/collector_proxy_test.go modules/llm-proxy/
git commit -m "usage: fleet usage parser + llm-proxy ledger append"
```

---

## Done criteria (sub-project 1)

- `cd modules/incus-dispatcher && go test -race ./... && go vet ./...` green; `cd modules/llm-proxy && go test -race ./... && go vet ./...` green.
- `dispatcher usage ingest <transcript>` records real Claude Code interactive usage (de-duped); `dispatcher usage` prints per-provider used + window anchor/reset/time-left immediately (visibility-sooner), and a calibrated remaining estimate once a limit-event has been recorded.
- Fleet calls through the llm-proxy append fleet usage to the same ledger (or Step 3b deferred to sub-project 2 with a logged reason).
- No enforcement (correct — that is sub-project 2: the reserve cap consumes `Estimator.Estimate`).

## Explicit v1 deferrals (honest spec simplifications)

- **No-usage-block token-count fallback** (spec error-handling bullet): the `UsageEvent.Estimated` flag exists, but v1 collectors *skip* responses with no `usage` block rather than computing a local token-count fallback. All target providers (Anthropic/OpenAI/ollama) report usage, so the fallback is YAGNI for v1; add it only if a real provider is found that omits usage.
- **Ceiling = most-recent calibration point** (spec said "decayed/rolling over recent points"). Most-recent is the simplest rolling instantiation and tracks plan/behavior changes; a decayed average over the last N points is a later refinement if single-point noise proves a problem.
- **Remaining shown as a label, not a numeric range**, while uncalibrated. The honest "uncalibrated (learns ceiling after first limit-hit)" label is shown; a numeric confidence band is a readout polish item, not a measurement gap.
- **Recording the LimitEvent on a real 429** is out of scope here (no live provider exhaustion to observe in unit tests); the estimator + ledger fully support it, and the proxy/Claude-Code collectors will append a `LimitEvent` when they see a rate-limit response — wired alongside sub-project 2's enforcement, which is where real exhaustion handling lives.
