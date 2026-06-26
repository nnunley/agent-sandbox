package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ledgerLine is the discriminated JSONL record: exactly one of Usage/Limit is set.
type ledgerLine struct {
	Kind  string      `json:"kind"` // "usage" | "limit"
	Usage *UsageEvent `json:"usage,omitempty"`
	Limit *LimitEvent `json:"limit,omitempty"`
}

// Ledger is an append-only, fsync-durable JSONL store of usage and limit events.
// It reconstructs in-memory state on open and is safe for concurrent use.
type Ledger struct {
	mu     sync.Mutex
	path   string
	events []UsageEvent
	limits []LimitEvent
}

// OpenLedger opens (creating if absent) the ledger at path and reconstructs prior records.
// A malformed line is skipped with a stderr warning, never fatal.
func OpenLedger(path string) (*Ledger, error) {
	l := &Ledger{path: path}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var rec ledgerLine
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			fmt.Fprintf(os.Stderr, "usage ledger: skipping malformed line: %v\n", err)
			continue
		}
		switch rec.Kind {
		case "usage":
			if rec.Usage != nil {
				l.events = append(l.events, *rec.Usage)
			}
		case "limit":
			if rec.Limit != nil {
				l.limits = append(l.limits, *rec.Limit)
			}
		}
	}
	return l, sc.Err()
}

func (l *Ledger) appendLine(rec ledgerLine) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync() // durability: the line must reach disk before we return
}

// Append records a usage event durably and in memory.
func (l *Ledger) Append(e UsageEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.appendLine(ledgerLine{Kind: "usage", Usage: &e}); err != nil {
		return err
	}
	l.events = append(l.events, e)
	return nil
}

// AppendLimit records an exhaustion/limit event durably and in memory.
func (l *Ledger) AppendLimit(le LimitEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.appendLine(ledgerLine{Kind: "limit", Limit: &le}); err != nil {
		return err
	}
	l.limits = append(l.limits, le)
	return nil
}

// Events returns a defensive copy of all usage events in append order.
func (l *Ledger) Events() []UsageEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]UsageEvent, len(l.events))
	copy(out, l.events)
	return out
}

// Limits returns a defensive copy of all limit events in append order.
func (l *Ledger) Limits() []LimitEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]LimitEvent, len(l.limits))
	copy(out, l.limits)
	return out
}
