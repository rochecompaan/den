# Project-specific network domains

Date: 2026-10-11

Status: Conversational design approved. Written specification awaits user review.

## Purpose and agreed scope

Projects need access to extra network domains without changing Den's shared permissions for every project.
The first version adds network domains through Nix configuration.
It supports project development shells through devenv and direct constructors.
Home Manager exposes the same configuration.

The user approved these requirements:

- Add `network.extraAllowedDomains` for Claude and Pi.
- Add project domains to the bundled allowlist, rather than replace it.
- Create the package policy at build time, rather than add launcher inputs.
- Accept lowercase hostnames and leading `*.` wildcard patterns.
- Reject URLs, ports, IP addresses, and a bare `*` during Nix evaluation.
- Keep all existing denials and non-network permissions.
- Add the exact domain `chatgpt.com` to Den's default policy.
- Test domain decisions, wildcard behavior, denied domains, invalid input, and both agents.

The feature does not add repository-local discovery, environment overrides, policy-file replacement, or filesystem customization.
It does not add `*.chatgpt.com`, `api.openai.com`, or other provider domains to the defaults.

## Public configuration

An allowlist is a list of permitted domains.
`network.extraAllowedDomains` is a list of strings with an empty default.
Each agent has its own list.

In an existing devenv configuration that imports `inputs.den.devenvModules.den`, use:

```nix
programs.den.pi = {
  enable = true;
  network.extraAllowedDomains = [
    "api.example.com"
    "*.internal.example.com"
  ];
};

programs.den.claude = {
  enable = true;
  network.extraAllowedDomains = [ "api.example.com" ];
};
```

Home Manager uses the same declarations after importing `inputs.den.homeModules.den`.
Direct constructors accept the same nested configuration.
Within an existing flake output that defines `system`, use:

```nix
inputs.den.lib.${system}.mkPi {
  network.extraAllowedDomains = [ "api.example.com" ];
}
```

`mkClaude` accepts the same `network` attribute.
The packaged defaults keep an empty extra list.
Configuring Pi does not change Claude's permissions, or the reverse.

Permissions belong to the built package, not to the current repository path.
A project selects its package through its development shell.
A Home Manager package uses its configured permissions in every directory where that package runs.
Den does not choose permissions from a Git root or working directory.

## Accepted values and errors

A hostname identifies a network host by name.
An accepted hostname contains at least two non-empty labels separated by dots.
Each label contains lowercase ASCII letters, digits, or hyphens.
A label cannot start or end with a hyphen.
Each label has at most 63 characters, and the hostname has at most 253 characters.
The final label must contain a letter, which also excludes numeric IPv4 address forms.

A wildcard is a pattern that permits multiple matching hosts.
A wildcard entry starts with exactly one `*.` and has an accepted hostname after that prefix.
The prefix does not count toward the hostname length limit.
Fence's existing wildcard matching applies.
Den does not implement a second network matcher.

Nix evaluation rejects these inputs:

- A `network` value that is not an attribute set.
- Unknown attributes inside `network`.
- An `extraAllowedDomains` value that is not a list of strings.
- Empty strings, whitespace, uppercase letters, or a trailing dot.
- Empty labels, oversized labels, or oversized hostnames.
- Single-label hostnames such as `localhost`.
- IPv4 or IPv6 addresses, including bracketed IPv6 forms.
- Schemes, paths, queries, fragments, or port suffixes.
- A bare `*`, an embedded wildcard, or multiple wildcard prefixes.

Den rejects malformed values rather than trimming whitespace or changing letter case.
It does not resolve hostnames during evaluation.
Error messages identify `network.extraAllowedDomains` and the invalid entry or its index.
Messages for unknown attributes identify the invalid configuration field.

Duplicates are valid.
The merge removes duplicate entries and preserves the first occurrence.
It preserves the order of bundled domains, followed by extra domains in configuration order.

## Policy construction and security rules

An immutable policy is a policy file that the sandbox cannot change.
The shared sandbox builder creates that policy in the Nix store when extra domains exist.
It reads the bundled JSON and changes only `network.allowedDomains`.
The resulting list combines the bundled domains and the extra domains without duplicates.

If the extra list is empty, the builder keeps the bundled policy path.
The bundle itself gains one default entry, `chatgpt.com`.
That exact entry does not grant access to its subdomains.
Existing wildcard or default entries do not otherwise change.

The existing manifest points `basePolicy` to the selected policy file.
A manifest describes a package to the launcher.
The manifest remains version 2 and gains no fields.
The launcher continues to create the private per-launch policy from that file.
It still adds the exact RepoWolf endpoint and existing runtime grants.

