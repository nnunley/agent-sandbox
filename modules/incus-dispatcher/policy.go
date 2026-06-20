package main

import (
	"fmt"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// TemplateRule describes a pre-vetted, immutable worker template the daemon is
// allowed to launch. AllowWorkerOrigin gates whether a directive AUTHORED BY A
// WORKER (origin "worker:<id>") may propose this template — the D1 authority
// split: workers propose, policy disposes. Privileged/sensitive templates set
// AllowWorkerOrigin=false so a compromised or drifting worker cannot escalate by
// pushing a child directive that proposes them.
type TemplateRule struct {
	AllowWorkerOrigin bool
}

// Policy is the allowlist of launchable templates + origin rules.
type Policy struct {
	Templates map[string]TemplateRule
}

// OriginOrchestrator is the trusted origin (directives the orchestrator authors).
const OriginOrchestrator = "orchestrator"

// ValidateTemplate enforces D1: the proposed template must be in the allowlist,
// and the directive's origin must be permitted to use it. Returns nil if the
// directive may launch its proposed template.
func (p *Policy) ValidateTemplate(d queue.Directive) error {
	if d.Template == "" {
		return fmt.Errorf("policy: directive %s has no template", d.ID)
	}
	rule, ok := p.Templates[d.Template]
	if !ok {
		return fmt.Errorf("policy: template %q not in allowlist", d.Template)
	}
	if isWorkerOrigin(d.Origin) && !rule.AllowWorkerOrigin {
		return fmt.Errorf("policy: worker-origin not allowed for privileged templates: origin %q template %q", d.Origin, d.Template)
	}
	return nil
}

// isWorkerOrigin reports whether an origin denotes a worker-authored directive.
// Anything other than the trusted orchestrator origin is treated as worker-level
// (fail-closed): an unknown/empty origin gets the least privilege.
func isWorkerOrigin(origin string) bool {
	return origin != OriginOrchestrator
}
