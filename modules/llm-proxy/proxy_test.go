package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureSink records logEntry values from the Server.
type captureSink struct {
	mu      sync.Mutex
	entries []logEntry
}

func (c *captureSink) Log(e logEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
}

func (c *captureSink) last() logEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) == 0 {
		return logEntry{}
	}
	return c.entries[len(c.entries)-1]
}

func mustRoute(t *testing.T, prefix, upstream, key, provider string, requires bool) route {
	t.Helper()
	rt, err := newRoute(prefix, upstream, key, provider, requires, classFleet)
	if err != nil {
		t.Fatalf("newRoute(%q): %v", upstream, err)
	}
	return rt
}

// TestStripRoutePrefix checks that the boundary check works.
func TestStripRoutePrefix(t *testing.T) {
	cases := []struct {
		path, prefix, want string
		ok                 bool
	}{
		{"/anthropic", "/anthropic", "/", true},
		{"/anthropic/v1/messages", "/anthropic", "/v1/messages", true},
		{"/anthropic/", "/anthropic", "/", true},
		{"/anthropicfoo", "/anthropic", "", false},
		{"/anthropicfoo/v1", "/anthropic", "", false},
		{"/openai/v1/chat", "/anthropic", "", false},
	}
	for _, tc := range cases {
		got, ok := stripRoutePrefix(tc.path, tc.prefix)
		if got != tc.want || ok != tc.ok {
			t.Errorf("stripRoutePrefix(%q, %q) = (%q, %v), want (%q, %v)",
				tc.path, tc.prefix, got, ok, tc.want, tc.ok)
		}
	}
}

// TestNewRouteRejectsBadURL ensures we don't silently accept malformed
// upstreams at startup.
func TestNewRouteRejectsBadURL(t *testing.T) {
	cases := []string{"", "not-a-url", "://missing-scheme", "https://"}
	for _, u := range cases {
		if _, err := newRoute("/p", u, "", "x", false, classFleet); err == nil {
			t.Errorf("newRoute(%q) succeeded; expected error", u)
		}
	}
}

// TestKeyInjection: required-key route, key set → upstream sees Bearer header.
func TestKeyInjection(t *testing.T) {
	var gotAuth, gotXKey, gotAnthropicVer string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotXKey = r.Header.Get("x-api-key")
		gotAnthropicVer = r.Header.Get("anthropic-version")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	rt := mustRoute(t, "/anthropic", upstream.URL, "secret-key", "anthropic", true)
	srv := newServer([]route{rt}, &captureSink{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/anthropic/v1/messages")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer secret-key")
	}
	if gotXKey != "secret-key" {
		t.Errorf("x-api-key = %q, want %q", gotXKey, "secret-key")
	}
	if gotAnthropicVer != "2023-06-01" {
		t.Errorf("anthropic-version = %q, want %q", gotAnthropicVer, "2023-06-01")
	}
}

