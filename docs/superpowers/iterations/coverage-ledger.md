# Coverage Ledger

Maps each normative source section to extracted stories/scenarios. Source: the
2026-06-18 design doc (primary) + coordinator-bootstrap + dispatcher-productization.

| Source section | Status | Notes |
|---|---|---|
| Governing constraint (Mac-off) | covered | e2e failure-recovery scenario (JOURNEY/SCENARIO) |
| Architecture — four planes + topology | covered | EPIC-001 (20 stories) |
| Isolation tiers | covered | fast nspawn / hard microVM, trust domains |
| D1 intent/template + origin validation | covered | privilege-escalation-denial scenarios |
| D2 substrate-agnostic backend | covered | container-first, microVM benchmark-gated |
| D3 state passthrough (lean-ctx) | covered | diary/knowledge/handoff; ctx_handoff = validation spike |
| D4 coordination loop + escalation ladder | covered | pass/fail/escalate/park + human lane |
| D5 teardown + reaper | covered | stop-before-delete regression |
| D6 audit log | covered | append-only, swappable-to-tamper-evident |
| Prioritization (Eisenhower × Temporal) | covered | single-writer projection, rescore authority, aging |
| Directive contract | covered | intent/template/origin/importance/deadline; no access_cmd/root |
| One-shot lifecycle | covered | JOURNEY-0001 walking skeleton |
| Skills (agent-skills-nix) | covered | declarative vendoring |
| Service discovery (no coredns v1) | covered | static injected endpoints + dnsmasq |
| OPEN: queue substrate | DEFERRED | stories carry [BLOCKED-ON-SUBSTRATE-DECISION]; not scheduled until decided |
| Spikes: ctx_handoff round-trip, fast-tier latency | VALIDATION | obligations, not corpus stories; ITER-0000 |

**Process note:** the PAR omission review (extraction step 3) was deferred as a
judgment call — the source is a single coherent design authored in-session, and a
human review checkpoint follows extraction. Can be run on demand before scoping.

**No `gap` chunks. No `story-only` observable-behavior chunks.** Validators pass.
