package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// SCENARIO-0020 — Worker accesses provider through the broker proxy without ever holding a
// raw credential (STORY-0048). Rescoped for ITER-0002 to the achievable seam: a container/
// local worker (a plain HTTP client) against the real proxy. The microVM-specific host
// credential-socket isolation is proven later in ITER-0005; here we prove the broker
// contract: the worker sends NO key, the proxy is the sole credential holder and injects it
// upstream, any worker-supplied credential is stripped, and every request is audited.
func TestScenario0020_WorkerReachesProviderOnlyViaBroker(t *testing.T) {
	var upstreamAuth, upstreamXKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		upstreamXKey = r.Header.Get("x-api-key")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	const realKey = "sk-ant-master-secret"
	audit := &captureSink{}
	rt := mustRoute(t, "/anthropic", upstream.URL, realKey, "anthropic", true)
	srv := newServer([]route{rt}, audit)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// The worker is a plain client that holds NO provider credential and, defensively, even
	// tries to smuggle one. It reaches the provider ONLY through the broker surface.
	req, _ := http.NewRequest("POST", ts.URL+"/anthropic/v1/messages", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer worker-supplied-attacker-key")
	req.Header.Set("x-api-key", "worker-supplied-attacker-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("worker request via broker failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status via broker = %d, want 200", resp.StatusCode)
	}

	// Observable: the proxy is the sole credential holder — upstream sees the proxy's real
	// key, NOT the worker-supplied one (worker never injects credentials into the provider).
	if upstreamAuth != "Bearer "+realKey {
		t.Fatalf("upstream Authorization = %q, want the broker's real key (worker key leaked or absent)", upstreamAuth)
	}
	if upstreamXKey != realKey {
		t.Fatalf("upstream x-api-key = %q, want the broker's real key", upstreamXKey)
	}
	if strings.Contains(upstreamAuth, "worker-supplied-attacker-key") || upstreamXKey == "worker-supplied-attacker-key" {
		t.Fatalf("worker-supplied credential reached the provider — broker did not strip it")
	}

	// Observable: every brokered request is audited at the proxy level.
	last := audit.last()
	if last.Path == "" || last.Provider != "anthropic" || last.StatusCode != http.StatusOK {
		t.Fatalf("request not audited at proxy: %+v", last)
	}
}
