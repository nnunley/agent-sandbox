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
        microvm.nixosModules.host
        ./host/configuration.nix
      ];
    };
  };
}
