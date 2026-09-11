{ pkgs }:

let
  inherit (pkgs) lib;
  pname = "pi-coding-agent";
  version = "0.84.4";
  packageName = "@earendil-works/pi-coding-agent";
  tarballHash = "sha256-W852bRnDzroY8/uq2RxEnJ+dc5gfnjQA7O+TIAbwaWg=";
  lockHash = "sha256-/xfQaHHRD9Riiv+hqSHfFzvx+GBeByzCqgpO5Oi0cc4=";
  npmDepsHash = "sha256-rSUYLw/RoIZ2f6gMwpSUdDqGECFUnm0KnNu/uCLYbpE=";
  patchHash = "sha256-0DVX2CG8Wrcz1yG5kHp/hBIL51r4UwmuAe+RdlmI7jE=";
  lock = ./pi-0.84.4-package-lock.json;
  patch = ../../patches/pi-0.84.4-den-hardening.patch;
  actualLockHash = builtins.hashFile "sha256" lock;
  actualPatchHash = builtins.hashFile "sha256" patch;
  expectedLockHash = builtins.convertHash {
    hash = lockHash;
    toHashFormat = "base16";
  };
  expectedPatchHash = builtins.convertHash {
    hash = patchHash;
    toHashFormat = "base16";
  };
in
assert lib.assertMsg (lib.versionAtLeast pkgs.nodejs_22.version "22.19.0")
  "Den requires Node >=22.19.0; got ${pkgs.nodejs_22.version}";
assert lib.assertMsg (actualLockHash == expectedLockHash)
  "Pi package lock changed without updating its pinned hash";
assert lib.assertMsg (actualPatchHash == expectedPatchHash)
  "Pi hardening patch changed without updating its pinned hash";
pkgs.buildNpmPackage (finalAttrs: {
  inherit pname version npmDepsHash;
  nodejs = pkgs.nodejs_22;
  src = pkgs.fetchurl {
    url = "https://registry.npmjs.org/@earendil-works/pi-coding-agent/-/pi-coding-agent-${version}.tgz";
    hash = tarballHash;
  };

  patches = [ patch ];
  postPatch = ''
    rm npm-shrinkwrap.json
    cp ${lock} package-lock.json
  '';
  dontNpmBuild = true;
  nativeBuildInputs = [ pkgs.makeWrapper ];

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
    inherit actualLockHash actualPatchHash lockHash npmDepsHash patchHash tarballHash;
    nodejs = pkgs.nodejs_22;
    packageRoot = "${finalAttrs.finalPackage}/lib/node_modules/${packageName}";
    packageLock = lock;
    hardeningPatch = patch;
  };

  meta = {
    description = "Den-hardened Pi coding agent";
    homepage = "https://github.com/badlogic/pi-mono";
    mainProgram = "pi";
    license = lib.licenses.mit;
  };
})
