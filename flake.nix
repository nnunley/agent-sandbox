{
  description = "Agent Sandbox: Firecracker micro-VMs on Incus via NixOS";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    microvm = {
      url = "github:microvm-nix/microvm.nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, microvm }: let
    system = "x86_64-linux";
    pkgs = nixpkgs.legacyPackages.${system};
    linuxPkgs = pkgs; # NixOS container workers read from a shared nix store on Linux
  in {
    # Project Nix packages (built on a Nix host — the macOS dev box has no Nix; see the
    # `nix-packaging` skill). cxdb = the StrongDM cxdb context store, S3-sync feature-gated off.
    packages.${system}.cxdb = pkgs.callPackage ./pkgs/cxdb { };

    nixosConfigurations.agent-host = nixpkgs.lib.nixosSystem {
      inherit system;
      modules = [
        # Incus/LXC container support (boot.isContainer, filesystems, etc.)
        "${nixpkgs}/nixos/modules/virtualisation/lxc-container.nix"
        microvm.nixosModules.host
        ./host/configuration.nix
      ];
    };

    # devShell for dispatcher development and worker toolchain.
    # This closure is built once and populated into the shared nix store volume,
    # allowing NixOS workers to access tools without repeating builds.
    devShells.${system}.default = linuxPkgs.mkShell {
      name = "dispatcher-dev";
      description = "Dispatcher development shell with git, go, gnumake";
      buildInputs = with linuxPkgs; [
        git
        go
        gnumake
        pkg-config
        bash
        perl # dev tooling (patch authoring, text munging) — e.g. cxdb packaging edits
      ];
    };
  };
}
