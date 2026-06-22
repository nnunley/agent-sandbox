package main

import (
	"context"
	"fmt"
	"strings"
)

// Hard-tier worker-VM defaults (match guests/worker-vm.nix).
const (
	defaultWorkerUnit = "microvm@worker-vm.service" // per-task Firecracker microVM unit
	defaultWorkerMAC  = "02:00:00:00:00:02"         // worker-vm tap MAC → dnsmasq lease lookup
	defaultWorkerUser = "worker"                    // non-root worker user (claude refuses root)
	dnsmasqLeases     = "/var/lib/dnsmasq/dnsmasq.leases"
)

// FirecrackerRunner is the HARD isolation-tier backend (STORY-0022). It runs each task in a
// per-task Firecracker microVM with its OWN kernel (hardware isolation) for sensitive /
// untrusted lanes. The runner boots the worker microVM unit, resolves the guest IP from its
// dnsmasq lease, SSHes in as the non-root worker, runs the task, and tears the VM down by
// `systemctl stop` (Cleanup) — NEVER `incus delete` (microVMs are systemd units, and the hot
// path stays incus-delete-free per D5).
//
// v1 reuses the single declarative worker-vm unit (stop/start per task); per-task UNIQUE VM
// names + multi-domain provisioning are the same deferred graft as STORY-0024's multi-tenancy
// (ITER-0006+). The Runner seam is unchanged, so that grafts in without touching the daemon.
type FirecrackerRunner struct {
	remote    string // incus remote (e.g. ndn-desktop)
	guest     string // LXC container hosting the micro-VM (agent-host)
	unit      string // worker microVM systemd unit
	workerMAC string // worker-vm MAC, used to resolve its DHCP lease IP
	sshUser   string // in-guest user to SSH as
	exec      hostExec
}

// NewFirecrackerRunner builds a Hard-tier runner for the given incus remote with the
// production worker-vm defaults and the real exec transport.
func NewFirecrackerRunner(remote string) *FirecrackerRunner {
	return &FirecrackerRunner{
		remote:    remote,
		guest:     defaultCoordGuest, // agent-host
		unit:      defaultWorkerUnit,
		workerMAC: defaultWorkerMAC,
		sshUser:   defaultWorkerUser,
		exec:      realHostExec,
	}
}

// Run boots the per-task worker microVM, runs the task command in-guest, and returns the
// result. Teardown is left to Cleanup (systemctl stop).
func (r *FirecrackerRunner) Run(ctx context.Context, task Task) (*Result, error) {
	if err := task.validate(); err != nil {
		return nil, fmt.Errorf("invalid task: %w", err)
	}
	// Single agent-host-side script: boot → wait unit-ready → resolve lease IP → wait sshd →
	// run the task command in-guest. The final ssh's exit status is the task's exit code.
	script := fmt.Sprintf(`set -o pipefail
U=%s
systemctl start "$U"
for i in $(seq 1 200); do systemctl is-active "$U" 2>/dev/null | grep -q '^active' && systemctl show -p MainPID --value "$U" 2>/dev/null | grep -qE '^[1-9][0-9]*$' && break; sleep 0.1; done
IP=$(awk '/%s/{print $3}' %s | head -1)
[ -n "$IP" ] || { echo "no lease for worker microVM" >&2; exit 70; }
ssh-keygen -R "$IP" >/dev/null 2>&1 || true
for i in $(seq 1 120); do ssh -o StrictHostKeyChecking=no -o ConnectTimeout=3 -o BatchMode=yes %s@"$IP" true 2>/dev/null && break; sleep 0.5; done
ssh -o StrictHostKeyChecking=no -o BatchMode=yes %s@"$IP" %s`,
		r.unit, r.workerMAC, dnsmasqLeases, r.sshUser, r.sshUser, shellSingleQuote(strings.Join(task.Cmd, " ")))

	out, code, err := r.exec(ctx, "incus", "exec", r.remote+":"+r.guest, "--", "bash", "-lc", script)
	res := &Result{ExitCode: code, Stdout: string(out), ContainerName: r.unit}
	if err != nil {
		return res, fmt.Errorf("firecracker hard-tier transport: %w", err)
	}
	return res, nil
}

// Cleanup tears the per-task microVM down via `systemctl stop` — never `incus delete`.
func (r *FirecrackerRunner) Cleanup() error {
	if r.exec == nil || r.unit == "" {
		return nil
	}
	_, _, err := r.exec(context.Background(), "incus", "exec", r.remote+":"+r.guest, "--",
		"bash", "-lc", "systemctl stop "+r.unit)
	if err != nil {
		return fmt.Errorf("firecracker hard-tier teardown: %w", err)
	}
	return nil
}
