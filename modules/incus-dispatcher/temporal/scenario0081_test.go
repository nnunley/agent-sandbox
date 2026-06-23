package temporal

import (
	"sync"
	"testing"
	"time"
)

// TestScenario0081SoleWriterEnforcement validates sole writer enforcement for SCENARIO-0081.
// SCENARIO-0081: Single-Writer Invariant (STORY-0041, STORY-0046)
// Setup: Temporal projects (importance, deadline) → (EffectivePriority, NotBefore)
// - D1: Queue process reads EffectivePriority, forwards to laneq (should work)
// - D3: Queue tries to write EffectivePriority directly (should fail + error)
func TestScenario0081SoleWriterEnforcement(t *testing.T) {
	gd := NewGuardedDirective("D1", ImportanceHigh, nil)

	t.Logf("=== Sole Writer Enforcement ===")

	// Setup: Temporal writes initial priority
	err := gd.SetEffectivePriority(WriterRoleTemporal, 4000)
	if err != nil {
		t.Fatalf("Temporal write failed: %v", err)
	}
	t.Logf("✓ D1 (Temporal writes EffectivePriority): priority=4000")

	// D1: Queue reads EffectivePriority
	priority := gd.GetEffectivePriority()
	if priority != 4000 {
		t.Errorf("Queue read: got %d, want 4000", priority)
	}
	t.Logf("✓ D1 (Queue reads EffectivePriority): priority=%d", priority)

	// D3: Queue tries to write EffectivePriority (should fail)
	err = gd.SetEffectivePriority(WriterRoleQueue, 3500)
	if err == nil {
		t.Errorf("D3 (Queue writes EffectivePriority): err=nil, want error")
	}
	t.Logf("✓ D3 (Queue write rejected): %v", err)

	// Verify write was blocked (value unchanged)
	priority = gd.GetEffectivePriority()
	if priority != 4000 {
		t.Errorf("After rejected write: got %d, want 4000 (unchanged)", priority)
	}
}

// TestScenario0081ConcurrentReads validates that D2 concurrent readers don't corrupt state.
// D2: Another daemon reads EffectivePriority concurrently (no conflicts)
func TestScenario0081ConcurrentReads(t *testing.T) {
	gd := NewGuardedDirective("D2", ImportanceCritical, nil)

	// Temporal writes initial value
	err := gd.SetEffectivePriority(WriterRoleTemporal, 4500)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	t.Logf("=== Concurrent Reads (D2) ===")

	// D2: Multiple readers concurrently read EffectivePriority
	const numReaders = 10
	results := make([]int, numReaders)
	var wg sync.WaitGroup

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = gd.GetEffectivePriority()
		}(i)
	}

	wg.Wait()

	// Verify all reads got the same value
	for i, val := range results {
		if val != 4500 {
			t.Errorf("Reader %d: got %d, want 4500", i, val)
		}
	}
	t.Logf("✓ D2 (concurrent reads): all %d readers got consistent value 4500", numReaders)
}

// TestScenario0081UnauthorizedWriteRejected validates that non-Temporal writes are rejected.
// D3: Queue tries to write EffectivePriority directly (should fail + error)
func TestScenario0081UnauthorizedWriteRejected(t *testing.T) {
	gd := NewGuardedDirective("D3", ImportanceMedium, nil)

	t.Logf("=== Unauthorized Write Rejection (D3) ===")

	// D3: Queue tries to write EffectivePriority
	err := gd.SetEffectivePriority(WriterRoleQueue, 3000)
	if err == nil {
		t.Errorf("Queue write: err=nil, want error")
	}
	t.Logf("✓ D3 (Queue write rejected): %v", err)

	// D3 alternative: Human tries to write NotBefore
	future := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	err = gd.SetNotBefore(WriterRoleHuman, future)
	if err == nil {
		t.Errorf("Human write NotBefore: err=nil, want error")
	}
	t.Logf("✓ D3 (Human write NotBefore rejected): %v", err)
}

