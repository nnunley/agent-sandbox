package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type benchManifest struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Tasks   []string `json:"tasks"`
}

type benchTaskMeta struct {
	Repo      string `json:"repo"`
	Ref       string `json:"ref"`
	OracleRef string `json:"oracle_ref"`
}

// LoadBenchSuite reads suites/<name>/<version>/ and returns a deterministic in-memory suite.
func LoadBenchSuite(root, suiteID string) (*BenchSuite, error) {
	name, version, err := parseSuiteID(suiteID)
	if err != nil {
		return nil, err
	}

	base := filepath.Join(root, "suites", name, version)
	base, err = filepath.Abs(base)
	if err != nil {
		return nil, fmt.Errorf("resolve suite path: %w", err)
	}
	manifestPath := filepath.Join(base, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest benchManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.Name == "" {
		manifest.Name = name
	}
	if manifest.Version == "" {
		manifest.Version = version
	}

	hasher := sha256.New()
	hasher.Write(manifestData)
	suite := &BenchSuite{
		Name:    manifest.Name,
		Version: manifest.Version,
		Tasks:   make([]BenchTaskSpec, 0, len(manifest.Tasks)),
	}

	for _, taskName := range manifest.Tasks {
		taskDir := filepath.Join(base, "tasks", taskName)
		briefData, err := os.ReadFile(filepath.Join(taskDir, "brief.txt"))
		if err != nil {
			return nil, fmt.Errorf("read brief %s: %w", taskName, err)
		}
		metaData, err := os.ReadFile(filepath.Join(taskDir, "meta.json"))
		if err != nil {
			return nil, fmt.Errorf("read meta %s: %w", taskName, err)
		}
		var meta benchTaskMeta
		if err := json.Unmarshal(metaData, &meta); err != nil {
			return nil, fmt.Errorf("parse meta %s: %w", taskName, err)
		}
		hasher.Write([]byte(taskName))
		hasher.Write(briefData)
		hasher.Write(metaData)
		suite.Tasks = append(suite.Tasks, BenchTaskSpec{
			Name:      taskName,
			Brief:     strings.TrimRight(string(briefData), "\n"),
			Repo:      resolveSuitePath(base, meta.Repo),
			Ref:       meta.Ref,
			OracleRef: resolveSuitePath(base, meta.OracleRef),
		})
	}

	suite.Hash = hex.EncodeToString(hasher.Sum(nil))
	return suite, nil
}

func resolveSuitePath(base, value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(filepath.Join(base, value))
}

func parseSuiteID(suiteID string) (name, version string, err error) {
	parts := strings.SplitN(suiteID, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid suite %q: want name@version", suiteID)
	}
	return parts[0], parts[1], nil
}
