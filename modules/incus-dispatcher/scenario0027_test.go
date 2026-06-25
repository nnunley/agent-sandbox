package main

import (
	"reflect"
	"testing"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// SCENARIO-0027 — D1: Child directive from worker inherits immutable provisioning (STORY-0049 AC-4).
// Worker-authored child directives carry task content only; provisioning is inherited and never privileged.
// Proves: (1) inheritance of the parent's template; (2) D1 accepts the inherited non-privileged child;
// (3) no escalation — a worker-origin privileged template is rejected by D1, AND inheritance gives the
// worker no way to CHOOSE a template (the constructor has no template param); (4) task-only — the
// Directive schema carries no root/privileged field, so a directive can never request privilege;
// (5) end-to-end — the child resolves to a non-privileged Task at the parent's inherited tier.
func TestScenario0027_ChildDirectiveProvisioning(t *testing.T) {
	p := testPolicy() // "fleet-go" (AllowWorkerOrigin=true), "fleet-go-root" (AllowWorkerOrigin=false)

	// Setup: parent directive from orchestrator to W1 with ordinary (non-privileged) template.
	parent := queue.Directive{
		ID:       "parent-directive",
		Intent:   "execute-task",
		Template: "fleet-go", // ordinary, AllowWorkerOrigin=true
		Origin:   OriginOrchestrator,
		Task:     "run unit tests",
		Repo:     "https://example.com/repo",
		Ref:      "main",
	}

	// Test 1: Inheritance — the child created by W1 inherits the parent's template; its own intent/task.
	child := NewChildDirective(parent, "W1", "worker-child-intent", "worker-child-task")
	if child.Template != parent.Template {
		t.Fatalf("child.Template=%q, parent.Template=%q: inheritance failed", child.Template, parent.Template)
	}
	if child.Origin != "worker:W1" {
		t.Fatalf("child.Origin=%q, want worker:W1", child.Origin)
	}
	if child.Intent != "worker-child-intent" || child.Task != "worker-child-task" {
		t.Fatalf("child task content not preserved: intent=%q task=%q", child.Intent, child.Task)
	}

	// Test 2: D1 accepts the inherited non-privileged child.
	if err := p.ValidateTemplate(child); err != nil {
		t.Fatalf("D1 rejected inherited non-privileged child: %v", err)
	}

	// Test 3: No escalation (D1, defense-in-depth) — a hand-forged worker-origin directive proposing
	// the PRIVILEGED template is rejected. fleet-go-root IS in the allowlist, so this rejection is
	// specifically the worker-origin-privileged gate (AC-3), not an unknown-template rejection.
	rogue := queue.Directive{ID: "rogue", Intent: "elevate", Template: "fleet-go-root", Origin: "worker:W1", Task: "install"}
	if err := p.ValidateTemplate(rogue); err == nil {
		t.Fatal("D1 permitted worker-origin privileged template: privilege-escalation vulnerability")
	}

	// Test 3b: No escalation (inheritance) — the AC-4-specific property. NewChildDirective has NO
	// template parameter, so a worker can never choose a template; it always inherits the parent's.
	// Even when the parent is privileged, the inherited child is worker-origin and D1 rejects it —
	// so a worker never runs a privileged template, even as the child of a privileged parent.
	privParent := queue.Directive{ID: "priv-parent", Template: "fleet-go-root", Origin: OriginOrchestrator, Intent: "x", Task: "y"}
	privChild := NewChildDirective(privParent, "W1", "ci", "ct")
	if privChild.Template != "fleet-go-root" {
		t.Fatalf("inheritance not immutable: privileged parent template not inherited, got %q", privChild.Template)
	}
	if err := p.ValidateTemplate(privChild); err == nil {
		t.Fatal("worker-origin child of a privileged parent must be rejected by D1 (no privileged worker runs)")
	}

	// Test 4: Task-only — the Directive schema carries NO root/privileged field, so a directive (parent
	// or child) can never REQUEST privilege; privilege is decided solely by the vetted template/tier.
	dt := reflect.TypeOf(queue.Directive{})
	for _, forbidden := range []string{"Root", "RunAsRoot", "AccessCmd", "Privileged", "Sudo"} {
		if _, has := dt.FieldByName(forbidden); has {
			t.Fatalf("queue.Directive has a %q field — a directive must not be able to request privilege", forbidden)
		}
	}

	// Test 5: End-to-end — the child resolves to a Task that runs under the parent's INHERITED tier and
	// is NEVER privileged. DefaultMapToTask derives no RunAsRoot from a directive (privilege comes only
	// from the template/tier), so a worker child directive cannot launch a root container.
	if got, want := p.TierFor(child.Template), p.TierFor(parent.Template); got != want {
		t.Fatalf("child tier=%q != parent tier=%q: provisioning not inherited", got, want)
	}
	task := DefaultMapToTask(child)
	if task.RunAsRoot {
		t.Fatalf("child resolved to a privileged Task (RunAsRoot=true) — a directive must not grant root")
	}
}
