package main

import (
	"strings"
	"testing"
)

func TestBuildScorecard_RanksByPassRateThenJudgeThenWallTime(t *testing.T) {
	suite := &BenchSuite{Name: "fleet-core", Version: "v1", Hash: "abc123"}
	results := []BenchTaskResult{
		{
			Candidate: BenchCandidate{Name: "alpha"},
			TaskName:  "t1",
			Passed:    true,
			JudgeScore: 0.8,
			WallTimeMs: 1200,
			TokensByProvider: map[string]BenchTokenCost{
				"openai": {InputTokens: 10, OutputTokens: 20, SpendUSD: 0.12},
			},
		},
		{Candidate: BenchCandidate{Name: "alpha"}, TaskName: "t2", Passed: false, WallTimeMs: 1500},
		{Candidate: BenchCandidate{Name: "beta"}, TaskName: "t1", Passed: true, JudgeScore: 0.7, WallTimeMs: 1000},
		{Candidate: BenchCandidate{Name: "beta"}, TaskName: "t2", Passed: true, JudgeScore: 0.6, WallTimeMs: 1300},
	}

	card := BuildScorecard(suite, results)
	if got := card.Ranking[0].Candidate.Name; got != "beta" {
		t.Fatalf("top rank=%q want beta", got)
	}
	if card.Suite.Hash != "abc123" {
		t.Fatalf("suite hash=%q", card.Suite.Hash)
	}
	if len(card.Candidates) != 2 {
		t.Fatalf("candidates=%d want 2", len(card.Candidates))
	}
	if got := card.Candidates[0].TaskResults[0].TaskName; got == "" {
		t.Fatal("expected per-task results to be retained")
	}
}

func TestBenchScorecard_RenderIncludesSuiteAndProviderCost(t *testing.T) {
	card := BenchScorecard{
		Suite: BenchSuiteRef{Name: "fleet-core", Version: "v1", Hash: "abc123"},
		Candidates: []BenchCandidateScore{
			{
				Candidate:  BenchCandidate{Name: "beta", Provider: ProviderOpenAI, Model: "gpt-4o-mini"},
				PassRate:   1.0,
				WallTimeMs: 2300,
				TokensByProvider: map[string]BenchTokenCost{
					"openai": {InputTokens: 10, OutputTokens: 20, SpendUSD: 0.12},
				},
			},
		},
		Ranking: []BenchRankingEntry{{Rank: 1, Candidate: BenchCandidate{Name: "beta"}}},
	}

	table := card.RenderTable()
	if !strings.Contains(table, "fleet-core@v1") || !strings.Contains(table, "abc123") {
		t.Fatalf("table missing suite info:\n%s", table)
	}
	if !strings.Contains(table, "0.12") {
		t.Fatalf("table missing spend:\n%s", table)
	}
	if _, err := card.MarshalJSON(); err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
}
