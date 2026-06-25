package main

// RuntimeMode enumerates the worker runtime modes (STORY-0013 AC-1).
// one_shot: worker consumes ONE item, performs bounded work, emits result, and EXITS.
// long_running: worker holds a STABLE identity, stays subscribed, processes multiple items, emits heartbeats.
type RuntimeMode string

const (
	RuntimeOneShot     RuntimeMode = "one_shot"
	RuntimeLongRunning RuntimeMode = "long_running"
)

// Valid reports whether m is one of the two declared runtime modes. The behavior methods below treat
// any non-long_running value (including the "" zero value) as one_shot-like — a deliberately SAFE
// default (no persistent state, exit after one) — but a caller that wants to detect a misconfigured
// or typo'd mode ("one-shot", "longrunning", ...) should use Valid() rather than relying on the
// silent fallback (STORY-0013 AC-1).
func (m RuntimeMode) Valid() bool {
	return m == RuntimeOneShot || m == RuntimeLongRunning
}

// StaysSubscribed reports whether this runtime mode requires a long-lived subscription (STORY-0013 AC-4).
// one_shot: false — exit after one message
// long_running: true — remain subscribed for multiple messages
func (m RuntimeMode) StaysSubscribed() bool {
	return m == RuntimeLongRunning
}

// RequiresHeartbeat reports whether this runtime mode requires periodic heartbeat/status emission (STORY-0013 AC-4).
// one_shot: false — emit result once and exit
// long_running: true — emit periodic heartbeats to signal liveness
func (m RuntimeMode) RequiresHeartbeat() bool {
	return m == RuntimeLongRunning
}

// RetriesInProcess reports whether this runtime mode may retry failed work internally (STORY-0013 AC-4).
// one_shot: false — fail once, exit, no in-process retry
// long_running: true — may retry transient failures before escalating
func (m RuntimeMode) RetriesInProcess() bool {
	return m == RuntimeLongRunning
}

// AllowsCache reports whether this runtime mode permits ephemeral state caching (STORY-0013 AC-4).
// one_shot: false — no ephemeral cache across messages (worker must be stateless)
// long_running: true — may maintain ephemeral caches/coordination state across messages
func (m RuntimeMode) AllowsCache() bool {
	return m == RuntimeLongRunning
}
