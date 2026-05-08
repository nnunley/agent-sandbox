// llm-proxy: path-prefix reverse proxy with API key injection and JSONL logging.
//
// Routes:
//   /anthropic/*   → https://api.anthropic.com/*  (injects Authorization: Bearer $ANTHROPIC_API_KEY)
//   /openai/*      → https://api.openai.com/*      (injects Authorization: Bearer $OPENAI_API_KEY)
//   /local-fast/*  → $LOCAL_FAST_URL  (default: http://ndn.local:8081, Gemma 4 E4B)
//   /local-large/* → $LOCAL_LARGE_URL (default: http://ndn.local:8082, Qwen3-Coder-Next)
//   /health        → 200 OK
//
// Agents configure: ANTHROPIC_BASE_URL=http://10.88.0.1:12071/anthropic
//
// Log format: JSONL to stdout, one record per request.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type logEntry struct {
	Timestamp  string `json:"ts"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Provider   string `json:"provider"`
	StatusCode int    `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
	Error      string `json:"error,omitempty"`
}

type route struct {
	prefix   string
	upstream *url.URL
	apiKey   string
	provider string
}

func newRoute(prefix, upstreamStr, apiKey, provider string) route {
	u, err := url.Parse(upstreamStr)
	if err != nil {
		log.Fatalf("invalid upstream URL %q: %v", upstreamStr, err)
	}
	return route{prefix: prefix, upstream: u, apiKey: apiKey, provider: provider}
}

func main() {
	listenAddr := envOrDefault("LLM_PROXY_ADDR", ":12071")

	routes := []route{
		newRoute("/anthropic", "https://api.anthropic.com", os.Getenv("ANTHROPIC_API_KEY"), "anthropic"),
		newRoute("/openai", "https://api.openai.com", os.Getenv("OPENAI_API_KEY"), "openai"),
		// Local llama.cpp instances on ndn.local — no API key needed
		newRoute("/local-fast", envOrDefault("LOCAL_FAST_URL", "http://ndn.local:8081"), "", "local-fast"),
		newRoute("/local-large", envOrDefault("LOCAL_LARGE_URL", "http://ndn.local:8082"), "", "local-large"),
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	for _, rt := range routes {
		rt := rt // capture
		mux.HandleFunc(rt.prefix+"/", func(w http.ResponseWriter, r *http.Request) {
			proxyRequest(w, r, rt)
		})
	}

	log.Printf("llm-proxy listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func proxyRequest(w http.ResponseWriter, r *http.Request, rt route) {
	start := time.Now()
	entry := logEntry{
		Timestamp: start.UTC().Format(time.RFC3339),
		Method:    r.Method,
		Path:      r.URL.Path,
		Provider:  rt.provider,
	}

	// Strip route prefix to get upstream path
	upstreamPath := strings.TrimPrefix(r.URL.Path, rt.prefix)
	if upstreamPath == "" {
		upstreamPath = "/"
	}

	target := *rt.upstream
	target.Path = upstreamPath
	target.RawQuery = r.URL.RawQuery

	// Read request body for logging (limited to avoid memory issues)
	var bodyReader io.Reader = r.Body
	var bytesIn int64
	if r.ContentLength > 0 {
		bytesIn = r.ContentLength
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), bodyReader)
	if err != nil {
		entry.Error = err.Error()
		entry.StatusCode = http.StatusBadGateway
		entry.DurationMs = time.Since(start).Milliseconds()
		writeLog(entry)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	// Copy headers from client request
	copyHeaders(outReq.Header, r.Header)

	// Inject API key (overrides any client-supplied key)
	if rt.apiKey != "" {
		outReq.Header.Set("Authorization", "Bearer "+rt.apiKey)
	}

	// Anthropic uses x-api-key in addition to Authorization
	if rt.provider == "anthropic" && rt.apiKey != "" {
		outReq.Header.Set("x-api-key", rt.apiKey)
		// Anthropic requires this header
		if outReq.Header.Get("anthropic-version") == "" {
			outReq.Header.Set("anthropic-version", "2023-06-01")
		}
	}

	outReq.Host = rt.upstream.Host

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(outReq)
	if err != nil {
		entry.Error = err.Error()
		entry.StatusCode = http.StatusBadGateway
		entry.DurationMs = time.Since(start).Milliseconds()
		writeLog(entry)
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	bytesOut, _ := io.Copy(w, resp.Body)

	entry.StatusCode = resp.StatusCode
	entry.DurationMs = time.Since(start).Milliseconds()
	entry.BytesIn = bytesIn
	entry.BytesOut = bytesOut
	writeLog(entry)
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func writeLog(e logEntry) {
	b, _ := json.Marshal(e)
	fmt.Println(string(b))
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
