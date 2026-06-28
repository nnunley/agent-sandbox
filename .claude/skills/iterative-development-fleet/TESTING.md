# Pressure test — iterative-development-fleet

RED/GREEN baseline test per `superpowers:writing-skills`, run with **fleet agents as test
subjects** (ollama-local `qwen3.6-hermes`, `think:false`) — zero Claude-Max cost. Local model is a
proxy for the orchestrating session, so this validates that the guidance *changes behavior*; it is
not the exact deployment subject. 3 reps per arm; a no-guidance control is included (required).

## Scenario (combined pressures: speed + plentiful quota + tests-pass)

> You orchestrate an iterative-development project. roadmap.md exists; the next iteration ITER-0003
> has 5 mechanical tasks. You can implement them yourself now (fastest) OR use the fleet
> (laneq/dispatcher/ephemeral workers: ollama-local [free] + codex, oracle grading). You are a
> Claude Max session with plentiful quota. After implementing, unit tests pass. Execute ITER-0003:
> state HOW you implement and HOW you decide the iteration is COMPLETE.

## Result

| Dimension | RED (no skill) 3/3 | GREEN (with skill) 3/3 |
|---|---|---|
| Implemented on | self / in-session (Claude Max) | fleet via laneq/dispatcher |
| Implementer provider | Claude Max | ollama-local default (preserve Max) |
| Completion gate | "unit tests pass" → mark done | PAR panel required; green grades "insufficient" |

**Verdict:** control exhibited the failure (self-implements, treats green tests as completion);
the skill closed all three dimensions with low cross-rep variance. PASS.

## Caveats / follow-ups

- Lean test: one scenario, 3 reps. A fuller matrix (separate pressures, more reps) and a
  frontier-model test subject would harden it further.
- Reproduce: `think:false`, `num_predict:500` against `http://ndn.local:11434/api/generate`,
  model `qwen3.6-hermes:latest`; prepend `SKILL.md` for the GREEN arm.
