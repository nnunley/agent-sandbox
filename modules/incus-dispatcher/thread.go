package main

import (
	"sort"
	"sync"
	"time"
)

// ResumeSummary is the soft progression hint carried across thread boundaries (STORY-0029 AC-1).
type ResumeSummary struct {
	PriorWork string `json:"prior_work"`
	NextStep  string `json:"next_step"`
}

// Thread is the continuity object for a unit of work (keyed by thread id = the Directive.ID
// lineage). It holds SOFT state only — never the authoritative diff/grade. Reuses ThreadStatus
// from threadstatus.go (do NOT redefine it).
type Thread struct {
	ID                string        `json:"thread_id"`
	Status            ThreadStatus  `json:"status"`
	CurrentBranch     string        `json:"current_branch"`
	CurrentWorkspace  string        `json:"current_workspace"`
	ResumeSummary     ResumeSummary `json:"resume_summary"`
	LastVerifiedState string        `json:"last_verified_state"`      // STORY-0029 AC-2
	OpenQuestions     []string      `json:"open_questions,omitempty"` // STORY-0029 AC-4a
	Supersedes        string        `json:"supersedes,omitempty"`     // STORY-0030 AC-1
	SupersededBy      string        `json:"superseded_by,omitempty"`  // STORY-0030 AC-1
	Deadline          *time.Time    `json:"deadline,omitempty"`       // preemptive ITER-0007; nil = none
	BudgetPolicy      *BudgetPolicy `json:"budget_policy,omitempty"`  // STORY-0036: multi-level budget enforcement
	Priority          int           `json:"priority"`                  // STORY-0037 AC-1: base priority for queue ordering (source of truth for sort order)
	AgingScore        float64       `json:"aging_score"`               // STORY-0037 AC-1: computed from elapsed time since last served
	LastServed        time.Time     `json:"last_served"`               // STORY-0037 AC-1: timestamp of last service (used for stale resurfacing)
	QueueClass        string        `json:"queue_class"`               // STORY-0037 AC-3: semantic label (urgent|active|incubating|maintenance) reflecting Priority intent; OrderThreads uses Priority, not QueueClass
}

// ThreadStore is a daemon-local, concurrency-safe registry of Threads keyed by Thread.ID.
// (Durable persistence is deferred to ITER-0006/0008 — in-memory is correct for now.)
type ThreadStore struct {
	mu      sync.Mutex
	threads map[string]Thread
}

// NewThreadStore returns an empty ThreadStore.
func NewThreadStore() *ThreadStore {
	return &ThreadStore{threads: make(map[string]Thread)}
}

// Put inserts or replaces the thread by t.ID.
func (s *ThreadStore) Put(t Thread) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.threads[t.ID] = t
}

// Get returns the thread with the given id. ok is false when absent.
func (s *ThreadStore) Get(id string) (Thread, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.threads[id]
	return t, ok
}

// ListAll returns all stored threads in a defensive copy (order unspecified).
func (s *ThreadStore) ListAll() []Thread {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Thread, 0, len(s.threads))
	for _, t := range s.threads {
		out = append(out, t)
	}
	return out
}

// MarkServed updates a thread's LastServed timestamp and resets its AgingScore to 0.
// Called when a thread is actually served (work completed). Uses an injected clock (now),
// not time.Now(), for deterministic testing. If the thread does not exist, this is a no-op.
// (STORY-0037 AC-4: ensures served threads become "fresh" and stop resurfacing).
func (s *ThreadStore) MarkServed(id string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.threads[id]
	if !ok {
		return // Thread does not exist; no-op.
	}
	t.LastServed = now
	t.AgingScore = 0 // Reset aging to mark thread as fresh.
	s.threads[id] = t
}

// AgingConfig encodes the stale-thread resurfacing policy (STORY-0037 AC-4).
// StaleThreshold is the duration after which a thread is considered stale and eligible for resurfacing.
type AgingConfig struct {
	StaleThreshold time.Duration
}

// ComputeEffectiveAgingScore computes the aging contribution to effective priority.
// Elapsed time is now - lastServed. Returns a non-negative score that increases with elapsed time.
// Formula: aging_score = min(1.0, elapsed_seconds / (staleThreshold_seconds / 2))
// This allows aging to boost priority up to 1.0 as the stale threshold approaches.
func ComputeEffectiveAgingScore(now, lastServed time.Time, staleThreshold time.Duration) float64 {
	if lastServed.IsZero() {
		// No prior service: treat as very old (maximum aging).
		return 1.0
	}
	elapsed := now.Sub(lastServed)
	if elapsed < 0 {
		// lastServed is in the future (clock skew); treat as not aged.
		return 0.0
	}
	// Aging increases proportionally to elapsed time, capped at 1.0 when half the stale threshold is reached.
	halfThreshold := staleThreshold / 2
	if halfThreshold <= 0 {
		return 0.0
	}
	score := float64(elapsed.Nanoseconds()) / float64(halfThreshold.Nanoseconds())
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// OrderThreads orders threads by effective priority (base priority + aging contribution)
// and applies stale-thread resurfacing to prevent starvation (STORY-0037 AC-2/AC-4).
// Returns a new slice ordered deterministically: higher effective priority first.
// Tie-breaks are stable by Thread ID (lexicographic).
func OrderThreads(threads []Thread, now time.Time, cfg *AgingConfig) []Thread {
	if cfg == nil {
		cfg = &AgingConfig{StaleThreshold: 7 * 24 * time.Hour}
	}

	// Create a copy for sorting.
	out := make([]Thread, len(threads))
	copy(out, threads)

	// Compute effective priority for each thread.
	type threadWithEffectivePriority struct {
		thread            Thread
		effectivePriority float64
		agingScore        float64
	}

	decorated := make([]threadWithEffectivePriority, len(out))
	for i, th := range out {
		agingScore := ComputeEffectiveAgingScore(now, th.LastServed, cfg.StaleThreshold)
		elapsed := now.Sub(th.LastServed)
		isStale := elapsed > cfg.StaleThreshold

		// Base priority as a float for mixed comparison.
		basePriority := float64(th.Priority)

		// Effective priority = base + aging contribution.
		// If stale, boost priority dramatically (e.g., add 100 so stale threads surface).
		effectivePriority := basePriority + agingScore
		if isStale {
			effectivePriority += 100.0 // Stale threads get a massive boost.
		}

		decorated[i] = threadWithEffectivePriority{
			thread:            th,
			effectivePriority: effectivePriority,
			agingScore:        agingScore,
		}
	}

	// Sort by effective priority (descending), then by ID (ascending) for stable tie-break.
	sort.SliceStable(decorated, func(i, j int) bool {
		if decorated[i].effectivePriority != decorated[j].effectivePriority {
			return decorated[i].effectivePriority > decorated[j].effectivePriority // Higher priority first
		}
		return decorated[i].thread.ID < decorated[j].thread.ID // Tie-break by ID
	})

	// Populate AgingScore in the result threads (computed from elapsed time).
	for i := range decorated {
		decorated[i].thread.AgingScore = decorated[i].agingScore
		out[i] = decorated[i].thread
	}

	return out
}
