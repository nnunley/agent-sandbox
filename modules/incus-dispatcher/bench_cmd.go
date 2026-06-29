package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type benchCommandDeps struct {
	stdout    io.Writer
	stderr    io.Writer
	newRunner func(runnerKind, remote string) (Runner, error)
}

type stringListFlag []string

func (s *stringListFlag) String() string { return strings.Join(*s, ",") }

func (s *stringListFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func runBenchCommand(args []string) int {
	return runBenchCommandWithDeps(args, defaultBenchCommandDeps())
}

func defaultBenchCommandDeps() benchCommandDeps {
	return benchCommandDeps{
		stdout: os.Stdout,
		stderr: os.Stderr,
		newRunner: func(runnerKind, remote string) (Runner, error) {
			switch runnerKind {
			case "client":
				return NewClientContainerRunner(remote)
			case "cli":
				return NewCLIContainerRunner(remote)
			default:
				return nil, fmt.Errorf("invalid runner: %s", runnerKind)
			}
		},
	}
}

func runBenchCommandWithDeps(args []string, deps benchCommandDeps) int {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	fs.SetOutput(deps.stderr)
	suiteID := fs.String("suite", "", "Suite identifier in name@version form")
	suiteRoot := fs.String("suite-root", ".", "Root containing the suites/ tree")
	outPath := fs.String("out", "", "Write scorecard JSON to this path")
	runnerKind := fs.String("runner", "client", "Runner implementation: client or cli")
	remote := fs.String("remote", DefaultRemote, "Incus remote name")
	dryRun := fs.Bool("dry-run", false, "Use the injected runner path without live model dispatch")
	var candidateFlags stringListFlag
	fs.Var(&candidateFlags, "candidate", "Candidate in name=provider:model form (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *suiteID == "" {
		fmt.Fprintln(deps.stderr, "bench: --suite is required")
		return 2
	}
	if len(candidateFlags) == 0 {
		fmt.Fprintln(deps.stderr, "bench: at least one --candidate is required")
		return 2
	}

	suite, err := LoadBenchSuite(*suiteRoot, *suiteID)
	if err != nil {
		fmt.Fprintf(deps.stderr, "bench: load suite: %v\n", err)
		return 1
	}
	candidates, err := parseBenchCandidates(candidateFlags)
	if err != nil {
		fmt.Fprintf(deps.stderr, "bench: parse candidates: %v\n", err)
		return 2
	}
	runner, err := deps.newRunner(*runnerKind, *remote)
	if err != nil {
		fmt.Fprintf(deps.stderr, "bench: runner: %v\n", err)
		return 1
	}

	bench := BenchRunner{Runner: runner, DryRun: *dryRun}
	results, err := bench.RunSuite(contextBackground(), suite, candidates)
	if err != nil {
		fmt.Fprintf(deps.stderr, "bench: run suite: %v\n", err)
		return 1
	}

	card := BuildScorecard(suite, results)
	fmt.Fprintln(deps.stdout, card.RenderTable())
	if *outPath != "" {
		data, err := card.MarshalJSON()
		if err != nil {
			fmt.Fprintf(deps.stderr, "bench: marshal scorecard: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*outPath, append(data, '\n'), 0644); err != nil {
			fmt.Fprintf(deps.stderr, "bench: write out: %v\n", err)
			return 1
		}
	}
	return 0
}

func parseBenchCandidates(values []string) ([]BenchCandidate, error) {
	candidates := make([]BenchCandidate, 0, len(values))
	for _, value := range values {
		nameModel := strings.SplitN(value, "=", 2)
		if len(nameModel) != 2 || nameModel[0] == "" {
			return nil, fmt.Errorf("invalid candidate %q", value)
		}
		providerModel := strings.SplitN(nameModel[1], ":", 2)
		if len(providerModel) != 2 || providerModel[0] == "" || providerModel[1] == "" {
			return nil, fmt.Errorf("invalid candidate %q", value)
		}
		provider := Provider(providerModel[0])
		if err := provider.ValidateProvider(); err != nil {
			return nil, err
		}
		candidates = append(candidates, BenchCandidate{
			Name:     nameModel[0],
			Provider: provider,
			Model:    providerModel[1],
		})
	}
	return candidates, nil
}

var contextBackground = func() context.Context {
	return context.Background()
}
