# Project Network Domains Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let each project's Nix configuration add network domains for Claude or Pi, and allow exact `chatgpt.com` by default.

**Architecture:** Share domain validation across constructors and modules. The sandbox builder merges extra domains into an immutable base policy in the Nix store. Keep the existing manifest and launcher flow.

**Tech Stack:** Nix, flake-parts, Home Manager, devenv, Go, jq, and Den's pinned Fence 0.1.58.

**Spec:** `docs/specs/2026-10-11-project-network-domains-design.md`, approved at commit `9757c78`.

## Global Constraints

- Use `network.extraAllowedDomains` for both agents, with an empty list as the default.
- Add domains to the bundled allowlist. Do not replace it or change existing denials.
- Add only exact `chatgpt.com` to the defaults. Do not add `*.chatgpt.com`, `api.openai.com`, or other provider domains.
- Accept lowercase ASCII hostnames with at least two labels. Each label has at most 63 characters, and the hostname has at most 253 characters.
- Allow letters, digits, and hyphens in labels. Reject leading or trailing hyphens. The final label must contain a letter.
- Accept one leading `*.` wildcard prefix. Apply the hostname length limit after removing that prefix.
- Reject URLs, ports, IP addresses, empty values, whitespace, uppercase letters, trailing dots, single-label names, and other wildcard forms.
- Reject unknown `network` attributes and values that are not a list of strings. Do not normalize malformed input or resolve DNS.
- Permit duplicates. Remove duplicates when constructing the policy, with bundled entries first and extra entries in input order.
- Keep manifest version 2 and add no manifest fields. Change no production Go code, filesystem rules, command rules, socket grants, or localhost grants.
- Support `mkClaude`, `mkPi`, Home Manager, and devenv. Permissions belong to each built package, not its working directory.
- Keep the same domain behavior on Linux and macOS. Preserve existing platform-specific runtime transformations.
- Do not update Pi, Fence, flake inputs, source hashes, or dependencies.
- Stage new Nix files before git-backed flake checks. Do not create tests that merely inspect source text, configuration declarations, or documentation.
- Use the existing worktree. Do not merge, push, or start delegated work without the user's execution choice.

## Review Focus

- Lazy Nix evaluation can hide invalid domains behind an unforced result or a disabled agent. Task 3 tests forced constructor results and disabled modules.
- Non-string values and unknown attributes can produce unclear builtin errors. Task 2 tests malformed structures and Task 3 checks public error messages.
- Hostname limits can have off-by-one errors, especially after a wildcard prefix. Task 2 tests 63/64-character labels and 253/254-character hostnames.
- A broad or conflicting domain grant can appear to bypass protected hosts. Task 4 tests Fence decisions for Git hosts, metadata, and telemetry.
- Lists can leak between agents, reorder defaults, or change unrelated permissions. Task 3 tests distinct packages, duplicate inputs, and the complete non-allowlist policy.

## Workspace and verification setup

Run commands from `/home/roche/projects/den/.worktrees/project-network-domains` on branch `feat/project-network-domains`.
The source baseline is `993ebda`. The spec commit is `9757c78`.
Baseline `go test -json ./...` passed with 637 test results passing, two skipped, and none failing.

Set the local system once:

```sh
SYSTEM=$(nix eval --impure --raw --expr builtins.currentSystem)
```

Capture complete build and test output with context-mode, then summarize failures and exit codes.
A failed baseline or build environment is not permission to bypass a check.
Keep documentation verification separate from runtime verification.

## File structure and responsibilities

