# Reserve-Cap Enforcement at the Proxy — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `llm-proxy` enforce a per-provider, per-window reserve cap on fleet LLM spend — deferring over-cap fleet calls while never blocking interactive traffic — by consuming the existing usage meter.

**Architecture:** Promote the `usage` meter to its own stdlib-only Go module shared by both `incus-dispatcher` and `llm-proxy`. Add a pure reserve-cap calculation to the meter. In the proxy, classify each request as fleet vs interactive by route prefix, run a pre-flight budget gate before forwarding fleet calls (refuse-fast with 429 + marker, no upstream call), meter every proxied response into the ledger, and record a calibration `LimitEvent` on an upstream 429.

**Tech Stack:** Go (stdlib only — `encoding/json`, `net/http`, `os`, `bufio`, `bytes`, `sync`, `time`, `math`), `httptest` for integration tests. No third-party dependencies in any module.

## Global Constraints

- **Standard library only** in all three modules — no new third-party dependencies.
- **`usage` becomes its own module** at `modules/usage` with module path `github.com/agent-sandbox/usage`; `incus-dispatcher` and `llm-proxy` consume it via `require` + a local `replace => ../usage`.
- **Injected clock:** pure logic (cap math, estimator) takes `now time.Time` as a parameter and never calls `time.Now()`; the proxy reads the real clock only at the request edge via `Server.now`.
- **Enforcement is pre-flight:** the budget gate decides before any upstream call and reads only already-recorded ledger usage; it must never depend on the current response.
- **Interactive is never capped:** only `classFleet` routes are gated; `classInteractive` routes always forward.
- **Cap formula:** `cap = ceiling × (1 − reservePct)`, per provider, for `usage.Window5h`. `AllowFleet` fails OPEN when there is no working ceiling (`CeilingEst == 0`).
- **Best-effort metering:** a ledger read/write failure must never block or break brokering — log and proceed; the gate fails open on ledger-read errors.
- **Tests run under `-race`:** `cd modules/usage && go test -race ./...`, `cd modules/incus-dispatcher && go test -race ./...` (note the pre-existing infra-only failure `TestFirecrackerRunner_Integration_RealWorkerVM` is unrelated), `cd modules/llm-proxy && go test -race ./...`. `go vet ./...` stays clean in every module.
- **Commit messages must NOT contain** `Claude`, `Generated with`, `Co-Authored-By: Claude`, or `claude.com/claude-code` (a pre-commit hook blocks them). Use plain human-style messages.
- **Defensive copies:** any method returning a slice of ledger state returns a copy (existing meter convention; unchanged here).

---

### Task 1: Promote `usage` to its own module

**Files:**
- Move: `modules/incus-dispatcher/usage/*.go` → `modules/usage/*.go` (all 8 source files + their `*_test.go`).
- Create: `modules/usage/go.mod`
- Modify: `modules/incus-dispatcher/go.mod` (add require + replace)
- Modify: `modules/incus-dispatcher/usage_cmd.go:9` (import path)

**Interfaces:**
- Produces: the module `github.com/agent-sandbox/usage` exporting the unchanged meter API (`OpenLedger`, `Ledger`, `UsageEvent`, `LimitEvent`, `Source`/`SourceFleet`/`SourceInteractive`, `Estimator`, `Estimate`, `Window5h`/`WindowWeekly`/`WindowClass`, `CurrentWindow`, `ParseClaudeTranscriptLine`, `ParseClaudeStreamLine`, `ParseAnthropicUsage`, `IngestTranscript`, `Report`).

- [ ] **Step 1: Move the package directory (git-aware)**

```bash
cd /Users/ndn/development/agent-sandbox
mkdir -p modules/usage
git mv modules/incus-dispatcher/usage/*.go modules/usage/
rmdir modules/incus-dispatcher/usage
```

- [ ] **Step 2: Create `modules/usage/go.mod`**

```
module github.com/agent-sandbox/usage

go 1.22
```

(`go 1.22` is the floor of the consumers; the meter is stdlib-only so this is safe for both `incus-dispatcher` and `llm-proxy`.)

- [ ] **Step 3: Point `incus-dispatcher` at the new module**

