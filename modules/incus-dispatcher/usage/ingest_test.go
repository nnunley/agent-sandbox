package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIngestTranscript_DedupsByTurnID(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "session.jsonl")
	// two assistant lines (distinct ids) + a noise line
	content := sampleTranscriptLine + "\n" +
		`{"type":"user","message":{"content":"hi"}}` + "\n" +
		`{"type":"assistant","timestamp":"2026-06-25T23:00:00.000Z","message":{"id":"msg_two","model":"claude-opus-4-8","usage":{"input_tokens":10,"output_tokens":20}}}` + "\n"
	if err := os.WriteFile(tr, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	l, _ := OpenLedger(filepath.Join(dir, "usage.jsonl"))

	added, skipped, err := IngestTranscript(l, tr)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if added != 2 || skipped != 0 {
		t.Fatalf("first ingest added=%d skipped=%d, want 2/0", added, skipped)
	}
	// Re-ingest the SAME file: everything is a duplicate now.
	added2, skipped2, _ := IngestTranscript(l, tr)
	if added2 != 0 || skipped2 != 2 {
		t.Fatalf("re-ingest added=%d skipped=%d, want 0/2 (de-dup)", added2, skipped2)
	}
	if len(l.Events()) != 2 {
		t.Fatalf("ledger has %d events, want 2", len(l.Events()))
	}
}
