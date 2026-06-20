package main

import (
	"sort"
	"strings"
)

// RawProviderKeyNames is the set of environment variable names that carry long-lived
// provider secrets which must NEVER be delivered to a worker. Workers reach providers
// only via the broker proxy (D2: the proxy is the sole credential holder).
var RawProviderKeyNames = map[string]bool{
	"ANTHROPIC_API_KEY":    true,
	"ANTHROPIC_AUTH_TOKEN": true,
	"OPENAI_API_KEY":       true,
	"OPENAI_KEY":           true,
	"OLLAMA_API_KEY":       true,
}

// looksLikeCredential reports whether an env var name appears to carry a secret.
// Fail-closed: workers reach providers only via the broker proxy, so anything that
// looks like a credential is withheld even if it is not on the explicit list.
func looksLikeCredential(name string) bool {
	upper := strings.ToUpper(name)
	return strings.HasSuffix(upper, "_API_KEY") ||
		strings.HasSuffix(upper, "_KEY") ||
		strings.HasSuffix(upper, "_TOKEN") ||
		strings.HasSuffix(upper, "_SECRET") ||
		strings.Contains(upper, "PASSWORD")
}

// SanitizeWorkerEnv returns a COPY of env with every raw provider credential removed,
// plus the sorted list of stripped variable names (for audit logging). Matching is
// case-INSENSITIVE on the variable name (ANTHROPIC_API_KEY and anthropic_api_key are
// both stripped). The input map is never mutated. A nil/empty map yields an empty
// (non-nil) map and a nil/empty stripped slice.
//
// A variable is stripped if EITHER its upper-case name is in RawProviderKeyNames
// (explicit blocklist) OR it matches a credential-like pattern via looksLikeCredential
// (fail-closed net for providers not yet on the explicit list).
func SanitizeWorkerEnv(env map[string]string) (clean map[string]string, stripped []string) {
	clean = make(map[string]string)
	for k, v := range env {
		if RawProviderKeyNames[strings.ToUpper(k)] || looksLikeCredential(k) {
			stripped = append(stripped, k)
		} else {
			clean[k] = v
		}
	}
	sort.Strings(stripped)
	return clean, stripped
}