- Create `nix/lib/network-options.nix`: validate only the network configuration and return its normalized attribute set.
- Create `nix/check-support/network-options.nix`: behavioral evaluation tests for that helper.
- Create `modules/checks/network-policy.nix`: register `network-options` and, in Task 3, `network-policy` checks.
- Create `nix/check-support/network-policy-fixtures.nix`: shared default and customized constructor fixtures for policy checks.
- Create `nix/check-support/network-policy.nix`: inspect the policies that actual constructor manifests select.
- Modify `nix/lib/options.nix` and `nix/lib/pi-options.nix`: accept and normalize `network`.
- Modify `nix/lib/module-options.nix`: expose the list for both agents and force its validation through agent assertions.
- Modify `nix/lib/mk-claude.nix` and `nix/lib/mk-pi.nix`: forward normalized `network`.
- Modify `nix/lib/mk-agent-sandbox.nix`: select the bundled policy or construct the additive policy.
- Modify `nix/check-support/package-api.nix`: test validation when callers use the shared builder directly.
- Modify `nix/check-support/module-api.nix` and `nix/check-support/pi-module-api.nix`: exercise module forwarding and invalid input.
- Modify `internal/policy/network_test.go`: guard the existing launch-time preservation of project domains.
- Modify `policy/fence.json`, `policy/README.md`, and the existing Go allowlist expectation: add and document the exact default grant.
- Modify `nix/check-support/fence-domain-policy_test.go.fixture` and `modules/checks/fence-policy.nix`: test actual Fence domain decisions.
- Modify `README.md`: explain the public interface, domain syntax, package scope, and denial precedence.

Keep the validator focused and small. Keep the short additive merge beside its only production caller in `mk-agent-sandbox.nix`.
Do not split the merge into another module solely to wrap a few lines.
`internal/policy/policy_test.go` already has 541 lines. Update its existing expected list only, rather than add another test section there.

### Task 1: Allow exact chatgpt.com in the default policy

**Files:**
- Modify: `nix/check-support/fence-domain-policy_test.go.fixture:11-59`
- Modify: `policy/fence.json`, `network.allowedDomains`
- Modify: `internal/policy/policy_test.go:17-22`
- Modify: `policy/README.md`, Den transformations

**Interfaces:**
- Consumes: the bundled JSON and existing `TestDenPolicyDomainDecisions(t *testing.T)` using Fence's `CreateDomainFilter`.
- Produces: a default policy that permits exact `chatgpt.com` while preserving every other rule.

- [ ] **Step 1: Add failing permission-decision cases to the existing Fence test table.**

```go
{name: "ChatGPT default is allowed", host: "chatgpt.com", port: 443, allowed: true},
{name: "ChatGPT subdomains are not implicit", host: "unlisted.chatgpt.com", port: 443, allowed: false},
{name: "OpenAI API is not implicit", host: "api.openai.com", port: 443, allowed: false},
```

- [ ] **Step 2: Run the packaged Fence check and confirm the exact-domain case fails.**

Run: `nix build ".#checks.${SYSTEM}.fence-policy" --no-link --print-build-logs`
Expected: the `chatgpt.com` decision is false rather than true. A fetch or compiler error does not prove the missing permission.

- [ ] **Step 3: Append `chatgpt.com` to the bundled allowlist and the existing Go expected list.**

Change no denied entries or other fields. Add one transformation note to `policy/README.md`.
Keep its original authenticated-source revision and digest unchanged.

- [ ] **Step 4: Run the Fence check and existing policy tests.**

Run: `nix build ".#checks.${SYSTEM}.fence-policy" --no-link --print-build-logs`
Run: `go test ./internal/policy -count=1`
Expected: both commands exit 0, including the negative subdomain and OpenAI API decisions.

- [ ] **Step 5: Commit only the Task 1 files.**

Commit subject: `feat(policy): allow exact chatgpt.com by default`

### Task 2: Define and test the shared network validator

**Files:**
- Create: `nix/lib/network-options.nix`
- Create: `nix/check-support/network-options.nix`
- Create: `modules/checks/network-policy.nix`

**Interfaces:**
- Consumes: `{ lib }` and a raw Nix value.
- Produces: `normalizeNetwork = import ./network-options.nix { inherit lib; };`
- `normalizeNetwork rawNetwork` returns `{ extraAllowedDomains = listOfStrings; }`. It returns the original valid list, including order and duplicates.
- `normalizeNetwork { }` returns `{ extraAllowedDomains = [ ]; }`.
- Invalid input throws an assertion naming `network.extraAllowedDomains` and the invalid entry or index. Invalid root values or unknown attributes identify the invalid field.
- Check support signature: `{ pkgs }: derivation`. The module registers `checks.network-options`.

