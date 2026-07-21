package usage

import (
	"bufio"
	"os"
)

// IngestTranscript appends usage events from a Claude Code transcript file to the ledger,
// skipping any whose (Source,TurnID) is already present (idempotent re-ingest).
func IngestTranscript(l *Ledger, transcriptPath string) (added, skipped int, err error) {
	seen := make(map[string]bool)
	for _, e := range l.Events() {
		if e.TurnID != "" {
			seen[string(e.Source)+"\x00"+e.TurnID] = true
		}
	}
	f, err := os.Open(transcriptPath)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		ev, ok := ParseClaudeTranscriptLine(sc.Bytes())
		if !ok {
			continue
		}
		key := string(ev.Source) + "\x00" + ev.TurnID
		if ev.TurnID != "" && seen[key] {
			skipped++
			continue
		}
		if err := l.Append(ev); err != nil {
			return added, skipped, err
		}
		seen[key] = true
		added++
	}
	return added, skipped, sc.Err()
}
