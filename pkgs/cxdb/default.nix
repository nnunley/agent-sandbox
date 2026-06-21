# cxdb-server — the CXDB AI context store (Turn-DAG + Blob CAS) binary-protocol server.
#
# Packaged from source (strongdm/cxdb, Apache-2.0) as a candidate distributed ContextProvider
# backend for the fleet (see docs/plans/2026-06-21-handoff-bundle-schema.md). Build/iterate on a Nix
# host (the local macOS dev box has no Nix): `nix build .#cxdb` on agent-host, or the callPackage
# one-liner in pkgs/README.md. See the `nix-packaging` skill for the workflow this followed.
{
  lib,
  rustPlatform,
  fetchFromGitHub,
  pkg-config,
}:

rustPlatform.buildRustPackage rec {
  pname = "cxdb-server";
  # No tagged release upstream; server/Cargo.toml is 0.1.0. Pin the commit and use the nixpkgs
  # "<version>-unstable-<date>" convention for VCS snapshots.
  version = "0.1.0-unstable-2026-06-21";

  src = fetchFromGitHub {
    owner = "strongdm";
    repo = "cxdb";
    rev = "0a599398b2d120ef2a0f69a11f6d7467956f110d";
    hash = "sha256-t6MD0kfAzEgQ2evC0qAB6GJIiTJzwTNX2YJVJlhapuI=";
  };

  # Upstream wires the AWS SDK (S3 blob-store sync) as an UNCONDITIONAL dependency, despite its own
  # comment calling it an "optional feature". This patch makes aws-config/aws-sdk-s3/tokio `optional`
  # behind a default-OFF `s3-sync` cargo feature and #[cfg]-gates the S3 code in main.rs/lib.rs. We
  # don't use S3 sync for a local/fleet context backend, so the default build skips the entire AWS
  # SDK tree — far smaller + faster. (Worth upstreaming: it matches their stated intent.)
  cargoPatches = [ ./s3-sync-optional.patch ];

  # Upstream ships NO Cargo.lock (it's a publishable workspace). We vendor a generated lock
  # (cargo generate-lockfile @ the pinned rev — 390 crates, all crates.io, no git deps) and place
  # it into the build tree. cargoLock.lockFile makes vendoring fully deterministic (the lock carries
  # crates.io checksums), so NO cargoHash is needed.
  cargoLock.lockFile = ./Cargo.lock;
  postPatch = ''
    install -m 0644 ${./Cargo.lock} Cargo.lock
  '';

  # Build only the server member of the workspace (clients/rust + cxtx aren't needed for the fleet).
  cargoBuildFlags = [ "-p" "cxdb-server" ];

  nativeBuildInputs = [ pkg-config ];
  # Pure-Rust dependency tree (tiny_http, not openssl). zstd/blake3/crc32fast/ring vendor their C and
  # build it with the stdenv `cc`, so no system libs are required.
  buildInputs = [ ];

  # Upstream tests want fixtures/network; keep the package build hermetic.
  doCheck = false;

  meta = {
    description = "CXDB binary-protocol server — AI Context Store with Turn DAG and Blob CAS";
    homepage = "https://github.com/strongdm/cxdb";
    license = lib.licenses.asl20;
    mainProgram = "cxdb-server";
    platforms = lib.platforms.linux;
  };
}
