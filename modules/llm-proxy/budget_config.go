package main

import (
	"strconv"
	"strings"
)

// configureBudget fills the Server's budget knobs from the environment via the
// injected getenv (so it is testable). Unset or invalid values keep the defaults
// already set by newServer.
func configureBudget(s *Server, getenv func(string) string) {
	// Wire the production ledger path (honors FLEET_USAGE_LEDGER); this is what
	// turns enforcement + metering on outside of tests.
	s.ledgerPath = usageLedgerPath()
	if v := getenv("LLM_PROXY_RESERVE_PCT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f < 1 {
			s.reservePct = f
		}
	}
	// Distinct providers from the routing table.
	seen := map[string]bool{}
	for _, rt := range s.routes {
		if seen[rt.provider] {
			continue
		}
		seen[rt.provider] = true
		key := strings.ToUpper(strings.ReplaceAll(rt.provider, "-", "_"))
		if v := getenv("LLM_PROXY_RESERVE_PCT_" + key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f < 1 {
				s.providerReserve[rt.provider] = f
			}
		}
		if v := getenv("LLM_PROXY_CEILING_PRIOR_" + key); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				s.priors[rt.provider] = n
			}
		}
	}
}
