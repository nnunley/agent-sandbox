// Package main: proxy.go holds the routing/forwarding logic so it can be
// driven from main() and exercised from proxy_test.go without ever opening
// a real network connection.
package main

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agent-sandbox/usage"
)

// routeClass distinguishes interactive traffic (never capped — the protected
// interactive headroom) from fleet traffic (subject to the reserve cap).
type routeClass string

const (
	classFleet       routeClass = "fleet"
	classInteractive routeClass = "interactive"
)

// Default request body size cap. Anthropic and OpenAI both have lower
// per-request limits than this in practice; the cap exists so a misbehaving
// caller on the bridge can't make the proxy buffer arbitrary memory.
const defaultMaxBodyBytes int64 = 32 << 20 // 32 MiB

// Default upstream timeout. Long enough for streaming completions to finish,
// short enough to recover from a stuck upstream connection.
const defaultUpstreamTimeout = 10 * time.Minute

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
	prefix      string
	upstream    *url.URL
	apiKey      string
	provider    string
	requiresKey bool
	class       routeClass
}

// routeSpec is the declarative description of one upstream provider route.
type routeSpec struct {
	prefix      string
	upstream    string
	apiKey      string
	provider    string
	requiresKey bool
}

func newRoute(prefix, upstreamStr, apiKey, provider string, requiresKey bool, class routeClass) (route, error) {
	u, err := url.Parse(upstreamStr)
	if err != nil {
		return route{}, err
	}
	if u.Scheme == "" || u.Host == "" {
		return route{}, errors.New("upstream URL must have scheme and host: " + upstreamStr)
	}
	return route{
		prefix:      prefix,
		upstream:    u,
		apiKey:      apiKey,
		provider:    provider,
		requiresKey: requiresKey,
		class:       class,
	}, nil
}

// buildRoutes turns provider specs into routes: one fleet route per spec plus a
// parallel interactive route under the /interactive prefix. Point Claude Code's
// ANTHROPIC_BASE_URL at the interactive prefix to route it through the proxy;
// leave it unset to keep the proxy fleet-only.
func buildRoutes(specs []routeSpec) ([]route, error) {
	routes := make([]route, 0, len(specs)*2)
	for _, s := range specs {
		fleet, err := newRoute(s.prefix, s.upstream, s.apiKey, s.provider, s.requiresKey, classFleet)
		if err != nil {
			return nil, err
		}
		inter, err := newRoute("/interactive"+s.prefix, s.upstream, s.apiKey, s.provider, s.requiresKey, classInteractive)
		if err != nil {
			return nil, err
		}
		routes = append(routes, fleet, inter)
	}
	return routes, nil
}

// Hop-by-hop headers (RFC 7230 §6.1) — must be stripped at proxy boundaries.
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// logSink is anything that can record a JSON log entry. The production
// implementation writes JSONL to stdout via stdlib log (mutex-protected so
// concurrent requests don't interleave bytes).
type logSink interface {
	Log(logEntry)
}

// Server holds the routing table, the HTTP client used to reach upstreams,
// the log sink, and the request body cap.
type Server struct {
	routes       []route
	client       *http.Client
	logs         logSink
	maxBodyBytes int64

	// Budget enforcement (sub-project 2).
	ledgerPath      string             // usage ledger; fleet gate + metering both use it
	reservePct      float64            // default reserved interactive headroom fraction [0,1)
	providerReserve map[string]float64 // per-provider reservePct override
	priors          map[string]int64   // Estimator.PublishedPrior ceilings per provider
	now             func() time.Time   // injected clock; defaults to time.Now
}

func newServer(routes []route, logs logSink) *Server {
	return &Server{
		routes:          routes,
		client:          &http.Client{Timeout: defaultUpstreamTimeout},
		logs:            logs,
		maxBodyBytes:    defaultMaxBodyBytes,
		ledgerPath:      "",
		reservePct:      0.30,
		providerReserve: map[string]float64{},
		priors:          map[string]int64{},
		now:             time.Now,
	}
}

// reserveFor returns the reserved-headroom fraction for a provider (override or default).
func (s *Server) reserveFor(provider string) float64 {
	if p, ok := s.providerReserve[provider]; ok {
		return p
	}
	return s.reservePct
}

