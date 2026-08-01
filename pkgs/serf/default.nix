{ lib
, buildGoModule
, go_1_25
, fetchFromGitHub
}:

# serf — prime-radiant's headless, multi-provider coding agent (native Ollama).
# Go workspace (go.work uses ./ + ./agent ./auth ./envvars ./llm …). We build ALL cmd/
# binaries (serf, serf-hub, serf-tui, llmcall, and the operational tools) — no subPackages
# restriction. Adopted as the fleet's ollama-local implementer, replacing claude-code+ccr.
buildGoModule rec {
  pname = "serf";
  version = "0-unstable-2026-07-22";

  src = fetchFromGitHub {
    owner = "prime-radiant-inc";
    repo = "serf";
    rev = "0b728736cf3e1aa41137e51a17e67b34c57c1108";
    hash = "sha256-coGVsFSGCZUl+bdOyUZreHptWAgihArRZ73nM+PG9QA=";
  };

  # go.work declares `go 1.25.6`; use the 1.25 toolchain and forbid network toolchain fetch.
  go = go_1_25;
  env.GOTOOLCHAIN = "local";

  # buildGoModule's default `go mod vendor` cannot run in workspace mode. Override the module
  # build phase to `go work vendor`, which vendors the whole go.work closure (main module + the
  # in-tree ./agent ./llm ./auth … sub-modules and all their external deps) into ./vendor. The
  # builder's install phase then copies ./vendor to $out for the vendorHash FOD.
  modBuildPhase = ''
    runHook preBuild
    go work vendor
    runHook postBuild
  '';

  # Vendored across the whole workspace.
  vendorHash = "sha256-SwP90urt1I8dBFgr7xRusULmnXhTMnMDxZIB/Yl0gxo=";

  # LLM/network-dependent tests; the grade path never runs these.
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
