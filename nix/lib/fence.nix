{ pkgs }:

let
  lib = pkgs.lib;
  upstream = pkgs.fence;
  patch = ../../patches/fence-0.1.58-den-tmpdir.patch;
  expectedVersion = "0.1.58";
  expectedSourceHash = "sha256-ACe3N4bXYJW6QDQHtRChFWOTXTZTbEUbZ4d8cuFRqMY=";
  expectedPatchHash = "578d1ab068cebfa1acbb39c0d452f455875f5b190b2e33774660426288e378f3";
  upstreamPatches = upstream.patches or [ ];
  patchHash = builtins.hashFile "sha256" patch;

  capabilities = {
    settings = true;
    claudePreToolUse = true;
    commandWrapper = true;
    linuxFeatures = true;
    exposeHostPath = true;
    denFenceTmpdir = true;
    strictDenyRead = true;
    argvRuntimePolicy = true;
    allowUnixSockets = true;
    allowLocalOutboundPorts = true;
  };

  patched = upstream.overrideAttrs (old: {
    patches = (old.patches or [ ]) ++ [ patch ];
    checkPhase = ''
      runHook preCheck
      go test ./cmd/fence -count=1
      go test ./internal/sandbox -run '^(TestEnsureSandboxTMPDIRHonorsDenFenceTMPDIR|TestGenerateProxyEnvVars)$' -count=1
      runHook postCheck
    '';
    passthru = (old.passthru or { }) // {
      denFenceCapabilities = capabilities;
      denFencePatchHash = patchHash;
    };
  });
in
assert lib.assertMsg (upstream.version == expectedVersion)
  "Den requires Fence ${expectedVersion}; refusing unknown version ${upstream.version}";
assert lib.assertMsg ((upstream.src.outputHash or null) == expectedSourceHash)
  "Den Fence ${expectedVersion} source hash drifted";
assert lib.assertMsg (builtins.length upstreamPatches == 0)
  "Den Fence ${expectedVersion} expected no upstream package patches";
assert lib.assertMsg (patchHash == expectedPatchHash)
  "Den Fence TMPDIR patch hash drifted";
assert lib.assertMsg (builtins.all (value: value) (builtins.attrValues capabilities))
  "Den Fence ${expectedVersion} lacks a required capability";
{
  package = patched;
  inherit capabilities patchHash;
  version = upstream.version;
  sourceHash = upstream.src.outputHash;
  patchCount = builtins.length patched.patches;
}