- [ ] **Step 1: Write named Nix behavioral cases using `lib.runTests` and the forced-failure helper below.**

```nix
normalize = import ../lib/network-options.nix { inherit (pkgs) lib; };
fails = value: !(builtins.tryEval (builtins.deepSeq (normalize value) true)).success;

testDefaults = { expr = normalize { }; expected = { extraAllowedDomains = [ ]; }; };
testPreservesValidInput = {
  expr = normalize { extraAllowedDomains = [ "api.example.com" "*.internal.example.com" "api.example.com" ]; };
  expected = { extraAllowedDomains = [ "api.example.com" "*.internal.example.com" "api.example.com" ]; };
};
```

`testRejectsMalformedInputs` must call `fails` on `null`, a string, a list, `{ unexpected = true; }`, and malformed `extraAllowedDomains` values.
Use `null`, a string instead of a list, and lists containing an integer, Boolean, null, or nested list.

`testRejectsDomainSyntax` must reject these strings:

```nix
[ "" " api.example.com" "api.example.com " "api.EXAMPLE.com" "api.example.com."
  "localhost" "api..example.com" "-api.example.com" "api-.example.com" "api_example.com"
  "127.0.0.1" "999.0.0.1" "::1" "[::1]" "api.123"
  "https://api.example.com" "api.example.com:443" "api.example.com/path"
  "api.example.com?query=1" "api.example.com#fragment" "*" "*example.com"
  "api.*.example.com" "*.*.example.com" ]
```

`testHostnameLengthBoundaries` must accept a 63-character label, reject a 64-character label, accept a 253-character hostname, and reject 254 characters.
Build the long hostnames from label lengths `[ 63 63 63 61 ]` and `[ 63 63 63 62 ]`, joined with dots.
Also accept `*.` plus the 253-character hostname, and reject `*.` plus the 254-character hostname.
`testAcceptsAsciiNames` must accept `api2.example.com` and `xn--bcher-kva.example`.

- [ ] **Step 2: Register and stage the new check files, then run the red check.**

Run: `git add modules/checks/network-policy.nix nix/check-support/network-options.nix`
Run: `nix build ".#checks.${SYSTEM}.network-options" --no-link --print-build-logs`
Expected: evaluation fails because the referenced validator does not exist yet.

- [ ] **Step 3: Implement the defined `normalizeNetwork` interface in `nix/lib/network-options.nix`.**

Assert structure before reading fields. Validate each string against the Global Constraints.
Remove one leading wildcard prefix only for hostname validation. Do not change the returned strings or list.
Use an entry index or JSON-quoted string in errors so malformed text cannot add raw diagnostic lines.

- [ ] **Step 4: Stage the helper and run the behavioral check.**

Run: `git add nix/lib/network-options.nix`
Run: `nix build ".#checks.${SYSTEM}.network-options" --no-link --print-build-logs`
Expected: exit 0, with `lib.runTests` returning no failed cases.

- [ ] **Step 5: Commit only the Task 2 files.**

Commit subject: `feat(network): validate additive domain configuration`

### Task 3: Wire package and module configuration to immutable policies

**Files:**
- Modify: `nix/lib/options.nix:6-85`, `nix/lib/pi-options.nix:6-105`
- Modify: `nix/lib/module-options.nix:53-137`
- Modify: `nix/lib/mk-claude.nix:3,43-44`, `nix/lib/mk-pi.nix:3,74-76`
- Modify: `nix/lib/mk-agent-sandbox.nix:3-8,84-105`
- Create: `nix/check-support/network-policy-fixtures.nix`
- Create: `nix/check-support/network-policy.nix`
- Modify: `modules/checks/network-policy.nix`
- Modify: `nix/check-support/package-api.nix`, `nix/check-support/module-api.nix`, `nix/check-support/pi-module-api.nix`
- Modify: `internal/policy/network_test.go`

