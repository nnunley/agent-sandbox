package queue

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseDirective_ValidRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := Directive{
		ID:         "d-001",
		Intent:     "run tests",
		Template:   "go-test",
		Origin:     "orchestrator",
		Importance: ImportanceHigh,
		Lane:       "default",
		Repo:       "https://example.com/repo",
		Ref:        "main",
		Task:       "go test ./...",
		Attempts:   1,
		NotBefore:  now,
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got, err := ParseDirective(data)
	if err != nil {
		t.Fatalf("ParseDirective returned unexpected error: %v", err)
	}

	if got.ID != d.ID {
		t.Errorf("ID: got %q, want %q", got.ID, d.ID)
	}
	if got.Intent != d.Intent {
		t.Errorf("Intent: got %q, want %q", got.Intent, d.Intent)
	}
	if got.Template != d.Template {
		t.Errorf("Template: got %q, want %q", got.Template, d.Template)
	}
	if got.Origin != d.Origin {
		t.Errorf("Origin: got %q, want %q", got.Origin, d.Origin)
	}
}

func TestParseDirective_RejectsAccessCmd(t *testing.T) {
	payload := []byte(`{"id":"x","intent":"test","template":"go-test","access_cmd":"rm -rf /"}`)

	_, err := ParseDirective(payload)
	if err == nil {
		t.Fatal("expected error for unknown field 'access_cmd', got nil")
	}
}

func TestParseDirective_RejectsRoot(t *testing.T) {
	payload := []byte(`{"id":"x","intent":"test","template":"go-test","root":true}`)

	_, err := ParseDirective(payload)
	if err == nil {
		t.Fatal("expected error for unknown field 'root', got nil")
	}
}

func TestParseDirective_RejectsArbitraryUnknownField(t *testing.T) {
	payload := []byte(`{"id":"x","intent":"test","template":"go-test","evil":"payload"}`)

	_, err := ParseDirective(payload)
	if err == nil {
		t.Fatal("expected error for unknown field 'evil', got nil")
	}
}

func TestParseDirective_RejectsTrailingGarbage(t *testing.T) {
	payload := []byte(`{"id":"x","intent":"test","template":"go-test"}` + `{"extra":"junk"}`)

	_, err := ParseDirective(payload)
	if err == nil {
		t.Fatal("expected error for trailing garbage after JSON object, got nil")
	}
}

func TestParseDirective_RejectsInvalidJSON(t *testing.T) {
	payload := []byte(`not json`)

	_, err := ParseDirective(payload)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
