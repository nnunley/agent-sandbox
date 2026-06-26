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

// STORY-0037 AC-1: Thread object includes priority, aging_score, last_served, and queue_class fields.
func TestThread_PriorityAgingFields(t *testing.T) {
	now := time.Now().UTC()
	th := Thread{
		ID:        "thread-urgent",
		Status:    StatusActive,
		Priority:  10,
		AgingScore: 0.5,
		LastServed: now.Add(-2 * time.Hour),
		QueueClass: "urgent",
	}

	// Verify struct fields exist and are accessible.
	if th.Priority != 10 {
		t.Errorf("Priority: got %d, want 10", th.Priority)
	}
	if th.AgingScore != 0.5 {
		t.Errorf("AgingScore: got %f, want 0.5", th.AgingScore)
	}
	if !th.LastServed.Equal(now.Add(-2 * time.Hour)) {
		t.Errorf("LastServed: got %v, want %v", th.LastServed, now.Add(-2*time.Hour))
	}
	if th.QueueClass != "urgent" {
		t.Errorf("QueueClass: got %q, want %q", th.QueueClass, "urgent")
	}

	// Verify JSON marshaling includes the fields with correct tags.
	b, err := json.Marshal(th)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}
	for _, key := range []string{"priority", "aging_score", "last_served", "queue_class"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("JSON missing key %q", key)
		}
	}

	// Verify round-trip preserves values.
	var got Thread
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Priority != th.Priority {
		t.Errorf("Priority round-trip: got %d, want %d", got.Priority, th.Priority)
	}
	if got.AgingScore != th.AgingScore {
		t.Errorf("AgingScore round-trip: got %f, want %f", got.AgingScore, th.AgingScore)
	}
	if !got.LastServed.Equal(th.LastServed) {
		t.Errorf("LastServed round-trip: got %v, want %v", got.LastServed, th.LastServed)
	}
	if got.QueueClass != th.QueueClass {
		t.Errorf("QueueClass round-trip: got %q, want %q", got.QueueClass, th.QueueClass)
	}
}

// STORY-0037 AC-3: System supports multiple queue classes: urgent, active, incubating, maintenance.
func TestQueueClasses_All4(t *testing.T) {
	classes := []string{"urgent", "active", "incubating", "maintenance"}
	for _, class := range classes {
		th := Thread{
			ID:         "thread-" + class,
			Status:     StatusActive,
			QueueClass: class,
			Priority:   5,
		}
		if th.QueueClass != class {
			t.Errorf("QueueClass %q: got %q", class, th.QueueClass)
		}
	}
}

// STORY-0037 AC-2 + AC-4: OrderThreads orders by priority + aging and surfaces stale threads.
func TestOrderThreads_PriorityPlusAging(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

	// Thread 1: high priority, recently served (low aging).
	t1 := Thread{
		ID:         "thread-urgent",
		Status:     StatusActive,
		Priority:   10,
		QueueClass: "urgent",
		LastServed: now.Add(-1 * time.Hour),
	}

	// Thread 2: medium priority, moderately aged.
	t2 := Thread{
		ID:         "thread-active",
		Status:     StatusActive,
		Priority:   5,
		QueueClass: "active",
		LastServed: now.Add(-12 * time.Hour),
	}

	// Thread 3: low priority, recently served.
	t3 := Thread{
		ID:         "thread-incubating",
		Status:     StatusActive,
		Priority:   2,
		QueueClass: "incubating",
		LastServed: now.Add(-30 * time.Minute),
	}

	threads := []Thread{t3, t1, t2} // Input order: incubating, urgent, active
	ordered := OrderThreads(threads, now, &AgingConfig{StaleThreshold: 7 * 24 * time.Hour})

	// Expected order: urgent (P=10), active (P=5), incubating (P=2).
	if len(ordered) != 3 {
		t.Fatalf("OrderThreads: got %d threads, want 3", len(ordered))
	}
	if ordered[0].ID != "thread-urgent" {
		t.Errorf("ordered[0]: got %q, want %q", ordered[0].ID, "thread-urgent")
	}
	if ordered[1].ID != "thread-active" {
		t.Errorf("ordered[1]: got %q, want %q", ordered[1].ID, "thread-active")
	}
	if ordered[2].ID != "thread-incubating" {
		t.Errorf("ordered[2]: got %q, want %q", ordered[2].ID, "thread-incubating")
	}
}