**Interfaces:**
- Consumes: Task 2's `normalizeNetwork rawNetwork` result.
- Constructors accept `network ? { }` and forward `options.network` to the shared builder.
- The shared builder accepts `network ? { }` and exposes normalized `denOptions.network`.
- `programs.den.{claude,pi}.network.extraAllowedDomains` is `types.listOf types.str`, defaults to `[ ]`, and uses the shared validator.
- Preserve exported `assertions = claude: ...` and `piAssertions = pi: ...`. Force domain validation there without invoking a disabled agent's constructor.
- Fixture signature: `{ pkgs, den }:` returns `{ defaults = { claude; pi; }; custom = { claude; pi; }; extraDomains = { claude; pi; }; }`.
- Fixture `den` is the root flake, so constructors use `den.lib.${pkgs.system}.mkClaude` and `mkPi`.
- Policy check signature: `{ pkgs, den }: derivation`, registered as `checks.network-policy`.
- Production manifest and Go interfaces do not change.

- [ ] **Step 1: Create the constructor fixtures and generated-policy behavior check.**

Use this common input, then append `claude.project.test` for Claude and `pi.project.test` for Pi:

```nix
[ "api.project.test" "*.services.project.test" "registry.npmjs.org" "api.project.test"
  "github.com" "*.github.com" "githubusercontent.com" "*.githubusercontent.com"
  "gitlab.com" "*.gitlab.com" "bitbucket.org" "*.bitbucket.org"
  "metadata.google.internal" "statsig.anthropic.com" ]
```

Read `basePolicy` from each actual `denManifest`, then load that JSON with jq.
The check must prove these outcomes for each agent:

- The allowlist equals the bundled list followed by unique new entries in their original order.
- `api.project.test` occurs once, and the already bundled `registry.npmjs.org` occurs once.
- The package has its own agent domain but not the other agent's domain.
- Deleting `network.allowedDomains` from default and custom JSON produces identical documents.
- Explicitly empty network lists select the default policy path and retain the default constructor output path.
- Invalid input fails after forcing `.outPath`, not just after reading the network field. Use uppercase, a URL, a non-string list entry, and an unknown network attribute.

Register `checks.network-policy` in the module from Task 2.
Use jq comparisons of produced policy data, not searches of Nix source.

- [ ] **Step 2: Extend the existing public-boundary suites with behavior cases.**

Configure non-empty network lists in their current enabled fixtures and compare selected packages with the corresponding direct constructor.
Give Claude and Pi different domains in the combined Home Manager and devenv fixtures.
Retain the existing disabled-constructor sentinels.

Force invalid disabled-agent values and exported assertions with `builtins.deepSeq`.
Test `API.example.com`, `https://api.example.com`, a non-string entry, and an unknown network attribute for both module integrations.
Expected: invalid configuration fails, while valid disabled agents do not call constructors.

In `package-api.nix`, add a test fixture `networkSandbox = network: derivation` using the existing `mkAgentSandbox`, `adapter.adapter`, and `dependencies`.
Pass `configDir = null`, `extraPkgs = [ ]`, `docker = { }`, `podman = { }`, and the fixture's network argument.
Assert that a forced `.outPath` fails for a URL, a non-string entry, and an unknown network attribute.
Keep the existing builder fixtures that omit `network` as backward-compatibility checks.

- [ ] **Step 3: Add `TestGenerateRetainsProjectDomains(t *testing.T)` to `internal/policy/network_test.go`.**

Use the existing `makePaths`, `readBase`, and resolver fixture pattern.
Add `api.project.test` and `*.services.project.test` to a decoded copy of the bundled base.
Create `dynamic := testDynamic(paths)`, then set `dynamic.Platform` and `dynamic.RepoWolfHostname`.
Call the existing `Generate(Base, Dynamic)` for `linux` and `darwin` with broker `broker.example.test`.
Decode the result into `document` and assert:

```go
for _, host := range []string{"api.project.test", "*.services.project.test", "broker.example.test"} {
    if !contains(got.Network.AllowedDomains, host) {
        t.Errorf("generated policy lost %q", host)
    }
}
if !reflect.DeepEqual(got.Network.DeniedDomains, deniedDomains) {
    t.Fatal("generated policy changed denied domains")
}
```

