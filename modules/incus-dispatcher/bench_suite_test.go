package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type suiteFixtureTask struct {
	name      string
	brief     string
	repo      string
	ref       string
	oracleRef string
}

func TestLoadBenchSuite_ComputesStableHashAndTasks(t *testing.T) {
	root := t.TempDir()
	writeSuiteFixture(t, root, "fleet-core", "v1", []suiteFixtureTask{
		{name: "task-a", brief: "fix A", oracleRef: "/oracle/a", repo: "/repo/a", ref: "main"},
		{name: "task-b", brief: "fix B", oracleRef: "/oracle/b", repo: "/repo/b", ref: "feature"},
	})

	suite, err := LoadBenchSuite(root, "fleet-core@v1")
	if err != nil {
		t.Fatalf("LoadBenchSuite: %v", err)
	}
	if suite.Name != "fleet-core" || suite.Version != "v1" {
		t.Fatalf("suite id mismatch: %+v", suite)
	}
	if len(suite.Tasks) != 2 {
		t.Fatalf("tasks=%d want 2", len(suite.Tasks))
	}
	if suite.Hash == "" {
		t.Fatal("expected non-empty content hash")
	}
	suite2, err := LoadBenchSuite(root, "fleet-core@v1")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if suite.Hash != suite2.Hash {
		t.Fatalf("hash not stable: %q vs %q", suite.Hash, suite2.Hash)
	}
	if got := suite.Tasks[0].Name; got != "task-a" {
		t.Fatalf("first task=%q want task-a", got)
	}
}

func writeSuiteFixture(t *testing.T, root, name, version string, tasks []suiteFixtureTask) {
	t.Helper()

	base := filepath.Join(root, "suites", name, version)
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatalf("mkdir suite: %v", err)
	}

	manifestTasks := make([]string, 0, len(tasks))
	for _, task := range tasks {
		manifestTasks = append(manifestTasks, task.name)
		taskDir := filepath.Join(base, "tasks", task.name)
		if err := os.MkdirAll(taskDir, 0755); err != nil {
			t.Fatalf("mkdir task: %v", err)
		}
		if err := os.WriteFile(filepath.Join(taskDir, "brief.txt"), []byte(task.brief+"\n"), 0644); err != nil {
			t.Fatalf("write brief: %v", err)
		}
		meta := map[string]string{
			"repo":       task.repo,
			"ref":        task.ref,
			"oracle_ref": task.oracleRef,
		}
		data, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			t.Fatalf("marshal meta: %v", err)
		}
		if err := os.WriteFile(filepath.Join(taskDir, "meta.json"), append(data, '\n'), 0644); err != nil {
			t.Fatalf("write meta: %v", err)
		}
	}

	manifest := map[string]interface{}{
		"name":    name,
		"version": version,
		"tasks":   manifestTasks,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "manifest.json"), append(data, '\n'), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
