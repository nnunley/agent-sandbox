package main

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// STORY-0029 AC-1: resume_summary JSON round-trip preserves prior_work and next_step.
func TestResumeSummaryJSONRoundTrip(t *testing.T) {
	rs := ResumeSummary{
		PriorWork: "implemented auth middleware",
		NextStep:  "write integration tests",
	}
	b, err := json.Marshal(rs)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Verify exact JSON tags.
	var raw map[string]string
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}
	if raw["prior_work"] != rs.PriorWork {
		t.Errorf("prior_work = %q, want %q", raw["prior_work"], rs.PriorWork)
	}
	if raw["next_step"] != rs.NextStep {
		t.Errorf("next_step = %q, want %q", raw["next_step"], rs.NextStep)
	}

	var got ResumeSummary
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != rs {
		t.Errorf("round-trip: got %+v, want %+v", got, rs)
	}
}

// STORY-0029 AC-1/AC-2: Thread JSON round-trip preserves all required fields with exact tags.
func TestThreadJSONRoundTrip(t *testing.T) {
	deadline := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	th := Thread{
		ID:               "thread-abc",
		Status:           StatusActive,
		CurrentBranch:    "feat/auth",
		CurrentWorkspace: "ws-1",
		ResumeSummary: ResumeSummary{
			PriorWork: "refactored handler",
			NextStep:  "add tests",
		},
		LastVerifiedState: "commit:abc123",
		Supersedes:        "thread-old",
		SupersededBy:      "thread-new",
		Deadline:          &deadline,
	}

	b, err := json.Marshal(th)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Verify exact JSON field names.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}
	for _, key := range []string{
		"thread_id", "status", "current_branch", "current_workspace",
		"resume_summary", "last_verified_state", "supersedes", "superseded_by", "deadline",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("JSON missing key %q", key)
		}
	}

	var got Thread
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ID != th.ID {
		t.Errorf("ID: got %q, want %q", got.ID, th.ID)
	}
	if got.Status != th.Status {
		t.Errorf("Status: got %q, want %q", got.Status, th.Status)
	}
	if got.CurrentBranch != th.CurrentBranch {
		t.Errorf("CurrentBranch: got %q, want %q", got.CurrentBranch, th.CurrentBranch)
	}
	if got.CurrentWorkspace != th.CurrentWorkspace {
		t.Errorf("CurrentWorkspace: got %q, want %q", got.CurrentWorkspace, th.CurrentWorkspace)
	}
	if got.ResumeSummary != th.ResumeSummary {
		t.Errorf("ResumeSummary: got %+v, want %+v", got.ResumeSummary, th.ResumeSummary)
	}
	if got.LastVerifiedState != th.LastVerifiedState {
		t.Errorf("LastVerifiedState: got %q, want %q", got.LastVerifiedState, th.LastVerifiedState)
	}
	if got.Supersedes != th.Supersedes {
		t.Errorf("Supersedes: got %q, want %q", got.Supersedes, th.Supersedes)
	}
	if got.SupersededBy != th.SupersededBy {
		t.Errorf("SupersededBy: got %q, want %q", got.SupersededBy, th.SupersededBy)
	}
	if got.Deadline == nil || !got.Deadline.Equal(*th.Deadline) {
		t.Errorf("Deadline: got %v, want %v", got.Deadline, th.Deadline)
	}
}

// STORY-0030 AC-1: supersedes/superseded_by are omitted from JSON when empty.
func TestThreadSupersedesOmitempty(t *testing.T) {
	th := Thread{
		ID:     "thread-xyz",
		Status: StatusQueued,
	}
	b, err := json.Marshal(th)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}
	if _, ok := raw["supersedes"]; ok {
		t.Error("supersedes should be omitted when empty")
	}
	if _, ok := raw["superseded_by"]; ok {
		t.Error("superseded_by should be omitted when empty")
	}
	if _, ok := raw["deadline"]; ok {
		t.Error("deadline should be omitted when nil")
	}

	// Round-trip with non-empty supersedes/superseded_by.
	th2 := Thread{
		ID:           "thread-next",
		Status:       StatusQueued,
		Supersedes:   "thread-xyz",
		SupersededBy: "thread-after",
	}
	b2, err := json.Marshal(th2)
	if err != nil {
		t.Fatalf("Marshal th2: %v", err)
	}
	var got2 Thread
	if err := json.Unmarshal(b2, &got2); err != nil {
		t.Fatalf("Unmarshal th2: %v", err)
	}
	if got2.Supersedes != th2.Supersedes {
		t.Errorf("Supersedes round-trip: got %q, want %q", got2.Supersedes, th2.Supersedes)
	}
	if got2.SupersededBy != th2.SupersededBy {
		t.Errorf("SupersededBy round-trip: got %q, want %q", got2.SupersededBy, th2.SupersededBy)
	}
}

// ThreadStore: Put then Get returns stored Thread + ok=true; unknown id → ok=false.
func TestThreadStorePutGet(t *testing.T) {
	s := NewThreadStore()

	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("Get on unknown id: want ok=false")
	}

	th := Thread{
		ID:     "thread-1",
		Status: StatusQueued,
		ResumeSummary: ResumeSummary{
			PriorWork: "setup",
			NextStep:  "run tests",
		},
	}
	s.Put(th)

	got, ok := s.Get("thread-1")
	if !ok {
		t.Fatal("Get after Put: want ok=true")
	}
	if got.ID != th.ID {
		t.Errorf("ID: got %q, want %q", got.ID, th.ID)
	}
	if got.ResumeSummary != th.ResumeSummary {
		t.Errorf("ResumeSummary: got %+v, want %+v", got.ResumeSummary, th.ResumeSummary)
	}

	// Replace (Put again with same ID).
	th.Status = StatusActive
	th.ResumeSummary.NextStep = "refactor"
	s.Put(th)
	got2, ok := s.Get("thread-1")
	if !ok {
		t.Fatal("Get after second Put: want ok=true")
	}
	if got2.Status != StatusActive {
		t.Errorf("Status after replace: got %q, want %q", got2.Status, StatusActive)
	}
	if got2.ResumeSummary.NextStep != "refactor" {
		t.Errorf("NextStep after replace: got %q, want %q", got2.ResumeSummary.NextStep, "refactor")
	}
}

// ThreadStore concurrency-safety: Puts/Gets from multiple goroutines must not race.
func TestThreadStoreConcurrency(t *testing.T) {
	s := NewThreadStore()
	const n = 100
	var wg sync.WaitGroup
	wg.Add(n * 2)

	for i := range n {
		go func(i int) {
			defer wg.Done()
			s.Put(Thread{ID: "thread-1", Status: StatusQueued})
			_ = i
		}(i)
		go func(i int) {
			defer wg.Done()
			s.Get("thread-1")
			_ = i
		}(i)
	}

	wg.Wait()
}
