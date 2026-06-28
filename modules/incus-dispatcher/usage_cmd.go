package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agent-sandbox/usage"
)

// defaultLedgerPath is where the usage ledger lives unless overridden by FLEET_USAGE_LEDGER.
func defaultLedgerPath() string {
	if p := os.Getenv("FLEET_USAGE_LEDGER"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fleet", "usage.jsonl")
}

// runUsageCommand implements `dispatcher usage [ingest <transcript>]`.
func runUsageCommand(args []string) int {
	path := defaultLedgerPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "usage:", err)
		return 1
	}
	l, err := usage.OpenLedger(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage:", err)
		return 1
	}
	if len(args) >= 2 && args[0] == "ingest" {
		added, skipped, err := usage.IngestTranscript(l, args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "usage ingest:", err)
			return 1
		}
		fmt.Printf("ingested %d new events (%d duplicates skipped)\n", added, skipped)
		return 0
	}
	usage.Report(l, usage.Estimator{}, time.Now(), os.Stdout)
	return 0
}
