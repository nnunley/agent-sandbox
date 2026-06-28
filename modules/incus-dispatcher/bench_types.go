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
