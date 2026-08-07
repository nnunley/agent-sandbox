package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// CapsuleRef is a typed pointer to a GraphRAG capsule (Project Capsule v1
// contract, docs/project-capsule-v1.md in graphrag-rs). Field order matches
// the canonical cross-language JSON field order.
type CapsuleRef struct {
	SchemaVersion      int    `json:"schema_version"`
	URI                string `json:"uri"`
	Kind               string `json:"kind"` // project | segment | portfolio
	CapsuleID          string `json:"capsule_id"`
	ProjectID          string `json:"project_id"`
	ContentFingerprint string `json:"content_fingerprint"` // sha256: + 64 lowercase hex
	GeneratedAt        string `json:"generated_at"`        // RFC 3339
}

func validCapsuleID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch {
		case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9', b == '.', b == '_', b == '-':
		default:
			return false
		}
	}
	return true
}

func validFingerprint(s string) bool {
	const prefix = "sha256:"
	if len(s) != len(prefix)+64 || s[:len(prefix)] != prefix {
		return false
	}
	for i := len(prefix); i < len(s); i++ {
		b := s[i]
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}

// Validate enforces the v1 capsule-ref contract.
func (c *CapsuleRef) Validate() error {
	if c.SchemaVersion != 1 {
		return fmt.Errorf("capsule ref: unsupported schema_version %d", c.SchemaVersion)
	}
	switch c.Kind {
	case "project", "segment", "portfolio":
	default:
		return fmt.Errorf("capsule ref: unknown kind %q", c.Kind)
	}
	if !validCapsuleID(c.CapsuleID) {
		return fmt.Errorf("capsule ref: invalid capsule_id %q", c.CapsuleID)
	}
	if !validCapsuleID(c.ProjectID) {
		return fmt.Errorf("capsule ref: invalid project_id %q", c.ProjectID)
	}
	if expected := "graphrag://capsules/v1/" + c.Kind + "/" + c.CapsuleID; c.URI != expected {
		return fmt.Errorf("capsule ref: uri %q does not match %q", c.URI, expected)
	}
	if !validFingerprint(c.ContentFingerprint) {
		return fmt.Errorf("capsule ref: invalid content_fingerprint %q", c.ContentFingerprint)
	}
	if _, err := time.Parse(time.RFC3339, c.GeneratedAt); err != nil {
		return fmt.Errorf("capsule ref: invalid generated_at %q: %w", c.GeneratedAt, err)
	}
	return nil
}

// ResumeRef is where a human resumes work. Wire-compatible with both the
// legacy opaque string form ("resume_ref": "docs/notes.md") and the typed
// CapsuleRef object form. Exactly one of Legacy/Capsule is set.
type ResumeRef struct {
	Legacy  string
	Capsule *CapsuleRef
}

// Display is the human-readable form used in attention brief reasons.
func (r *ResumeRef) Display() string {
	if r == nil {
		return ""
	}
	if r.Capsule != nil {
		return r.Capsule.URI
	}
	return r.Legacy
}

// UnmarshalJSON accepts a legacy string or a strict, validated CapsuleRef
// object. Unknown object fields and contract violations are rejected so the
// ParseDirective/DecodeListedDirective boundary stays strict.
func (r *ResumeRef) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("resume_ref: empty value")
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		*r = ResumeRef{Legacy: s}
		return nil
	}
	if trimmed[0] != '{' {
		return fmt.Errorf("resume_ref: expected string or capsule ref object")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var c CapsuleRef
	if err := dec.Decode(&c); err != nil {
		return fmt.Errorf("resume_ref: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("resume_ref: trailing data")
	}
	if err := c.Validate(); err != nil {
		return err
	}
	*r = ResumeRef{Capsule: &c}
	return nil
}

// MarshalJSON re-emits exactly the form that was decoded.
func (r ResumeRef) MarshalJSON() ([]byte, error) {
	if r.Capsule != nil {
		return json.Marshal(r.Capsule)
	}
	return json.Marshal(r.Legacy)
}
