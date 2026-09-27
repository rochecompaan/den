{ pkgs }:

let
  inherit (pkgs) lib;
  pname = "pi-coding-agent";
  version = "0.87.1";
  packageName = "@earendil-works/pi-coding-agent";
  tarballHash = "sha256-FCPuPGHnyWRk4cvzyNwk0wVss0EJlcNnGpjD7MUnVA8=";
  lockHash = "sha256-Cjn4JlMJY+XeEZYSBgoWLWvTZslBD0qnFk1HFzY9Za4=";
  npmDepsHash = "sha256-6INJmrxolx1pKeYMejhJdOuMHkas8mJJwOGfSaM4bDI=";
  patchHash = "sha256-po9h3qvFu3S1shL1oUQ1GMC7VAJwRy0nRPtQOK9k0AY=";
  toolResultPreviewPatchHash = "sha256-A8He1FEx49gnZ9FMVnWsqgKQaq5JrVeCvggxzwLLZkQ=";
  lock = ./pi-0.87.1-package-lock.json;
  patch = ../../patches/pi-0.87.1-den-hardening.patch;
  toolResultPreviewPatch = ./pi-tool-result-preview-dist.patch;
  actualLockHash = builtins.hashFile "sha256" lock;
  actualPatchHash = builtins.hashFile "sha256" patch;
  actualToolResultPreviewPatchHash = builtins.hashFile "sha256" toolResultPreviewPatch;
  expectedLockHash = builtins.convertHash {
    hash = lockHash;
    toHashFormat = "base16";
  };
  expectedPatchHash = builtins.convertHash {
    hash = patchHash;
    toHashFormat = "base16";
  };
  expectedToolResultPreviewPatchHash = builtins.convertHash {
    hash = toolResultPreviewPatchHash;
    toHashFormat = "base16";
  };
in
assert lib.assertMsg (lib.versionAtLeast pkgs.nodejs_22.version "22.19.0")
  "Den requires Node >=22.19.0; got ${pkgs.nodejs_22.version}";
assert lib.assertMsg (actualLockHash == expectedLockHash)
  "Pi package lock changed without updating its pinned hash";
assert lib.assertMsg (actualPatchHash == expectedPatchHash)
  "Pi hardening patch changed without updating its pinned hash";
assert lib.assertMsg (actualToolResultPreviewPatchHash == expectedToolResultPreviewPatchHash)
  "Pi tool-result preview patch changed without updating its pinned hash";
pkgs.buildNpmPackage (finalAttrs: {
  inherit pname version npmDepsHash;
  nodejs = pkgs.nodejs_22;
  src = pkgs.fetchurl {
    url = "https://registry.npmjs.org/@earendil-works/pi-coding-agent/-/pi-coding-agent-${version}.tgz";
    hash = tarballHash;
  };

  patches = [ patch toolResultPreviewPatch ];
  postPatch = ''
    rm npm-shrinkwrap.json
    cp ${lock} package-lock.json
  '';
  dontNpmBuild = true;
  doCheck = true;
  nativeBuildInputs = [ pkgs.makeWrapper ];
  checkPhase = ''
    runHook preCheck
    PI_PACKAGE_ROOT="$PWD" PI_PACKAGE_DIR="$PWD" \
      ${pkgs.nodejs_22}/bin/node ${./pi-tool-result-preview.test.mjs}
    runHook postCheck
  '';

  installPhase = ''
    runHook preInstall
    packageRoot="$out/lib/node_modules/${packageName}"
    mkdir -p "$packageRoot" "$out/bin"
    cp -R . "$packageRoot"
    makeWrapper ${pkgs.nodejs_22}/bin/node "$out/bin/pi" \
      --add-flags "$packageRoot/dist/cli.js"
    runHook postInstall
  '';

  passthru = {
    inherit actualLockHash actualPatchHash actualToolResultPreviewPatchHash lockHash npmDepsHash patchHash
      tarballHash toolResultPreviewPatchHash;
    nodejs = pkgs.nodejs_22;
    packageRoot = "${finalAttrs.finalPackage}/lib/node_modules/${packageName}";
    packageLock = lock;
    hardeningPatch = patch;
    inherit toolResultPreviewPatch;
  };

  meta = {
    description = "Den-hardened Pi coding agent";
    homepage = "https://github.com/badlogic/pi-mono";
    mainProgram = "pi";
    license = lib.licenses.mit;
  };
})
