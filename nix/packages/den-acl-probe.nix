{ lib, stdenv }:

stdenv.mkDerivation {
  pname = "den-acl-probe";
  version = "0.1.0";
  src = ./den-acl-probe;

  strictDeps = true;

  buildPhase = ''
    runHook preBuild
    $CC -std=c11 -Wall -Wextra -Werror -o den-acl-probe main.c
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    install -Dm755 den-acl-probe "$out/bin/den-acl-probe"
    runHook postInstall
  '';

  meta.platforms = lib.platforms.darwin;
}