Edit `modules/incus-dispatcher/usage_cmd.go` line 9, changing the import path:

```go
	"github.com/agent-sandbox/usage"
```

Append to `modules/incus-dispatcher/go.mod` (after the existing `require` block):

```
require github.com/agent-sandbox/usage v0.0.0

replace github.com/agent-sandbox/usage => ../usage
```

- [ ] **Step 4: Tidy and verify both modules build + test**

Run:
```bash
cd /Users/ndn/development/agent-sandbox/modules/usage && go test -race ./... && go vet ./...
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher && go mod tidy && go build ./... && go vet ./...
```
Expected: `modules/usage` → all meter tests PASS (the same suite that was green before the move), vet clean. `incus-dispatcher` builds and vets clean. (`go test -race ./...` in incus-dispatcher still has only the pre-existing infra-only `TestFirecrackerRunner_Integration_RealWorkerVM` failure.)

- [ ] **Step 5: Commit**

```bash
cd /Users/ndn/development/agent-sandbox
git add -A modules/usage modules/incus-dispatcher
git commit -m "usage: promote meter to standalone module shared by dispatcher + proxy"
```

---

### Task 2: Reserve-cap pure logic in the meter

**Files:**
- Create: `modules/usage/reserve.go`
- Test: `modules/usage/reserve_test.go`

**Interfaces:**
- Consumes: `Estimate` (fields `Used int64`, `CeilingEst int64`).
- Produces: `func (Estimate) FleetCap(reservePct float64) int64`, `func (Estimate) AllowFleet(reservePct float64) bool`.

- [ ] **Step 1: Write the failing test**

```go
package usage

import "testing"

func TestFleetCap(t *testing.T) {
	cases := []struct {
		name      string
		ceiling   int64
		reserve   float64
		wantCap   int64
	}{
		{"no ceiling -> no cap", 0, 0.30, 0},
		{"30pct reserve", 1_000_000, 0.30, 700_000},
		{"zero reserve -> full ceiling", 1_000_000, 0.0, 1_000_000},
		{"all reserved -> zero cap", 1_000_000, 1.0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := Estimate{CeilingEst: c.ceiling}
			if got := e.FleetCap(c.reserve); got != c.wantCap {
				t.Fatalf("FleetCap=%d want %d", got, c.wantCap)
			}
		})
	}
}

func TestAllowFleet(t *testing.T) {
	// Uncalibrated, no prior -> fail open regardless of Used.
	if !(Estimate{CeilingEst: 0, Used: 9_999_999}).AllowFleet(0.30) {
		t.Fatal("uncalibrated must fail open (allow)")
	}
	// ceiling 1_000_000, reserve 0.30 -> cap 700_000.
	if !(Estimate{CeilingEst: 1_000_000, Used: 699_999}).AllowFleet(0.30) {
		t.Fatal("Used below cap must allow")
	}
	if (Estimate{CeilingEst: 1_000_000, Used: 700_000}).AllowFleet(0.30) {
		t.Fatal("Used at cap must deny")
	}
	if (Estimate{CeilingEst: 1_000_000, Used: 800_000}).AllowFleet(0.30) {
		t.Fatal("Used over cap must deny")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/usage && go test ./... -run 'TestFleetCap|TestAllowFleet' -v`
Expected: FAIL — `e.FleetCap undefined` / `AllowFleet undefined`.

- [ ] **Step 3: Write minimal implementation**

```go
package usage

// FleetCap is the fleet spend ceiling for the active window: the working ceiling
// minus the reserved interactive headroom (reservePct of the ceiling). Returns 0
// when there is no working ceiling (CeilingEst <= 0), which the caller reads as
// "no cap — fail open". reservePct is assumed already validated to [0,1].
func (e Estimate) FleetCap(reservePct float64) int64 {
	if e.CeilingEst <= 0 {
		return 0
	}
	c := float64(e.CeilingEst) * (1 - reservePct)
	if c < 0 {
		c = 0
	}
	return int64(c)
}

// AllowFleet reports whether a new fleet call is within budget. With no working
// ceiling (uncalibrated AND no PublishedPrior, so CeilingEst == 0) it fails open.
// This is exactly the locked "prior -> enforce, no prior -> fail open" rule: the
// Estimator folds a configured PublishedPrior into CeilingEst, so a present prior
// yields a non-zero ceiling and enforces here.
func (e Estimate) AllowFleet(reservePct float64) bool {
	if e.CeilingEst <= 0 {
		return true
	}
	return e.Used < e.FleetCap(reservePct)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/usage && go test -race ./... -run 'TestFleetCap|TestAllowFleet' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/usage/reserve.go modules/usage/reserve_test.go
git commit -m "usage: FleetCap + AllowFleet reserve-cap pure logic"
```

