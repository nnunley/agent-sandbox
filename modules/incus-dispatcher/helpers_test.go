package main

import (
	"strings"
	"testing"
)

func TestWorkerToolPath_PrependsWorkerDirs(t *testing.T) {
	got := workerToolPath("")
	// The worker's local bin must come first so its agent tools win.
	if !strings.HasPrefix(got, "/home/worker/.local/bin:") {
		t.Fatalf("PATH = %q, want it to start with the worker local bin", got)
	}
	for _, want := range []string{
		"/etc/profiles/per-user/worker/bin",
		"/home/worker/.nix-profile/bin",
		"/run/current-system/sw/bin",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("PATH %q missing %q", got, want)
		}
	}
}

func TestWorkerToolPath_DedupesAndKeepsExisting(t *testing.T) {
	got := workerToolPath("/run/current-system/sw/bin:/opt/custom/bin")
	// Existing custom dir is preserved...
	if !strings.Contains(got, "/opt/custom/bin") {
		t.Errorf("PATH %q dropped caller dir /opt/custom/bin", got)
	}
	// ...and the duplicate system dir appears exactly once.
	if n := strings.Count(got, "/run/current-system/sw/bin"); n != 1 {
		t.Errorf("/run/current-system/sw/bin appears %d times, want 1 (deduped)", n)
	}
}
