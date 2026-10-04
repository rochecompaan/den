{ pkgs }:

let
  inherit (pkgs) lib;
  pname = "pi-coding-agent";
  version = "1.0.2";
  packageName = "@earendil-works/pi-coding-agent";
  tarballHash = "sha256-7aWueHU0O9kC/+VXGPtlskBtewOr7MXLidjkuwnO7aI=";
  lockHash = "sha256-Wgiqy9iKrXsLQqarwW33T80stN49CvbOu7/oMjl2kcA=";
  npmDepsHash = "sha256-09pPcE8QZLAezIm84ezQIdhAr385WvvvoVQNAv26Dio=";
  patchHash = "sha256-HsFdGyPLvJ7rVZr5OsmEpys3ilEL8HLVWEneL4vRCZ0=";
  toolResultPreviewPatchHash = "sha256-OaFDQQl4+8r3jFjzvCEjfxY15hHMFBI39w1WE/ZP7E0=";
  lock = ./pi-1.0.2-package-lock.json;
  patch = ../../patches/pi-1.0.2-den-hardening.patch;
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
    cp ${lock} package-lock.json
  '';
  dontNpmBuild = true;
  doCheck = true;
  nativeBuildInputs = [ pkgs.makeWrapper ];
  checkPhase = ''
    runHook preCheck
    PI_PACKAGE_ROOT="$PWD" PI_PACKAGE_DIR="$PWD" \
      ${pkgs.nodejs_22}/bin/node ${./pi-tool-result-preview.test.mjs}
    PI_PACKAGE_ROOT="$PWD" PI_PACKAGE_DIR="$PWD" \
      ${pkgs.nodejs_22}/bin/node ${./pi-session-persistence.test.mjs}
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
