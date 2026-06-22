package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// STORY-0021 AC-1/AC-3: the FAST tier drives the proven in-guest nspawn --ephemeral
// disposable-unit launcher (fleet-worker/unit/fleet-unit.sh) inside the durable coord VM.
// The runner shells: incus exec <remote>:<guest> -- ssh root@<coordIP> 'bash <launcher> run ...'.
func TestNspawnRunner_RunInvokesEphemeralLauncher(t *testing.T) {
	var got string
	r := &NspawnRunner{
		remote: "ndn-desktop", guest: "agent-host", coordIP: "10.88.0.2",
		launcher: "/root/fleet-unit.sh",
		exec: func(_ context.Context, name string, args ...string) ([]byte, int, error) {
			got = name + " " + strings.Join(args, " ")
			return []byte("READY\n"), 0, nil
		},
	}
	res, err := r.Run(context.Background(), Task{Name: "trusted-job", Cmd: []string{"echo", "hi"}})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	for _, want := range []string{"incus", "exec", "ndn-desktop:agent-host", "ssh", "10.88.0.2", "/root/fleet-unit.sh", "run"} {
		if !strings.Contains(got, want) {
			t.Errorf("launcher command %q missing %q", got, want)
		}
	}
	// D5 / STORY-0008 AC-2: fast-tier teardown is unit-kill (ephemeral COW discard) — the
	// runner must NEVER `incus delete` a disposable unit.
	if strings.Contains(got, "delete") {
		t.Errorf("runner command must not invoke incus delete: %q", got)
	}
}

// A nonzero TASK exit is a task failure carried in Result.ExitCode, not an infra error —
// so the daemon's passed()/escalation ladder handles it as a failed run, not a crash.
func TestNspawnRunner_NonzeroExitIsResultNotInfraError(t *testing.T) {
	r := &NspawnRunner{
		launcher: "/root/fleet-unit.sh",
		exec: func(_ context.Context, _ string, _ ...string) ([]byte, int, error) {
			return []byte("nope\n"), 7, nil
		},
	}
	res, err := r.Run(context.Background(), Task{Name: "j", Cmd: []string{"false"}})
	if err != nil {
		t.Fatalf("nonzero task exit must not surface as an infra error: %v", err)
	}
	if res.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", res.ExitCode)
	}
}

// A transport (incus/ssh) failure IS an infra error and surfaces.
func TestNspawnRunner_InfraErrorSurfaces(t *testing.T) {
	r := &NspawnRunner{
		launcher: "/root/fleet-unit.sh",
		exec: func(_ context.Context, _ string, _ ...string) ([]byte, int, error) {
			return nil, 0, errors.New("incus: command not found")
		},
	}
	if _, err := r.Run(context.Background(), Task{Name: "j", Cmd: []string{"x"}}); err == nil {
		t.Fatal("transport failure must surface as an error")
	}
}

// Cleanup is a no-op: ephemeral units self-teardown on exit (no durable resource to reap).
func TestNspawnRunner_CleanupNoOp(t *testing.T) {
	if err := (&NspawnRunner{}).Cleanup(); err != nil {
		t.Errorf("Cleanup = %v, want nil", err)
	}
}

// STORY-0021 AC-3: a TierFast template routes to the nspawn runner through the factory
// (template-driven selection), proving the Fast backend is registered, not the graft stub.
func TestNspawnRunner_RegisteredUnderTierFast(t *testing.T) {
	nsr := &NspawnRunner{launcher: "/root/fleet-unit.sh"}
	f := newStaticBackendFactory(map[IsolationTier]Runner{TierFast: nsr})
	got, err := f.SelectRunner(TierFast)
	if err != nil {
		t.Fatalf("SelectRunner(fast) err = %v", err)
	}
	if got != Runner(nsr) {
		t.Errorf("TierFast did not resolve to the NspawnRunner")
	}
}

// Integration: drive the REAL coord VM. Self-skips off-cluster (no Mac Nix/cluster), so the
// unit suite stays CI-green; on the cluster it proves the runner spins a real ephemeral unit.
func TestNspawnRunner_Integration_RealCoordVM(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if err := exec.Command("incus", "list", "ndn-desktop:").Run(); err != nil {
		t.Skip("incus remote ndn-desktop unreachable — cluster-only")
	}
	// Ensure the launcher is present in the durable coord VM (idempotent; production installs
	// it via provisioning, but the integration test stays self-contained).
	push := exec.Command("bash", "-lc",
		`incus exec ndn-desktop:agent-host -- bash -lc "ssh -o StrictHostKeyChecking=no -o BatchMode=yes root@10.88.0.2 'cat > /root/fleet-unit.sh && chmod +x /root/fleet-unit.sh'" < ../../fleet-worker/unit/fleet-unit.sh`)
	if out, err := push.CombinedOutput(); err != nil {
		t.Skipf("could not stage launcher into coord VM (coord VM down?): %v\n%s", err, out)
	}
	r := NewNspawnRunner("ndn-desktop")
	res, err := r.Run(context.Background(), Task{Name: "nspawn-itest", Cmd: []string{"echo", "READY"}})
	if err != nil {
		t.Fatalf("integration Run err = %v", err)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "READY") {
		t.Fatalf("integration result = %+v, want exit 0 + READY", res)
	}
}