Restore `resolveResolvConf` with `t.Cleanup`. Do not parallelize a test that changes this shared fixture.
This is a guard for existing Go behavior, so it can pass before Nix wiring exists.

- [ ] **Step 4: Stage new files and run the integration checks before production changes.**

Run: `git add modules/checks/network-policy.nix nix/check-support/network-policy.nix nix/check-support/network-policy-fixtures.nix`
Run: `nix build ".#checks.${SYSTEM}.network-policy" ".#checks.${SYSTEM}.package-api" ".#checks.${SYSTEM}.module-api" ".#checks.${SYSTEM}.pi-module-api" --no-link --print-build-logs`
Expected: constructors or modules reject the new `network` configuration. Keep that failure as the missing-feature evidence.

- [ ] **Step 5: Normalize and forward `network` through both constructors and the shared builder.**

Add `network` to each normalizer's accepted root fields and return the shared helper's result.
Add the optional constructor arguments and forward the normalized value.
The builder must also validate its own optional input because test adapters call it directly.

If extras are empty, use `../../policy/fence.json` as before.
Otherwise, read that JSON and create `pkgs.writeText "den-fence-policy.json"`.
Set only `network.allowedDomains` to `lib.unique (base.network.allowedDomains ++ options.network.extraAllowedDomains)`.
Preserve both nested and top-level fields when constructing the new JSON.
Point the existing manifest's `basePolicy` at this selected file. Add no runtime inputs.

- [ ] **Step 6: Expose and force shared module validation.**

Define a shared declaration for both agents with `types.listOf types.str` and default `[ ]`.
Its `apply` uses `(normalizeNetwork { extraAllowedDomains = domains; }).extraAllowedDomains`.
Extend the existing agent assertion functions to force that evaluated list, including when the agent is disabled.
Keep Home Manager and devenv forwarding through their existing module files.

- [ ] **Step 7: Run the integration checks and Go policy tests.**

Run: `gofmt -w internal/policy/network_test.go`
Run: `nix build ".#checks.${SYSTEM}.network-options" ".#checks.${SYSTEM}.network-policy" ".#checks.${SYSTEM}.package-api" ".#checks.${SYSTEM}.pi-package-api" ".#checks.${SYSTEM}.module-api" ".#checks.${SYSTEM}.pi-module-api" --no-link --print-build-logs`
Run: `go test ./internal/policy -count=1`
Expected: all commands exit 0.

Directly evaluate this malformed constructor and inspect the error:

```sh
nix eval --impure --json --show-trace --expr \
  "((builtins.getFlake (toString ./.)).lib.${SYSTEM}.mkPi {
    network.extraAllowedDomains = [ \"https://api.example.com\" ];
  }).outPath"
```

Expected: nonzero exit and an error naming `network.extraAllowedDomains` plus the offending value or its index.
Repeat the command with `mkClaude` instead of `mkPi`. This diagnostic inspection does not replace the behavioral checks.

- [ ] **Step 8: Commit only the Task 3 files.**

Commit subject: `feat(network): add project domain grants to sandbox packages`

### Task 4: Verify Fence decisions and document the feature

**Files:**
- Modify: `nix/check-support/fence-domain-policy_test.go.fixture`
- Modify: `modules/checks/fence-policy.nix`
- Modify: `README.md`, constructor, devenv, and Pi configuration sections

**Interfaces:**
- Consumes: Task 3's `network-policy-fixtures.nix` and each package's existing `denManifest.basePolicy`.
- Produces: `TestDenProjectPolicyDomainDecisions(t *testing.T)` using Fence's real `CreateDomainFilter`.
- Check-only inputs: `DEN_POLICY_PATH` selects a derived policy and `DEN_PROJECT_DOMAIN` identifies its agent-specific exact hostname.
- These variables exist only in the check process. They are not new launcher overrides.

- [ ] **Step 1: Add the project-policy consumer test.**

Load the selected policy with Fence's `config.Load` and use `CreateDomainFilter(cfg, false)`.
Do not append project domains inside the test or write another domain matcher.
Test this table with port 443:

