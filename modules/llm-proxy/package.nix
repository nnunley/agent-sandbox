{ lib, buildGoModule }:

buildGoModule {
  pname = "llm-proxy";
  version = "0.1.1";

  src = ./.;

  vendorHash = null; # no external deps — stdlib only

  # Run the test suite at build time. Catches regressions before deploy.
  doCheck = true;

  meta = {
    description = "Path-prefix reverse proxy with API key injection and JSONL logging";
    license = lib.licenses.mit;
    mainProgram = "llm-proxy";
  };
}
