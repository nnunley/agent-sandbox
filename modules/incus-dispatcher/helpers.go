package main

import (
	"context"
	"os/exec"
)

// newCmdContext creates a new exec.Cmd with the given context.
func newCmdContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}
