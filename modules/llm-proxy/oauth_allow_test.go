package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOAuthInjection(t *testing.T) {
	var gotAuth, gotXKey, gotBeta, gotVer string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotXKey = r.Header.Get("x-api-key")
		gotBeta = r.Header.Get("anthropic-beta")
		gotVer = r.Header.Get("anthropic-version")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rt := mustRoute(t, "/anthropic", upstream.URL, "oauth-tok", "anthropic", true)
	rt.oauth = true
	ts := httptest.NewServer(newServer([]route{rt}, &captureSink{}).Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/anthropic/v1/messages", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer worker-placeholder")
	req.Header.Set("x-api-key", "worker-junk")
	req.Header.Set("anthropic-beta", "fine-grained-tool-streaming-2025-05-14")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if gotAuth != "Bearer oauth-tok" {
		t.Errorf("Authorization = %q, want the proxy's OAuth token", gotAuth)
	}
	if gotXKey != "" {
		t.Errorf("x-api-key = %q, want none on an OAuth route", gotXKey)
	}
	if gotBeta != "fine-grained-tool-streaming-2025-05-14,"+anthropicOAuthBeta {
		t.Errorf("anthropic-beta = %q, want the client's betas plus %s", gotBeta, anthropicOAuthBeta)
	}
	if gotVer != "2023-06-01" {
		t.Errorf("anthropic-version = %q", gotVer)
	}
}

func TestOAuthBetaNotDuplicated(t *testing.T) {
	if got := withBeta("a, "+anthropicOAuthBeta, anthropicOAuthBeta); got != "a, "+anthropicOAuthBeta {
		t.Errorf("withBeta duplicated the flag: %q", got)
	}
	if got := withBeta("", anthropicOAuthBeta); got != anthropicOAuthBeta {
		t.Errorf("withBeta on empty = %q", got)
	}
}

func TestAnthropicSpecPrefersOAuth(t *testing.T) {
	s := anthropicSpec("sk-key", "oauth-tok")
	if !s.oauth || s.apiKey != "oauth-tok" {
		t.Errorf("with both set: oauth=%v key=%q, want the OAuth token", s.oauth, s.apiKey)
	}
	s = anthropicSpec("sk-key", "")
	if s.oauth || s.apiKey != "sk-key" {
		t.Errorf("API key only: oauth=%v key=%q", s.oauth, s.apiKey)
	}
	routes, err := buildRoutes([]routeSpec{anthropicSpec("", "oauth-tok")})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range routes {
		if !r.oauth {
			t.Errorf("route %s lost the OAuth mode", r.prefix)
		}
	}
}

func TestCallerAllowlist(t *testing.T) {
	hit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	sink := &captureSink{}
	srv := newServer([]route{mustRoute(t, "/anthropic", upstream.URL, "k", "anthropic", true)}, sink)
	_, only10, _ := net.ParseCIDR("10.0.0.0/8")
	srv.allow = []*net.IPNet{only10}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/anthropic/v1/messages")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status from a disallowed caller = %d, want 403", resp.StatusCode)
	}
	if hit {
		t.Error("a disallowed caller reached the upstream")
	}
	if e := sink.last(); e.StatusCode != http.StatusForbidden || !strings.Contains(e.Error, "caller not allowed") {
		t.Errorf("log entry = %+v, want a 403 caller-not-allowed record", e)
	}

	h, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(h.Body)
	h.Body.Close()
	if h.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Errorf("/health from a disallowed caller = %d %q, want 200", h.StatusCode, body)
	}
}

func TestDefaultAllowlistIsLoopback(t *testing.T) {
	srv := newServer(nil, &captureSink{})
	for ip, want := range map[string]bool{"127.0.0.1": true, "::1": true, "10.88.0.5": false, "192.168.86.20": false} {
		if got := srv.callerAllowed(net.JoinHostPort(ip, "1234")); got != want {
			t.Errorf("callerAllowed(%s) = %v, want %v", ip, got, want)
		}
	}
}

func TestParseAllow(t *testing.T) {
	_, eth0, _ := net.ParseCIDR("10.120.4.0/24")
	local := func() ([]*net.IPNet, error) { return []*net.IPNet{eth0}, nil }

	nets, err := parseAllow("10.88.0.0/24, local-subnets", local)
	if err != nil {
		t.Fatal(err)
	}
	srv := newServer(nil, &captureSink{})
	srv.allow = nets
	for ip, want := range map[string]bool{"10.88.0.7": true, "10.120.4.9": true, "10.120.5.9": false, "127.0.0.1": false} {
		if got := srv.callerAllowed(net.JoinHostPort(ip, "80")); got != want {
			t.Errorf("callerAllowed(%s) = %v, want %v", ip, got, want)
		}
	}

	for _, bad := range []string{"", " , ", "10.88.0.0", "nonsense/8"} {
		if _, err := parseAllow(bad, local); err == nil {
			t.Errorf("parseAllow(%q) succeeded, want an error", bad)
		}
	}
}