```go
{host: "api.project.test", allowed: true},
{host: os.Getenv("DEN_PROJECT_DOMAIN"), allowed: true},
{host: "worker.services.project.test", allowed: true},
{host: "unlisted.project.test", allowed: false},
{host: "github.com", allowed: false},
{host: "api.github.com", allowed: false},
{host: "raw.githubusercontent.com", allowed: false},
{host: "gitlab.com", allowed: false},
{host: "api.gitlab.com", allowed: false},
{host: "bitbucket.org", allowed: false},
{host: "api.bitbucket.org", allowed: false},
{host: "metadata.google.internal", allowed: false},
{host: "169.254.169.254", allowed: false},
{host: "statsig.anthropic.com", allowed: false},
```

Assert that the other agent's domain is denied in each run.
Require both check-only inputs instead of silently skipping a missing fixture.
Keep Task 1's default cases and the existing broker-IP cases in their original test.

- [ ] **Step 2: Run the consumer test against both generated constructor policies.**

Import the shared fixtures in `modules/checks/fence-policy.nix` using the root flake as `den`.
Keep the existing default-policy test invocation.
For each custom constructor, read its manifest's `basePolicy` and run the new test with the matching agent domain.
Reuse the check's pinned Fence source, vendored dependencies, and existing fixture-copy mechanism.

Run: `nix build ".#checks.${SYSTEM}.fence-policy" --no-link --print-build-logs`
Expected: default and both custom-policy tests pass without contacting public services.
No new production change is expected in this task. These tests close the downstream-consumer coverage gap.

- [ ] **Step 3: Update the README using the approved spec's examples.**

Document the nested list for `mkClaude`, `mkPi`, Home Manager, and devenv.
Explain accepted hostname and wildcard syntax, denied-host precedence, and the exact `chatgpt.com` default.
Explain that each built package owns its permissions and that a Home Manager package applies them in every working directory.
State that repository discovery, environment overrides, other policy customization, and DNS resolution during evaluation are absent.
Warn readers to review project Nix code before evaluation. Do not claim that immutable policy makes untrusted Nix safe.
Do not add documentation-content tests. Inspect the examples and run the corresponding existing module and package checks.

- [ ] **Step 4: Stage all remaining intended files and run final verification.**

```sh
git add README.md modules/checks/fence-policy.nix nix/check-support/fence-domain-policy_test.go.fixture
git diff --cached --check
go test ./... -count=1
nix build ".#checks.${SYSTEM}.network-options" ".#checks.${SYSTEM}.network-policy" ".#checks.${SYSTEM}.fence-policy" ".#checks.${SYSTEM}.package-api" ".#checks.${SYSTEM}.pi-package-api" ".#checks.${SYSTEM}.module-api" ".#checks.${SYSTEM}.pi-module-api" --no-link --print-build-logs
nix flake check --all-systems --no-build --accept-flake-config
nix flake check --accept-flake-config --print-build-logs
```

Expected: every available command exits 0. Record skips and environment blockers rather than calling them passes.
Run the corresponding checks on a supported macOS builder before claiming macOS runtime verification.
If no macOS builder is available, report that limitation and leave its runtime checks to CI.
A successful all-systems evaluation alone is not a macOS runtime test.
No Pi extension-load update check is required because this feature changes no Pi version, dependency, or packaged extension.

- [ ] **Step 5: Commit only the Task 4 files.**

Commit subject: `test(network): verify project domain decisions with Fence`

## Plan review and execution handoff

The plan has four sequential tasks. The validator, package construction, and consumer tests share tightly linked interfaces.
Native execution is the recommended starting choice for this scope.
The user must review this plan and choose native or subagent-driven execution before implementation starts.

For native execution, use `executing-plans`, implement in this session, and request one fresh whole-branch review after implementation.
For authorized subagent-driven execution, use `subagent-driven-development` and the canonical `reviewer`, not `code-reviewer`.
Keep one writer in this checkout. A writing child receives the required implementation and verification skills.
Read-only review work receives the artifact, spec, plan, base `993ebda`, head reference, and acceptance criteria.

After implementation and review, offer a local squash merge into `main`, a pull request, or leaving the branch as it is.
Do not integrate or push the branch until the user chooses.
