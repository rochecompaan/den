# Den Fence policy

## Authenticated source

This strict JSON policy derives from the authenticated local checkout at
`/home/roche/projects/engineering-handbook`, commit
`4be05d63af92cf79231313a20df22b3c144795d0`, path
`ai-tooling/sandboxing/fence.json`. The original file's SHA-256 digest is:

```text
bc4ec1509ffa812b5d89bf258b9adade1262c4f1f50be2d52ffa230270475f29
```

The vendored file is the build input. Den does not import permissions from
`jail.nix`.

## Den transformations

The source JSONC comments and trailing commas were removed to produce strict
JSON. Den then made only these policy changes:

- Replaced `network.allowedDomains` with the approved Anthropic, npm, Yarn,
  Python, Rust, Go, and Homebrew service domain set.
- Replaced `network.deniedDomains` with the protected GitHub, GitLab, and
  Bitbucket families plus the retained metadata and Statsig telemetry entries.
- Kept `allowPty` and `filesystem.allowGitConfig` enabled.
- Enabled `filesystem.defaultDenyRead` and `filesystem.strictDenyRead`.
- Emptied all static read, execute, and write grants. The generator adds only
  validated per-launch paths.
- Set `filesystem.denyRead` to the exact shared
  `nix/lib/protected-paths.nix` list, including the added Git configuration
  protections.
- Retained the reference secret-file write denials and added write denials for
  Fence 0.1.58's implicit `~/.npm/_logs`, `~/.fence/debug`, `/tmp/fence`, and
  `/private/tmp/fence` paths. Maintainer-approved Option A uses Fence's effective
  `~` expansion instead of ineffective literal `$HOME` entries.
- Retained the enabled reference command denials, `useDefaults`, and the
  accepted `chroot` shared-binary limitation; enabled Linux argv-aware runtime
  enforcement. The generator removes that Linux-only setting on macOS.

At launch, Den adds the exact RepoWolf broker hostname, a read-only CA file,
validated closure and operational reads, selected writable worktree/state/
scratch/socket paths, platform-specific socket and host-port fields, and
higher-precedence write denials for default state in custom mode, repository
Git configuration, and the private policy file and directory. Writable entries
also receive read grants because macOS Fence treats read and write operations
separately. Denials take precedence over grants.

For requested container host ports, Linux receives the exact sorted,
deduplicated list. macOS Fence cannot enforce an exact port list: any requested
port enables all localhost outbound ports, so the generated macOS policy does
not claim an exact-port restriction.
