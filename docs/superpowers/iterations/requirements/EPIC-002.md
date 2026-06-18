# EPIC-002 — Isolation tiers & micro-VM

**Summary:** Isolation tiers & micro-VM
**Stories:** STORY-0021, STORY-0022, STORY-0023, STORY-0024, STORY-0025
**Primary sources:** `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 0/5 done

## STORY-0021

**Epic:** EPIC-002 — Isolation tiers & micro-VM
**Title:** Fast isolation tier for trusted lanes uses namespace-based containers

**As a** dispatcher routing executor
**I want** to select nspawn --ephemeral NixOS containers for trusted lanes
**So that** cheap tasks spin up in sub-seconds with shared VM kernel namespace isolation

**Acceptance criteria:**
- AC-1: Fast tier: nspawn --ephemeral NixOS container unit with namespace isolation (shared VM kernel) · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0005`
- AC-2: Fast tier spin-up is sub-second using warm /nix store · impact:`local` · seam:`integration` · scenario:`SCENARIO-0005`
- AC-3: Fast tier used for trusted lanes and cheap iteration (template-driven selection) · impact:`local` · seam:`integration` · scenario:`SCENARIO-0005`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:81-87`

**Status:** pending

## STORY-0022

**Epic:** EPIC-002 — Isolation tiers & micro-VM
**Title:** Hard isolation tier for sensitive lanes uses per-task micro-VMs

**As a** dispatcher routing executor
**I want** to select per-task Firecracker microVM for sensitive/untrusted lanes
**So that** trading-platform and untrusted domains get hardware-level isolation

**Acceptance criteria:**
- AC-1: Hard tier: per-task Firecracker microVM (optionally wrapped in NixOS container) with hardware isolation · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0006`
- AC-2: Hard tier spin-up is hundreds of milliseconds (amortized cost over task lifetime) · impact:`local` · seam:`integration` · scenario:`SCENARIO-0006`
- AC-3: Hard tier used for sensitive/untrusted lanes (e.g., trading-platform domain) · impact:`local` · seam:`integration` · scenario:`SCENARIO-0006`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:81-87`

**Status:** pending

## STORY-0023

**Epic:** EPIC-002 — Isolation tiers & micro-VM
**Title:** Isolation tier selected per directive template

**As a** directive author
**I want** to specify isolation tier (fast/hard) in the task template
**So that** dispatcher selects the right substrate based on trust domain requirements

**Acceptance criteria:**
- AC-1: Template declares isolation tier selection (Fast or Hard) via D1 mechanism · impact:`local` · seam:`integration`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:81-89`

**Status:** pending

## STORY-0024

**Epic:** EPIC-002 — Isolation tiers & micro-VM
**Title:** Micro-VM is hardware trust boundary for multi-tenancy

**As a** fleet security architect
**I want** one micro-VM per trust domain with cheap disposable units inside
**So that** multi-tenancy falls out naturally and trust boundaries are hardware-enforced

**Acceptance criteria:**
- AC-1: Each micro-VM is a hardware trust boundary (own kernel, own scheduling) · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0007`
- AC-2: One VM per trust domain; disposable units run inside that VM · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0007`
- AC-3: Multi-tenancy architecture falls out from trust-domain VM topology · impact:`cross-surface` · seam:`e2e` · scenario:`SCENARIO-0007`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:88-89`

**Status:** pending

## STORY-0025

**Epic:** EPIC-002 — Isolation tiers & micro-VM
**Title:** Benchmark disposable-unit spin-up vs VM boot cost

**As a** performance optimizer
**I want** to measure nspawn-container vs per-task-microVM spin-up with real boot-readiness probe
**So that** we pick the substrate with evidence, not assumption

**Acceptance criteria:**
- AC-1: Benchmark measures disposable-unit (nspawn-container) spin-up time with boot-readiness probe inside live VM · impact:`none` · seam:`process-level`
- AC-2: Benchmark measures per-task-microVM spin-up time (refocuses spike #7 away from VM-boot amortization) · impact:`none` · seam:`process-level`
- AC-3: VM boot-to-ready is one-time amortized cost and is NOT the number that picks substrate · impact:`none` · seam:`unit`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:91-94`

**Status:** pending