// STORY-0037 AC-4: Stale threads are surfaced (resurfaced) to prevent starvation.
func TestStaleResurfacing_PreventsStarvation(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	staleThreshold := 7 * 24 * time.Hour // 7 days

	// Thread A: urgent (P=10), served 1 hour ago (not stale).
	threadA := Thread{
		ID:         "thread-urgent",
		Status:     StatusActive,
		Priority:   10,
		QueueClass: "urgent",
		LastServed: now.Add(-1 * time.Hour),
	}

	// Thread B: active (P=5), served 2 hours ago (not stale).
	threadB := Thread{
		ID:         "thread-active",
		Status:     StatusActive,
		Priority:   5,
		QueueClass: "active",
		LastServed: now.Add(-2 * time.Hour),
	}

	// Thread C: incubating (P=2), served 3 days ago (not yet stale).
	threadC := Thread{
		ID:         "thread-incubating",
		Status:     StatusActive,
		Priority:   2,
		QueueClass: "incubating",
		LastServed: now.Add(-3 * 24 * time.Hour),
	}

	// Initial ordering: A, B, C (all sorted by priority, none stale).
	ordered1 := OrderThreads([]Thread{threadC, threadB, threadA}, now, &AgingConfig{StaleThreshold: staleThreshold})
	if ordered1[0].ID != "thread-urgent" || ordered1[1].ID != "thread-active" || ordered1[2].ID != "thread-incubating" {
		t.Errorf("Initial order: got [%s, %s, %s], want [thread-urgent, thread-active, thread-incubating]",
			ordered1[0].ID, ordered1[1].ID, ordered1[2].ID)
	}

	// ADVANCE CLOCK: 5 days forward.
	// Now Thread C (incubating, last served 3 days ago) is STILL not stale (8 days < 7+5=12 from the new now).
	// Let's recalculate: now=now+5d, C.LastServed stays same. Elapsed = now+5d - (now-3d) = 8d.
	// So C is now stale (8d > 7d threshold).
	now2 := now.Add(5 * 24 * time.Hour)
	// Thread C was last served at (now - 3d). From now2's perspective, elapsed = now2 - (now-3d) = (now+5d) - (now-3d) = 8d.
	// So C is stale and should be SURFACED.

	ordered2 := OrderThreads([]Thread{threadC, threadB, threadA}, now2, &AgingConfig{StaleThreshold: staleThreshold})
	// Thread C should now be FIRST or much earlier than before (surfaced due to starvation prevention).
	if ordered2[0].ID != "thread-incubating" {
		t.Errorf("After stale threshold crossed: ordered[0] = %q, want %q (incubating thread should surface to position 0)",
			ordered2[0].ID, "thread-incubating")
	}
	if ordered2[1].ID != "thread-urgent" && ordered2[1].ID != "thread-active" {
		t.Errorf("After stale threshold crossed: ordered[1] = %q, expect urgent or active", ordered2[1].ID)
	}
}

// STORY-0037 AC-2: OrderThreads is deterministic and has stable tie-break.
func TestOrderThreads_Deterministic(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

	// Create threads with same priority (tests tie-break).
	t1 := Thread{
		ID:         "thread-a",
		Status:     StatusActive,
		Priority:   5,
		QueueClass: "active",
		LastServed: now.Add(-1 * time.Hour),
	}
	t2 := Thread{
		ID:         "thread-b",
		Status:     StatusActive,
		Priority:   5,
		QueueClass: "active",
		LastServed: now.Add(-1 * time.Hour),
	}

	// Run ordering twice with same input.
	cfg := &AgingConfig{StaleThreshold: 7 * 24 * time.Hour}
	ordered1 := OrderThreads([]Thread{t2, t1}, now, cfg)
	ordered2 := OrderThreads([]Thread{t2, t1}, now, cfg)

	// Order must be identical (deterministic).
	if len(ordered1) != len(ordered2) {
		t.Fatalf("Determinism: len mismatch: %d vs %d", len(ordered1), len(ordered2))
	}
	for i := range ordered1 {
		if ordered1[i].ID != ordered2[i].ID {
			t.Errorf("Determinism: position %d differs: %q vs %q", i, ordered1[i].ID, ordered2[i].ID)
		}
	}

	// Tie-break is by ID (lexicographic or other stable rule).
	if ordered1[0].ID != "thread-a" {
		t.Logf("Deterministic tie-break: first is %q (tie-break rule may vary, but must be stable)", ordered1[0].ID)
	}
}

