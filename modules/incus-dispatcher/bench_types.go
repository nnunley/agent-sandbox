package main

// BenchSuite identifies a versioned benchmark suite and its loaded tasks.
type BenchSuite struct {
	Name    string
	Version string
	Hash    string
	Tasks   []BenchTaskSpec
}

// BenchTaskSpec is one visible bench task plus its hidden oracle checkout.
type BenchTaskSpec struct {
	Name      string
	Brief     string
	Repo      string
	Ref       string
	OracleRef string
}

type BenchCandidate struct {
	Name     string   `json:"name"`
	Provider Provider `json:"provider,omitempty"`
	Model    string   `json:"model,omitempty"`
}

type BenchTokenCost struct {
	InputTokens  int64   `json:"input_tokens,omitempty"`
	OutputTokens int64   `json:"output_tokens,omitempty"`
	SpendUSD     float64 `json:"spend_usd,omitempty"`
}

type BenchTaskResult struct {
	Candidate        BenchCandidate              `json:"candidate"`
	TaskName         string                      `json:"task_name"`
	Status           string                      `json:"status,omitempty"`
	Passed           bool                        `json:"passed"`
	JudgeScore       float64                     `json:"judge_score,omitempty"`
	WallTimeMs       int64                       `json:"wall_time_ms,omitempty"`
	TokensByProvider map[string]BenchTokenCost   `json:"tokens_by_provider,omitempty"`
	Reason           string                      `json:"reason,omitempty"`
}

type BenchSuiteRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

type BenchCandidateScore struct {
	Candidate        BenchCandidate             `json:"candidate"`
	TaskResults      []BenchTaskResult          `json:"task_results"`
	Passed           int                        `json:"passed"`
	Total            int                        `json:"total"`
	PassRate         float64                    `json:"pass_rate"`
	AverageJudge     float64                    `json:"average_judge,omitempty"`
	WallTimeMs       int64                      `json:"wall_time_ms"`
	TokensByProvider map[string]BenchTokenCost  `json:"tokens_by_provider,omitempty"`
}

type BenchRankingEntry struct {
	Rank      int                 `json:"rank"`
	Candidate BenchCandidate      `json:"candidate"`
	PassRate  float64             `json:"pass_rate"`
	Judge     float64             `json:"judge_score,omitempty"`
	WallTimeMs int64              `json:"wall_time_ms"`
}

type BenchScorecard struct {
	Suite      BenchSuiteRef         `json:"suite"`
	Candidates []BenchCandidateScore `json:"candidates"`
	Ranking    []BenchRankingEntry   `json:"ranking"`
}