---

### Task 3: Proxy route classification (fleet vs interactive)

**Files:**
- Modify: `modules/llm-proxy/proxy.go` (add `routeClass`, `route.class`, `newRoute` param)
- Modify: `modules/llm-proxy/main.go` (extract a testable `buildRoutes`, register interactive variants)
- Test: `modules/llm-proxy/routes_test.go`

**Interfaces:**
- Produces: `type routeClass string` with `classFleet`/`classInteractive`; `route` gains `class routeClass`; `newRoute(prefix, upstreamStr, apiKey, provider string, requiresKey bool, class routeClass) (route, error)`; `type routeSpec struct{prefix, upstream, apiKey, provider string; requiresKey bool}`; `func buildRoutes(specs []routeSpec) ([]route, error)`.

- [ ] **Step 1: Write the failing test**

```go
package main

import "testing"

func TestBuildRoutes_AddsInteractiveVariants(t *testing.T) {
	specs := []routeSpec{
		{prefix: "/anthropic", upstream: "https://api.anthropic.com", apiKey: "k", provider: "anthropic", requiresKey: true},
		{prefix: "/openai", upstream: "https://api.openai.com", apiKey: "k", provider: "openai", requiresKey: true},
	}
	routes, err := buildRoutes(specs)
	if err != nil {
		t.Fatalf("buildRoutes: %v", err)
	}
	// Every spec yields a fleet route AND an interactive route.
	if len(routes) != 4 {
		t.Fatalf("len(routes)=%d want 4", len(routes))
	}
	byPrefix := map[string]route{}
	for _, r := range routes {
		byPrefix[r.prefix] = r
	}
	if byPrefix["/anthropic"].class != classFleet {
		t.Fatalf("/anthropic class=%q want fleet", byPrefix["/anthropic"].class)
	}
	ia, ok := byPrefix["/interactive/anthropic"]
	if !ok {
		t.Fatal("missing /interactive/anthropic route")
	}
	if ia.class != classInteractive {
		t.Fatalf("/interactive/anthropic class=%q want interactive", ia.class)
	}
	if ia.provider != "anthropic" {
		t.Fatalf("interactive provider=%q want anthropic", ia.provider)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/llm-proxy && go test ./... -run TestBuildRoutes -v`
Expected: FAIL — `undefined: routeSpec` / `buildRoutes` / `classFleet`.

- [ ] **Step 3: Add the class type + field, update `newRoute` (`proxy.go`)**

Add near the top of `proxy.go` (after the imports):

```go
// routeClass distinguishes interactive traffic (never capped — the protected
// interactive headroom) from fleet traffic (subject to the reserve cap).
type routeClass string

const (
	classFleet       routeClass = "fleet"
	classInteractive routeClass = "interactive"
)
```

Add `class` to the `route` struct:

```go
type route struct {
	prefix      string
	upstream    *url.URL
	apiKey      string
	provider    string
	requiresKey bool
	class       routeClass
}
```

Update `newRoute` to take and set the class:

```go
func newRoute(prefix, upstreamStr, apiKey, provider string, requiresKey bool, class routeClass) (route, error) {
	u, err := url.Parse(upstreamStr)
	if err != nil {
		return route{}, err
	}
	if u.Scheme == "" || u.Host == "" {
		return route{}, errors.New("upstream URL must have scheme and host: " + upstreamStr)
	}
	return route{
		prefix:      prefix,
		upstream:    u,
		apiKey:      apiKey,
		provider:    provider,
		requiresKey: requiresKey,
		class:       class,
	}, nil
}
```

- [ ] **Step 4: Add `routeSpec` + `buildRoutes` and use them in `main.go`**