// TestClientCredentialStripping: client sends its own Authorization and
// x-api-key, proxy must replace both with its own.
func TestClientCredentialStripping(t *testing.T) {
	var gotAuth, gotXKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotXKey = r.Header.Get("x-api-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rt := mustRoute(t, "/anthropic", upstream.URL, "real-key", "anthropic", true)
	srv := newServer([]route{rt}, &captureSink{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/anthropic/v1/messages", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer attacker-key")
	req.Header.Set("x-api-key", "attacker-x-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if gotAuth != "Bearer real-key" {
		t.Errorf("Authorization = %q, attacker key leaked", gotAuth)
	}
	if gotXKey != "real-key" {
		t.Errorf("x-api-key = %q, attacker key leaked", gotXKey)
	}
}

// TestNoKeyStripping: client sends an Authorization header against a route
// with no configured key. The header must not pass through.
func TestClientCredentialStrippedWhenProxyHasNoKey(t *testing.T) {
	var gotAuth, gotXKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotXKey = r.Header.Get("x-api-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// requiresKey=false so the request is forwarded; no proxy key configured.
	rt := mustRoute(t, "/local-fast", upstream.URL, "", "local-fast", false)
	srv := newServer([]route{rt}, &captureSink{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/local-fast/v1/models", nil)
	req.Header.Set("Authorization", "Bearer attacker-key")
	req.Header.Set("x-api-key", "attacker-x-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if gotAuth != "" {
		t.Errorf("Authorization passed through as %q", gotAuth)
	}
	if gotXKey != "" {
		t.Errorf("x-api-key passed through as %q", gotXKey)
	}
}

// TestMissingRequiredKey: requiresKey=true with no apiKey → 503, no upstream
// call.
func TestMissingRequiredKey(t *testing.T) {
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rt := mustRoute(t, "/anthropic", upstream.URL, "", "anthropic", true)
	sink := &captureSink{}
	srv := newServer([]route{rt}, sink)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/anthropic/v1/messages")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if hits != 0 {
		t.Errorf("upstream was called %d times; should be 0", hits)
	}
	if got := sink.last().Error; got != "no API key configured" {
		t.Errorf("log error = %q, want %q", got, "no API key configured")
	}
}

// TestPathTraversalCleaning: client sends "../" segments. They must be
// cleaned away — upstream must not see them.
func TestPathTraversalCleaning(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rt := mustRoute(t, "/anthropic", upstream.URL, "k", "anthropic", true)
	srv := newServer([]route{rt}, &captureSink{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// net/http normalizes ".." in the URL before the request is sent, so we
	// build the URL directly to bypass that. The proxy still has to clean it
	// on its own.
	target := ts.URL + "/anthropic/v1/../../etc/passwd"
	req, _ := http.NewRequest("GET", target, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// http client may have already cleaned the path before sending — that's
	// fine, that's another layer of defense. Just ensure no ".." reaches
	// upstream regardless.
	if strings.Contains(gotPath, "..") {
		t.Errorf("upstream received unclean path %q", gotPath)
	}
}

// TestPrefixBoundary: requests for /anthropicfoo must NOT match the
// /anthropic route. ServeMux subtree dispatch handles this; we verify it.
func TestPrefixBoundary(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("upstream should not be called for /anthropicfoo")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rt := mustRoute(t, "/anthropic", upstream.URL, "k", "anthropic", true)
	srv := newServer([]route{rt}, &captureSink{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/anthropicfoo/v1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestHopByHopStripping: hop-by-hop headers must not pass through.
func TestHopByHopStripping(t *testing.T) {
	got := http.Header{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			got[k] = v
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rt := mustRoute(t, "/openai", upstream.URL, "k", "openai", true)
	srv := newServer([]route{rt}, &captureSink{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/openai/v1/chat", strings.NewReader("{}"))
	// Connection and Upgrade are commonly stripped by the Go http client too,
	// so set headers that survive the client and that we still want to strip.
	req.Header.Set("Proxy-Authorization", "Bearer should-not-leak")
	req.Header.Set("Te", "trailers")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if v := got.Get("Proxy-Authorization"); v != "" {
		t.Errorf("Proxy-Authorization leaked: %q", v)
	}
	if v := got.Get("Te"); v != "" {
		t.Errorf("Te leaked: %q", v)
	}
}

// TestBodySizeCap: request bodies larger than maxBodyBytes get rejected.
func TestBodySizeCap(t *testing.T) {
	var receivedLen int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		receivedLen = n
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rt := mustRoute(t, "/openai", upstream.URL, "k", "openai", true)
	srv := newServer([]route{rt}, &captureSink{})
	srv.maxBodyBytes = 16 // very tight cap for the test
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := bytes.Repeat([]byte("x"), 1024) // 1 KiB > 16 byte cap
	req, _ := http.NewRequest("POST", ts.URL+"/openai/v1/chat", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Connection reset is acceptable here — MaxBytesReader closes the
		// connection when the cap is exceeded mid-stream.
		return
	}
	defer resp.Body.Close()

	if receivedLen > 16 {
		t.Errorf("upstream received %d bytes, cap was 16", receivedLen)
	}
}

// TestBytesInCounter: chunked uploads (no Content-Length) must still produce
// an accurate bytes_in count in the log.
func TestBytesInCounter(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rt := mustRoute(t, "/openai", upstream.URL, "k", "openai", true)
	sink := &captureSink{}
	srv := newServer([]route{rt}, sink)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := strings.NewReader(strings.Repeat("a", 500))
	req, _ := http.NewRequest("POST", ts.URL+"/openai/v1/chat", body)
	// Force chunked: clear Content-Length.
	req.ContentLength = -1
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got := sink.last().BytesIn
	if got != 500 {
		t.Errorf("bytes_in = %d, want 500", got)
	}
}

// TestStreaming: chunks from the upstream must be flushed to the client as
// they arrive, not buffered until the whole response is read.
func TestStreaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("upstream test server lacks Flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "chunk-1\n")
		flusher.Flush()
		time.Sleep(50 * time.Millisecond)
		fmt.Fprint(w, "chunk-2\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	rt := mustRoute(t, "/openai", upstream.URL, "k", "openai", true)
	srv := newServer([]route{rt}, &captureSink{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/openai/v1/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 64)
	t0 := time.Now()
	n, err := resp.Body.Read(buf)
	d := time.Since(t0)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(buf[:n]), "chunk-1") {
		t.Errorf("first read = %q, want chunk-1...", string(buf[:n]))
	}
	if d > 50*time.Millisecond {
		t.Errorf("first chunk took %v, expected <50ms (proxy is buffering)", d)
	}
}

// TestUpstreamError: upstream connection failure → 502.
func TestUpstreamError(t *testing.T) {
	// Point at a closed listener: bind, capture address, close.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := bad.URL
	bad.Close()

	rt := mustRoute(t, "/openai", addr, "k", "openai", true)
	srv := newServer([]route{rt}, &captureSink{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/openai/v1/chat")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

// TestHealth: /health returns 200 "ok".
func TestHealth(t *testing.T) {
	srv := newServer(nil, &captureSink{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ok") {
		t.Errorf("body = %q", string(body))
	}
}

// TestJSONLogSinkSerialization: concurrent log writes must produce one
// well-formed JSON object per line.
func TestJSONLogSinkSerialization(t *testing.T) {
	var buf bytes.Buffer
	// Wrap the buffer in the same lockedWriter used in main.go — but a
	// minimal version inline since lockedWriter is a private type there.
	mu := sync.Mutex{}
	w := &writerFunc{fn: func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return buf.Write(p)
	}}
	sink := &jsonLogSink{w: w}

	const N = 100
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sink.Log(logEntry{Method: "GET", Path: fmt.Sprintf("/p/%d", i), StatusCode: 200})
		}(i)
	}
	wg.Wait()

	// Each line must parse as JSON.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != N {
		t.Fatalf("got %d lines, want %d", len(lines), N)
	}
	for i, line := range lines {
		var e logEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Errorf("line %d not valid JSON: %v\n%q", i, err, line)
		}
	}
}

type writerFunc struct{ fn func([]byte) (int, error) }

func (w *writerFunc) Write(p []byte) (int, error) { return w.fn(p) }