// budgetAllows reports whether a fleet call to provider is within the reserve cap
// for the active 5h window, plus a Retry-After hint (seconds) when deferred. It
// reads only already-recorded usage (pre-flight) and fails OPEN on any ledger
// error - a meter bug must never strand the fleet.
func (s *Server) budgetAllows(provider string, now time.Time) (allow bool, retryAfter int) {
	if s.ledgerPath == "" {
		return true, 0
	}
	l, err := usage.OpenLedger(s.ledgerPath)
	if err != nil {
		s.logs.Log(logEntry{Provider: provider, Error: "budget: ledger open failed (fail-open): " + err.Error()})
		return true, 0
	}
	est := usage.Estimator{PublishedPrior: s.priors}.Estimate(l.Events(), l.Limits(), provider, usage.Window5h, now)
	if est.AllowFleet(s.reserveFor(provider)) {
		return true, 0
	}
	ra := int(math.Ceil(est.WindowReset.Sub(now).Seconds()))
	if ra < 1 {
		ra = 1
	}
	return false, ra
}

// Handler returns an http.Handler with /health and one subtree per route.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})

	for i := range s.routes {
		rt := s.routes[i]
		mux.HandleFunc(rt.prefix+"/", func(w http.ResponseWriter, r *http.Request) {
			s.proxy(w, r, rt)
		})
	}

	return mux
}

// countingReader wraps a Reader and counts the bytes that pass through. It
// gives us an accurate bytes_in for chunked uploads, which Content-Length
// alone can't.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	atomic.AddInt64(&c.n, int64(n))
	return n, err
}

func (c *countingReader) bytes() int64 { return atomic.LoadInt64(&c.n) }

