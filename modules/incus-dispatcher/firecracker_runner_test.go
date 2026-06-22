package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// STORY-0022 AC-1/AC-3: the HARD tier runs each task in a per-task Firecracker microVM
// (own kernel ⇒ hardware isolation). The runner boots the worker microVM unit, SSHes into
// the guest (resolved via its dnsmasq lease), runs the task, and tears the VM down by
// `systemctl stop` — NEVER `incus delete` (microVMs are systemd units, not incus instances).
func TestFirecrackerRunner_RunBootsWorkerVMAndRunsInGuest(t *testing.T) {
	var got string
	r := &FirecrackerRunner{
		remote: "ndn-desktop", guest: "agent-host",
		unit: "microvm@worker-vm.service", workerMAC: "02:00:00:00:00:02", sshUser: "worker",
		exec: func(_ context.Context, name string, args ...string) ([]byte, int, error) {
			got += name + " " + strings.Join(args, " ") + "\n"
			return []byte("READY\n"), 0, nil
		},
	}
	res, err := r.Run(context.Background(), Task{Name: "sensitive-job", Cmd: []string{"echo", "hi"}})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	for _, want := range []string{"incus", "agent-host", "systemctl start", "microvm@worker-vm.service", "02:00:00:00:00:02", "ssh", "worker@"} {
		if !strings.Contains(got, want) {
			t.Errorf("boot/run command\n%s\nmissing %q", got, want)
		}
	}
	if strings.Contains(got, "delete") {
		t.Errorf("hard-tier runner must not invoke incus delete:\n%s", got)
	}
}

// Cleanup tears the per-task VM down via systemctl stop — not incus delete (D5).
func TestFirecrackerRunner_CleanupStopsVMNotIncusDelete(t *testing.T) {
	var got string
	r := &FirecrackerRunner{
		guest: "agent-host", unit: "microvm@worker-vm.service",
		exec: func(_ context.Context, name string, args ...string) ([]byte, int, error) {
			got = name + " " + strings.Join(args, " ")
			return nil, 0, nil
		},
	}
	if err := r.Cleanup(); err != nil {
		t.Fatalf("Cleanup err = %v", err)
	}
	if !strings.Contains(got, "systemctl stop microvm@worker-vm.service") {
		t.Errorf("Cleanup did not stop the VM: %q", got)
	}
	if strings.Contains(got, "delete") {
		t.Errorf("Cleanup must not invoke incus delete: %q", got)
	}
}

// A nonzero task exit is a task failure (Result.ExitCode), not an infra error.
func TestFirecrackerRunner_NonzeroExitIsResultNotInfraError(t *testing.T) {
	r := &FirecrackerRunner{
		unit: "microvm@worker-vm.service",
		exec: func(_ context.Context, _ string, _ ...string) ([]byte, int, error) {
			return []byte("fail\n"), 5, nil
		},
	}
	res, err := r.Run(context.Background(), Task{Name: "j", Cmd: []string{"false"}})
	if err != nil {
		t.Fatalf("nonzero task exit must not be an infra error: %v", err)
	}
	if res.ExitCode != 5 {
		t.Errorf("ExitCode = %d, want 5", res.ExitCode)
	}
}

// A transport failure IS an infra error and surfaces.
func TestFirecrackerRunner_InfraErrorSurfaces(t *testing.T) {
	r := &FirecrackerRunner{
		unit: "microvm@worker-vm.service",
		exec: func(_ context.Context, _ string, _ ...string) ([]byte, int, error) {
			return nil, 0, errors.New("incus: command not found")
		},
	}
	if _, err := r.Run(context.Background(), Task{Name: "j", Cmd: []string{"x"}}); err == nil {
		t.Fatal("transport failure must surface as an error")
	}
}

// STORY-0022 AC-3: a TierHard template routes to the Firecracker runner through the factory.
func TestFirecrackerRunner_RegisteredUnderTierHard(t *testing.T) {
	fr := &FirecrackerRunner{unit: "microvm@worker-vm.service"}
	f := newStaticBackendFactory(map[IsolationTier]Runner{TierHard: fr})
	got, err := f.SelectRunner(TierHard)
	if err != nil {
		t.Fatalf("SelectRunner(hard) err = %v", err)
	}
	if got != Runner(fr) {
		t.Errorf("TierHard did not resolve to the FirecrackerRunner")
	}
}

// Integration: boot the REAL worker microVM, run a command in-guest, stop it. Self-skips
// off-cluster so the unit suite stays CI-green.
func TestFirecrackerRunner_Integration_RealWorkerVM(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if err := exec.Command("incus", "list", "ndn-desktop:").Run(); err != nil {
		t.Skip("incus remote ndn-desktop unreachable — cluster-only")
	}
	r := NewFirecrackerRunner("ndn-desktop")
	defer r.Cleanup()
	res, err := r.Run(context.Background(), Task{Name: "fc-itest", Cmd: []string{"echo", "READY"}})
	if err != nil {
		t.Fatalf("integration Run err = %v", err)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "READY") {
		t.Fatalf("integration result = %+v, want exit 0 + READY", res)
	}
}
