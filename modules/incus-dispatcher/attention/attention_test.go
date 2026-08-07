package attention

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

type source struct {
	ds  []queue.Directive
	err error
}

func (s source) List() ([]queue.Directive, error) { return s.ds, s.err }
func item(id, intent string, m queue.Attention) queue.Directive {
	return queue.Directive{ID: id, Intent: intent, Importance: queue.ImportanceHigh, Lane: queue.HumanLane, Attention: &m}
}
func TestWalkingSkeletonFourNamedItems(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	future := now.Add(48 * time.Hour)
	ds := []queue.Directive{
		item("packet", "Prime Radiant packet approval", queue.Attention{Actor: "human+agent", Kind: "commitment", EstimatedHumanMinutes: 5, CompletionGate: "approve packet"}),
		item("google", "Google application follow-up", queue.Attention{Actor: "human+agent", Kind: "commitment", ExternalWaiting: true, EstimatedHumanMinutes: 10, CompletionGate: "send follow-up"}),
		item("leit", "forest-rs/leit PR #18: Bruce review window", queue.Attention{Actor: "human+agent", Kind: "commitment", EstimatedHumanMinutes: 5}),
		item("compiler", "let-go compiler project: context-warm maker segment", queue.Attention{Actor: "human", Kind: "maker", ContextKey: "let-go/compiler", ResumeRef: &queue.ResumeRef{Capsule: &queue.CapsuleRef{
			SchemaVersion:      1,
			URI:                "graphrag://capsules/v1/project/graphrag-rs-current",
			Kind:               "project",
			CapsuleID:          "graphrag-rs-current",
			ProjectID:          "graphrag-rs",
			ContentFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			GeneratedAt:        "2026-08-07T00:00:00Z",
		}}}),
	}
	ds[2].NotBefore = future
	b, err := Build(source{ds: ds}, now, Options{CommitmentFloorComplete: false, ContextKey: "let-go/compiler"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Selections) != 2 {
		t.Fatalf("got %d: %#v", len(b.Selections), b)
	}
	if b.Selections[0].Directive.ID != "packet" || b.Selections[1].Directive.ID != "compiler" {
		t.Fatalf("wrong brief: %#v", b)
	}
	for _, s := range b.Selections {
		if s.Reason == "" {
			t.Fatal("missing reason")
		}
	}
	if !strings.Contains(b.Selections[1].Reason, "graphrag://capsules/v1/project/graphrag-rs-current") {
		t.Fatalf("maker reason should carry the capsule URI: %q", b.Selections[1].Reason)
	}
}
func TestNotBeforeExcluded(t *testing.T) {
	n := time.Now()
	d := item("x", "x", queue.Attention{Actor: "human", Kind: "commitment"})
	d.NotBefore = n.Add(time.Hour)
	b, _ := Build(source{ds: []queue.Directive{d}}, n, Options{})
	if len(b.Selections) != 0 {
		t.Fatal("future included")
	}
}
func TestCommitmentFloorCompleteSwitch(t *testing.T) {
	n := time.Now()
	ds := []queue.Directive{item("c", "c", queue.Attention{Actor: "human", Kind: "commitment"}), item("m", "m", queue.Attention{Actor: "human", Kind: "maker"})}
	b, _ := Build(source{ds: ds}, n, Options{CommitmentFloorComplete: true})
	if got := ids(b); !reflect.DeepEqual(got, []string{"m"}) {
		t.Fatal(got)
	}
}
func TestDeterministicTieBreak(t *testing.T) {
	n := time.Now()
	ds := []queue.Directive{item("b", "b", queue.Attention{Actor: "human", Kind: "commitment"}), item("a", "a", queue.Attention{Actor: "human", Kind: "commitment"})}
	b, _ := Build(source{ds: ds}, n, Options{})
	if b.Selections[0].Directive.ID != "a" {
		t.Fatal("not ID stable")
	}
}
func TestOldDirectiveCompatibility(t *testing.T) {
	d, err := queue.ParseDirective([]byte(`{"id":"old","intent":"old","template":"t","origin":"orchestrator","importance":"normal","attempts":0}`))
	if err != nil || d.Attention != nil {
		t.Fatalf("%v %#v", err, d)
	}
}
func TestHumanLaneNotGeneralExecution(t *testing.T) {
	if queue.GeneralExecutionLane == queue.HumanLane || queue.AgentClaimable(queue.Directive{Lane: queue.HumanLane, Attention: &queue.Attention{Actor: "human"}}) {
		t.Fatal("human work claimable")
	}
	if !queue.AgentClaimable(queue.Directive{Lane: queue.GeneralExecutionLane}) {
		t.Fatal("old agent work blocked")
	}
}
func TestListAndDecodeFailure(t *testing.T) {
	want := errors.New("boom")
	if _, err := Build(source{err: want}, time.Now(), Options{}); !errors.Is(err, want) {
		t.Fatalf("%v", err)
	}
	if _, err := queue.DecodeListedDirective([]byte(`{"new_field":1}`)); err == nil {
		t.Fatal("decode accepted")
	}
}
func ids(b Brief) []string {
	var x []string
	for _, s := range b.Selections {
		x = append(x, s.Directive.ID)
	}
	return x
}
