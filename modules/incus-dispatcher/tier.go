package main

// IsolationTier is the trust/isolation substrate a worker template runs on. It is a
// property of the vetted template (D1 mechanism — see Policy/TemplateRule), never an
// author-settable Directive field: a worker cannot downgrade isolation by proposing a
// weaker tier. The dispatcher resolves the tier from the validated template and the
// BackendFactory selects the runner that implements it.
type IsolationTier string

const (
	// TierFast is namespace-based isolation (nspawn --ephemeral inside the VM guest) for
	// trusted lanes and cheap iteration. Sub-second spin-up on a warm /nix store.
	TierFast IsolationTier = "fast"

	// TierHard is hardware isolation (per-task Firecracker microVM) for sensitive/untrusted
	// lanes (e.g. trading-platform domains). It is also the fail-safe default.
	TierHard IsolationTier = "hard"
)