In `main.go`, replace the inline `specs := []struct{...}{...}` literal and the `routes := make(...)` loop (lines ~35–59) with a named type and a call to `buildRoutes`. First add (package-level, in `proxy.go` next to `route`):

```go
// routeSpec is the declarative description of one upstream provider route.
type routeSpec struct {
	prefix      string
	upstream    string
	apiKey      string
	provider    string
	requiresKey bool
}

// buildRoutes turns provider specs into routes: one fleet route per spec plus a
// parallel interactive route under the /interactive prefix. Point Claude Code's
// ANTHROPIC_BASE_URL at the interactive prefix to route it through the proxy;
// leave it unset to keep the proxy fleet-only.
func buildRoutes(specs []routeSpec) ([]route, error) {
	routes := make([]route, 0, len(specs)*2)
	for _, s := range specs {
		fleet, err := newRoute(s.prefix, s.upstream, s.apiKey, s.provider, s.requiresKey, classFleet)
		if err != nil {
			return nil, err
		}
		inter, err := newRoute("/interactive"+s.prefix, s.upstream, s.apiKey, s.provider, s.requiresKey, classInteractive)
		if err != nil {
			return nil, err
		}
		routes = append(routes, fleet, inter)
	}
	return routes, nil
}
```

Then in `main.go` rewrite the specs literal as `[]routeSpec{...}` (same values as today) and replace the build loop with:

```go
	routes, err := buildRoutes(specs)
	if err != nil {
		log.Fatalf("invalid routes: %v", err)
	}
```

(Remove the now-unused per-spec `newRoute` loop. `buildRoutes` registers `/interactive/...` automatically, so `Server.Handler` will mount them with no further change.)

- [ ] **Step 5: Fix existing `newRoute` call sites broken by the new `class` param**

The signature change breaks two call sites in `proxy_test.go` (the shared helper `mustRoute` at line 39 and an inline error-case call at line 73). Update both to pass `classFleet`:

`modules/llm-proxy/proxy_test.go` line ~41 (inside `mustRoute`):

```go
	rt, err := newRoute(prefix, upstream, key, provider, requires, classFleet)
```

`modules/llm-proxy/proxy_test.go` line ~73 (the error-case assertion):

```go
		if _, err := newRoute("/p", u, "", "x", false, classFleet); err == nil {
```

(`scenario0020_test.go` builds its route via `mustRoute`, so no further edits there. There are no positional `route{}` literals, so adding the struct field breaks nothing else.)

- [ ] **Step 6: Run test + build to verify**

Run: `cd modules/llm-proxy && go test ./... -run TestBuildRoutes -v && go build ./... && go vet ./...`
Expected: test PASS; build + vet clean (the whole package compiles, including the updated existing tests).

- [ ] **Step 7: Commit**

```bash
git add modules/llm-proxy/proxy.go modules/llm-proxy/main.go modules/llm-proxy/proxy_test.go modules/llm-proxy/routes_test.go
git commit -m "llm-proxy: classify routes fleet vs interactive (+ interactive prefix)"
```

---

### Task 4: Proxy → usage wiring + streaming usage scanner

**Files:**
- Modify: `modules/llm-proxy/go.mod` (require + replace)
- Create: `modules/llm-proxy/usage_meter.go` (ledger path helper, `usageScanner`, `sourceForClass`)
- Test: `modules/llm-proxy/usage_meter_test.go`

**Interfaces:**
- Consumes: `usage.UsageEvent`, `usage.Source`/`SourceFleet`/`SourceInteractive`.
- Produces: `func usageLedgerPath() string`; `type usageScanner` implementing `io.Writer` with `func (*usageScanner) result(now time.Time) (usage.UsageEvent, bool)`; `func newUsageScanner(provider string) *usageScanner`; `func sourceForClass(c routeClass) usage.Source`.

