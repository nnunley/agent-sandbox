package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// runGradeCommand implements `incus-dispatcher grade` (STORY-0068 AC-1/AC-2):
// apply a worker diff to a clean checkout, run the oracle gates, and print the
// structured grade JSON {passed, clusterA, check_generated, untagged_fails, e2e}
// to stdout. The grader is the source of truth (anti-reward-hack, STORY-0072
// AC-2); the worker's own result.json is never consulted here.
//
// Usage:
//
//	incus-dispatcher grade --checkout <clean-repo> --diff <worker.diff> [--out grade.json]
func runGradeCommand(args []string) int {
	fs := flag.NewFlagSet("grade", flag.ContinueOnError)
	checkout := fs.String("checkout", "", "Path to a clean checkout to grade against (cloned, then patched)")
	diffPath := fs.String("diff", "", "Path to the worker's unified diff to apply before grading")
	outPath := fs.String("out", "", "Write grade JSON to this path (also printed to stdout)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *checkout == "" {
		fmt.Fprintln(os.Stderr, "grade: --checkout is required")
		return 2
	}

	var diff []byte
	if *diffPath != "" {
		b, err := os.ReadFile(*diffPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "grade: read diff: %v\n", err)
			return 2
		}
		diff = b
	}

	report, _, err := RunGrade(context.Background(), *checkout, diff, defaultGradeGates(), defaultGeneratedExcludes()...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grade: %v\n", err)
		return 1
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "grade: marshal: %v\n", err)
		return 1
	}
	fmt.Println(string(data))
	if *outPath != "" {
		if err := os.WriteFile(*outPath, append(data, '\n'), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "grade: write out: %v\n", err)
			return 1
		}
	}

	// Exit non-zero when the grade did not pass, so callers/CI can branch on it.
	if !report.Passed {
		return 1
	}
	return 0
}
