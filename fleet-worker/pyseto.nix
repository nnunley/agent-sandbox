# fleet-worker/pyseto.nix
#
# ITER-0007c: pyseto (PASETO v4.public implementation) — NOT in nixpkgs (25.11),
# packaged here because laneq's grant-auth interceptor (laneq.auth / laneq.grpc_auth)
# imports it. Called from laneq.nix via python3Packages.callPackage.
#
# pyseto 1.9.3 (dajiaji/pyseto) uses the uv_build build backend; runtime deps
# argon2-cffi / cryptography / iso8601 / pycryptodomex are all in nixpkgs.
#
{ lib
, buildPythonPackage
, fetchPypi
, uv-build
, argon2-cffi
, cryptography
, iso8601
, pycryptodomex
}:

buildPythonPackage rec {
  pname = "pyseto";
  version = "1.9.3";
  pyproject = true;

  src = fetchPypi {
    inherit pname version;
    hash = "sha256-muLDJRaDkzlHymNubb96/kX3Jg7eW4k/wM3jMfjZIBA=";
  };

  nativeBuildInputs = [ uv-build ];

  propagatedBuildInputs = [
    argon2-cffi
    cryptography
    iso8601
    pycryptodomex
  ];

  # Upstream test suite pulls extra dev deps + vectors; an import check is enough
  # to prove the package + its native deps resolve (laneq's own tests exercise the API).
  doCheck = false;
  pythonImportsCheck = [ "pyseto" ];

  meta = {
    description = "A Python implementation of PASETO/PASERK";
    homepage = "https://github.com/dajiaji/pyseto";
    license = lib.licenses.mit;
    maintainers = [ ];
  };
}
