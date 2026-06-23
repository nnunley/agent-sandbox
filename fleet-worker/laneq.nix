# fleet-worker/laneq.nix
#
# ITER-0006b T1 Fix: laneq as buildPythonPackage (not buildPythonApplication).
# This enables both:
#   1. Console scripts (laneq-grpc, laneq, laneq-mcp) installed to $out/bin
#   2. Importable library (laneq.grpc.laneq_pb2_grpc, etc.) for clients via withPackages
#
# Regenerates proto stubs in-build with grpcio-tools from nixos-25.11 to ensure
# version compatibility (nixpkgs grpcio 1.76.0 vs fork's committed 1.81 stubs).
# Runs fork's gRPC handler tests (72 passed) as checkPhase to prove stub compatibility.
#
# Exposed:
#   - Console scripts: laneq-grpc (entry: laneq.grpc_server:main), laneq, laneq-mcp
#   - Importable: laneq.grpc.laneq_pb2_grpc (for gRPC clients via withPackages)
#
{ lib, pkgs, fetchFromGitHub, python3Packages }:

python3Packages.buildPythonPackage {
  pname = "laneq";
  version = "0.4.0";
  pyproject = true;  # laneq uses hatchling (pyproject.toml) build backend

  src = fetchFromGitHub {
    owner = "nnunley";
    repo = "laneq";
    rev = "2d1b59eb05641e65377c482752dff12b21c5e6f4";
    hash = "sha256-6/E1tTMdRnV9JLUhsIO/Y99tpysLeM/e7jbnwOno8iU=";
  };

  # Propagated deps: grpcio and protobuf (match what grpcio-tools was compiled against)
  propagatedBuildInputs = with python3Packages; [
    grpcio
    protobuf
  ];

  # Native build deps: grpcio-tools for in-build proto stub regeneration, hatchling for build,
  # and pytest/pytest-asyncio for running the fork's gRPC handler tests
  nativeBuildInputs = with python3Packages; [
    hatchling
    grpcio-tools
    pytest
    pytest-asyncio
  ];

  # Regenerate proto stubs in-build so they match the nixpkgs grpcio runtime
  # (committed stubs are from grpcio 1.81; nixos-25.11 has 1.76).
  # This overrides the committed stubs in src/laneq/grpc/laneq_pb2*.py files.
  postPatch = ''
    mkdir -p src/laneq/grpc
    # Create empty __init__.py if not present
    touch src/laneq/grpc/__init__.py

    # Regenerate proto stubs: -I proto locates the proto file;
    # --python_out and --grpc_python_out write to src/laneq/grpc
    python -m grpc_tools.protoc \
      -I proto \
      --python_out=src/laneq/grpc \
      --grpc_python_out=src/laneq/grpc \
      proto/laneq.proto

    # Fix import statement in laneq_pb2_grpc.py: grpcio-tools 1.76 generates
    # "import laneq_pb2", but we need "from . import laneq_pb2" for proper
    # package-relative imports.
    sed -i 's/^import laneq_pb2 as laneq__pb2/from . import laneq_pb2 as laneq__pb2/' \
      src/laneq/grpc/laneq_pb2_grpc.py
  '';

  # Run the fork's actual gRPC handler tests against the regenerated stubs.
  # This proves the regenerated stubs (grpcio-tools 1.76) are compatible with
  # the server's RPC handlers by running REAL handler logic, not just protobuf
  # serialization. Tests exercise Push, Take, Peek, and error cases against the
  # real LaneqServicer (grpc.aio). Verified: 72 passed in ~24s.
  #
  # NOTE on building: on the cluster's UNPRIVILEGED Incus LXC, `nix build` must
  # pass `--no-sandbox` (the LXC cannot set up Nix's sandbox mounts — established
  # cluster norm, see CLAUDE.md). This is an LXC limitation, NOT a package one:
  # the checkPhase uses only in-process grpc.aio (127.0.0.1 loopback, available
  # under a normal Nix sandbox) + a temp SQLite DB, so the package builds under
  # the default sandbox on a normal NixOS host.
  doCheck = true;
  checkPhase = ''
    echo "Running gRPC handler tests against regenerated stubs..."
    pytest -xvs tests/test_grpc_handlers.py tests/test_grpc_status_codes.py
  '';

  # Ensure laneq-grpc command is discoverable from entry point
  # (pyproject.toml: [project.scripts] laneq-grpc = "laneq.grpc_server:main")
  meta = {
    description = "gRPC server for laneq queue operations";
    license = lib.licenses.mit;
    maintainers = [ ];
  };
}
