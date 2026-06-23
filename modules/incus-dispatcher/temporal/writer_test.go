package temporal

import (
	"testing"
	"time"
)

// TestTemporalCanWriteEffectivePriority validates that Temporal can write EffectivePriority.
func TestTemporalCanWriteEffectivePriority(t *testing.T) {
	gd := NewGuardedDirective("D1", ImportanceHigh, nil)

	err := gd.SetEffectivePriority(WriterRoleTemporal, 4000)
	if err != nil {
		t.Errorf("SetEffectivePriority(Temporal): err=%v, want nil", err)
	}

	if got := gd.GetEffectivePriority(); got != 4000 {
		t.Errorf("GetEffectivePriority(): got %d, want 4000", got)
	}
	t.Logf("✓ Temporal can write EffectivePriority: 4000")
}

// TestQueueCannotWriteEffectivePriority validates that Queue cannot write EffectivePriority.
func TestQueueCannotWriteEffectivePriority(t *testing.T) {
	gd := NewGuardedDirective("D2", ImportanceMedium, nil)

	err := gd.SetEffectivePriority(WriterRoleQueue, 3000)
	if err == nil {
		t.Errorf("SetEffectivePriority(Queue): err=nil, want error")
	}

	t.Logf("✓ Queue cannot write EffectivePriority: %v", err)
}

// TestHumanCannotWriteEffectivePriority validates that Human cannot write EffectivePriority.
func TestHumanCannotWriteEffectivePriority(t *testing.T) {
	gd := NewGuardedDirective("D3", ImportanceLow, nil)

	err := gd.SetEffectivePriority(WriterRoleHuman, 2000)
	if err == nil {
		t.Errorf("SetEffectivePriority(Human): err=nil, want error")
	}

	t.Logf("✓ Human cannot write EffectivePriority: %v", err)
}

// TestTemporalCanWriteNotBefore validates that Temporal can write NotBefore.
func TestTemporalCanWriteNotBefore(t *testing.T) {
	gd := NewGuardedDirective("D4", ImportanceHigh, nil)
	future := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)

	err := gd.SetNotBefore(WriterRoleTemporal, future)
	if err != nil {
		t.Errorf("SetNotBefore(Temporal): err=%v, want nil", err)
	}

	if got := gd.GetNotBefore(); got != future {
		t.Errorf("GetNotBefore(): got %v, want %v", got, future)
	}
	t.Logf("✓ Temporal can write NotBefore: %s", future.Format("2006-01-02"))
}

// TestQueueCannotWriteNotBefore validates that Queue cannot write NotBefore.
func TestQueueCannotWriteNotBefore(t *testing.T) {
	gd := NewGuardedDirective("D5", ImportanceMedium, nil)
	future := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)

	err := gd.SetNotBefore(WriterRoleQueue, future)
	if err == nil {
		t.Errorf("SetNotBefore(Queue): err=nil, want error")
	}

	t.Logf("✓ Queue cannot write NotBefore: %v", err)
}

// TestHumanCannotWriteNotBefore validates that Human cannot write NotBefore.
func TestHumanCannotWriteNotBefore(t *testing.T) {
	gd := NewGuardedDirective("D6", ImportanceLow, nil)
	future := time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC)

	err := gd.SetNotBefore(WriterRoleHuman, future)
	if err == nil {
		t.Errorf("SetNotBefore(Human): err=nil, want error")
	}

	t.Logf("✓ Human cannot write NotBefore: %v", err)
}

// TestMultipleReadersCanReadEffectivePriority validates that multiple readers can safely read EffectivePriority.
func TestMultipleReadersCanReadEffectivePriority(t *testing.T) {
	gd := NewGuardedDirective("D7", ImportanceCritical, nil)

	// Temporal writes
	err := gd.SetEffectivePriority(WriterRoleTemporal, 4500)
	if err != nil {
		t.Fatalf("SetEffectivePriority(Temporal): err=%v", err)
	}

	// Multiple readers (Queue, Human) read the value concurrently
	// In a real system, these would run in separate goroutines,
	// but for this test we simulate sequential reads
	value1 := gd.GetEffectivePriority()
	value2 := gd.GetEffectivePriority()
	value3 := gd.GetEffectivePriority()

	if value1 != 4500 || value2 != 4500 || value3 != 4500 {
		t.Errorf("Multiple reads: got %d, %d, %d; want all 4500", value1, value2, value3)
	}

	t.Logf("✓ Multiple readers safely read EffectivePriority: all got 4500")
}

// TestGuardedDirectiveValidation validates the writer invariant.
func TestGuardedDirectiveValidation(t *testing.T) {
	gd := NewGuardedDirective("D8", ImportanceHigh, nil)

	// Set valid values via Temporal
	err1 := gd.SetEffectivePriority(WriterRoleTemporal, 4000)
	future := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	err2 := gd.SetNotBefore(WriterRoleTemporal, future)

	if err1 != nil || err2 != nil {
		t.Fatalf("Setup failed: %v, %v", err1, err2)
	}

	// Validate invariant (should pass for GuardedDirective)
	err := ValidateWriterInvariant(gd)
	if err != nil {
		t.Errorf("ValidateWriterInvariant: err=%v, want nil", err)
	}

	t.Logf("✓ GuardedDirective writer invariant validated")
}

// TestWriterUnauthorizedWriteRejected validates that unauthorized writes are rejected.
func TestWriterUnauthorizedWriteRejected(t *testing.T) {
	gd := NewGuardedDirective("D9", ImportanceMedium, nil)

	tests := []struct {
		name      string
		role      WriterRole
		operation func() error
	}{
		{
			"Queue writes Priority",
			WriterRoleQueue,
			func() error { return gd.SetEffectivePriority(WriterRoleQueue, 3000) },
		},
		{
			"Human writes Priority",
			WriterRoleHuman,
			func() error { return gd.SetEffectivePriority(WriterRoleHuman, 2000) },
		},
		{
			"Queue writes NotBefore",
			WriterRoleQueue,
			func() error { return gd.SetNotBefore(WriterRoleQueue, time.Now()) },
		},
		{
			"Human writes NotBefore",
			WriterRoleHuman,
			func() error { return gd.SetNotBefore(WriterRoleHuman, time.Now()) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.operation()
			if err == nil {
				t.Errorf("expected error, got nil")
			}
			t.Logf("Rejected: %v", err)
		})
	}
}

// TestWriterTemporalRescore simulates Temporal rescoring and re-projecting.
func TestWriterTemporalRescore(t *testing.T) {
	gd := NewGuardedDirective("D10", ImportanceHigh, nil)

	// Step 1: Temporal projects initial priority
	priority1 := ComputeEffectivePriority(ImportanceHigh, QuadrantQ2)
	err := gd.SetEffectivePriority(WriterRoleTemporal, priority1)
	if err != nil {
		t.Fatalf("Initial projection failed: %v", err)
	}
	t.Logf("Step 1 (initial projection): priority=%d (Q2)", priority1)

	// Step 2: Directive is rescored to Critical by approver
	// (Temporal updates the projection)
	priority2 := ComputeEffectivePriority(ImportanceCritical, QuadrantQ1)
	err = gd.SetEffectivePriority(WriterRoleTemporal, priority2)
	if err != nil {
		t.Fatalf("Rescore projection failed: %v", err)
	}
	t.Logf("Step 2 (rescore projection): priority=%d (Critical→Q1)", priority2)

	if priority2 <= priority1 {
		t.Errorf("Rescore should increase priority: was %d, now %d", priority1, priority2)
	}

	t.Logf("✓ Temporal rescore: consistent re-projection")
}
