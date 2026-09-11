{ pkgs, den, darwinPackages }:

let
  pi = den.packages.pi;
  mkPi = den.lib.mkPi;
  constructed = mkPi { };
in
assert builtins.hasAttr "pi" den.packages;
assert builtins.hasAttr "mkPi" den.lib;
assert pi.meta.mainProgram == "pi";
assert constructed.outPath == pi.outPath;
assert den.packages.default.outPath == den.packages.claude.outPath;
assert darwinPackages.x86_64.default.outPath == darwinPackages.x86_64.claude.outPath;
assert darwinPackages.aarch64.default.outPath == darwinPackages.aarch64.claude.outPath;
assert darwinPackages.x86_64.pi.meta.mainProgram == "pi";
assert darwinPackages.aarch64.pi.meta.mainProgram == "pi";
assert darwinPackages.x86_64.pi.denManifest != null;
assert darwinPackages.aarch64.pi.denManifest != null;
pkgs.runCommand "pi-package-api" { } ''
  test -x ${pi}/bin/pi
  test "$(find ${pi}/bin -mindepth 1 -maxdepth 1 -printf '%f\n')" = pi
  touch "$out"
''
