package main

import (
	"testing"
)

func TestSanitizeWorkerEnv_stripsKnownKeys(t *testing.T) {
	input := map[string]string{
		"ANTHROPIC_API_KEY": "sk-ant-secret",
		"OPENAI_API_KEY":    "sk-openai-secret",
		"DEBUG":             "1",
	}
	clean, stripped := SanitizeWorkerEnv(input)

	if _, ok := clean["ANTHROPIC_API_KEY"]; ok {
		t.Error("ANTHROPIC_API_KEY must not appear in clean env")
	}
	if _, ok := clean["OPENAI_API_KEY"]; ok {
		t.Error("OPENAI_API_KEY must not appear in clean env")
	}
	if v, ok := clean["DEBUG"]; !ok || v != "1" {
		t.Errorf("DEBUG must survive with value '1', got ok=%v v=%q", ok, v)
	}

	if len(stripped) != 2 {
		t.Fatalf("expected 2 stripped keys, got %d: %v", len(stripped), stripped)
	}
	if stripped[0] != "ANTHROPIC_API_KEY" || stripped[1] != "OPENAI_API_KEY" {
		t.Errorf("stripped must be sorted: got %v", stripped)
	}
}

func TestSanitizeWorkerEnv_caseInsensitive(t *testing.T) {
	input := map[string]string{
		"anthropic_api_key": "sk-ant-secret",
		"KEEP":              "yes",
	}
	clean, stripped := SanitizeWorkerEnv(input)

	if _, ok := clean["anthropic_api_key"]; ok {
		t.Error("lower-case anthropic_api_key must be stripped")
	}
	if v, ok := clean["KEEP"]; !ok || v != "yes" {
		t.Errorf("KEEP must survive, got ok=%v v=%q", ok, v)
	}
	if len(stripped) != 1 || stripped[0] != "anthropic_api_key" {
		t.Errorf("stripped must contain original-case key name: %v", stripped)
	}
}

func TestSanitizeWorkerEnv_inputNotMutated(t *testing.T) {
	input := map[string]string{
		"ANTHROPIC_API_KEY": "sk-ant-secret",
		"DEBUG":             "1",
	}
	SanitizeWorkerEnv(input)

	if _, ok := input["ANTHROPIC_API_KEY"]; !ok {
		t.Error("original input map must not be mutated")
	}
	if len(input) != 2 {
		t.Errorf("original input map length changed: %d", len(input))
	}
}

func TestSanitizeWorkerEnv_nilMap(t *testing.T) {
	clean, stripped := SanitizeWorkerEnv(nil)

	if clean == nil {
		t.Error("clean must be non-nil for nil input")
	}
	if len(clean) != 0 {
		t.Errorf("clean must be empty, got %v", clean)
	}
	if len(stripped) != 0 {
		t.Errorf("stripped must be empty, got %v", stripped)
	}
}

func TestSanitizeWorkerEnv_allProviderKeys(t *testing.T) {
	input := map[string]string{
		"ANTHROPIC_API_KEY":    "v1",
		"ANTHROPIC_AUTH_TOKEN": "v2",
		"OPENAI_API_KEY":       "v3",
		"OPENAI_KEY":           "v4",
		"OLLAMA_API_KEY":       "v5",
		"SAFE":                 "safe",
	}
	clean, stripped := SanitizeWorkerEnv(input)

	if len(stripped) != 5 {
		t.Errorf("expected 5 stripped keys, got %d: %v", len(stripped), stripped)
	}
	if v, ok := clean["SAFE"]; !ok || v != "safe" {
		t.Errorf("SAFE must survive, got ok=%v v=%q", ok, v)
	}
	if len(clean) != 1 {
		t.Errorf("clean should have exactly 1 entry, got %d: %v", len(clean), clean)
	}
}

func TestSanitizeWorkerEnv_patternNet(t *testing.T) {
	// Vars not in the explicit list but matching credential-like patterns must be stripped.
	input := map[string]string{
		"GEMINI_API_KEY": "gemini-secret", // _API_KEY suffix, not in explicit list
		"SOME_TOKEN":     "tok123",         // _TOKEN suffix
		"DB_PASSWORD":    "hunter2",        // contains PASSWORD
		"X_SECRET":       "shh",            // _SECRET suffix
		"DEBUG":          "1",
		"PATH":           "/usr/bin",
		"HOME":           "/root",
		"NId":            "nope",
	}
	clean, stripped := SanitizeWorkerEnv(input)

	for _, cred := range []string{"GEMINI_API_KEY", "SOME_TOKEN", "DB_PASSWORD", "X_SECRET"} {
		if _, ok := clean[cred]; ok {
			t.Errorf("%s must be stripped but survived in clean", cred)
		}
	}
	benign := map[string]string{"DEBUG": "1", "PATH": "/usr/bin", "HOME": "/root", "NId": "nope"}
	for k, want := range benign {
		if got, ok := clean[k]; !ok || got != want {
			t.Errorf("benign var %s must survive with value %q, got ok=%v v=%q", k, want, ok, got)
		}
	}
	if len(stripped) != 4 {
		t.Fatalf("expected 4 stripped, got %d: %v", len(stripped), stripped)
	}
	// sorted: DB_PASSWORD < GEMINI_API_KEY < SOME_TOKEN < X_SECRET
	expected := []string{"DB_PASSWORD", "GEMINI_API_KEY", "SOME_TOKEN", "X_SECRET"}
	for i, want := range expected {
		if stripped[i] != want {
			t.Errorf("stripped[%d]: want %q, got %q", i, want, stripped[i])
		}
	}
}

func TestLooksLikeCredential(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"GEMINI_API_KEY", true},
		{"SOME_TOKEN", true},
		{"DB_PASSWORD", true},
		{"X_SECRET", true},
		{"MY_KEY", true},
		{"gemini_api_key", true}, // case-insensitive
		{"some_token", true},
		{"DEBUG", false},
		{"PATH", false},
		{"HOME", false},
		{"NId", false},
		{"MONKEY", false}, // ends with KEY but not _KEY
	}
	for _, tc := range cases {
		got := looksLikeCredential(tc.name)
		if got != tc.want {
			t.Errorf("looksLikeCredential(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
