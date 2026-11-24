package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func cidrs(t *testing.T, list ...string) []*net.IPNet {
	t.Helper()
	var out []*net.IPNet
	for _, c := range list {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

func TestAnthropicPathScope(t *testing.T) {
	var hits []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	routes, err := buildRoutes([]routeSpec{
		{prefix: "/anthropic", upstream: upstream.URL, apiKey: "tok", provider: "anthropic", requiresKey: true, oauth: true},
		{prefix: "/openai", upstream: upstream.URL, apiKey: "k", provider: "openai", requiresKey: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(newServer(routes, &captureSink{}).Handler())
	defer ts.Close()

	cases := map[string]int{
		"/anthropic/v1/messages":              http.StatusOK,
		"/anthropic/v1/messages/count_tokens": http.StatusOK,
		"/anthropic/v1/models":                http.StatusOK,
		"/anthropic/v1/models/claude-x":       http.StatusOK,
		"/anthropic/v1/organizations/me":      http.StatusForbidden,
		"/anthropic/api/oauth/profile":        http.StatusForbidden,
		"/anthropic/v1/files":                 http.StatusForbidden,
		"/anthropic/v1/messages/batches":      http.StatusForbidden,
		"/openai/v1/anything":                 http.StatusOK, // the scope is Anthropic's only
	}
	for p, want := range cases {
		resp, err := http.Post(ts.URL+p, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("POST %s = %d, want %d", p, resp.StatusCode, want)
		}
	}
	for _, h := range hits {
		if strings.Contains(h, "organizations") || strings.Contains(h, "oauth") || strings.Contains(h, "files") || strings.Contains(h, "batches") {
			t.Errorf("out-of-scope path %s reached the upstream", h)
		}
	}
}

func TestInteractiveRoutesUseTheirOwnAllowlist(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer upstream.Close()
	routes, err := buildRoutes([]routeSpec{{prefix: "/anthropic", upstream: upstream.URL, apiKey: "k", provider: "anthropic", requiresKey: true}})
	if err != nil {
		t.Fatal(err)
	}
	srv := newServer(routes, &captureSink{})
	// Loopback stands in for a worker network: allowed on fleet routes, not on interactive ones.
	srv.allow = cidrs(t, "127.0.0.0/8")
	srv.interactiveAllow = cidrs(t, "192.0.2.0/24")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for p, want := range map[string]int{"/anthropic/v1/messages": http.StatusOK, "/interactive/anthropic/v1/messages": http.StatusForbidden} {
		resp, err := http.Post(ts.URL+p, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("POST %s = %d, want %d", p, resp.StatusCode, want)
		}
	}
}

func TestDefaultInteractiveAllowlistIsLoopback(t *testing.T) {
	srv := newServer(nil, &captureSink{})
	if !inNets(srv.interactiveAllow, net.ParseIP("127.0.0.1")) || inNets(srv.interactiveAllow, net.ParseIP("10.88.0.5")) {
		t.Errorf("default interactive allowlist = %v, want loopback only", srv.interactiveAllow)
	}
}

func TestForwardedHeadersIgnored(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer upstream.Close()
	srv := newServer([]route{mustRoute(t, "/anthropic", upstream.URL, "k", "anthropic", true)}, &captureSink{})
	srv.allow = cidrs(t, "10.88.0.0/24")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/anthropic/v1/messages", nil)
	req.Header.Set("X-Forwarded-For", "10.88.0.5")
	req.Header.Set("X-Real-IP", "10.88.0.5")
	req.Header.Set("Forwarded", "for=10.88.0.5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a forwarding header claiming an allowed address got %d, want 403", resp.StatusCode)
	}
}

func TestCallerAllowedAddressForms(t *testing.T) {
	srv := newServer(nil, &captureSink{})
	srv.allow = cidrs(t, "10.88.0.0/24")
	for addr, want := range map[string]bool{
		"[::ffff:10.88.0.5]:4000": true, // IPv4-mapped IPv6 is the same host
		"[fe80::1%eth0]:4000":     false,
		"10.88.1.5:4000":          false,
		"garbage":                 false,
	} {
		if got := srv.callerAllowed(addr); got != want {
			t.Errorf("callerAllowed(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestOAuthBetaMergesEveryHeaderLine(t *testing.T) {
	var got []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Values("anthropic-beta")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	rt := mustRoute(t, "/anthropic", upstream.URL, "tok", "anthropic", true)
	rt.oauth = true
	ts := httptest.NewServer(newServer([]route{rt}, &captureSink{}).Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/anthropic/v1/messages", nil)
	req.Header.Add("anthropic-beta", "first-1")
	req.Header.Add("anthropic-beta", "second-2,"+anthropicOAuthBeta)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(got) != 1 || got[0] != "first-1,second-2,"+anthropicOAuthBeta {
		t.Errorf("anthropic-beta = %q, want every client flag once plus the OAuth flag once", got)
	}
}

func TestUpstreamRedirectNotFollowed(t *testing.T) {
	followed := false
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed = true }))
	defer elsewhere.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	ts := httptest.NewServer(newServer([]route{mustRoute(t, "/anthropic", upstream.URL, "k", "anthropic", true)}, &captureSink{}).Handler())
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Post(ts.URL+"/anthropic/v1/messages", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if followed {
		t.Error("the proxy followed an upstream redirect, carrying the request to another host")
	}
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("status = %d, want the upstream's 307 passed back", resp.StatusCode)
	}
}
