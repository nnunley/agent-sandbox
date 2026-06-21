---
name: nix-packaging
description: Use when packaging external software (a Rust/Go/C project, CLI, or server) as a Nix package in this repo, adding a pkgs/<name>/default.nix or a flake packages output, computing a fetchFromGitHub/cargo hash, or when a nix build fails on a missing Cargo.lock, missing system dep, or hash mismatch. This repo's macOS dev box has NO local Nix — builds run on the agent-host container.
---

# nix-packaging

## Overview

Package external software as a Nix derivation in `pkgs/<name>/default.nix`, exposed via the flake's
`packages` output. **The local macOS machine has no Nix** — every `nix build`/`nix-build`/hash
computation runs on the **`agent-host`** Incus container (`incus exec agent-host -- …`, workdir
`/root/nixpkg-build`). Author files locally, push with `incus file push`, build remotely.

Reference pedagogy (the fake-hash trick): INRIA "first package"
(https://nix-tutorial.gitlabpages.inria.fr/nix-tutorial/first-package.html).

## Workflow

1. **Don't reinvent.** Check it isn't already packaged: `mcp__nixos__nix {"action":"search","query":"<name>"}`
   (nixpkgs) and `source:"flakehub"`; and look for `flake.nix`/`default.nix` in the upstream repo.
2. **Pick the builder by language:** Rust → `rustPlatform.buildRustPackage`; Go → `buildGoModule`;
   C/generic → `stdenv.mkDerivation`. (Confirm the attr exists via `mcp__nixos__nix`.)
3. **Pin the source.** Get the commit: `curl -s https://api.github.com/repos/<o>/<r>/commits/HEAD`.
   Get the src hash on agent-host: `nix store prefetch-file --unpack --json https://github.com/<o>/<r>/archive/<rev>.tar.gz | grep -oE 'sha256-[A-Za-z0-9+/=]+'` — OR use `lib.fakeHash`, build, copy the
   real hash from the "hash mismatch … wanted/got" error (the INRIA trick).
4. **Write `pkgs/<name>/default.nix`** as a `callPackage`-style function `{ lib, <builder>, fetchFromGitHub, … }:`.
5. **Build + iterate on agent-host:** `nix-build -E 'with import <nixpkgs> {}; callPackage ./<name> {}'`.
   Add `nativeBuildInputs`/`buildInputs` as errors demand (start minimal: `pkg-config`).
6. **Expose via the flake:** add `packages.${system}.<name> = pkgs.callPackage ./pkgs/<name> { };`.
   Verify `nix build .#<name>` on agent-host (commit first — see gotchas).
7. **Verify the artifact:** binary runs, and `nix-store -qR result | grep -i <unwanted-dep>` is empty.

## Rust specifics (the common wrinkles)

| Situation | Do this |
|---|---|
| Upstream has **no `Cargo.lock`** (publishable lib/workspace) | Generate one on agent-host (`nix shell nixpkgs#cargo --command cargo generate-lockfile`), vendor it to `pkgs/<name>/Cargo.lock`, use `cargoLock.lockFile = ./Cargo.lock;` + `postPatch = "install -m0644 ${./Cargo.lock} Cargo.lock";`. Deterministic — **no `cargoHash` needed**. |
| Lock has **git deps** | `cargoLock.outputHashes` per git crate (else `cargoHash` won't vendor them). |
| Lock is crates.io-only | `cargoLock.lockFile` (above) or `cargoHash` (fake → real from the error). |
| **Workspace** — only need one member | `cargoBuildFlags = [ "-p" "<member-pkg>" ];` |
| A heavy dep is **mis-declared non-optional** (e.g. an SDK behind a comment calling it "optional" but not feature-gated) | `cargoPatches = [ ./feature.patch ]` that makes the deps `optional = true`, adds a default-OFF `[features]` entry, and `#[cfg(feature="…")]`-gates the code (module decl, `use`, init + shutdown). Default build then skips the whole tree. Generate the patch with `git diff` in the source clone. |
| Native C in crates (zstd/blake3/ring) | Usually fine — `cc` from stdenv vendors it. No system lib unless a `-sys` crate needs `pkg-config` + the lib. |

## Gotchas (each cost a cycle)

- **`flake build` only sees git-tracked files.** In a check dir, `git add -A && git commit` BEFORE
  `nix build .#<name>`, or eval fails with "path …/pkgs/<name> does not exist".
- **No `perl` on the agent-host base.** For text munging use `nix shell nixpkgs#perl --command …`, or
  edit files locally (pull → Edit → push) and `git diff` for patches. (`perl` is now in the devShell.)
- **`nix store prefetch-file --unpack`** of the GitHub archive gives the hash `fetchFromGitHub` wants
  (it unpacks too). Don't hand it the un-unpacked tarball hash.
- **No tagged release →** version `"<x.y.z>-unstable-YYYY-MM-DD"` (nixpkgs VCS-snapshot convention).
- The dispatch/`tee` shell exit code lies; read the actual build/`grade.json` output, not `$?`.

## Worked example

`pkgs/cxdb/` in this repo: a Rust workspace with no upstream lock AND a mis-declared AWS-SDK dep —
both wrinkles above, handled (generated lock + `s3-sync-optional.patch` feature-gate). Result: a
6.1M `cxdb-server`, zero AWS in the closure, exposed as `packages.x86_64-linux.cxdb`.