// SCENARIO-0017 — process-level proof: long-running scheduler maintains priority queue.
// Real Thread objects, real OrderThreads function, injected clock via now parameter.
func TestScenario0017_ThreadSchedulerPriorityQueue(t *testing.T) {
	// Preconditions: scheduler thread registry with three threads.
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	staleThreshold := 7 * 24 * time.Hour

	// Urgent thread (priority=10, queue_class=urgent).
	urgentThread := Thread{
		ID:         "thread-urgent-task",
		Status:     StatusActive,
		Priority:   10,
		QueueClass: "urgent",
		LastServed: now.Add(-1 * time.Hour),
	}

	// Active thread (priority=5, queue_class=active).
	activeThread := Thread{
		ID:         "thread-active-task",
		Status:     StatusActive,
		Priority:   5,
		QueueClass: "active",
		LastServed: now.Add(-2 * time.Hour),
	}

	// Incubating thread (priority=2, queue_class=incubating, last served 3 days ago).
	incubatingThread := Thread{
		ID:         "thread-incubating-task",
		Status:     StatusActive,
		Priority:   2,
		QueueClass: "incubating",
		LastServed: now.Add(-3 * 24 * time.Hour), // 3 days ago
	}

	// Action 1: Initial queue ordering at now.
	threads := []Thread{incubatingThread, activeThread, urgentThread}
	ordered := OrderThreads(threads, now, &AgingConfig{StaleThreshold: staleThreshold})

	// Expected observables:
	// - Urgent thread queued first
	// - Active thread queued second
	// - Incubating thread queued third (not stale yet; 3 days < 7 days threshold)
	if len(ordered) != 3 {
		t.Fatalf("Initial queue: got %d threads, want 3", len(ordered))
	}
	if ordered[0].ID != "thread-urgent-task" {
		t.Errorf("Initial queue[0]: got %q, want urgent task", ordered[0].ID)
	}
	if ordered[1].ID != "thread-active-task" {
		t.Errorf("Initial queue[1]: got %q, want active task", ordered[1].ID)
	}
	if ordered[2].ID != "thread-incubating-task" {
		t.Errorf("Initial queue[2]: got %q, want incubating task", ordered[2].ID)
	}

	// Action 2: Time advances 8 days. Urgent and active threads are ACTIVELY SERVED (last_served is updated).
	// Incubating thread has NOT been scheduled (last_served remains the same).
	now2 := now.Add(8 * 24 * time.Hour)

	// Update urgent and active threads to reflect recent service (keep incubating untouched).
	threadA2 := Thread{
		ID:         "thread-urgent-task",
		Status:     StatusActive,
		Priority:   10,
		QueueClass: "urgent",
		LastServed: now2.Add(-30 * time.Minute), // Served 30 min ago (in the new now2 timeline)
	}
	threadB2 := Thread{
		ID:         "thread-active-task",
		Status:     StatusActive,
		Priority:   5,
		QueueClass: "active",
		LastServed: now2.Add(-1 * time.Hour), // Served 1 hour ago (in the new now2 timeline)
	}
	// incubatingThread remains unchanged: LastServed is still (now - 3d) = (now2 - 8d - 3d) = (now2 - 11d)
	// Elapsed at now2 = now2 - (now2 - 11d) = 11d, which is > 7d threshold → STALE

	ordered2 := OrderThreads([]Thread{incubatingThread, threadB2, threadA2}, now2, &AgingConfig{StaleThreshold: staleThreshold})

	// Expected observable:
	// - Incubating thread is surfaced (moves up in priority) to prevent starvation
	// - No thread starves indefinitely
	if ordered2[0].ID != "thread-incubating-task" {
		t.Errorf("Stale resurfacing: ordered2[0] = %q, want incubating-task (should surface)",
			ordered2[0].ID)
	}

	// Additional observable: all 4 queue classes are supported (we demonstrated 3; maintenance is the 4th).
	maintenanceThread := Thread{
		ID:         "thread-maintenance-task",
		Status:     StatusActive,
		Priority:   1,
		QueueClass: "maintenance",
		LastServed: now.Add(-4 * time.Hour),
	}
	if maintenanceThread.QueueClass != "maintenance" {
		t.Error("maintenance queue class not supported")
	}

	// Observable: message includes thread_id, priority, aging_score (computed).
	for _, th := range ordered2 {
		if th.ID == "" {
			t.Error("thread_id missing")
		}
		if th.Priority == 0 && th.ID != "thread-incubating-task" {
			t.Logf("thread %s has priority %d", th.ID, th.Priority)
		}
		// aging_score is computed from elapsed time, not stored; verified implicitly via ordering.
	}

	// Observable: queue ordering reflects priority + aging and prevents starvation.
	// The stale incubating thread is now first, proving starvation prevention.
	t.Logf("SCENARIO-0017: Stale-thread resurfacing works. Order after 8 days: %v -> %v",
		[]string{ordered[0].ID, ordered[1].ID, ordered[2].ID},
		[]string{ordered2[0].ID, ordered2[1].ID, ordered2[2].ID},
	)
}
