# fleet-worker/laneq.nix
#
# ITER-0006b T0: Nix package for laneq gRPC server (nnunley/laneq@2d1b59e grpc-binding).
# Regenerates proto stubs in-build with grpcio-tools from nixos-25.11 to ensure
# version compatibility (nixpkgs grpcio 1.76.0 vs fork's committed 1.81 stubs).
#
# Exposed command: laneq-grpc (maps to laneq.grpc_server:main entry point)
#
{ lib, pkgs, fetchFromGitHub, python3Packages }:

python3Packages.buildPythonApplication {
  pname = "laneq-grpc";
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
  # serialization. Tests exercise Push, Take, Peek, and error cases over the
  # in-process mock gRPC server.
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
