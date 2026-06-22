package queue

import (
	"encoding/json"
	"testing"
	"time"
)

// TestScenario0045 proves STORY-0064 AC-1..AC-14: the directive contract schema.
// This test demonstrates that ParseDirective enforces the strict ingestion boundary:
// required fields parse; optional fields parse when present; access_cmd and root are
// rejected; max_attempts is accepted (deprecated but wire-present per AC-12).
//
// AC-15 (temporal projection) and AC-16 (propose-vs-set authority) are explicitly
// OUT of scope (deferred to ITER-0007 — Temporal projection).
//
// AC-2 validation half (template-vs-allowlist + origin authority) is already proven
// by ITER-0002 D1 ValidateTemplate (policy.go + policy_test.go/scenario_d1_test.go);
// this test proves only the schema-level half (field presence + parsing).
func TestScenario0045(t *testing.T) {
	// AC-1: intent field present + parseable
	t.Run("AC-1_intent_present_parseable", func(t *testing.T) {
		d := Directive{
			Intent:     "run tests",
			Template:   "go-test",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			Lane:       "default",
			Repo:       "https://example.com/repo",
			Ref:        "main",
			Task:       "go test ./...",
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Intent != "run tests" {
			t.Errorf("Intent: got %q, want %q", got.Intent, "run tests")
		}
	})

	// AC-2: template field present and parses (validation half deferred to D1).
	t.Run("AC-2_template_present_parses", func(t *testing.T) {
		d := Directive{
			Intent:     "run tests",
			Template:   "go-test",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			Lane:       "default",
			Repo:       "https://example.com/repo",
			Ref:        "main",
			Task:       "go test ./...",
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Template != "go-test" {
			t.Errorf("Template: got %q, want %q", got.Template, "go-test")
		}
	})

	// AC-3: origin field is orchestrator or worker:<id> (schema-level proof;
	// daemon-sets-it enforcement is elsewhere, D1).
	t.Run("AC-3_origin_orchestrator", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Origin != "orchestrator" {
			t.Errorf("Origin: got %q, want orchestrator", got.Origin)
		}
	})

	t.Run("AC-3_origin_worker_id", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "worker:w-123",
			Importance: ImportanceNormal,
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Origin != "worker:w-123" {
			t.Errorf("Origin: got %q, want worker:w-123", got.Origin)
		}
	})

	// AC-4: importance field is enum (high, normal, low) and parses.
	t.Run("AC-4_importance_high", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceHigh,
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Importance != ImportanceHigh {
			t.Errorf("Importance: got %q, want high", got.Importance)
		}
	})

	t.Run("AC-4_importance_normal", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Importance != ImportanceNormal {
			t.Errorf("Importance: got %q, want normal", got.Importance)
		}
	})

	t.Run("AC-4_importance_low", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceLow,
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Importance != ImportanceLow {
			t.Errorf("Importance: got %q, want low", got.Importance)
		}
	})

	// AC-5: deadline field is OPTIONAL and parses as *time.Time.
	t.Run("AC-5_deadline_absent", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			// Deadline is omitted
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Deadline != nil {
			t.Errorf("Deadline: expected nil, got %v", got.Deadline)
		}
	})

	t.Run("AC-5_deadline_present", func(t *testing.T) {
		deadline := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			Deadline:   &deadline,
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Deadline == nil {
			t.Errorf("Deadline: expected non-nil, got nil")
		} else if *got.Deadline != deadline {
			t.Errorf("Deadline: got %v, want %v", *got.Deadline, deadline)
		}
	})

	// AC-6: lane field is present and parses.
	t.Run("AC-6_lane_present", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			Lane:       "default",
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Lane != "default" {
			t.Errorf("Lane: got %q, want default", got.Lane)
		}
	})

	// AC-7: repo field is present and parses.
	t.Run("AC-7_repo_present", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			Repo:       "https://example.com/repo",
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Repo != "https://example.com/repo" {
			t.Errorf("Repo: got %q, want https://example.com/repo", got.Repo)
		}
	})

	// AC-8: ref field is present and parses.
	t.Run("AC-8_ref_present", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			Ref:        "main",
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Ref != "main" {
			t.Errorf("Ref: got %q, want main", got.Ref)
		}
	})

	// AC-9: task field is present and parses.
	t.Run("AC-9_task_present", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			Task:       "go test ./...",
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Task != "go test ./..." {
			t.Errorf("Task: got %q, want go test ./...", got.Task)
		}
	})

	// AC-10: handoff_in field is OPTIONAL and parses as string.
	t.Run("AC-10_handoff_in_absent", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			// HandoffIn is omitted
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.HandoffIn != "" {
			t.Errorf("HandoffIn: expected empty, got %q", got.HandoffIn)
		}
	})

	t.Run("AC-10_handoff_in_present", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			HandoffIn:  "bundle-12345",
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.HandoffIn != "bundle-12345" {
			t.Errorf("HandoffIn: got %q, want bundle-12345", got.HandoffIn)
		}
	})

	// AC-11: grade field is OPTIONAL GradeSpec with oracle_ref, cmd, expect.
	t.Run("AC-11_grade_absent", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			// Grade is omitted
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Grade != nil {
			t.Errorf("Grade: expected nil, got %v", got.Grade)
		}
	})

	t.Run("AC-11_grade_present", func(t *testing.T) {
		d := Directive{
			Intent:     "test",
			Template:   "t",
			Origin:     "orchestrator",
			Importance: ImportanceNormal,
			Grade: &GradeSpec{
				OracleRef: "oracle@v1",
				Cmd:       "go test -tags oracle ./...",
				Expect: map[string]any{
					"exit_code": 0,
					"tests":     42,
				},
			},
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.Grade == nil {
			t.Fatalf("Grade: expected non-nil, got nil")
		}
		if got.Grade.OracleRef != "oracle@v1" {
			t.Errorf("Grade.OracleRef: got %q, want oracle@v1", got.Grade.OracleRef)
		}
		if got.Grade.Cmd != "go test -tags oracle ./..." {
			t.Errorf("Grade.Cmd: got %q, want go test -tags oracle ./...", got.Grade.Cmd)
		}
		if got.Grade.Expect["exit_code"] != float64(0) {
			t.Errorf("Grade.Expect[exit_code]: got %v, want 0", got.Grade.Expect["exit_code"])
		}
	})

	// AC-12: max_attempts field is DEPRECATED (superseded by D4 escalation ladder).
	// ITER-0001 retained for wire compatibility; not read by coordinator.
	// Test: field parses without error; presence is sufficient (comment documents deprecation).
	t.Run("AC-12_max_attempts_deprecated_but_wire_present", func(t *testing.T) {
		d := Directive{
			Intent:      "test",
			Template:    "t",
			Origin:      "orchestrator",
			Importance:  ImportanceNormal,
			MaxAttempts: 3, // DEPRECATED: superseded by D4 graduated escalation ladder.
		}
		data, _ := json.Marshal(d)
		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}
		if got.MaxAttempts != 3 {
			t.Errorf("MaxAttempts: got %d, want 3 (note: deprecated, retained for wire compat)", got.MaxAttempts)
		}
	})

	// AC-13: access_cmd field is REJECTED (unknown field).
	t.Run("AC-13_access_cmd_rejected", func(t *testing.T) {
		payload := []byte(`{"intent":"test","template":"t","origin":"orchestrator","access_cmd":"rm -rf /"}`)
		_, err := ParseDirective(payload)
		if err == nil {
			t.Fatal("expected error for unknown field 'access_cmd', got nil")
		}
	})

	// AC-14: root field is REJECTED (unknown field).
	t.Run("AC-14_root_rejected", func(t *testing.T) {
		payload := []byte(`{"intent":"test","template":"t","origin":"orchestrator","root":true}`)
		_, err := ParseDirective(payload)
		if err == nil {
			t.Fatal("expected error for unknown field 'root', got nil")
		}
	})

	// Comprehensive: fully-populated valid directive with all optional fields present.
	t.Run("comprehensive_all_fields", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		deadline := now.Add(24 * time.Hour)
		d := Directive{
			ID:         "d-001",
			Intent:     "run tests",
			Template:   "go-test",
			Origin:     "orchestrator",
			Importance: ImportanceHigh,
			Deadline:   &deadline,
			Lane:       "default",
			Repo:       "https://example.com/repo",
			Ref:        "main",
			Task:       "go test ./...",
			HandoffIn:  "bundle-xyz",
			Grade: &GradeSpec{
				OracleRef: "oracle@v1",
				Cmd:       "go test -tags oracle ./...",
				Expect: map[string]any{
					"exit_code": 0,
				},
			},
			MaxAttempts: 3,
			Attempts:    1,
			NotBefore:   now,
		}
		data, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		got, err := ParseDirective(data)
		if err != nil {
			t.Fatalf("ParseDirective: %v", err)
		}

		// Verify all fields preserved.
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
		if got.Importance != d.Importance {
			t.Errorf("Importance: got %q, want %q", got.Importance, d.Importance)
		}
		if got.Deadline == nil || *got.Deadline != deadline {
			t.Errorf("Deadline: got %v, want %v", got.Deadline, &deadline)
		}
		if got.Lane != d.Lane {
			t.Errorf("Lane: got %q, want %q", got.Lane, d.Lane)
		}
		if got.Repo != d.Repo {
			t.Errorf("Repo: got %q, want %q", got.Repo, d.Repo)
		}
		if got.Ref != d.Ref {
			t.Errorf("Ref: got %q, want %q", got.Ref, d.Ref)
		}
		if got.Task != d.Task {
			t.Errorf("Task: got %q, want %q", got.Task, d.Task)
		}
		if got.HandoffIn != d.HandoffIn {
			t.Errorf("HandoffIn: got %q, want %q", got.HandoffIn, d.HandoffIn)
		}
		if got.Grade == nil || got.Grade.OracleRef != "oracle@v1" {
			t.Errorf("Grade.OracleRef: got %v, want oracle@v1", got.Grade)
		}
		if got.MaxAttempts != d.MaxAttempts {
			t.Errorf("MaxAttempts: got %d, want %d", got.MaxAttempts, d.MaxAttempts)
		}
		if got.Attempts != d.Attempts {
			t.Errorf("Attempts: got %d, want %d", got.Attempts, d.Attempts)
		}
		if got.NotBefore != d.NotBefore {
			t.Errorf("NotBefore: got %v, want %v", got.NotBefore, d.NotBefore)
		}
	})
}