**Behavior:** `usageScanner` is an `io.Writer` fed via `io.TeeReader` as the response streams to the client, so it never buffers the whole body, reorders, or delays client bytes. It is line-oriented: it handles Anthropic SSE (`data: {message_start…}` carries input/cache under `message.usage`; `data: {message_delta…}` carries cumulative `output_tokens` at top level) and a single non-streaming JSON body (top-level `usage` + `model`). It accumulates input/cache/output across events and emits one `UsageEvent` at `result`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestUsageScanner_NonStreaming(t *testing.T) {
	body := `{"model":"claude-opus-4-8","usage":{"input_tokens":120,"cache_creation_input_tokens":5,"cache_read_input_tokens":7,"output_tokens":33}}`
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
```

(The two `usageSource*()` helpers in the last test are local shims so the test file does not import `usage` directly — define them in the test file:)

```go
import "github.com/agent-sandbox/usage"

func usageSourceInteractive() usage.Source { return usage.SourceInteractive }
func usageSourceFleet() usage.Source       { return usage.SourceFleet }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/llm-proxy && go test ./... -run 'TestUsageScanner|TestSourceForClass' -v`
Expected: FAIL — `undefined: newUsageScanner` (and a module error until the require/replace is added in Step 3).

- [ ] **Step 3: Add the cross-module dependency**

Append to `modules/llm-proxy/go.mod`:

```
require github.com/agent-sandbox/usage v0.0.0

replace github.com/agent-sandbox/usage => ../usage
```

Then run `cd modules/llm-proxy && go mod tidy` (pulls only the stdlib-only `usage` module; the proxy's dependency graph stays otherwise empty).

- [ ] **Step 4: Write the implementation (`usage_meter.go`)**

```go
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
	Model   string `json:"model"`
	Usage   *tokenBlock `json:"usage"`
	Message struct {
		Model string      `json:"model"`
		Usage *tokenBlock `json:"usage"`
	} `json:"message"`
}

