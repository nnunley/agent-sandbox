{
  # let-go fleet worker environment — the declarative "golden" contents.
  # Pairs with the incus tricks from the headless-claude-fleet-incus playbook:
  # this flake describes WHAT the worker needs (claude CLI + toolchain);
  # incus handles delivery (git bundle + `incus file push`) and harvest (worker.diff).
  #
  # Worker usage inside a NixOS container (non-root `worker` user, shared /nix cache):
  #   nix develop /path/to/fleet-worker --command bash runner.sh
  # which puts `claude`, `go`, `git`, `make`, `jq` on PATH from a pinned closure.
  description = "let-go fleet worker: headless claude CLI + Go 1.26 toolchain";

  # Binary cache for the AI-agent CLIs (claude-code, lean-ctx) so an unprivileged
  # worker SUBSTITUTES them prebuilt instead of building (which fails in the nix
  # sandbox inside an unprivileged container — plan #27.2). Run the worker's
  # `nix develop` with --accept-flake-config to pick these up.
  nixConfig = {
    extra-substituters = [ "https://cache.numtide.com" ];
    extra-trusted-public-keys = [ "niks3.numtide.com-1:DTx8wZduET09hRmMtKdQDxNNthLQETkc/yaX7M4qK0g=" ];
  };

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    # Daily-updated, binary-cached (cache.numtide.com) AI-agent CLIs
    # (claude-code, codex, gemini-cli, qwen-code, ...). claude-code SUBSTITUTES
    # prebuilt -> no local build -> sidesteps the nix sandbox-build issue.
    # Run the worker's `nix develop` with --accept-flake-config to pick up its cache.
    llm-agents.url = "github:numtide/llm-agents.nix";
  };

  outputs = { self, nixpkgs, llm-agents }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; config.allowUnfree = true; };
      agents = llm-agents.packages.${system};  # cached claude-code + alt-provider CLIs
      # let-go needs Go 1.26; fall back to default go if the pin is absent.
      goPkg = pkgs.go_1_26 or pkgs.go;
    in {
      devShells.${system}.default = pkgs.mkShell {
        packages = [
          agents.claude-code    # headless `claude -p` CLI (numtide llm-agents.nix — cached, daily-updated)
          agents.lean-ctx       # context-intelligence layer (also from llm-agents.nix): MCP cheap/cached
                                #   reads (~13 tok) + map/signatures/density file modes + shell-output
                                #   compression. Subsumes rtk. Worker runs `lean-ctx init --agent claude`
                                #   so headless `claude -p` reads via ctx_* and Bash output is compressed.
          # agents.codex agents.gemini-cli agents.qwen-code  # alt-provider CLIs for cheap workers (#22)
          goPkg                 # Go 1.26 for the let-go build
          pkgs.gnumake          # `make generate` / `make check-generated`
          pkgs.git              # bundle clone + `git diff` harvest
          pkgs.coreutils
          pkgs.bashInteractive
          pkgs.jq               # stream-json fleet-filter
          pkgs.curl
          pkgs.ripgrep          # rg — plain fallback search (lean-ctx preferred)
          pkgs.ast-grep         # sg — structural search fallback
        ];
        # claude-code refuses --dangerously-skip-permissions as root; the worker
        # user must be non-root (enforced at container level, not here).
      };
    };
}
