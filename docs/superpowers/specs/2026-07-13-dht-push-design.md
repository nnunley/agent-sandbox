# dht-push — P2P encrypted notifications, fleet → mobile

**Status:** design approved in brainstorming (2026-07-13); spec for review.
**Track A (stopgap)** unblocks the daily-briefing tenant now; **Track B** is the real project. Lives here because the publisher integrates with the fleet; extract to its own repo when the Rust workspace lands.

## Goals

1. Fleet notifications reach Norman's phone with no third party able to read content and no dependency on external services staying up.
2. No open inbound ports on the phone or laptop, ever. One reachable rendezvous (relay + Kademlia bootstrap) is unavoidable — hole-punching requires it. Placement decision: forward one UDP port to agent-host, OR run the tiny relay on a ~$4 VPS (it only ever sees sealed envelopes and DHT chatter, so sovereignty is preserved either way).
3. Permissively-licensed artifact (MIT/Apache) — portfolio-grade OSS.
4. Android first; iOS phase 2 via content-free APNs wake pings.

## Non-goals (v1)

Group fan-out, message history sync, media payloads, iOS, replacing ntfy for anything but Norman's own devices.

## Track A — stopgap (this week)

Self-hosted **ntfy** on the NAS; briefing chain delivers via one `curl`. Android: ntfy app (doubles as a UnifiedPush distributor). iOS: ntfy app — its APNs relay carries only "poll your server" pings; content stays on the NAS. Privacy = owned server + TLS, not payload-E2E; Track B closes that gap. **This decides the briefing project's open delivery question: ntfy.**

## Track B — architecture

**Foundation: rust-libp2p (MIT).** Kademlia (discovery) + DCUtR hole-punching + circuit-relay-v2 (fallback) + QUIC/Noise transport. The DHT is for **discovery only**; delivery is direct connections; the queue is **laneq** on agent-host.

```
[fleet chains] → laneq (per-device lane) → publisher daemon (agent-host, Rust)
                                              │ Kademlia FIND_NODE(phone PeerId)
                                              │ QUIC direct (DCUtR punch) — or via relay
                                              ▼
                                     phone app (foreground svc)
                                     decrypt → notify → ack → lane item done
```

### Components

1. **Envelope** (shared crate): payload sealed to recipient's X25519 key (libsodium sealed box), signed by sender Ed25519, UnifiedPush-compatible body, ≤4KB. Defense in depth — content stays sealed even relayed. Custom libp2p `request-response` protocol `/dht-push/notify/1`; the response is the ack.
2. **Publisher daemon** (agent-host): laneq consumer per device lane; maintains fleet identity keypair; delivery loop = resolve PeerId via Kademlia → dial (QUIC, DCUtR, relay fallback) → send envelope → on ack, complete lane item; on failure, item stays queued (pull covers it). Also runs the **relay + Kademlia bootstrap node** — the one component that must be reachable (see Goals #2 for the placement trade-off).
3. **Android app**: rust core cross-compiled via NDK, exposed to Kotlin with **uniffi**; foreground service keeps the libp2p node alive; on connect-in, decrypt → post notification → ack. Implements the **UnifiedPush distributor API** so other apps ride the same channel. **Pull path:** on wake/network-change, dial the fleet (its PeerId resolved via Kademlia) and drain the lane — push latency degrades to pull cadence, never to loss.
4. **Pairing**: QR code = phone PeerId + X25519 pubkey + bootstrap multiaddr; fleet side registers the device (creates lane, stores keys). Manual, two devices, one time.
5. **iOS (phase 2)**: same rust core via uniffi; agent-host proxy sends a content-free APNs ping when a lane is non-empty; woken app runs the ordinary pull path. Requires Apple dev account.

### Failure semantics

Hole-punch fails → relay. Relay down → item waits in lane; phone's next pull drains it. Phone offline for days → lane accumulates; nothing lost, nothing expires by default (lane hygiene = existing laneq requeue/drop tooling). Silence is visible via the briefing dead-man chain, which rides this same channel.

### Security notes

PeerIds are public keys; Kademlia lookups expose that *someone* queries a PeerId and the phone's current multiaddrs to DHT participants — content is never in the DHT and always sealed. The DHT is private-by-default (own bootstrap; joining public IPFS DHT is a later choice). Threat model documented before any public release.

### Testing

1. Two desktop nodes, loopback: envelope + notify protocol round-trip.
2. Publisher against real laneq on agent-host; fake device node; kill/restart both sides mid-queue.
3. NAT lab: phone on cellular, punch vs relay paths forced.
4. Battery: 24h foreground-service soak on the real phone, measured.

## Milestones

- **M0 (Track A):** ntfy up; briefing delivers. (Belongs to the briefing project.)
- **M1:** Rust workspace — envelope crate + publisher skeleton + desktop test peer; loopback round-trip green.
- **M2:** laneq integration + relay/bootstrap on agent-host; delivery across real NAT from a laptop node.
- **M3:** Android app (uniffi, foreground service, notify + ack + pull); replace ntfy for fleet notifications.
- **M4:** UnifiedPush distributor API; battery soak; threat-model doc.
- **M5 (later):** iOS wake-proxy phase.

## Open questions

1. Rendezvous placement: agent-host port-forward vs. tiny VPS (see Goals #2). Private DHT of ~2 nodes is degenerate-but-fine for v1 — revisit if the project goes public.
2. uniffi vs JNI-by-hand for the Kotlin bridge (decide at M3; uniffi presumed).
3. Whether the publisher daemon and the fleet coordinator share a process or stay separate units (presumed separate; coordinator stays zero-LLM and this stays zero-coordinator).
