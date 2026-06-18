package main

import (
	"testing"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

func testPolicy() *Policy {
	return &Policy{Templates: map[string]TemplateRule{
		"fleet-go":        {AllowWorkerOrigin: true},  // ordinary worker template
		"fleet-go-root":   {AllowWorkerOrigin: false}, // privileged: orchestrator-only
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
