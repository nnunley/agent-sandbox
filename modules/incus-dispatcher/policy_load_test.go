package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemplates(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "templates.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadPolicy_ReadsRules(t *testing.T) {
	p, err := LoadPolicy(writeTemplates(t, `{
		"fleet-go":      {"allow_worker_origin": true, "tier": "fast"},
		"fleet-go-root": {"tier": "hard"},
		"fleet-default": {}
	}`))
	if err != nil {
		t.Fatalf("LoadPolicy: %v", err)
	}
	want := map[string]TemplateRule{
		"fleet-go":      {AllowWorkerOrigin: true, Tier: TierFast},
		"fleet-go-root": {AllowWorkerOrigin: false, Tier: TierHard},
		"fleet-default": {},
	}
	if len(p.Templates) != len(want) {
		t.Fatalf("got %d templates, want %d: %+v", len(p.Templates), len(want), p.Templates)
	}
	for name, rule := range want {
		if got := p.Templates[name]; got != rule {
			t.Errorf("template %q: got %+v, want %+v", name, got, rule)
		}
	}
	if got := p.TierFor("fleet-default"); got != TierHard {
		t.Errorf("unset tier resolves to %q, want hard", got)
	}
}

func TestLoadPolicy_Rejects(t *testing.T) {
	cases := map[string]struct{ body, wantErr string }{
		"empty allowlist": {`{}`, "no templates"},
		"unknown tier":    {`{"x": {"tier": "medium"}}`, `unknown tier "medium"`},
		"unknown field":   {`{"x": {"allow_root": true}}`, "allow_root"},
		"not json":        {`fleet-go`, "templates"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadPolicy(writeTemplates(t, c.body))
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("got err %v, want one containing %q", err, c.wantErr)
			}
		})
	}
}

func TestLoadPolicy_MissingFile(t *testing.T) {
	if _, err := LoadPolicy(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("want an error for a missing file")
	}
}

func TestServeCommand_RequiresTemplates(t *testing.T) {
	if rc := runServeCommand(nil); rc != 2 {
		t.Fatalf("serve without --templates returned %d, want 2", rc)
	}
}

func TestServeCommand_BadTemplatesFileFails(t *testing.T) {
	path := writeTemplates(t, `{}`)
	if rc := runServeCommand([]string{"--templates", path}); rc != 1 {
		t.Fatalf("serve with an empty allowlist returned %d, want 1", rc)
	}
}