// TestScenario0081TemporalRescore validates D4: only Temporal can update priority fields.
// D4: Temporal rescores, re-projects EffectivePriority (only Temporal writes)
func TestScenario0081TemporalRescore(t *testing.T) {
	gd := NewGuardedDirective("D4", ImportanceHigh, nil)

	t.Logf("=== Temporal Rescore and Re-projection (D4) ===")

	// Initial projection: High importance + Q2 = priority 3000
	priority1 := ComputeEffectivePriority(ImportanceHigh, QuadrantQ2)
	err := gd.SetEffectivePriority(WriterRoleTemporal, priority1)
	if err != nil {
		t.Fatalf("Initial projection failed: %v", err)
	}
	t.Logf("Initial: priority=%d (High+Q2)", priority1)

	// Rescore: Item rescored to Critical + Q1 = priority 4000
	priority2 := ComputeEffectivePriority(ImportanceCritical, QuadrantQ1)
	err = gd.SetEffectivePriority(WriterRoleTemporal, priority2)
	if err != nil {
		t.Fatalf("Rescore projection failed: %v", err)
	}
	t.Logf("Rescore: priority=%d (Critical+Q1)", priority2)

	// Verify priority increased
	if priority2 <= priority1 {
		t.Errorf("Rescore should increase priority: was %d, now %d", priority1, priority2)
	}
	t.Logf("✓ D4 (Temporal rescore): priority increased from %d to %d", priority1, priority2)
}

// TestScenario0081ConcurrentReadersAndTemporalWrite validates D5 with realistic scenario:
// Multiple readers access EffectivePriority while Temporal occasionally updates it.
// D5: Multiple concurrent readers verify consistency
func TestScenario0081ConcurrentReadersAndTemporalWrite(t *testing.T) {
	gd := NewGuardedDirective("D5", ImportanceMedium, nil)

	// Initial write
	err := gd.SetEffectivePriority(WriterRoleTemporal, 3000)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	t.Logf("=== Concurrent Readers + Temporal Updates (D5) ===")

	// D5: Multiple concurrent readers verify consistency
	const numReaders = 5
	const numReads = 20
	readResults := make([]int, numReaders*numReads)
	var wg sync.WaitGroup
	var readIdx int
	var mu sync.Mutex

	// Launch reader goroutines
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for j := 0; j < numReads; j++ {
				val := gd.GetEffectivePriority()
				mu.Lock()
				readResults[readIdx] = val
				readIdx++
				mu.Unlock()
				time.Sleep(time.Millisecond) // Simulate processing
			}
		}(i)
	}

	// Temporal updates priority periodically
	wg.Add(1)
	go func() {
		defer wg.Done()
		values := []int{3000, 3100, 3200}
		for _, v := range values {
			time.Sleep(10 * time.Millisecond)
			err := gd.SetEffectivePriority(WriterRoleTemporal, v)
			if err != nil {
				t.Logf("Update error: %v", err)
			}
		}
	}()

	wg.Wait()

	// Verify all reads got valid values (one of the written values)
	validValues := map[int]bool{3000: true, 3100: true, 3200: true}
	for i, val := range readResults {
		if !validValues[val] {
			t.Errorf("Read %d: got %d, want one of 3000/3100/3200", i, val)
		}
	}
	t.Logf("✓ D5 (concurrent reads/writes): %d reads all consistent", numReaders*numReads)
}

// TestMultipleDirectivesIndependent validates that multiple directives
// enforce the invariant independently.
func TestMultipleDirectivesIndependent(t *testing.T) {
	gd1 := NewGuardedDirective("D1", ImportanceHigh, nil)
	gd2 := NewGuardedDirective("D2", ImportanceMedium, nil)

	t.Logf("=== Multiple Directives Independence ===")

	// Temporal writes to both
	err1 := gd1.SetEffectivePriority(WriterRoleTemporal, 4000)
	err2 := gd2.SetEffectivePriority(WriterRoleTemporal, 3000)
	if err1 != nil || err2 != nil {
		t.Fatalf("Setup failed: %v, %v", err1, err2)
	}

	// Queue tries to write to gd1 (should fail)
	err1 = gd1.SetEffectivePriority(WriterRoleQueue, 3500)
	if err1 == nil {
		t.Errorf("gd1 Queue write: err=nil, want error")
	}

	// gd2 should be unaffected
	val2 := gd2.GetEffectivePriority()
	if val2 != 3000 {
		t.Errorf("gd2 after gd1 rejection: got %d, want 3000", val2)
	}

	t.Logf("✓ Multiple directives: invariant enforced independently")
}
