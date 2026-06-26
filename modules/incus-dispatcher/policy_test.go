package main

import (
	"strings"
	"sync"
	"testing"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

func testPolicy() *Policy {
	return &Policy{Templates: map[string]TemplateRule{
		"fleet-go":      {AllowWorkerOrigin: true},  // ordinary worker template
		"fleet-go-root": {AllowWorkerOrigin: false}, // privileged: orchestrator-only
	}}
}

func TestValidateTemplate_OrchestratorAllowed(t *testing.T) {
	p := testPolicy()
	for _, tmpl := range []string{"fleet-go", "fleet-go-root"} {
		d := queue.Directive{ID: "x", Template: tmpl, Origin: OriginOrchestrator}
		if err := p.ValidateTemplate(d); err != nil {
			t.Errorf("orchestrator + %q = %v, want allowed", tmpl, err)
		}
	}
}

func TestValidateTemplate_UnknownRejected(t *testing.T) {
	p := testPolicy()
	d := queue.Directive{ID: "x", Template: "not-a-template", Origin: OriginOrchestrator}
	if err := p.ValidateTemplate(d); err == nil {
		t.Fatal("unknown template was allowed, want rejected")
	}
}

func TestValidateTemplate_MissingRejected(t *testing.T) {
	p := testPolicy()
	if err := p.ValidateTemplate(queue.Directive{ID: "x", Origin: OriginOrchestrator}); err == nil {
		t.Fatal("empty template was allowed, want rejected")
	}
}

func TestValidateTemplate_WorkerOnNonPrivilegedAllowed(t *testing.T) {
	p := testPolicy()
	d := queue.Directive{ID: "x", Template: "fleet-go", Origin: "worker:abc"}
	if err := p.ValidateTemplate(d); err != nil {
		t.Errorf("worker + non-privileged template = %v, want allowed", err)
	}
}

// The core D1 escalation-prevention case: a worker-authored directive proposing a
// privileged template MUST be denied.
func TestValidateTemplate_WorkerOnPrivilegedDenied(t *testing.T) {
	p := testPolicy()
	d := queue.Directive{ID: "x", Template: "fleet-go-root", Origin: "worker:abc"}
	if err := p.ValidateTemplate(d); err == nil {
		t.Fatal("worker proposing privileged template was allowed — privilege escalation!")
	}
}

// Fail-closed: an unknown/empty origin is treated as worker-level (least privilege).
func TestValidateTemplate_UnknownOriginFailsClosed(t *testing.T) {
	p := testPolicy()
	d := queue.Directive{ID: "x", Template: "fleet-go-root", Origin: ""}
	if err := p.ValidateTemplate(d); err == nil {
		t.Fatal("empty origin allowed privileged template, want fail-closed denial")
	}
}

// AC-1 / SCENARIO-0074: the worker-origin denial error must contain the exact substring
// "worker-origin not allowed for privileged templates" so audit tooling can pattern-match it.
func TestValidateTemplate_WorkerOriginDenialMessage(t *testing.T) {
	const want = "worker-origin not allowed for privileged templates"
	p := testPolicy()
	d := queue.Directive{ID: "x", Template: "fleet-go-root", Origin: "worker:evil"}
	err := p.ValidateTemplate(d)
	if err == nil {
		t.Fatal("expected denial, got nil")
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("denial message %q does not contain %q", err.Error(), want)
	}
}

// AC-2: ValidateTemplate is deterministic and race-free under concurrent evaluation against
// a shared *Policy (the allowlist is an immutable map after construction).
func TestValidateTemplate_ConcurrentDeterministic(t *testing.T) {
	p := testPolicy()

	type testCase struct {
		d       queue.Directive
		wantErr bool
	}
	cases := []testCase{
		{queue.Directive{ID: "a", Template: "fleet-go-root", Origin: OriginOrchestrator}, false}, // orchestrator + privileged → always nil
		{queue.Directive{ID: "b", Template: "fleet-go-root", Origin: "worker:x"}, true},          // worker + privileged → always error
		{queue.Directive{ID: "c", Template: "fleet-go", Origin: "worker:x"}, false},              // worker + ordinary → always nil
	}

	const goroutines = 50
	errs := make([]error, goroutines)
	wants := make([]bool, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		tc := cases[i%len(cases)]
		wants[i] = tc.wantErr
		wg.Add(1)
		go func(idx int, d queue.Directive) {
			defer wg.Done()
			errs[idx] = p.ValidateTemplate(d)
		}(i, tc.d)
	}
	wg.Wait()

	for i, err := range errs {
		if (err != nil) != wants[i] {
			t.Errorf("goroutine %d: got err=%v, wantErr=%v", i, err, wants[i])
		}
	}
}
