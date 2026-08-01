{ lib
, buildGoModule
, go_1_25
, fetchFromGitHub
}:

# serf — prime-radiant's headless, multi-provider coding agent (native Ollama).
# Lives in fleet-worker/ (not top-level pkgs/) because only this dir is delivered to workers
# and to the cache publisher (populate-nix-cache.sh) — it must be built with the worker's own
# nixpkgs pin so the cached store path matches what the worker's `nix develop` requests.
# Go workspace (go.work uses ./ + ./agent ./auth ./envvars ./llm …). Builds ALL cmd/ binaries
# (serf, serf-hub, serf-tui, llmcall, …). Fleet's ollama-local implementer (replaces claude+ccr).
buildGoModule rec {
  pname = "serf";
  version = "0-unstable-2026-07-22";

  src = fetchFromGitHub {
    owner = "prime-radiant-inc";
    repo = "serf";
    rev = "0b728736cf3e1aa41137e51a17e67b34c57c1108";
    hash = "sha256-coGVsFSGCZUl+bdOyUZreHptWAgihArRZ73nM+PG9QA=";
  };

  # go.work declares `go 1.25.6`; use a >=1.25.6 toolchain and forbid network toolchain fetch.
  go = go_1_25;
  env.GOTOOLCHAIN = "local";

  # buildGoModule's default `go mod vendor` cannot run in workspace mode. Override to
  # `go work vendor`, which vendors the whole go.work closure (main module + in-tree
  # sub-modules + all external deps) into ./vendor; the install phase copies it for the FOD.
  modBuildPhase = ''
    runHook preBuild
    go work vendor
    runHook postBuild
  '';

  # Vendored across the whole workspace (deps are go-minor-version independent, so this hash
  # is stable across nixpkgs pins).
  vendorHash = "sha256-SwP90urt1I8dBFgr7xRusULmnXhTMnMDxZIB/Yl0gxo=";

  # LLM/network-dependent tests; never run on the grade path.
  doCheck = false;

  ldflags = [ "-s" "-w" ];

  meta = {
    description = "Headless multi-provider coding agent with native Ollama support";
    homepage = "https://github.com/prime-radiant-inc/serf";
    license = lib.licenses.mit;
    mainProgram = "serf";
    platforms = lib.platforms.linux;
  };
}
