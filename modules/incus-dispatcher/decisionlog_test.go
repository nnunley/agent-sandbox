package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// compile-time interface check (AC-4)
var (
	_ DecisionLog = (*JSONLDecisionLog)(nil)
	_ DecisionLog = (*MemoryDecisionLog)(nil)
)

var fixedNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// TestJSONLDecisionLog_OneLinePerAppend verifies AC-1: each Append emits exactly
// one line, the line ends with "\n", and prior lines are never rewritten.
func TestJSONLDecisionLog_OneLinePerAppend(t *testing.T) {
	var buf bytes.Buffer
	dl := NewJSONLDecisionLog(&buf, func() time.Time { return fixedNow })

	d1 := Decision{DirectiveID: "d1", Grade: "pass", Rule: "r1", Action: "done", Ts: fixedNow}
	if err := dl.Append(d1); err != nil {
		t.Fatalf("first Append: %v", err)
	}
	afterFirst := buf.String()
	if !strings.HasSuffix(afterFirst, "\n") {
		t.Fatalf("first line does not end with newline: %q", afterFirst)
	}
	if lines := strings.Split(strings.TrimSuffix(afterFirst, "\n"), "\n"); len(lines) != 1 {
		t.Fatalf("after 1 Append: got %d lines, want 1: %q", len(lines), afterFirst)
	}

	d2 := Decision{DirectiveID: "d2", Grade: "fail", Rule: "r2", Action: "requeue", Ts: fixedNow}
	if err := dl.Append(d2); err != nil {
		t.Fatalf("second Append: %v", err)
	}
	afterSecond := buf.String()

	// append-only: the first line must survive intact (AC-1)
	if !strings.HasPrefix(afterSecond, afterFirst) {
		t.Fatalf("prior line was rewritten: before=%q after=%q", afterFirst, afterSecond)
	}
	if lines := strings.Split(strings.TrimSuffix(afterSecond, "\n"), "\n"); len(lines) != 2 {
		t.Fatalf("after 2 Appends: got %d lines, want 2", len(lines))
	}
}

// TestJSONLDecisionLog_OrderPreserved verifies AC-1: lines appear in Append order.
func TestJSONLDecisionLog_OrderPreserved(t *testing.T) {
	var buf bytes.Buffer
	dl := NewJSONLDecisionLog(&buf, func() time.Time { return fixedNow })

	ids := []string{"first", "second", "third"}
	for _, id := range ids {
		if err := dl.Append(Decision{DirectiveID: id, Ts: fixedNow}); err != nil {
			t.Fatalf("Append %q: %v", id, err)
		}
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != len(ids) {
		t.Fatalf("got %d lines, want %d", len(lines), len(ids))
	}
	for i, line := range lines {
		var d Decision
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatalf("line %d not valid JSON: %v — %q", i, err, line)
		}
		if d.DirectiveID != ids[i] {
			t.Errorf("line %d: DirectiveID=%q, want %q", i, d.DirectiveID, ids[i])
		}
	}
}

// TestJSONLDecisionLog_ZeroTsStamped verifies AC-2: a zero Ts is replaced by now().
func TestJSONLDecisionLog_ZeroTsStamped(t *testing.T) {
	stamp := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	calls := 0
	var buf bytes.Buffer
	dl := NewJSONLDecisionLog(&buf, func() time.Time {
		calls++
		return stamp
	})

	if err := dl.Append(Decision{DirectiveID: "x"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if calls != 1 {
		t.Fatalf("now() called %d times, want 1", calls)
	}

	var d Decision
	if err := json.Unmarshal([]byte(strings.TrimSuffix(buf.String(), "\n")), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !d.Ts.Equal(stamp) {
		t.Errorf("Ts=%v, want %v (zero Ts must be stamped with now())", d.Ts, stamp)
	}
}

// TestJSONLDecisionLog_NonZeroTsPreserved verifies AC-2: a non-zero Ts is not overwritten.
func TestJSONLDecisionLog_NonZeroTsPreserved(t *testing.T) {
	explicit := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	wrong := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var buf bytes.Buffer
	dl := NewJSONLDecisionLog(&buf, func() time.Time { return wrong })

	if err := dl.Append(Decision{DirectiveID: "x", Ts: explicit}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	var d Decision
	if err := json.Unmarshal([]byte(strings.TrimSuffix(buf.String(), "\n")), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !d.Ts.Equal(explicit) {
		t.Errorf("Ts=%v, want %v (non-zero Ts must not be overwritten)", d.Ts, explicit)
	}
}

// TestJSONLDecisionLog_AllFields verifies AC-2: all Decision fields round-trip through JSON.
func TestJSONLDecisionLog_AllFields(t *testing.T) {
	var buf bytes.Buffer
	dl := NewJSONLDecisionLog(&buf, func() time.Time { return fixedNow })

	want := Decision{
		DirectiveID: "dir-42",
		Grade:       "pass:score=90",
		Rule:        "retry-on-fail",
		Action:      "requeue",
		Ts:          fixedNow,
	}
	if err := dl.Append(want); err != nil {
		t.Fatalf("Append: %v", err)
	}

	var got Decision
	if err := json.Unmarshal([]byte(strings.TrimSuffix(buf.String(), "\n")), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.DirectiveID != want.DirectiveID {
		t.Errorf("DirectiveID=%q, want %q", got.DirectiveID, want.DirectiveID)
	}
	if got.Grade != want.Grade {
		t.Errorf("Grade=%q, want %q", got.Grade, want.Grade)
	}
	if got.Rule != want.Rule {
		t.Errorf("Rule=%q, want %q", got.Rule, want.Rule)
	}
	if got.Action != want.Action {
		t.Errorf("Action=%q, want %q", got.Action, want.Action)
	}
	if !got.Ts.Equal(want.Ts) {
		t.Errorf("Ts=%v, want %v", got.Ts, want.Ts)
	}
}

// TestMemoryDecisionLog_RecordsInOrder verifies AC-4: Records() returns all
// appended Decisions in the order they were appended.
func TestMemoryDecisionLog_RecordsInOrder(t *testing.T) {
	dl := NewMemoryDecisionLog()

	want := []Decision{
		{DirectiveID: "a", Action: "done"},
		{DirectiveID: "b", Action: "requeue"},
		{DirectiveID: "c", Action: "park"},
	}
	for _, d := range want {
		if err := dl.Append(d); err != nil {
			t.Fatalf("Append %q: %v", d.DirectiveID, err)
		}
	}

	got := dl.Records()
	if len(got) != len(want) {
		t.Fatalf("Records() len=%d, want %d", len(got), len(want))
	}
	for i, d := range got {
		if d.DirectiveID != want[i].DirectiveID {
			t.Errorf("Records()[%d].DirectiveID=%q, want %q", i, d.DirectiveID, want[i].DirectiveID)
		}
		if d.Action != want[i].Action {
			t.Errorf("Records()[%d].Action=%q, want %q", i, d.Action, want[i].Action)
		}
	}
}

// TestMemoryDecisionLog_EmptyRecords verifies an empty log returns nil/empty slice.
func TestMemoryDecisionLog_EmptyRecords(t *testing.T) {
	dl := NewMemoryDecisionLog()
	if got := dl.Records(); len(got) != 0 {
		t.Fatalf("Records() on empty log = %v, want empty", got)
	}
}
