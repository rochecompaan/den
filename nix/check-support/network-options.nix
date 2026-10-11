{ pkgs }:

let
  lib = pkgs.lib;
  normalize = import ../lib/network-options.nix { inherit lib; };
  fails = value: !(builtins.tryEval (builtins.deepSeq (normalize value) true)).success;
  repeat = count: lib.concatStrings (builtins.genList (_: "a") count);
  longHostname = lastLength: lib.concatStringsSep "." (map repeat [ 63 63 63 lastLength ]);
  acceptsDomain = value: !fails { extraAllowedDomains = [ value ]; };
  failures = lib.runTests {
    testDefaults = {
      expr = normalize { };
      expected = { extraAllowedDomains = [ ]; };
    };
    testPreservesValidInput = {
      expr = normalize {
        extraAllowedDomains = [ "api.example.com" "*.internal.example.com" "api.example.com" ];
      };
      expected = {
        extraAllowedDomains = [ "api.example.com" "*.internal.example.com" "api.example.com" ];
      };
    };
    testRejectsMalformedInputs = {
      expr = lib.all fails [
        null
        "api.example.com"
        [ ]
        true
        42
        { unexpected = true; }
        { extraAllowedDomains = null; }
        { extraAllowedDomains = "api.example.com"; }
        { extraAllowedDomains = [ 42 ]; }
        { extraAllowedDomains = [ true ]; }
        { extraAllowedDomains = [ null ]; }
        { extraAllowedDomains = [ [ "api.example.com" ] ]; }
        { extraAllowedDomains = [ { host = "api.example.com"; } ]; }
      ];
      expected = true;
    };
    testRejectsDomainSyntax = {
      expr = lib.all (value: fails { extraAllowedDomains = [ value ]; }) [
        "" " api.example.com" "api.example.com " "api.EXAMPLE.com" "api.example.com."
        "localhost" "api..example.com" "-api.example.com" "api-.example.com" "api_example.com"
        "127.0.0.1" "999.0.0.1" "::1" "[::1]" "api.123"
        "https://api.example.com" "api.example.com:443" "api.example.com/path"
        "api.example.com?query=1" "api.example.com#fragment" "*" "*example.com"
        "api.*.example.com" "*.*.example.com" "bücher.example" "api\n.example.com" "api\t.example.com"
      ];
      expected = true;
    };
    testHostnameLengthBoundaries = {
      expr = map acceptsDomain [
        "${repeat 63}.example.com"
        "${repeat 64}.example.com"
        (longHostname 61)
        (longHostname 62)
        "*.${longHostname 61}"
        "*.${longHostname 62}"
      ];
      expected = [ true false true false true false ];
    };
    testAcceptsAsciiNames = {
      expr = normalize { extraAllowedDomains = [ "api2.example.com" "xn--bcher-kva.example" ]; };
      expected = { extraAllowedDomains = [ "api2.example.com" "xn--bcher-kva.example" ]; };
    };
  };
in
assert lib.assertMsg (failures == [ ]) "network option tests failed: ${builtins.toJSON failures}";
pkgs.runCommand "network-options" { } ''
  touch "$out"
''
