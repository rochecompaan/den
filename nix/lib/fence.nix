{ pkgs }:

let
  lib = pkgs.lib;
  upstream = pkgs.fence;
  patch = ../../patches/fence-0.1.58-den-tmpdir.patch;
  readOnlyMaskPatch = ../../patches/fence-0.1.58-den-readonly-mask.patch;
  bootstrapAttestationPatch = ../../patches/fence-0.1.58-den-bootstrap-attestation.patch;
  macosNestedDenyPatch = ../../patches/fence-0.1.58-den-macos-nested-deny.patch;
  expectedVersion = "0.1.58";
  expectedSourceHash = "sha256-ACe3N4bXYJW6QDQHtRChFWOTXTZTbEUbZ4d8cuFRqMY=";
  expectedPatchHash = "4be4f0266a0a79da10002893752ea8185915f6ecfb146513946bde8a96e41e2a";
  expectedReadOnlyMaskPatchHash = "750d15f9c6eee70f916de3044c60d937f9d67c39c0178da1a4da8acf8d6b990b";
  expectedBootstrapAttestationPatchHash = "d1f3f8d31ceb276998e5df0dd476e9e56ea7cd79517cf2bc0330bb498036c15f";
  expectedMacosNestedDenyPatchHash = "194ed4989bc0ce8ebf2b3f1a73cad85f911bb39dede4c4fd7f343a10cf9e21ee";
  upstreamPatches = upstream.patches or [ ];
  patchHash = builtins.hashFile "sha256" patch;
  readOnlyMaskPatchHash = builtins.hashFile "sha256" readOnlyMaskPatch;
  bootstrapAttestationPatchHash = builtins.hashFile "sha256" bootstrapAttestationPatch;
  macosNestedDenyPatchHash = builtins.hashFile "sha256" macosNestedDenyPatch;

  capabilities = {
    settings = true;
    claudePreToolUse = true;
    commandWrapper = true;
    linuxFeatures = true;
    exposeHostPath = true;
    denFenceTmpdir = true;
    strictDenyRead = true;
    linuxReadOnlyDenyReadMasks = true;
    argvRuntimePolicy = true;
    attestedBootstrapTransitions = true;
    allowUnixSockets = true;
    allowLocalOutboundPorts = true;
    darwinNestedDenyCarveouts = true;
  };

  patched = upstream.overrideAttrs (old: {
    patches = (old.patches or [ ]) ++ [ patch readOnlyMaskPatch bootstrapAttestationPatch macosNestedDenyPatch ];
    checkPhase = ''
      runHook preCheck
      go test ./cmd/fence -count=1
      go test ./internal/sandbox -run '^(TestEnsureSandboxTMPDIRHonorsDenFenceTMPDIR|TestGenerateProxyEnvVars|TestWrapCommandMacOS_PinsSandboxExecAbsolutePath|TestGenerateReadRules_CarvesNestedDeniedPathsOutOfAllow|TestGenerateWriteRules_CarvesNestedDeniedPathOutOfAllow|TestGenerateSandboxProfile_ReadOnlyDenyKeepsSpecificMoveProtection|TestLinuxLateMountPlanner_MaskedAncestorWinsOverChildReadOnly|TestLinuxLateMountPlanner_ReadOnlyThenMaskDirPreservesReadOnlyMask|TestWrapCommandLinuxWithOptions_DenyReadDirectoryWinsOverSamePathDenyWrite|TestWrapCommandLinuxWithOptions_DenyReadDirectoryWinsOverChildDenyWrite)$' -count=1
      go test ./internal/sandbox -run '^(TestEvaluateLinuxRuntimeExecDecisionForCandidate_(OrdinaryExecStillChecksContinueSafety|AttestedBootstrapSequenceSurvivesThreadCountVariation|RejectsInvalidBootstrapAttestations|RejectsOutOfOrderBootstrapTransition|RejectsDuplicateBootstrapTransition)|TestSendLinuxRuntimeExecDecision_CommitsOnlyAfterSuccessfulResponse|TestValidateLinuxArgvExecBootstrapTransitionsRejectsInvalidSequence|TestReadLinuxBootstrapProcessInfoAt_(VerifiesExecutableIdentityAndArgv|RejectsOversizedCmdline)|TestParseLinuxProcCmdlineRejectsMalformedInput|TestIsLinuxBootstrapExecPath_OnlyAllowsStagedExecutables)$' -count=1
      runHook postCheck
    '';
    passthru = (old.passthru or { }) // {
      denFenceCapabilities = capabilities;
      denFencePatchHash = patchHash;
      denFenceReadOnlyMaskPatchHash = readOnlyMaskPatchHash;
      denFenceBootstrapAttestationPatchHash = bootstrapAttestationPatchHash;
      denFenceMacosNestedDenyPatchHash = macosNestedDenyPatchHash;
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
assert lib.assertMsg (readOnlyMaskPatchHash == expectedReadOnlyMaskPatchHash)
  "Den Fence read-only mask patch hash drifted";
assert lib.assertMsg (bootstrapAttestationPatchHash == expectedBootstrapAttestationPatchHash)
  "Den Fence bootstrap attestation patch hash drifted";
assert lib.assertMsg (macosNestedDenyPatchHash == expectedMacosNestedDenyPatchHash)
  "Den Fence macOS nested deny patch hash drifted";
assert lib.assertMsg (builtins.all (value: value) (builtins.attrValues capabilities))
  "Den Fence ${expectedVersion} lacks a required capability";
{
  package = patched;
  inherit capabilities patchHash readOnlyMaskPatchHash bootstrapAttestationPatchHash macosNestedDenyPatchHash;
  version = upstream.version;
  sourceHash = upstream.src.outputHash;
  patchCount = builtins.length patched.patches;
}
