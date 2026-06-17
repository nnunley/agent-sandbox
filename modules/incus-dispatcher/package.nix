{ lib, buildGoModule }:

buildGoModule rec {
  pname = "incus-dispatcher";
  version = "0.1.0";

  src = ./.;

  # No external dependencies — stdlib only
  vendorHash = null;

  # Run the test suite at build time
  doCheck = true;

  # Skip tests that require a live incus remote
  checkFlags = [ "-short" ];

  meta = {
    description = "Ephemeral Incus container launcher for running isolated tasks";
    license = lib.licenses.mit;
    mainProgram = "incus-dispatcher";
  };
}
