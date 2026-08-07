package queue

import (
	"encoding/json"
	"testing"
)

// Identical cross-language fixture tested by the graphrag-rs Rust suite.
const capsuleRefFixture = `{"schema_version":1,"uri":"graphrag://capsules/v1/project/graphrag-rs-current","kind":"project","capsule_id":"graphrag-rs-current","project_id":"graphrag-rs","content_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","generated_at":"2026-08-07T00:00:00Z"}`

func TestCapsuleRefFixtureRoundtrip(t *testing.T) {
	var r ResumeRef
	if err := json.Unmarshal([]byte(capsuleRefFixture), &r); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if r.Capsule == nil {
		t.Fatal("expected typed capsule ref")
	}
	if r.Capsule.CapsuleID != "graphrag-rs-current" || r.Capsule.ProjectID != "graphrag-rs" || r.Capsule.Kind != "project" {
		t.Fatalf("bad decode: %+v", r.Capsule)
	}
	out, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != capsuleRefFixture {
		t.Fatalf("roundtrip mismatch:\n got %s\nwant %s", out, capsuleRefFixture)
	}
	if got := r.Display(); got != "graphrag://capsules/v1/project/graphrag-rs-current" {
		t.Fatalf("display: %s", got)
	}
}

func TestResumeRefLegacyStringCompatibility(t *testing.T) {
	// Old wire bodies carry resume_ref as an opaque string; they must decode
	// and re-encode unchanged.
	body := `{"actor":"human","work_kind":"maker","resume_ref":"docs/notes.md#resume"}`
	var a Attention
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		t.Fatalf("legacy decode: %v", err)
	}
	if a.ResumeRef == nil || a.ResumeRef.Legacy != "docs/notes.md#resume" || a.ResumeRef.Capsule != nil {
		t.Fatalf("bad legacy ref: %+v", a.ResumeRef)
	}
	if a.ResumeRef.Display() != "docs/notes.md#resume" {
		t.Fatalf("legacy display: %s", a.ResumeRef.Display())
	}
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Attention
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if back.ResumeRef.Legacy != "docs/notes.md#resume" {
		t.Fatalf("legacy roundtrip lost: %+v", back.ResumeRef)
	}
}

func TestResumeRefOmittedStaysAbsent(t *testing.T) {
	var a Attention
	if err := json.Unmarshal([]byte(`{"actor":"human","work_kind":"commitment"}`), &a); err != nil {
		t.Fatal(err)
	}
	if a.ResumeRef != nil {
		t.Fatalf("expected nil ResumeRef, got %+v", a.ResumeRef)
	}
	out, _ := json.Marshal(a)
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if _, present := m["resume_ref"]; present {
		t.Fatalf("resume_ref should be omitted: %s", out)
	}
}

func TestCapsuleRefObjectValidation(t *testing.T) {
	bad := []string{
		// unknown field
		`{"schema_version":1,"uri":"graphrag://capsules/v1/project/x","kind":"project","capsule_id":"x","project_id":"p","content_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","generated_at":"2026-08-07T00:00:00Z","extra":true}`,
		// wrong version
		`{"schema_version":2,"uri":"graphrag://capsules/v1/project/x","kind":"project","capsule_id":"x","project_id":"p","content_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","generated_at":"2026-08-07T00:00:00Z"}`,
		// bad kind
		`{"schema_version":1,"uri":"graphrag://capsules/v1/task/x","kind":"task","capsule_id":"x","project_id":"p","content_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","generated_at":"2026-08-07T00:00:00Z"}`,
		// URI/id mismatch
		`{"schema_version":1,"uri":"graphrag://capsules/v1/project/other","kind":"project","capsule_id":"x","project_id":"p","content_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","generated_at":"2026-08-07T00:00:00Z"}`,
		// bad fingerprint
		`{"schema_version":1,"uri":"graphrag://capsules/v1/project/x","kind":"project","capsule_id":"x","project_id":"p","content_fingerprint":"sha256:XYZ","generated_at":"2026-08-07T00:00:00Z"}`,
		// bad id charset
		`{"schema_version":1,"uri":"graphrag://capsules/v1/project/has space","kind":"project","capsule_id":"has space","project_id":"p","content_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","generated_at":"2026-08-07T00:00:00Z"}`,
		// bad timestamp
		`{"schema_version":1,"uri":"graphrag://capsules/v1/project/x","kind":"project","capsule_id":"x","project_id":"p","content_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","generated_at":"yesterday"}`,
	}
	for i, body := range bad {
		var r ResumeRef
		if err := json.Unmarshal([]byte(body), &r); err == nil {
			t.Errorf("case %d: expected rejection, got %+v", i, r)
		}
	}
}

func TestParseDirectiveRejectsInvalidCapsuleRef(t *testing.T) {
	body := `{"id":"d1","intent":"x","template":"t","origin":"orchestrator","importance":"normal","attempts":0,"attention":{"actor":"human","work_kind":"maker","resume_ref":{"schema_version":9,"uri":"u","kind":"project","capsule_id":"x","project_id":"p","content_fingerprint":"nope","generated_at":"now"}}}`
	if _, err := ParseDirective([]byte(body)); err == nil {
		t.Fatal("expected strict boundary rejection of invalid capsule ref")
	}
}

func TestParseDirectiveAcceptsTypedCapsuleRef(t *testing.T) {
	body := `{"id":"d1","intent":"x","template":"t","origin":"orchestrator","importance":"normal","attempts":0,"attention":{"actor":"human","work_kind":"maker","resume_ref":` + capsuleRefFixture + `}}`
	d, err := ParseDirective([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Attention == nil || d.Attention.ResumeRef == nil || d.Attention.ResumeRef.Capsule == nil {
		t.Fatalf("typed ref not preserved: %+v", d.Attention)
	}
	if d.Attention.ResumeRef.Capsule.URI != "graphrag://capsules/v1/project/graphrag-rs-current" {
		t.Fatalf("bad uri: %s", d.Attention.ResumeRef.Capsule.URI)
	}
}
