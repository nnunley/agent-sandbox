package main

import (
	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// NewChildDirective creates a worker-authored child directive that inherits the parent's
// provisioning (template) and carries only task content. The template is immutable and
// inherited; the child cannot escalate privilege because:
// (1) Directive schema forbids root/access_cmd flags (D1 premise),
// (2) If a worker tries to propose a privileged template, Policy.ValidateTemplate rejects
//
//	worker-origin directives for privileged templates (D1 gate).
//
// The returned directive has Origin set to "worker:<workerID>" and Intent/Task set to
// the supplied child content. The Template is copied from parent, enforcing that the
// child runs under the parent's vetted isolation tier and provisioning.
func NewChildDirective(parent queue.Directive, workerID, childIntent, childTask string) queue.Directive {
	return queue.Directive{
		// Inherit parent's template (immutable provisioning — the child cannot choose a new/privileged template).
		Template: parent.Template,

		// Mark as worker-authored so D1 can enforce AllowWorkerOrigin rules.
		Origin: "worker:" + workerID,

		// Child task content (intent and task are supplied by the worker).
		Intent: childIntent,
		Task:   childTask,

		// Preserve parent context fields for lineage/tracing (optional but good for observability).
		// The Directive struct does not have a Parent field, so lineage is captured implicitly
		// through ID naming or logging; if a future version adds a Parent field, this would be:
		// Parent: parent.ID,
	}
}
