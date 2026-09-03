{ pkgs }:

let
  lib = pkgs.lib;
  upstream = pkgs.fence;
  patch = ../../patches/fence-0.1.58-den-tmpdir.patch;
  expectedVersion = "0.1.58";
  expectedSourceHash = "sha256-ACe3N4bXYJW6QDQHtRChFWOTXTZTbEUbZ4d8cuFRqMY=";
  expectedPatchHash = "4be4f0266a0a79da10002893752ea8185915f6ecfb146513946bde8a96e41e2a";
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
      go test ./internal/sandbox -run '^(TestEnsureSandboxTMPDIRHonorsDenFenceTMPDIR|TestGenerateProxyEnvVars|TestWrapCommandMacOS_PinsSandboxExecAbsolutePath)$' -count=1
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
