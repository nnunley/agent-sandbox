{ lib, buildGoModule }:

buildGoModule {
  pname = "llm-proxy";
  version = "0.1.0";

  src = ./.;

  vendorHash = null; # no external deps — stdlib only

  meta = {
    description = "Path-prefix reverse proxy with API key injection and JSONL logging";
    license = lib.licenses.mit;
    mainProgram = "llm-proxy";
  };
}