// usageScanner observes a passing response stream and extracts token usage
// without buffering the whole body, reordering, or delaying client bytes. It is
// fed via io.TeeReader, so its Write receives exactly the bytes sent to the client.
type usageScanner struct {
	provider                       string
	tail                           []byte
	in, cacheCreate, cacheRead, out int64
	model                          string
	got                            bool
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd modules/llm-proxy && go test -race ./... -run 'TestUsageScanner|TestSourceForClass' -v`
Expected: PASS (all four), race-clean.

- [ ] **Step 6: Commit**

```bash
git add modules/llm-proxy/go.mod modules/llm-proxy/go.sum modules/llm-proxy/usage_meter.go modules/llm-proxy/usage_meter_test.go
git commit -m "llm-proxy: wire usage module + streaming-safe usage scanner"
```

---

### Task 5: Pre-flight budget gate (refuse over-cap fleet calls)

**Files:**
- Modify: `modules/llm-proxy/proxy.go` (Server fields, `now`, `reserveFor`, `budgetAllows`, gate in `proxy`)
- Modify: `modules/llm-proxy/proxy.go` `newServer` (defaults)
- Test: `modules/llm-proxy/budget_gate_test.go`

**Interfaces:**
- Consumes: `usage.OpenLedger`, `usage.Estimator`, `usage.Window5h`, `Estimate.AllowFleet`.
- Produces: `Server` fields `ledgerPath string`, `reservePct float64`, `providerReserve map[string]float64`, `priors map[string]int64`, `now func() time.Time`; `func (*Server) reserveFor(provider string) float64`; `func (*Server) budgetAllows(provider string, now time.Time) (allow bool, retryAfter int)`.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/llm-proxy && go test ./... -run TestBudgetGate -v`
Expected: FAIL — `s.ledgerPath undefined` / `budgetAllows undefined`.

- [ ] **Step 3: Add Server fields, defaults, and the gate (`proxy.go`)**

Add `"math"` to the `proxy.go` import block and `"github.com/agent-sandbox/usage"`.

Extend the `Server` struct:

```go
type Server struct {
	routes       []route
	client       *http.Client
	logs         logSink
	maxBodyBytes int64

	// Budget enforcement (sub-project 2).
	ledgerPath      string             // usage ledger; fleet gate + metering both use it
	reservePct      float64            // default reserved interactive headroom fraction [0,1)
	providerReserve map[string]float64 // per-provider reservePct override
	priors          map[string]int64   // Estimator.PublishedPrior ceilings per provider
	now             func() time.Time   // injected clock; defaults to time.Now
}
```

Set defaults in `newServer`. **Leave `ledgerPath` empty by default** — an empty path means "no ledger configured → gate fails open and metering is skipped," which keeps every existing proxy test hermetic (they never touch the dev machine's real `~/.fleet/usage.jsonl`). Production wires the real path via `configureBudget` (Task 7); budget tests set `ledgerPath` explicitly.

```go
func newServer(routes []route, logs logSink) *Server {
	return &Server{
		routes:          routes,
		client:          &http.Client{Timeout: defaultUpstreamTimeout},
		logs:            logs,
		maxBodyBytes:    defaultMaxBodyBytes,
		ledgerPath:      "", // set by configureBudget in production; "" => gate fails open
		reservePct:      0.30,
		providerReserve: map[string]float64{},
		priors:          map[string]int64{},
		now:             time.Now,
	}
}
```

Add the gate methods:

```go
// reserveFor returns the reserved-headroom fraction for a provider (override or default).
func (s *Server) reserveFor(provider string) float64 {
	if p, ok := s.providerReserve[provider]; ok {
		return p
	}
	return s.reservePct
}

// budgetAllows reports whether a fleet call to provider is within the reserve cap
// for the active 5h window, plus a Retry-After hint (seconds) when deferred. It
// reads only already-recorded usage (pre-flight) and fails OPEN on any ledger
// error — a meter bug must never strand the fleet.
func (s *Server) budgetAllows(provider string, now time.Time) (allow bool, retryAfter int) {
	if s.ledgerPath == "" {
		return true, 0 // no ledger configured -> fail open
	}
	l, err := usage.OpenLedger(s.ledgerPath)
	if err != nil {
		s.logs.Log(logEntry{Provider: provider, Error: "budget: ledger open failed (fail-open): " + err.Error()})
		return true, 0
	}
	est := usage.Estimator{PublishedPrior: s.priors}.Estimate(l.Events(), l.Limits(), provider, usage.Window5h, now)
	if est.AllowFleet(s.reserveFor(provider)) {
		return true, 0
	}
	ra := int(math.Ceil(est.WindowReset.Sub(now).Seconds()))
	if ra < 1 {
		ra = 1
	}
	return false, ra
}
```

- [ ] **Step 4: Insert the gate into `proxy` before the upstream call**

In `proxy`, immediately after the `finish` closure is defined (before the prefix-stripping block at ~line 156), add:

```go
	// Pre-flight reserve-cap gate: fleet traffic only. Interactive is the
	// protected headroom and is never deferred. Decision reads only the ledger's
	// already-recorded usage, so it is independent of this response.
	if rt.class == classFleet {
		if allow, retryAfter := s.budgetAllows(rt.provider, s.now()); !allow {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.Header().Set("X-Budget-Deferred", "1")
			http.Error(w, "budget-deferred: fleet reserve cap reached for "+rt.provider, http.StatusTooManyRequests)
			finish(http.StatusTooManyRequests, "budget-deferred", 0, 0)
			return
		}
	}
```

Add `"strconv"` to the `proxy.go` imports.

- [ ] **Step 5: Run tests + build to verify**

Run: `cd modules/llm-proxy && go test -race ./... -run TestBudgetGate -v && go build ./... && go vet ./...`
Expected: all three gate tests PASS; build + vet clean. (Existing proxy tests still pass because `newServer` leaves `ledgerPath` empty, so the gate fails open with no behavior change for non-budget tests.)

- [ ] **Step 6: Commit**

```bash
git add modules/llm-proxy/proxy.go modules/llm-proxy/budget_gate_test.go
git commit -m "llm-proxy: pre-flight reserve-cap gate defers over-cap fleet calls"
```

---

### Task 6: Response metering + 429 calibration

**Files:**
- Modify: `modules/llm-proxy/proxy.go` (response path in `proxy`: tee through scanner, append usage, calibrate on 429)
- Test: `modules/llm-proxy/metering_test.go`

**Interfaces:**
- Consumes: `usageScanner` (Task 4), `usage.OpenLedger`/`Append`/`AppendLimit`, `usage.Estimator`, `usage.Window5h`, `sourceForClass`.
- Produces: no new exported surface; wires metering into `proxy`.

- [ ] **Step 1: Write the failing test**

```go
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
		w.WriteHeader(http.StatusTooManyRequests) // upstream says rate-limited
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/llm-proxy && go test ./... -run TestMetering -v`
Expected: FAIL — no events/limits recorded (metering not wired yet).

- [ ] **Step 3: Wire metering into the response path (`proxy.go`)**

Replace the existing tail of `proxy` from `bytesOut := streamCopy(w, resp.Body)` through `finish(resp.StatusCode, "", bytesIn, bytesOut)` with:

```go
	// Meter the response into the ledger as it streams to the client. The scanner
	// is fed via TeeReader, so it never buffers the whole body, reorders, or
	// delays client bytes.
	scanner := newUsageScanner(rt.provider)
	bytesOut := streamCopy(w, io.TeeReader(resp.Body, scanner))

	var bytesIn int64
	if counter != nil {
		bytesIn = counter.bytes()
	}

	// Metering is best-effort and only when a ledger is configured (empty path =>
	// disabled, e.g. hermetic non-budget tests). A failure never affects brokering.
	if s.ledgerPath != "" {
		if ev, ok := scanner.result(s.now()); ok {
			ev.Source = sourceForClass(rt.class)
			if l, err := usage.OpenLedger(s.ledgerPath); err == nil {
				_ = l.Append(ev)
			}
		}
		// Calibration: an upstream rate-limit reveals the realized ceiling — record
		// a LimitEvent at the window usage observed just before this call.
		if resp.StatusCode == http.StatusTooManyRequests {
			if l, err := usage.OpenLedger(s.ledgerPath); err == nil {
				est := usage.Estimator{PublishedPrior: s.priors}.Estimate(l.Events(), l.Limits(), rt.provider, usage.Window5h, s.now())
				_ = l.AppendLimit(usage.LimitEvent{
					Provider:    rt.provider,
					WindowClass: usage.Window5h.Name,
					UsedAt:      est.Used,
					Ts:          s.now(),
				})
			}
		}
	}

	finish(resp.StatusCode, "", bytesIn, bytesOut)
```

- [ ] **Step 4: Run tests + full proxy suite to verify**

Run: `cd modules/llm-proxy && go test -race ./... -run TestMetering -v && go test -race ./... && go vet ./...`
Expected: metering tests PASS; the full proxy suite (existing + new) PASS race-clean; vet clean.

- [ ] **Step 5: Commit**

```bash
git add modules/llm-proxy/proxy.go modules/llm-proxy/metering_test.go
git commit -m "llm-proxy: meter responses to ledger + calibrate on upstream 429"
```

---

### Task 7: Budget configuration from environment

**Files:**
- Create: `modules/llm-proxy/budget_config.go`
- Test: `modules/llm-proxy/budget_config_test.go`
- Modify: `modules/llm-proxy/main.go` (call `configureBudget` after `newServer`)

**Interfaces:**
- Consumes: `Server` budget fields (Task 5).
- Produces: `func configureBudget(s *Server, getenv func(string) string)`.

**Behavior:** read `LLM_PROXY_RESERVE_PCT` (default 0.30, validated to `[0,1)`), per-provider override `LLM_PROXY_RESERVE_PCT_<PROVIDER>` (uppercased, `-`→`_`), and prior ceilings `LLM_PROXY_CEILING_PRIOR_<PROVIDER>` (int) for every distinct provider in the routes. Invalid values are ignored (keep the default) with no crash.

- [ ] **Step 1: Write the failing test**

```go
package main

import "testing"

func TestConfigureBudget(t *testing.T) {
	rt, _ := newRoute("/anthropic", "https://api.anthropic.com", "k", "anthropic", true, classFleet)
	ro, _ := newRoute("/openai", "https://api.openai.com", "k", "openai", true, classFleet)
	s := newServer([]route{rt, ro}, &nopLogSink{})

	env := map[string]string{
		"LLM_PROXY_RESERVE_PCT":             "0.25",
		"LLM_PROXY_RESERVE_PCT_ANTHROPIC":   "0.40",
		"LLM_PROXY_CEILING_PRIOR_ANTHROPIC": "2000000",
		"LLM_PROXY_RESERVE_PCT_OPENAI":      "bogus", // ignored
	}
	configureBudget(s, func(k string) string { return env[k] })

	if s.reservePct != 0.25 {
		t.Fatalf("reservePct=%v want 0.25", s.reservePct)
	}
	if s.reserveFor("anthropic") != 0.40 {
		t.Fatalf("anthropic reserve=%v want 0.40", s.reserveFor("anthropic"))
	}
	if s.reserveFor("openai") != 0.25 {
		t.Fatalf("openai reserve=%v want default 0.25 (bogus ignored)", s.reserveFor("openai"))
	}
	if s.priors["anthropic"] != 2_000_000 {
		t.Fatalf("anthropic prior=%d want 2000000", s.priors["anthropic"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/llm-proxy && go test ./... -run TestConfigureBudget -v`
Expected: FAIL — `undefined: configureBudget`.

- [ ] **Step 3: Write the implementation (`budget_config.go`)**

```go
package main

import (
	"strconv"
	"strings"
)

// configureBudget fills the Server's budget knobs from the environment via the
// injected getenv (so it is testable). Unset or invalid values keep the defaults
// already set by newServer.
func configureBudget(s *Server, getenv func(string) string) {
	// Wire the production ledger path (honors FLEET_USAGE_LEDGER); this is what
	// turns enforcement + metering on outside of tests.
	s.ledgerPath = usageLedgerPath()
	if v := getenv("LLM_PROXY_RESERVE_PCT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f < 1 {
			s.reservePct = f
		}
	}
	// Distinct providers from the routing table.
	seen := map[string]bool{}
	for _, rt := range s.routes {
		if seen[rt.provider] {
			continue
		}
		seen[rt.provider] = true
		key := strings.ToUpper(strings.ReplaceAll(rt.provider, "-", "_"))
		if v := getenv("LLM_PROXY_RESERVE_PCT_" + key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f < 1 {
				s.providerReserve[rt.provider] = f
			}
		}
		if v := getenv("LLM_PROXY_CEILING_PRIOR_" + key); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				s.priors[rt.provider] = n
			}
		}
	}
}
```

- [ ] **Step 4: Call it from `main.go`**

After `srv := newServer(routes, logs)` (and the existing `LLM_PROXY_MAX_BODY` block), add:

```go
	configureBudget(srv, os.Getenv)
```

- [ ] **Step 5: Run tests + build to verify**

Run: `cd modules/llm-proxy && go test -race ./... -run TestConfigureBudget -v && go build ./... && go vet ./...`
Expected: test PASS; build + vet clean.

- [ ] **Step 6: Commit**

```bash
git add modules/llm-proxy/budget_config.go modules/llm-proxy/budget_config_test.go modules/llm-proxy/main.go
git commit -m "llm-proxy: budget config (reservePct + per-provider + prior ceilings) from env"
```

---

## Done criteria (sub-project 2)

- `cd modules/usage && go test -race ./... && go vet ./...` green.
- `cd modules/incus-dispatcher && go build ./... && go vet ./...` green (the only `-race` failure remains the pre-existing infra-only `TestFirecrackerRunner_Integration_RealWorkerVM`).
- `cd modules/llm-proxy && go test -race ./... && go vet ./... && go build ./...` green.
- A fleet call (`/anthropic/...`) whose window usage exceeds `ceiling × (1 − reservePct)` is refused with `429 + X-Budget-Deferred: 1 + Retry-After` and **never reaches upstream**; the same usage via `/interactive/anthropic/...` is forwarded.
- Proxied responses append `UsageEvent`s with the correct `Source`; an upstream `429` appends a calibrating `LimitEvent` at the current window usage.
- The proxy's dependency graph contains only the stdlib-only `usage` module (no gRPC/protobuf/temporal pulled in).

## Explicit v1 deferrals (carried from the spec)

- **TOCTOU soft cap:** concurrent near-cap fleet calls can modestly overshoot; the reserve absorbs the slack. Hard per-request serialization is deferred.
- **Non-Anthropic streaming usage shapes:** the scanner targets Anthropic's SSE/non-streaming shapes; OpenAI/ollama streaming-usage extraction beyond the documented stub is a later refinement (their non-streaming top-level `usage` still parses).
- **Producer / supervisor / tiered PAR panel:** sub-projects 3–5.