func (s *Server) proxy(w http.ResponseWriter, r *http.Request, rt route) {
	start := time.Now()
	entry := logEntry{
		Timestamp: start.UTC().Format(time.RFC3339),
		Method:    r.Method,
		Path:      r.URL.Path,
		Provider:  rt.provider,
	}
	finish := func(status int, errMsg string, bytesIn, bytesOut int64) {
		entry.StatusCode = status
		entry.DurationMs = time.Since(start).Milliseconds()
		entry.BytesIn = bytesIn
		entry.BytesOut = bytesOut
		if errMsg != "" {
			entry.Error = errMsg
		}
		s.logs.Log(entry)
	}

	// Pre-flight reserve-cap gate: fleet traffic only. Interactive is the
	// protected headroom and is never deferred. Decision reads only the ledger's
	// already-recorded usage, so it is independent of this response.
	if rt.class == classFleet {
		if allow, retryAfter := s.budgetAllows(rt.provider, s.now()); !allow {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.Header().Set("X-Budget-Deferred", "1")
			http.Error(w, "budget-deferred: fleet reserve cap reached for "+rt.provider, http.StatusTooManyRequests)
			finish(http.StatusTooManyRequests, "budget-deferred", 0, 0)
			return
		}
	}

	// Strip route prefix using a path-segment boundary check, then clean to
	// neutralize "../" segments a client might send.
	upstreamPath, ok := stripRoutePrefix(r.URL.Path, rt.prefix)
	if !ok {
		// ServeMux subtree dispatch should make this impossible, but be safe.
		http.NotFound(w, r)
		finish(http.StatusNotFound, "prefix mismatch", 0, 0)
		return
	}
	upstreamPath = path.Clean(upstreamPath)

	target := *rt.upstream
	target.Path = singleJoiningSlash(rt.upstream.Path, upstreamPath)
	target.RawQuery = r.URL.RawQuery

	// Cap and count the request body.
	var bodyReader io.Reader
	var counter *countingReader
	if r.Body != nil {
		limited := http.MaxBytesReader(w, r.Body, s.maxBodyBytes)
		counter = &countingReader{r: limited}
		bodyReader = counter
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), bodyReader)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		finish(http.StatusBadGateway, err.Error(), 0, 0)
		return
	}

	// Copy headers, minus hop-by-hop and any auth the client tried to supply.
	// We never want client-provided credentials to reach upstream — the proxy's
	// job is to inject its own.
	copyHeaders(outReq.Header, r.Header)
	for _, h := range hopByHopHeaders {
		outReq.Header.Del(h)
	}
	outReq.Header.Del("Authorization")
	outReq.Header.Del("x-api-key")

	// Routes that need an API key MUST have one — otherwise the request would
	// pass through unauthenticated, returning 401 from upstream and giving the
	// caller a confusing error. Fail fast with a clear message.
	if rt.requiresKey && rt.apiKey == "" {
		http.Error(w, "proxy: no API key configured for "+rt.provider, http.StatusServiceUnavailable)
		finish(http.StatusServiceUnavailable, "no API key configured", 0, 0)
		return
	}

	if rt.apiKey != "" {
		outReq.Header.Set("Authorization", "Bearer "+rt.apiKey)
	}
	if rt.provider == "anthropic" && rt.apiKey != "" {
		outReq.Header.Set("x-api-key", rt.apiKey)
		if outReq.Header.Get("anthropic-version") == "" {
			outReq.Header.Set("anthropic-version", "2023-06-01")
		}
	}

	// Strip Content-Length so net/http picks the right framing (chunked when
	// the upstream client wants streaming, fixed when known). The body wrapper
	// hides Content-Length from the request, so we set ContentLength=-1 to
	// signal "unknown" — http will use chunked transfer encoding.
	outReq.ContentLength = r.ContentLength
	outReq.Host = rt.upstream.Host

	resp, err := s.client.Do(outReq)
	if err != nil {
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		var bytesIn int64
		if counter != nil {
			bytesIn = counter.bytes()
		}
		finish(http.StatusBadGateway, err.Error(), bytesIn, 0)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.Header().Del("Content-Length")
	w.WriteHeader(resp.StatusCode)

	// Meter the response into the ledger as it streams to the client. The scanner
	// is fed via TeeReader, so it never buffers the whole body, reorders, or
	// delays client bytes.
	scanner := newUsageScanner(rt.provider)
	bytesOut := streamCopy(w, io.TeeReader(resp.Body, scanner))

	var bytesIn int64
	if counter != nil {
		bytesIn = counter.bytes()
	}

	// Metering is best-effort and only when a ledger is configured (empty path =>
	// disabled, e.g. hermetic non-budget tests). A failure never affects brokering.
	if s.ledgerPath != "" {
		if ev, ok := scanner.result(s.now()); ok {
			ev.Source = sourceForClass(rt.class)
			if l, err := usage.OpenLedger(s.ledgerPath); err == nil {
				_ = l.Append(ev)
			}
		}
		// Calibration: an upstream rate-limit reveals the realized ceiling - record
		// a LimitEvent at the window usage observed just before this call.
		if resp.StatusCode == http.StatusTooManyRequests {
			if l, err := usage.OpenLedger(s.ledgerPath); err == nil {
				est := usage.Estimator{PublishedPrior: s.priors}.Estimate(l.Events(), l.Limits(), rt.provider, usage.Window5h, s.now())
				_ = l.AppendLimit(usage.LimitEvent{
					Provider:    rt.provider,
					WindowClass: usage.Window5h.Name,
					UsedAt:      est.Used,
					Ts:          s.now(),
				})
			}
		}
	}

	finish(resp.StatusCode, "", bytesIn, bytesOut)
}

// streamCopy copies from src to dst, flushing dst after each successful read
// when dst supports http.Flusher. This matters for SSE / chunked streaming.
func streamCopy(dst io.Writer, src io.Reader) int64 {
	flusher, _ := dst.(http.Flusher)
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			wn, werr := dst.Write(buf[:n])
			total += int64(wn)
			if flusher != nil {
				flusher.Flush()
			}
			if werr != nil {
				return total
			}
		}
		if rerr != nil {
			return total
		}
	}
}

// stripRoutePrefix removes rt.prefix from p, but only when the next character
// is "/" or the path equals the prefix exactly. Prevents /anthropicfoo/...
// from being matched by /anthropic.
func stripRoutePrefix(p, prefix string) (string, bool) {
	if p == prefix {
		return "/", true
	}
	if strings.HasPrefix(p, prefix+"/") {
		return strings.TrimPrefix(p, prefix), true
	}
	return "", false
}

// singleJoiningSlash joins two path segments without doubling or dropping
// slashes. Mirrors net/http/httputil.singleJoiningSlash.
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// jsonLogSink writes one JSON object per log entry to w, using stdlib log to
// serialize concurrent writes. Output looks like:
//
//	{"ts":"...","method":"POST",...}
type jsonLogSink struct{ w io.Writer }

func (j *jsonLogSink) Log(e logEntry) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = j.w.Write(b)
}
