package main

import "sort"

func BuildScorecard(suite *BenchSuite, results []BenchTaskResult) BenchScorecard {
	card := BenchScorecard{}
	if suite != nil {
		card.Suite = BenchSuiteRef{Name: suite.Name, Version: suite.Version, Hash: suite.Hash}
	}

	grouped := map[string]*BenchCandidateScore{}
	order := make([]string, 0)
	for _, result := range results {
		key := result.Candidate.Name
		score := grouped[key]
		if score == nil {
			score = &BenchCandidateScore{
				Candidate:        result.Candidate,
				TokensByProvider: map[string]BenchTokenCost{},
			}
			grouped[key] = score
			order = append(order, key)
		}
		score.TaskResults = append(score.TaskResults, result)
		score.Total++
		if result.Passed {
			score.Passed++
		}
		score.WallTimeMs += result.WallTimeMs
		for provider, cost := range result.TokensByProvider {
			current := score.TokensByProvider[provider]
			current.InputTokens += cost.InputTokens
			current.OutputTokens += cost.OutputTokens
			current.SpendUSD += cost.SpendUSD
			score.TokensByProvider[provider] = current
		}
		if result.JudgeScore > 0 {
			score.AverageJudge += result.JudgeScore
		}
	}

	card.Candidates = make([]BenchCandidateScore, 0, len(grouped))
	for _, key := range order {
		score := grouped[key]
		if score.Total > 0 {
			score.PassRate = float64(score.Passed) / float64(score.Total)
		}
		var judgeCount int
		for _, result := range score.TaskResults {
			if result.JudgeScore > 0 {
				judgeCount++
			}
		}
		if judgeCount > 0 {
			score.AverageJudge = score.AverageJudge / float64(judgeCount)
		}
		card.Candidates = append(card.Candidates, *score)
	}

	sort.Slice(card.Candidates, func(i, j int) bool {
		left := card.Candidates[i]
		right := card.Candidates[j]
		if left.PassRate != right.PassRate {
			return left.PassRate > right.PassRate
		}
		if left.AverageJudge != right.AverageJudge {
			return left.AverageJudge > right.AverageJudge
		}
		if left.WallTimeMs != right.WallTimeMs {
			return left.WallTimeMs < right.WallTimeMs
		}
		return left.Candidate.Name < right.Candidate.Name
	})

	card.Ranking = make([]BenchRankingEntry, 0, len(card.Candidates))
	for i, candidate := range card.Candidates {
		card.Ranking = append(card.Ranking, BenchRankingEntry{
			Rank:       i + 1,
			Candidate:  candidate.Candidate,
			PassRate:   candidate.PassRate,
			Judge:      candidate.AverageJudge,
			WallTimeMs: candidate.WallTimeMs,
		})
	}
	return card
}
