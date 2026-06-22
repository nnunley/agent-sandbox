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

    # Declarative skills vendoring (STORY-0077 / STORY-0078). Kyure-A/agent-skills-nix
    # provides the selectSkills/mkBundle library; the upstream skills repo is a NON-FLAKE
    # source consumed as `flake = false` and curated by id. Both hash-pinned via flake.lock.
    agent-skills-nix.url = "github:Kyure-A/agent-skills-nix";
    agent-skills-nix.inputs.nixpkgs.follows = "nixpkgs";
    agent-skills = {
      url = "github:selamy-labs/agent-skills";
      flake = false;
    };
  };

  outputs = { self, nixpkgs, llm-agents, agent-skills-nix, agent-skills }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; config.allowUnfree = true; };
      agents = llm-agents.packages.${system};  # cached claude-code + alt-provider CLIs
      # let-go needs Go 1.26; fall back to default go if the pin is absent.
      goPkg = pkgs.go_1_26 or pkgs.go;

      # --- Curated skills bundle (STORY-0077 / STORY-0078) -----------------------------
      # The agent-skills-nix library (discoverCatalog / selectSkills / mkBundle).
      skillsLib = agent-skills-nix.lib.agent-skills;
      # Upstream layout, validated on the cluster 2026-06-22 (STORY-0078): a flat
      # skills/<name>/SKILL.md tree → subdir="skills", filter.maxDepth=1, no idPrefix.
      # `path` (not `input`) resolves the source directly, decoupled from agent-skills-nix's
      # own inputs. See docs/plans/2026-06-22-skills-layout-validation.md.
      skillSources = {
        upstream = {
          path = "${agent-skills}";
          subdir = "skills";
          filter.maxDepth = 1;
        };
      };
      skillCatalog = skillsLib.discoverCatalog skillSources;
      # The 13-skill curated subset (STORY-0077 AC-2). All confirmed present upstream.
      curatedSkillIds = [
        "using-laneq" "low-level-executor-task-spec" "process-aware-done"
        "verify-from-system-of-record" "verify-real-artifact" "gate-before-push"
        "graceful-shutdown-stateful-agents" "restart-resilience" "yield-on-wait"
        "push-over-polling" "credential-proxy" "context-anchored-patching"
        "agent-otel-trajectory"
      ];
      skillSelection = skillsLib.selectSkills {
        catalog = skillCatalog;
        sources = skillSources;
        allowlist = curatedSkillIds;
      };
      # mkBundle copy-trees ONLY the curated SKILL.md dirs (small derivation — no toolchain,
      # no golden), the standalone STORY-0078 gate `nix build .#agent-skills-bundle`.
      agentSkillsBundle = skillsLib.mkBundle {
        inherit pkgs;
        selection = skillSelection;
        name = "agent-skills-bundle";
      };
    in {
      # STORY-0078 standalone proof + STORY-0077 input: the curated skills bundle.
      packages.${system}.agent-skills-bundle = agentSkillsBundle;

      devShells.${system}.default = pkgs.mkShell {
        packages = [
          agents.claude-code    # headless `claude -p` CLI (numtide llm-agents.nix — cached, daily-updated)
          agents.lean-ctx       # context-intelligence layer (also from llm-agents.nix): MCP cheap/cached
                                #   reads (~13 tok) + map/signatures/density file modes + shell-output
                                #   compression. Subsumes rtk. Worker runs `lean-ctx init --agent claude`
                                #   so headless `claude -p` reads via ctx_* and Bash output is compressed.
          agents.claude-code-router  # `ccr` — Anthropic↔OpenAI translator so claude-code can
                                #   drive a local OpenAI-compatible model (Ollama qwen3.6 via the
                                #   llm-proxy /ollama route). Cheap-local-implementer tier (#22).
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
