// llm-proxy: path-prefix reverse proxy with API key injection and JSONL logging.
//
// Routes:
//
//	/anthropic/*   → https://api.anthropic.com/*  (injects Authorization: Bearer $ANTHROPIC_OAUTH_TOKEN plus the
//	                 OAuth beta flag when set, else Bearer $ANTHROPIC_API_KEY + x-api-key)
//	/openai/*      → https://api.openai.com/*      (injects Authorization: Bearer $OPENAI_API_KEY)
//	/ollama-cloud/*→ $OLLAMA_CLOUD_URL (injects Authorization: Bearer $OLLAMA_CLOUD_API_KEY)
//	/ollama/*      → $OLLAMA_URL       (default: http://ndn.local:11434; local Ollama setup)
//	/local-fast/*  → $LOCAL_FAST_URL  (default: http://ndn.local:8081)
//	/local-large/* → $LOCAL_LARGE_URL (default: http://ndn.local:8082)
//	/health        → 200 OK
//
// Agents configure (agent-host.incus from Incus containers, 10.88.0.1 from microVMs):
//   ANTHROPIC_BASE_URL=http://agent-host.incus:12071/anthropic
//   OPENAI_BASE_URL=http://agent-host.incus:12071/openai
//   OLLAMA_CLOUD_BASE_URL=http://agent-host.incus:12071/ollama-cloud
//
// Only callers in $LLM_PROXY_ALLOW may use the routes: comma-separated CIDRs
// and "local-subnets" (every subnet this host is attached to). Unset means
// loopback only; the NixOS module sets the worker networks.
//
// Log format: JSONL to stdout, one record per request.

package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

func main() {
	listenAddr := envOrDefault("LLM_PROXY_ADDR", ":12071")

	specs := []routeSpec{
		anthropicSpec(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("ANTHROPIC_OAUTH_TOKEN")),
		{prefix: "/openai", upstream: "https://api.openai.com", apiKey: os.Getenv("OPENAI_API_KEY"), provider: "openai", requiresKey: true},
		{prefix: "/ollama-cloud", upstream: envOrDefault("OLLAMA_CLOUD_URL", "https://ollama.ai"), apiKey: os.Getenv("OLLAMA_CLOUD_API_KEY"), provider: "ollama-cloud", requiresKey: true},
		// Local Ollama instance (user's local setup at http://ndn.local:11434)
		{prefix: "/ollama", upstream: envOrDefault("OLLAMA_URL", "http://ndn.local:11434"), provider: "ollama"},
		// Local llama.cpp instances — no API key needed
		{prefix: "/local-fast", upstream: envOrDefault("LOCAL_FAST_URL", "http://ndn.local:8081"), provider: "local-fast"},
		{prefix: "/local-large", upstream: envOrDefault("LOCAL_LARGE_URL", "http://ndn.local:8082"), provider: "local-large"},
	}
	allow, err := parseAllow(envOrDefault("LLM_PROXY_ALLOW", "127.0.0.0/8,::1/128"), localSubnets)
	if err != nil {
		log.Fatalf("invalid LLM_PROXY_ALLOW: %v", err)
	}
	// /interactive routes skip the fleet reserve cap, so workers must not reach them.
	interactiveAllow, err := parseAllow(envOrDefault("LLM_PROXY_INTERACTIVE_ALLOW", "127.0.0.0/8,::1/128"), localSubnets)
	if err != nil {
		log.Fatalf("invalid LLM_PROXY_INTERACTIVE_ALLOW: %v", err)
	}

	routes, err := buildRoutes(specs)
	if err != nil {
		log.Fatalf("invalid routes: %v", err)
	}

	// Buffered, mutex-protected log writer to stdout. Wraps os.Stdout so
	// concurrent requests don't interleave bytes within a single JSON line.
	out := &lockedWriter{w: bufio.NewWriter(os.Stdout)}
	defer out.Flush()
	logs := &jsonLogSink{w: out}

	srv := newServer(routes, logs)
	srv.allow, srv.interactiveAllow = allow, interactiveAllow
	log.Printf("llm-proxy: fleet callers allowed from %v, interactive from %v", allow, interactiveAllow)
	if v := os.Getenv("LLM_PROXY_MAX_BODY"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			srv.maxBodyBytes = n
		} else {
			log.Printf("warning: ignoring LLM_PROXY_MAX_BODY=%q (not a positive int)", v)
		}
	}
	configureBudget(srv, os.Getenv)

	httpSrv := &http.Server{
		Addr:              listenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No ReadTimeout / WriteTimeout — long-running streaming completions
		// need indefinite read/write windows. The upstream client.Timeout in
		// proxy.go bounds total upstream call duration.
	}

	log.Printf("llm-proxy listening on %s", listenAddr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}

// lockedWriter serializes Write calls so JSON log lines from concurrent
// goroutines never interleave. bufio.Writer is not goroutine-safe on its own.
type lockedWriter struct {
	mu sync.Mutex
	w  *bufio.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, err := l.w.Write(p)
	// Flush per write so log lines aren't buffered in stdout when there's
	// no churn. JSONL one-per-line is the contract.
	if ferr := l.w.Flush(); err == nil {
		err = ferr
	}
	return n, err
}

func (l *lockedWriter) Flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.w.Flush()
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