All existing denials keep precedence over allowed entries.
An explicitly requested denied hostname can pass syntax rules but remains blocked by Fence.
Extra grants cannot remove GitHub, GitLab, Bitbucket, metadata, or telemetry denials.
RepoWolf remains the only permitted route to the protected Git-host families.

The feature does not change command rules, filesystem rules, socket permissions, or localhost port permissions.
The generated policy keeps those fields unchanged before the existing launcher adds runtime values.
Linux and macOS receive the same configured domain list.
Existing platform-specific runtime transformations remain unchanged.

Den does not reread project configuration during a running session.
Review network grants before evaluating a project's Nix configuration.
A new shell can evaluate edited project files.
Creating an immutable package policy does not make untrusted Nix code safe.

## Modules and data flow

The implementation uses the current configuration flow:

```text
Project Nix or module configuration
  -> Claude/Pi option normalization
  -> Shared sandbox builder
  -> Bundled or derived policy in the Nix store
  -> Existing manifest.basePolicy
  -> Existing launcher policy generation
  -> Fence
```

The Claude and Pi normalizers accept `network` and use shared domain syntax rules.
The Home Manager and devenv declarations expose the list and report malformed values through the same rules.
A small shared Nix helper owns those rules to prevent agent-specific differences.
It does not expose arbitrary Fence JSON overrides.

The constructors pass the normalized `network` configuration to `mk-agent-sandbox.nix`.
The shared builder owns the additive merge and policy-file selection.
It keeps its existing required dependencies and adapters.
Existing builder callers that omit `network` retain the empty default.

The main affected paths are:

- `nix/lib/options.nix` and `nix/lib/pi-options.nix` for normalization.
- `nix/lib/module-options.nix` for Home Manager and devenv declarations.
- `nix/lib/mk-claude.nix` and `nix/lib/mk-pi.nix` for forwarding.
- `nix/lib/mk-agent-sandbox.nix` for policy construction.
- A shared helper under `nix/lib/` for domain syntax rules.
- `policy/fence.json` and `policy/README.md` for the exact default grant and its documented transformation.
- Existing policy checks and their fixtures for executable permission decisions.
- `README.md` for examples, package scope, syntax, and denial precedence.

Production Go code does not need a change.
Existing Go policy tests can receive updated expectations or new behavior cases where needed.

## Verification and acceptance criteria

Tests must prove behavior rather than assert source text or static configuration structure.
Reusable syntax rules and network permission decisions justify automated tests.
Use direct evaluation, builds, and existing checks for static declarations and documentation.
Do not add tests that merely search Nix source or documentation for strings.

Acceptance requires these outcomes:

- The default policy permits `chatgpt.com` but not a previously unlisted subdomain of `chatgpt.com`.
- A configured exact hostname is permitted, while an unrelated hostname remains denied.
- A configured wildcard permits a matching subdomain and does not permit an unrelated domain.
- Denied hosts remain denied after a conflicting exact or wildcard grant.
- Metadata and telemetry denials remain intact.
- Empty extra lists retain the default behavior, apart from the agreed `chatgpt.com` addition.
- Duplicate input and overlap with bundled domains produce no duplicate output entries.
- Malformed values and unknown `network` attributes fail Nix evaluation with useful errors.
- Both constructors and both module integrations carry their agent's domains into the selected policy.
- Selecting different lists for Claude and Pi does not leak permissions between agents.
- Derived policy construction changes no fields outside `network.allowedDomains`.
- Existing per-launch broker, filesystem, command, socket, and platform rules still pass their regression tests.

Use the packaged Fence domain filter to exercise the generated default and customized policies.
Extend the existing Fence test fixture rather than recreate its matching rules in a test helper.
These tests must not contact public services.
Nix behavioral checks must evaluate valid and invalid configurations and consume their results.
They must not merely assert the text of an option declaration.

Run `go test ./...`, the relevant package/module and Fence checks, and the full flake check after implementation.
Make sure that Linux and macOS checks cover the shared domain behavior in the supported test environments.
Report any platform that cannot run locally and its remaining CI requirement.
No Pi, Fence, or flake-input update is part of this feature.

## Decision and review gate

The approved approach is a build-time additive merge.
A launch-time merge would add manifest and launcher behavior without serving the initial scope.
An arbitrary policy replacement would expose unrelated permissions and is out of scope.

The specification is ready for user review.
After written-spec approval, create the implementation plan under `docs/plans/`.
Implementation starts only after plan review and selection of an execution method.
