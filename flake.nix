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
  in {
    nixosConfigurations.agent-host = nixpkgs.lib.nixosSystem {
      inherit system;
      modules = [
        # Incus/LXC container support (boot.isContainer, filesystems, etc.)
        "${nixpkgs}/nixos/modules/virtualisation/lxc-container.nix"
        microvm.nixosModules.host
        ./host/configuration.nix
      ];
    };
  };
}
