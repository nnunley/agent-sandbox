package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// hostExec runs a host command, returning combined output, the remote command's exit
// code, and an error ONLY for transport/infra failures — NOT for a nonzero task exit
// (a failed task is carried in the exit code so the daemon's escalation ladder handles it).
type hostExec func(ctx context.Context, name string, args ...string) ([]byte, int, error)

// Fast-tier coord-VM defaults (match guests/coordinator-vm.nix + fleet-worker/unit/fleet-unit.sh).
const (
	defaultCoordGuest   = "agent-host"        // LXC container hosting the micro-VM(s)
	defaultCoordIP      = "10.88.0.2"         // durable coord VM static IP on br-microvm
	defaultUnitLauncher = "/root/fleet-unit.sh"
)

// NspawnRunner is the FAST isolation-tier backend (STORY-0021). It executes a task as a
// systemd-nspawn --ephemeral disposable unit INSIDE the durable coordinator micro-VM,
// sharing the VM kernel (PID/mount/IPC/UTS namespace isolation) over the warm read-only
// /nix store. Teardown is the ephemeral COW discard + process kill — never `incus delete`
// (STORY-0008 AC-2 / D5; incus is unreachable from inside the guest).
//
// Topology: the runner shells from the dispatch host into agent-host (the LXC hosting the
// micro-VM), SSHes into the coord VM, and invokes the proven launcher. When the coordinator
// daemon eventually runs INSIDE the VM, the same launcher runs locally — only the transport
// changes, behind the unchanged Runner seam.
type NspawnRunner struct {
	remote   string // incus remote (e.g. ndn-desktop)
	guest    string // LXC container hosting the micro-VM (agent-host)
	coordIP  string // durable coord VM IP on br-microvm
	launcher string // in-guest launcher path (fleet-unit.sh)
	exec     hostExec
}

// NewNspawnRunner builds a Fast-tier runner for the given incus remote using the production
// coord-VM defaults and the real exec transport.
func NewNspawnRunner(remote string) *NspawnRunner {
	return &NspawnRunner{
		remote:   remote,
		guest:    defaultCoordGuest,
		coordIP:  defaultCoordIP,
		launcher: defaultUnitLauncher,
		exec:     realHostExec,
	}
}

// realHostExec runs the command via os/exec, mapping a nonzero remote exit to (out, code,
// nil) and a true transport failure (binary missing, etc.) to (out, 0, err).
func realHostExec(ctx context.Context, name string, args ...string) ([]byte, int, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, ee.ExitCode(), nil // remote command ran and exited nonzero
		}
		return out, 0, err // could not run the transport at all
	}
	return out, 0, nil
}

// Run executes the task as an ephemeral nspawn unit in the coord VM (Fast tier).
func (r *NspawnRunner) Run(ctx context.Context, task Task) (*Result, error) {
	if err := task.validate(); err != nil {
		return nil, fmt.Errorf("invalid task: %w", err)
	}
	unit := generateContainerName(task.Name)
	// Command run INSIDE the coord VM: the ephemeral nspawn unit runs the task command.
	inner := fmt.Sprintf("bash %s run %s %s", r.launcher, unit, shellSingleQuote(strings.Join(task.Cmd, " ")))
	ssh := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o BatchMode=yes root@%s %s", r.coordIP, shellSingleQuote(inner))
	out, code, err := r.exec(ctx, "incus", "exec", r.remote+":"+r.guest, "--", "bash", "-lc", ssh)
	res := &Result{ExitCode: code, Stdout: string(out), ContainerName: unit}
	if err != nil {
		return res, fmt.Errorf("nspawn fast-tier transport: %w", err)
	}
	return res, nil
}

// Cleanup is a no-op: ephemeral units self-teardown on exit, leaving no durable resource.
func (r *NspawnRunner) Cleanup() error { return nil }

// shellSingleQuote wraps s in single quotes, escaping embedded single quotes, so it can be
// passed through a nested shell (incus exec → bash -lc → ssh → bash -lc) intact.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
