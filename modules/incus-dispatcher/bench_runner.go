package main

import "context"

type BenchRunner struct {
	Runner Runner
	DryRun bool
}

func (b BenchRunner) RunSuite(ctx context.Context, suite *BenchSuite, candidates []BenchCandidate) ([]BenchTaskResult, error) {
	results := make([]BenchTaskResult, 0, len(candidates)*len(suite.Tasks))
	for _, candidate := range candidates {
		for _, taskSpec := range suite.Tasks {
			task := Task{
				Name:                    taskSpec.Name,
				Repo:                    taskSpec.Repo,
				Ref:                     taskSpec.Ref,
				Cmd:                     []string{"bash", "-lc", taskSpec.Brief},
				ImageName:               DefaultImageName,
				Timeout:                 DefaultTimeout,
				Env:                     map[string]string{},
				Provider:                candidate.Provider,
				Model:                   candidate.Model,
				ExternalGradingCheckout: taskSpec.OracleRef,
			}
			if err := applyProviderRouting(&task); err != nil {
				return nil, err
			}
			result, err := b.Runner.Run(ctx, task)
			if err != nil {
				_ = b.Runner.Cleanup()
				results = append(results, BenchTaskResult{
					Candidate: candidate,
					TaskName:  taskSpec.Name,
					Status:    "error",
					Reason:    err.Error(),
				})
				continue
			}
			_ = b.Runner.Cleanup()
			results = append(results, benchTaskResultFromRun(candidate, taskSpec, result))
		}
	}
	return results, nil
}

func benchTaskResultFromRun(candidate BenchCandidate, taskSpec BenchTaskSpec, result *Result) BenchTaskResult {
	taskResult := BenchTaskResult{
		Candidate: candidate,
		TaskName:  taskSpec.Name,
		Status:    "passed",
	}
	if result == nil {
		taskResult.Status = "error"
		return taskResult
	}
	taskResult.Passed = passed(result, nil)
	if !taskResult.Passed {
		taskResult.Status = "failed"
	}
	taskResult.WallTimeMs = result.Duration.Milliseconds()
	if result.TokensIn > 0 || result.TokensOut > 0 || result.SpendUSD > 0 {
		taskResult.TokensByProvider = map[string]BenchTokenCost{
			string(candidate.Provider): {
				InputTokens:  result.TokensIn,
				OutputTokens: result.TokensOut,
				SpendUSD:     result.SpendUSD,
			},
		}
	}
	return taskResult
}
