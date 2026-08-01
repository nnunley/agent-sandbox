# Fleet graphrag Retrieval (slice 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give a fleet run relevance-ranked opening context, retrieved from the fleet's own soft state via a first-party graphrag gRPC service, without ever affecting run correctness.

**Architecture:** A new `RetrievalProvider` interface (parallel to the existing `ContextProvider`) is implemented by `GraphRAGProvider`, a gRPC client of the graphrag service. The daemon ingests soft state (the handoff bundle's curated knowledge) into graphrag on handoff creation, and at work-claim retrieves top-k ranked facts and materializes them into a context file on the shared volume, passed to the worker by env var. Everything is best-effort soft state: with retrieval disabled (`NoopRetrievalProvider`, the default), runs behave exactly as today.

**Tech Stack:** Go (stdlib + `google.golang.org/grpc`), `buf` for protobuf codegen (same toolchain as `queue/laneqpb/`).

## Global Constraints

- Module path: `github.com/agent-sandbox/incus-dispatcher` (all dispatcher files are `package main`).
- Soft-state invariants (from `context.go`, STORY-0018): the retrieval subsystem is **best-effort**, is **never a work source**, and **never affects a run's grade**. Every call site swallows errors.
- Run all Go commands from `/Users/ndn/development/agent-sandbox/modules/incus-dispatcher` (relative paths break elsewhere).
- Go is installed locally; use `go test`/`go build`/`go vet`. The incus container has no Go.
- Commit messages must NOT contain `Claude Code`, `Generated with Claude`, `Co-Authored-By: Claude`, or `claude.com/claude-code` (pre-commit hook blocks them). Human-authored summaries only.
- This plan is **fleet-side only**. The graphrag service implementing the proto is a tracked prerequisite in graphrag's own roadmap; here it is exercised via a fake gRPC client in tests. Community/synthesis results only appear once that service and its synthesis workers exist — out of scope (see "Deferred").

## Proto contract (frozen; source of truth for generated types)

`RankedResult{Text, Score, SourceRef, Via}`; `Via ∈ {"base","community"}`. `StoreRef{Store, Root}`. RPCs: `Ingest(IngestRequest{Store, SourceRef, Content})`, `Ask(AskRequest{Store, Query, Top}) → AskResponse{Results}`, `SubmitSynthesisStep(SynthesisStepResult{Nonce, Result}) → SubmitAck{Merged}`. Slice 1 uses only `Ingest` and `Ask`.

---

## File Structure

- `retrieval.go` (create) — `RetrievalProvider` interface, `RankedFact`, `NoopRetrievalProvider`, materialization + ingest helpers.
- `retrieval_test.go` (create) — unit tests for Noop, materialization, ingest helper.
- `graphragpb/graphrag.proto` (create) — the frozen contract.
- `graphragpb/buf.yaml`, `graphragpb/buf.gen.yaml` (create) — mirror `queue/laneqpb/`.
- `graphragpb/graphrag.pb.go`, `graphragpb/graphrag_grpc.pb.go` (generated, committed).
- `graphrag_provider.go` (create) — `GraphRAGProvider` gRPC-client adapter.
- `graphrag_provider_test.go` (create) — adapter tests against a fake `graphragpb.GraphRAGClient`.
- `daemon.go` (modify) — add `Retrieval RetrievalProvider` field + `retrievalProvider()`; ingest at `emitRetryHandoff`; materialize in `RunOnce` before `runner.Run`.
- `serve_cmd.go` (modify) — construct and wire the provider.

---

### Task 1: RetrievalProvider interface, RankedFact, Noop default

**Files:**
- Create: `retrieval.go`
- Test: `retrieval_test.go`

**Interfaces:**
- Produces:
  - `type RankedFact struct { Text string; Score float64; SourceRef string; Via string }`
  - `type RetrievalProvider interface { Retrieve(threadID, query string, k int) ([]RankedFact, error); Ingest(sourceRef string, content []byte) error }`
  - `type NoopRetrievalProvider struct{}` implementing both (returns `nil,nil` / `nil`).

- [ ] **Step 1: Write the failing test**

```go
// retrieval_test.go
package main

import "testing"

func TestNoopRetrievalProvider_DropsEverything(t *testing.T) {
	var rp RetrievalProvider = NoopRetrievalProvider{}
	got, err := rp.Retrieve("thread-1", "some brief", 5)
	if err != nil {
		t.Fatalf("Retrieve err = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("Retrieve = %v, want nil (Noop drops everything)", got)
	}
	if err := rp.Ingest("src-1", []byte("data")); err != nil {
		t.Fatalf("Ingest err = %v, want nil", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ -run TestNoopRetrievalProvider_DropsEverything -v`
Expected: FAIL — `undefined: RetrievalProvider` / `undefined: NoopRetrievalProvider`.

- [ ] **Step 3: Write minimal implementation**

```go
// retrieval.go
package main

// RankedFact is one relevance-ranked piece of fleet soft state returned by a RetrievalProvider.
// Via is "base" (hybrid RRF, always available) or "community" (needed downstream synthesis).
type RankedFact struct {
	Text      string
	Score     float64
	SourceRef string // provenance: originating soft-state artifact
	Via       string
}

// RetrievalProvider is the read/base-index seam onto the fleet graphrag store. It parallels
// ContextProvider (context.go): SOFT STATE ONLY — best-effort, never a work source, and its
// loss or failure must never change a run's correctness (STORY-0018 AC-4/AC-5).
type RetrievalProvider interface {
	Retrieve(threadID, query string, k int) ([]RankedFact, error)
	Ingest(sourceRef string, content []byte) error
}

// NoopRetrievalProvider is the default: it drops all reads and writes. It is the mechanism that
// keeps correctness independent of retrieval — with it configured, a run behaves exactly as it
// did before this subsystem existed.
type NoopRetrievalProvider struct{}

func (NoopRetrievalProvider) Retrieve(string, string, int) ([]RankedFact, error) { return nil, nil }
func (NoopRetrievalProvider) Ingest(string, []byte) error                        { return nil }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ -run TestNoopRetrievalProvider_DropsEverything -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add retrieval.go retrieval_test.go
git commit -m "feat(retrieval): RetrievalProvider seam + Noop default"
```

---

### Task 2: Freeze the graphrag proto and generate the Go client

**Files:**
- Create: `graphragpb/graphrag.proto`, `graphragpb/buf.yaml`, `graphragpb/buf.gen.yaml`
- Generated (commit): `graphragpb/graphrag.pb.go`, `graphragpb/graphrag_grpc.pb.go`

**Interfaces:**
- Produces (generated Go, package `graphragpb`): `StoreRef{Store, Root string}`, `IngestRequest{Store *StoreRef, SourceRef string, Content []byte}`, `IngestResponse{}`, `AskRequest{Store *StoreRef, Query string, Top uint32}`, `RankedResult{Text string, Score float64, SourceRef string, Via string}`, `AskResponse{Results []*RankedResult}`, `SynthesisStepResult{Nonce string, Result []byte}`, `SubmitAck{Merged bool}`, and the client interface `GraphRAGClient` with `Ingest`, `Ask`, `SubmitSynthesisStep`.

- [ ] **Step 1: Write the proto**

```proto
// graphragpb/graphrag.proto
syntax = "proto3";

package graphrag.v1;

option go_package = "github.com/agent-sandbox/incus-dispatcher/graphragpb;graphragpb";

// GraphRAG is the fleet-facing surface of the first-party graphrag service. The service owns the
// store and is its sole writer. Slice 1 uses only Ingest + Ask.
service GraphRAG {
  rpc Ingest(IngestRequest) returns (IngestResponse);
  rpc Ask(AskRequest) returns (AskResponse);
  rpc SubmitSynthesisStep(SynthesisStepResult) returns (SubmitAck);
}

message StoreRef {
  string store = 1;
  string root = 2;
}

message IngestRequest {
  StoreRef store = 1;
  string source_ref = 2;
  bytes content = 3;
}
message IngestResponse {}

message AskRequest {
  StoreRef store = 1;
  string query = 2;
  uint32 top = 3;
}
message RankedResult {
  string text = 1;
  double score = 2;
  string source_ref = 3;
  string via = 4; // "base" | "community"
}
message AskResponse {
  repeated RankedResult results = 1;
}

message SynthesisStepResult {
  string nonce = 1;
  bytes result = 2;
}
message SubmitAck {
  bool merged = 1;
}
```

- [ ] **Step 2: Write the buf config (mirror laneqpb)**

```yaml
# graphragpb/buf.yaml
version: v1
breaking:
  use:
    - FILE
lint:
  use:
    - STANDARD
  except:
    - PACKAGE_DIRECTORY_MATCH
    - SERVICE_SUFFIX
```

```yaml
# graphragpb/buf.gen.yaml
version: v1
managed:
  enabled: false
plugins:
  - plugin: go
    out: .
    opt: paths=source_relative
  - plugin: go-grpc
    out: .
    opt: paths=source_relative
```

- [ ] **Step 3: Generate**

Run: `cd graphragpb && buf generate && cd ..`
Expected: creates `graphragpb/graphrag.pb.go` and `graphragpb/graphrag_grpc.pb.go`.
(If `buf` is unavailable, this is the sole step needing the buf toolchain — identical to how `queue/laneqpb/` was generated. Install buf, or generate on a host that has it, then commit the outputs.)

- [ ] **Step 4: Verify it builds**

Run: `go build ./graphragpb/`
Expected: builds with no errors.

- [ ] **Step 5: Commit**

```bash
git add graphragpb/
git commit -m "feat(graphragpb): freeze graphrag gRPC contract + generated client"
```

---

### Task 3: GraphRAGProvider — gRPC client adapter

**Files:**
- Create: `graphrag_provider.go`
- Test: `graphrag_provider_test.go`

**Interfaces:**
- Consumes: `graphragpb.GraphRAGClient`, `graphragpb.AskRequest/AskResponse/RankedResult/IngestRequest/StoreRef` (Task 2); `RankedFact`, `RetrievalProvider` (Task 1).
- Produces: `type GraphRAGProvider struct { Client graphragpb.GraphRAGClient; Store graphragpb.StoreRef; Timeout time.Duration }` implementing `RetrievalProvider`.

- [ ] **Step 1: Write the failing test**

```go
// graphrag_provider_test.go
package main

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-sandbox/incus-dispatcher/graphragpb"
	"google.golang.org/grpc"
)

// fakeGraphRAGClient implements graphragpb.GraphRAGClient for tests.
type fakeGraphRAGClient struct {
	askResp   *graphragpb.AskResponse
	askErr    error
	lastAsk   *graphragpb.AskRequest
	lastIngest *graphragpb.IngestRequest
	ingestErr error
}

func (f *fakeGraphRAGClient) Ingest(_ context.Context, in *graphragpb.IngestRequest, _ ...grpc.CallOption) (*graphragpb.IngestResponse, error) {
	f.lastIngest = in
	return &graphragpb.IngestResponse{}, f.ingestErr
}
func (f *fakeGraphRAGClient) Ask(_ context.Context, in *graphragpb.AskRequest, _ ...grpc.CallOption) (*graphragpb.AskResponse, error) {
	f.lastAsk = in
	return f.askResp, f.askErr
}
func (f *fakeGraphRAGClient) SubmitSynthesisStep(_ context.Context, _ *graphragpb.SynthesisStepResult, _ ...grpc.CallOption) (*graphragpb.SubmitAck, error) {
	return &graphragpb.SubmitAck{}, nil
}

func TestGraphRAGProvider_Retrieve_MapsAndPreservesProvenance(t *testing.T) {
	fc := &fakeGraphRAGClient{askResp: &graphragpb.AskResponse{Results: []*graphragpb.RankedResult{
		{Text: "prefer tokio", Score: 0.82, SourceRef: "handoff:t1", Via: "community"},
		{Text: "api is v2", Score: 0.71, SourceRef: "handoff:t1#2", Via: "base"},
	}}}
	p := &GraphRAGProvider{Client: fc, Store: graphragpb.StoreRef{Store: "proj", Root: "/srv/g"}}

	got, err := p.Retrieve("t1", "which async runtime", 5)
	if err != nil {
		t.Fatalf("Retrieve err = %v", err)
	}
	if len(got) != 2 || got[0].Text != "prefer tokio" || got[0].Via != "community" || got[0].SourceRef != "handoff:t1" {
		t.Fatalf("mapping wrong: %+v", got)
	}
	if fc.lastAsk.Query != "which async runtime" || fc.lastAsk.Top != 5 || fc.lastAsk.Store.Store != "proj" {
		t.Fatalf("request wrong: %+v", fc.lastAsk)
	}
}

func TestGraphRAGProvider_Retrieve_BestEffortOnRPCError(t *testing.T) {
	p := &GraphRAGProvider{Client: &fakeGraphRAGClient{askErr: errors.New("service down")}, Store: graphragpb.StoreRef{Store: "proj"}}
	got, err := p.Retrieve("t1", "q", 3)
	if err != nil || got != nil {
		t.Fatalf("want nil,nil on RPC error; got %v, %v", got, err)
	}
}

func TestGraphRAGProvider_Ingest_BestEffortOnRPCError(t *testing.T) {
	p := &GraphRAGProvider{Client: &fakeGraphRAGClient{ingestErr: errors.New("down")}, Store: graphragpb.StoreRef{Store: "proj"}}
	if err := p.Ingest("src", []byte("x")); err != nil {
		t.Fatalf("Ingest must be best-effort; got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ -run TestGraphRAGProvider -v`
Expected: FAIL — `undefined: GraphRAGProvider`.

- [ ] **Step 3: Write minimal implementation**

```go
// graphrag_provider.go
package main

import (
	"context"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/graphragpb"
)

// GraphRAGProvider is the RetrievalProvider adapter over the graphrag gRPC service. It does no
// text scraping: it speaks the frozen proto and maps RankedResult -> RankedFact. Every RPC error
// degrades to empty soft state (nil), never a returned error — callers treat retrieval as
// best-effort (STORY-0018 AC-4).
type GraphRAGProvider struct {
	Client  graphragpb.GraphRAGClient
	Store   graphragpb.StoreRef
	Timeout time.Duration // 0 -> defaultRetrievalTimeout
}

const defaultRetrievalTimeout = 5 * time.Second

func (p *GraphRAGProvider) ctx() (context.Context, context.CancelFunc) {
	d := p.Timeout
	if d == 0 {
		d = defaultRetrievalTimeout
	}
	return context.WithTimeout(context.Background(), d)
}

func (p *GraphRAGProvider) Retrieve(threadID, query string, k int) ([]RankedFact, error) {
	ctx, cancel := p.ctx()
	defer cancel()
	store := p.Store
	resp, err := p.Client.Ask(ctx, &graphragpb.AskRequest{Store: &store, Query: query, Top: uint32(k)})
	if err != nil || resp == nil {
		return nil, nil // best-effort: service down -> no context, run proceeds
	}
	out := make([]RankedFact, 0, len(resp.Results))
	for _, r := range resp.Results {
		out = append(out, RankedFact{Text: r.Text, Score: r.Score, SourceRef: r.SourceRef, Via: r.Via})
	}
	return out, nil
}

func (p *GraphRAGProvider) Ingest(sourceRef string, content []byte) error {
	ctx, cancel := p.ctx()
	defer cancel()
	store := p.Store
	_, _ = p.Client.Ingest(ctx, &graphragpb.IngestRequest{Store: &store, SourceRef: sourceRef, Content: content})
	return nil // best-effort: an ingest failure must never fail the caller's write
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ -run TestGraphRAGProvider -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add graphrag_provider.go graphrag_provider_test.go
git commit -m "feat(retrieval): GraphRAGProvider gRPC-client adapter"
```

---

### Task 4: Materialize retrieved context into a worker-readable file

**Files:**
- Modify: `retrieval.go`
- Test: `retrieval_test.go`

**Interfaces:**
- Consumes: `RetrievalProvider`, `RankedFact` (Task 1); `Task` (`types.go`, has `Env map[string]string`).
- Produces:
  - `const RetrievalEnvContextFile = "FLEET_RETRIEVED_CONTEXT"`
  - `func materializeRetrievedContext(rp RetrievalProvider, root, threadID, query string, k int, task *Task) error` — retrieves top-k, writes them as JSON to `<root>/<threadID>/retrieved-context.json`, sets `task.Env[RetrievalEnvContextFile]` to that path. No results / any error → no file, no env, nil return.

- [ ] **Step 1: Write the failing test**

```go
// add to retrieval_test.go
import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type stubRetrieval struct {
	facts []RankedFact
	err   error
}

func (s stubRetrieval) Retrieve(string, string, int) ([]RankedFact, error) { return s.facts, s.err }
func (s stubRetrieval) Ingest(string, []byte) error                        { return nil }

func TestMaterializeRetrievedContext_WritesFileAndSetsEnv(t *testing.T) {
	root := t.TempDir()
	task := &Task{Env: map[string]string{}}
	rp := stubRetrieval{facts: []RankedFact{{Text: "use tokio", Score: 0.9, SourceRef: "h:1", Via: "base"}}}

	if err := materializeRetrievedContext(rp, root, "t1", "brief", 5, task); err != nil {
		t.Fatalf("err = %v", err)
	}
	path := task.Env[RetrievalEnvContextFile]
	if path != filepath.Join(root, "t1", "retrieved-context.json") {
		t.Fatalf("env path = %q", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var got []RankedFact
	if err := json.Unmarshal(b, &got); err != nil || len(got) != 1 || got[0].Text != "use tokio" {
		t.Fatalf("file content wrong: %s (err %v)", b, err)
	}
}

func TestMaterializeRetrievedContext_NoResultsNoFileNoEnv(t *testing.T) {
	root := t.TempDir()
	task := &Task{Env: map[string]string{}}
	if err := materializeRetrievedContext(stubRetrieval{}, root, "t1", "brief", 5, task); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := task.Env[RetrievalEnvContextFile]; ok {
		t.Fatalf("env must be unset when there are no results")
	}
}

func TestMaterializeRetrievedContext_ErrorIsBestEffort(t *testing.T) {
	task := &Task{Env: map[string]string{}}
	if err := materializeRetrievedContext(stubRetrieval{err: os.ErrClosed}, t.TempDir(), "t1", "b", 5, task); err != nil {
		t.Fatalf("must swallow retrieval error; got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ -run TestMaterializeRetrievedContext -v`
Expected: FAIL — `undefined: materializeRetrievedContext` / `undefined: RetrievalEnvContextFile`.

- [ ] **Step 3: Write minimal implementation (append to retrieval.go)**

```go
// append to retrieval.go
import (
	"encoding/json"
	"os"
	"path/filepath"
)

// RetrievalEnvContextFile names the env var whose value is the path to the materialized
// retrieved-context JSON file inside the worker's environment (worker-side consumption is
// out of module, mirroring LLM_PROXY_ADDR).
const RetrievalEnvContextFile = "FLEET_RETRIEVED_CONTEXT"

// materializeRetrievedContext retrieves top-k ranked facts for a run and writes them as JSON to
// <root>/<threadID>/retrieved-context.json, recording the path in task.Env. It is BEST-EFFORT:
// any retrieval or filesystem error, or an empty result, leaves the task untouched and returns
// nil — a run must never fail or change because context could not be assembled.
func materializeRetrievedContext(rp RetrievalProvider, root, threadID, query string, k int, task *Task) error {
	facts, err := rp.Retrieve(threadID, query, k)
	if err != nil || len(facts) == 0 {
		return nil
	}
	b, err := json.Marshal(facts)
	if err != nil {
		return nil
	}
	dir := filepath.Join(root, threadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	path := filepath.Join(dir, "retrieved-context.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return nil
	}
	if task.Env == nil {
		task.Env = map[string]string{}
	}
	task.Env[RetrievalEnvContextFile] = path
	return nil
}
```

(Merge the new `import` block into the existing one at the top of `retrieval.go` rather than adding a second `import (...)`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ -run TestMaterializeRetrievedContext -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add retrieval.go retrieval_test.go
git commit -m "feat(retrieval): materialize top-k context into worker-readable file"
```

---

### Task 5: Ingest helper for handoff soft state

**Files:**
- Modify: `retrieval.go`
- Test: `retrieval_test.go`

**Interfaces:**
- Consumes: `RetrievalProvider` (Task 1).
- Produces: `func ingestHandoffKnowledge(rp RetrievalProvider, bundlePath, threadID, runID string)` — reads `<bundlePath>/knowledge.jsonl` and, if present and non-empty, calls `rp.Ingest("handoff:"+threadID+":"+runID, content)`. Best-effort, returns nothing.

- [ ] **Step 1: Write the failing test**

```go
// add to retrieval_test.go
type recordingIngest struct {
	sourceRef string
	content   []byte
	calls     int
}

func (r *recordingIngest) Retrieve(string, string, int) ([]RankedFact, error) { return nil, nil }
func (r *recordingIngest) Ingest(sourceRef string, content []byte) error {
	r.calls++
	r.sourceRef, r.content = sourceRef, content
	return nil
}

func TestIngestHandoffKnowledge_IngestsBundleKnowledge(t *testing.T) {
	bundle := t.TempDir()
	if err := os.WriteFile(filepath.Join(bundle, "knowledge.jsonl"), []byte(`{"k":"v"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rp := &recordingIngest{}
	ingestHandoffKnowledge(rp, bundle, "t1", "r1")
	if rp.calls != 1 || rp.sourceRef != "handoff:t1:r1" || string(rp.content) != `{"k":"v"}` {
		t.Fatalf("ingest wrong: calls=%d ref=%q content=%q", rp.calls, rp.sourceRef, rp.content)
	}
}

func TestIngestHandoffKnowledge_NoFileNoCall(t *testing.T) {
	rp := &recordingIngest{}
	ingestHandoffKnowledge(rp, t.TempDir(), "t1", "r1") // no knowledge.jsonl present
	if rp.calls != 0 {
		t.Fatalf("must not ingest when no knowledge file exists; calls=%d", rp.calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ -run TestIngestHandoffKnowledge -v`
Expected: FAIL — `undefined: ingestHandoffKnowledge`.

- [ ] **Step 3: Write minimal implementation (append to retrieval.go)**

```go
// append to retrieval.go
// ingestHandoffKnowledge base-indexes a freshly created handoff bundle's curated knowledge into
// the fleet store. This is the one live soft-state production site today (daemon emitRetryHandoff).
// Best-effort: a missing/empty file or any error is silently ignored.
func ingestHandoffKnowledge(rp RetrievalProvider, bundlePath, threadID, runID string) {
	if bundlePath == "" {
		return
	}
	content, err := os.ReadFile(filepath.Join(bundlePath, "knowledge.jsonl"))
	if err != nil || len(content) == 0 {
		return
	}
	_ = rp.Ingest("handoff:"+threadID+":"+runID, content)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ -run TestIngestHandoffKnowledge -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add retrieval.go retrieval_test.go
git commit -m "feat(retrieval): ingest handoff curated knowledge into fleet store"
```

---

### Task 6: Wire retrieval into the daemon

**Files:**
- Modify: `daemon.go` (struct at `:31-49`; `emitRetryHandoff` around `:116`; `RunOnce` — claim at `:190`, `runner.Run` at `:259`)
- Modify: `serve_cmd.go` (`Daemon{...}` literal at `:73-80`)
- Test: `daemon_test.go` (append)

**Interfaces:**
- Consumes: `RetrievalProvider`, `NoopRetrievalProvider`, `materializeRetrievedContext`, `RetrievalEnvContextFile`, `ingestHandoffKnowledge` (Tasks 1/4/5).
- Produces: `Daemon.Retrieval RetrievalProvider` field; `Daemon.retrievalProvider() RetrievalProvider` (Noop fallback); `Daemon.RetrievalRoot string` (where context files are materialized).

- [ ] **Step 1: Write the failing test**

```go
// add to daemon_test.go
func TestDaemon_retrievalProvider_DefaultsToNoop(t *testing.T) {
	dm := &Daemon{}
	if _, ok := dm.retrievalProvider().(NoopRetrievalProvider); !ok {
		t.Fatalf("default retrievalProvider must be NoopRetrievalProvider, got %T", dm.retrievalProvider())
	}
}

func TestMaterialize_CorrectnessIndependence_NoopSetsNoContext(t *testing.T) {
	// With the Noop provider (the default), materialization must leave the task's env clean:
	// correctness/behavior is identical to pre-retrieval.
	task := &Task{Env: map[string]string{"FLEET_PROVIDER": "anthropic"}}
	dm := &Daemon{}
	_ = materializeRetrievedContext(dm.retrievalProvider(), t.TempDir(), "t1", "brief", 5, task)
	if _, ok := task.Env[RetrievalEnvContextFile]; ok {
		t.Fatalf("Noop provider must not add retrieved-context env")
	}
	if task.Env["FLEET_PROVIDER"] != "anthropic" {
		t.Fatalf("unrelated env must be preserved")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ -run 'TestDaemon_retrievalProvider_DefaultsToNoop|TestMaterialize_CorrectnessIndependence' -v`
Expected: FAIL — `dm.retrievalProvider undefined`.

- [ ] **Step 3: Add the field and fallback to daemon.go**

In the `Daemon` struct (`daemon.go:31-49`), add after the `Context ContextProvider` line:

```go
	Context        ContextProvider
	Retrieval      RetrievalProvider // soft-state retrieval/base-index (optional; defaults to NoopRetrievalProvider). Best-effort; never affects correctness.
	RetrievalRoot  string            // dir under which retrieved-context files are materialized (shared volume). Empty -> os.TempDir().
```

After the existing `ctxProvider()` method (around `daemon.go:53-58`), add:

```go
// retrievalProvider returns the configured RetrievalProvider, or a NoopRetrievalProvider when
// none is set. Retrieval is soft state: it is never a work source and never affects correctness.
func (dm *Daemon) retrievalProvider() RetrievalProvider {
	if dm.Retrieval != nil {
		return dm.Retrieval
	}
	return NoopRetrievalProvider{}
}

func (dm *Daemon) retrievalRoot() string {
	if dm.RetrievalRoot != "" {
		return dm.RetrievalRoot
	}
	return os.TempDir()
}
```

(Ensure `os` is imported in `daemon.go`; add it to the import block if absent.)

- [ ] **Step 4: Run the two unit tests to verify they pass**

Run: `go test ./ -run 'TestDaemon_retrievalProvider_DefaultsToNoop|TestMaterialize_CorrectnessIndependence' -v`
Expected: PASS.

- [ ] **Step 5: Wire ingest at handoff creation**

In `emitRetryHandoff` (`daemon.go`), replace the discarded `CreateHandoff` result (`daemon.go:116`):

```go
	_, _ = dm.ctxProvider().CreateHandoff(d.ID, runID, st)
```

with capture + ingest:

```go
	bundlePath, _ := dm.ctxProvider().CreateHandoff(d.ID, runID, st)
	ingestHandoffKnowledge(dm.retrievalProvider(), bundlePath, d.ID, runID)
```

- [ ] **Step 6: Wire retrieval materialization before the worker runs**

In `RunOnce`, between where the task is built (`mapFn(d)`) and `runner.Run(ctx, task)` (`daemon.go:259`), add a best-effort materialization. Locate the `task :=` assignment produced by `mapFn`; immediately after it insert:

```go
	// Best-effort: assemble relevance-ranked soft-state context for this run. Failure or an empty
	// result leaves the task unchanged (correctness is independent of retrieval — STORY-0018 AC-4).
	query := task.Name
	if len(task.Cmd) > 0 {
		query = task.Name + " " + strings.Join(task.Cmd, " ")
	}
	_ = materializeRetrievedContext(dm.retrievalProvider(), dm.retrievalRoot(), d.ID, query, defaultRetrievalTopK, &task)
```

Add near the top of `daemon.go` (with the other consts) and ensure `strings` is imported:

```go
const defaultRetrievalTopK = 8
```

(If `task` is not addressable as `&task` at that point — e.g. it is used by value — pass `&task` after the `mapFn` result is stored in a local `task` variable; the existing code already holds it in `task` for `runner.Run(ctx, task)`.)

- [ ] **Step 7: Wire the provider in serve_cmd.go**

In the `Daemon{...}` literal (`serve_cmd.go:73-80`), the provider defaults to Noop with no change required. To enable graphrag when configured, add after the literal (guarded by an address flag/env so the default stays Noop):

```go
	if addr := os.Getenv("FLEET_GRAPHRAG_ADDR"); addr != "" {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("graphrag retrieval disabled: %v", err)
		} else {
			dm.Retrieval = &GraphRAGProvider{
				Client: graphragpb.NewGraphRAGClient(conn),
				Store:  graphragpb.StoreRef{Store: os.Getenv("FLEET_GRAPHRAG_STORE"), Root: os.Getenv("FLEET_GRAPHRAG_ROOT")},
			}
			dm.RetrievalRoot = os.Getenv("FLEET_GRAPHRAG_ROOT")
		}
	}
```

Add imports to `serve_cmd.go`: `"os"`, `"log"`, `"google.golang.org/grpc"`, `"google.golang.org/grpc/credentials/insecure"`, `"github.com/agent-sandbox/incus-dispatcher/graphragpb"`.

- [ ] **Step 8: Build, vet, and run the full package tests**

Run: `go build ./... && go vet ./... && go test ./ -v`
Expected: builds clean; all tests pass, including the pre-existing daemon suite (proving retrieval wiring did not disturb the run loop).

- [ ] **Step 9: Commit**

```bash
git add daemon.go serve_cmd.go daemon_test.go
git commit -m "feat(retrieval): wire graphrag ingest + retrieval into the daemon run loop"
```

---

## Deferred (explicitly out of scope for this plan)

- **Synthesis-step worker + laneq producer** (spec build-step 5): the graphrag service mints nonces and enqueues synthesis-step directives; an ephemeral worker claims them, runs the LLM via the llm-proxy, and calls `SubmitSynthesisStep`. Gated on (a) the graphrag service existing and (b) a frozen synthesis-step laneq directive shape. Until then, only `Via:"base"` results appear — which is correct and useful. This is the next plan.
- **Broader ingest coverage:** when `WriteDiary`/`ShareKnowledge` get wired into the daemon (currently defined but uncalled), add `rp.Ingest` calls at those sites so the corpus grows beyond handoff knowledge.
- **Context-window token budgeting** (spec slice 2) and **project-soul surfacing** (slice 3).

---

## Self-Review

**Spec coverage:**
- Goal 1 (relevance-ranked opening context) → Tasks 3, 4, 6. ✓
- Goal 2 (synthesis as fleet work) → Deferred (explicitly), with `Via` plumbed so base works now. ✓ (scoped)
- Goal 3 (soft-state invariants) → Noop default (Task 1), best-effort adapter (Task 3), best-effort materialize/ingest (Tasks 4/5), correctness-independence test (Task 6). ✓
- Contract (proto) → Task 2. ✓
- `RankedFact{Text,Score,SourceRef,Via}` → Task 1, consumed unchanged in Tasks 3/4. ✓

**Placeholder scan:** No TBD/TODO; every code step has complete code. The one environmental dependency (`buf`) is called out explicitly in Task 2, not hidden.

**Type consistency:** `RetrievalProvider` (Retrieve + Ingest), `RankedFact` fields, `GraphRAGProvider{Client,Store,Timeout}`, `materializeRetrievedContext(rp,root,threadID,query,k,task)`, `ingestHandoffKnowledge(rp,bundlePath,threadID,runID)`, `RetrievalEnvContextFile`, `defaultRetrievalTopK`, `retrievalProvider()`/`retrievalRoot()` — names identical across all tasks that reference them. Generated `graphragpb` types match the proto field names (proto `source_ref` → Go `SourceRef`). ✓
