# Den

Den packages Claude Code behind Fence and RepoWolf. You use the normal `claude`
command. Den does not provide a separate runtime command.

## Architecture

Each launch uses this process flow:

```text
claude launcher -> Fence -> Claude Code -> RepoWolf gh and Git SSH clients
```

The launcher validates runtime values and creates a private Fence policy. Fence
enforces filesystem, process, command, and network rules around Claude Code.
RepoWolf authorizes GitHub API and Git operations. RepoWolf is the only route to
GitHub from the sandbox.

Fence is a process sandbox, not a virtual machine (VM). A VM has a separate
kernel and gives a stronger isolation boundary. Fence does not give a VM-grade
guarantee.

Den supports these systems:

- `x86_64-linux`
- `aarch64-linux`
- `x86_64-darwin`
- `aarch64-darwin`

## Prerequisites

You need Nix with flakes enabled. You also need access to an existing RepoWolf
server. Den supplies RepoWolf clients, but it does not supply or manage the
server.

Set these three variables before each launch:

- `REPOWOLF_ENDPOINT` is the HTTPS origin of the RepoWolf server.
- `REPOWOLF_TOKEN` is the RepoWolf client token.
- `REPOWOLF_CA_FILE` identifies the certificate-authority file for that server.

`REPOWOLF_ENDPOINT` has these exact rules:

- The scheme must be lowercase `https`.
- The hostname must be lowercase ASCII DNS with at least two labels.
- The hostname cannot have a trailing dot and cannot be an IP address.
- A port is optional. It must be from 1 through 65535, without a leading zero.
- The path must be empty or `/`.
- User information, a query, a fragment, and opaque URL forms are invalid.
- The hostname cannot be GitHub, GitLab, Bitbucket, or a subdomain of one.

`REPOWOLF_TOKEN` must start with `rw1_`. The remaining 43 characters must be the
canonical, unpadded Base64 URL encoding of exactly 32 bytes.

`REPOWOLF_CA_FILE` must resolve to an existing regular file. The file cannot be
a symbolic link and must have a read-permission bit. The launcher converts an
accepted relative path to an absolute path.

Load the token without putting it in shell history or terminal output. For
example:

```bash
export REPOWOLF_ENDPOINT='https://broker.example.com'
IFS= read -r -s REPOWOLF_TOKEN
export REPOWOLF_TOKEN
export REPOWOLF_CA_FILE="$HOME/.config/repowolf/ca.pem"
claude
```

Do not print RepoWolf values in logs or troubleshooting output.

## Install and integrate

All interfaces install an executable named `claude`. The default package and
the `claude` package are the same artifact.

### Flake package

Install the flake package in the current profile:

```bash
nix profile install github:rochecompaan/den#claude
claude
```

A flake can also add the package to a development shell:

```nix
{
  inputs.den.url = "github:rochecompaan/den";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = inputs@{ nixpkgs, ... }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
    in
    {
      devShells.${system}.default = pkgs.mkShell {
        packages = [ inputs.den.packages.${system}.claude ];
      };
    };
}
```

Enter the shell, and then start Claude:

```bash
nix develop
claude
```

### Direct constructor

Use `inputs.den.lib.${system}.mkClaude` when you need package customization:

```nix
{
  inputs.den.url = "github:rochecompaan/den";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = inputs@{ nixpkgs, ... }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
      claude = inputs.den.lib.${system}.mkClaude {
        configDir = null;
        extraPkgs = [ pkgs.neovim ];
      };
    in
    {
      devShells.${system}.default = pkgs.mkShell {
        packages = [ claude ];
      };
    };
}
```

`inputs.den.lib.${system}.mkClaude { }` has the same behavior as the flake
package.

### Home Manager

Import `homeModules.den`, and then enable `programs.den.claude`:

```nix
{ inputs, pkgs, ... }:
{
  imports = [ inputs.den.homeModules.den ];

  programs.den.claude = {
    enable = true;
    configDir = null;
    extraPkgs = [ pkgs.neovim ];
  };
}
```

Apply the Home Manager configuration, and then start Claude:

```bash
home-manager switch --flake .
claude
```

### devenv

Import `devenvModules.den` in the devenv module:

```nix
{ inputs, pkgs, ... }:
{
  imports = [ inputs.den.devenvModules.den ];

  programs.den.claude = {
    enable = true;
    configDir = null;
    extraPkgs = [ pkgs.neovim ];
  };

  enterShell = ''
    export CLAUDE_CONFIG_DIR="$PWD/.devenv/state/claude"
  '';
}
```

The shell export uses an absolute project-state path because `$PWD` is
absolute. Keep `configDir = null` so the runtime variable can select this path.
Add the state directory to `.gitignore`:

```gitignore
/.devenv/state/claude/
```

Enter the environment, and then start Claude:

```bash
devenv shell
claude
```

You can use the same export in a direnv `.envrc`:

```bash
export CLAUDE_CONFIG_DIR="$PWD/.devenv/state/claude"
```

## Public configuration

The constructor, Home Manager module, and devenv module use the same option
tree. All values are optional. These are the current defaults:

```nix
{ pkgs }:
{
  configDir = null;
  extraPkgs = [ ];

  docker = {
    enable = false;
    package = pkgs.docker-client;
    composePackage = pkgs.docker-compose;
    socketPath = null;
    hostPorts = [ ];
  };

  podman = {
    enable = false;
    package = pkgs.podman;
    composePackage = pkgs.podman-compose;
    socketPath = null;
    hostPorts = [ ];
  };
}
```

For either module, put these values below `programs.den.claude`. The modules
also add an `enable` Boolean, which defaults to `false`.

`configDir` and each `socketPath` are `null` or absolute path strings.
`extraPkgs` is a list of Nix packages. Each `hostPorts` value is a unique list
of integers from 1 through 65535. A non-empty list requires its container
integration to be enabled.

## Claude configuration directory

The launcher selects the Claude configuration directory in this order:

1. A non-null constructor or module `configDir` value.
2. The inherited `CLAUDE_CONFIG_DIR` value.
3. The default `$HOME/.claude` directory.

The first two choices select custom mode. A custom value must be an absolute
path. Den does not resolve a relative value against the working directory or
Git root. A `null` option permits the runtime variable and default fallback.

### Default mode

Default mode grants write access to these Claude state paths:

- `$HOME/.claude/`
- `$HOME/.claude.json`
- `$HOME/.config/claude/`

### Custom mode

Custom mode grants Claude-state writes only to the canonical custom directory.
It denies writes to all three default paths, even when the working tree
contains one of them.

A missing custom directory is created with mode `0700`. Its parent directory
must already exist. An existing directory must meet all these rules:

- The invoking user owns it.
- Its exact mode is `0700`, without special permission bits.
- Its owner can read, write, and enter it.
- No group or other user has permission through its mode or ACL.
- Its final path component is not a symbolic link.

An access control list (ACL) is an extra list of permissions for users and
groups. Den rejects an ACL that grants another principal access. It also
rejects an ACL that removes owner write access. Den inspects a new directory
because a parent ACL can create inherited permissions. If that inspection
fails, Den removes the new directory and stops.

Den resolves existing parent components before validation. Each canonical
ancestor must prevent replacement by another principal. An ancestor needs the
sticky exception only if another principal can write it through group or other
mode bits, or through an ACL. The sticky ancestor must be owned by root or the
invoking user. A sticky directory restricts who can replace its entries. An
ancestor writable only by its owner does not need the sticky bit.

The overlap validator uses protected roots, not filesystem deny globs. The
protected directory roots are:

- `$HOME/.ssh`
- `$HOME/.gnupg`
- `$HOME/.aws`
- `$HOME/.config/gcloud`
- `$HOME/.kube`
- `$HOME/.docker`
- `$HOME/.config/git`

The protected files are:

- `$HOME/.pypirc`
- `$HOME/.netrc`
- `$HOME/.git-credentials`
- `$HOME/.cargo/credentials`
- `$HOME/.cargo/credentials.toml`
- `$HOME/.gitconfig`

The three default Claude paths are also protected:

- `$HOME/.claude/`
- `$HOME/.claude.json`
- `$HOME/.config/claude/`

A custom directory cannot equal, contain, or be contained by any protected
root, file, or default Claude path. This rule rejects broad paths such as `/`
and `$HOME`. Filesystem deny rules take precedence over working-tree and
state-directory grants.

Immediately before Fence starts, Den repeats validation of the path, owner,
mode, ACL, and ancestors. It also compares the directory device and inode. The
launch stops if the directory identity or any validated property changed.

`CLAUDE_CONFIG_DIR` relocates settings, history, plugins, and other
filesystem-backed Claude state. On Linux, it also relocates file-backed Claude
credentials. Authentication from environment variables does not move and
remains available inside Fence.

On macOS, Claude credentials can remain in Keychain. `configDir` and
`CLAUDE_CONFIG_DIR` do not isolate or relocate Keychain credentials. Den does
not replace `HOME`.

## Extra packages and `PATH`

`extraPkgs` adds tools only to the generated Claude artifact, sandbox policy,
and sandbox `PATH`. It does not add packages to the host package set, user
profile, or host `PATH`. Nix store paths remain visible to the host as normal
Nix artifacts.

The sandbox `PATH` has this order:

1. RepoWolf clients
2. Fence
3. minimal Git
4. Bash
5. Coreutils
6. the launcher tools
7. Claude Code
8. enabled Docker and Podman clients
9. `extraPkgs`

RepoWolf clients always come first. An extra package cannot replace the
controlled `gh`, Git SSH helper, or other earlier tools.

## Docker and Podman

Docker and Podman are disabled by default. Enabling one adds its configured
client and Compose package. Den does not start a daemon or a Podman machine.

A daemon socket is a local Unix special file that gives commands to a service.
Den exposes only the selected socket, not its parent directory or credential
configuration.

### Docker

Docker socket discovery uses this order:

1. `docker.socketPath`
2. `DOCKER_HOST`
3. `$XDG_RUNTIME_DIR/docker.sock`
4. `$HOME/.docker/run/docker.sock`
5. `/run/docker.sock`
6. `/var/run/docker.sock`

An explicit socket path must be absolute. `DOCKER_HOST` must have the exact
form `unix:///absolute/path`. Authorities, queries, fragments, percent-encoded
paths, relative paths, and remote transports are invalid.

Den resolves socket symbolic links. The final target must be an existing Unix
socket. Den then sets `DOCKER_HOST` to the canonical local Unix endpoint.

### Podman

Podman socket discovery uses this order:

1. `podman.socketPath`
2. `CONTAINER_HOST`
3. `$XDG_RUNTIME_DIR/podman/podman.sock`
4. `/run/user/<uid>/podman/podman.sock`

On Linux, an unset `XDG_RUNTIME_DIR` becomes `/run/user/<uid>`. The selected
socket must belong to the invoking user. These rules keep the default Podman
connection rootless. Rootless means that the Podman service runs as the user,
not as root.

On macOS, configure `podman.socketPath` or a local Unix `CONTAINER_HOST` when no
conventional socket exists. Den does not inspect or start a Podman machine.

An explicit socket path must be absolute. `CONTAINER_HOST` must have the exact
form `unix:///absolute/path`. Remote, relative, malformed, and percent-encoded
endpoints are invalid. Den resolves symbolic links and exports only the
canonical local Unix endpoint.

### Host ports

`docker.hostPorts` and `podman.hostPorts` grant access from the sandbox to host
localhost. Den combines, sorts, and removes duplicates across the two enabled
integrations. An empty combined list grants no host-local outbound access.

On Linux, Fence permits only the exact combined ports. On macOS, any requested
port permits all localhost ports because Fence cannot enforce exact localhost
ports there. Thus, one Darwin port request widens access to every service on
host localhost.

**WARNING:** Enable a container socket only for trusted work. A daemon socket
can create containers, mount host paths, and reach resources outside Fence.

Docker daemon access can provide effective host-root control. Rootless Podman
still controls the user's containers, files, and network. Either socket can
materially weaken or bypass the sandbox. Host-port access also expands the
network boundary.

## Arguments, environment, and process behavior

Den always adds `--dangerously-skip-permissions` because Fence supplies the
permission boundary. You cannot pass these Den-owned flags in bare or
`--flag=value` form:

- `--settings`
- `--permission-mode`
- `--dangerously-skip-permissions`

All other arguments keep their values and order. This includes empty
arguments, arguments with spaces, plugin arguments, and MCP configuration
arguments.

On macOS, Den also rejects `--bare` and every `--bare=value` form. Claude Code
2.1.158 uses these forms to skip hooks. Den removes inherited
`CLAUDE_CODE_SIMPLE` for the same reason. Linux preserves `--bare` and
`CLAUDE_CODE_SIMPLE` as ordinary user choices because the security hook is
Darwin-only.

The launcher replaces the inherited `PATH` and all inherited `REPOWOLF_*`
values. It validates and reinstalls only the three required RepoWolf values.
It also removes these credential and Git transport variables:

- `GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, and
  `GITHUB_ENTERPRISE_TOKEN`
- `SSH_AUTH_SOCK`, `GIT_ASKPASS`, `SSH_ASKPASS`, `GIT_SSH`, and
  `GIT_SSH_COMMAND`
- `GIT_CONFIG_GLOBAL`, `GIT_CONFIG_SYSTEM`, `GIT_CONFIG_PARAMETERS`, and
  `GIT_CONFIG_COUNT`
- all inherited `GIT_CONFIG_KEY_*` and `GIT_CONFIG_VALUE_*` entries
- inherited `GIT_TERMINAL_PROMPT`, `DOCKER_HOST`, and `CONTAINER_HOST`

Validated local container endpoints replace the last two variables when their
integrations are enabled. Den sets `GIT_TERMINAL_PROMPT=0`, controlled Git URL
rewrites, and the immutable RepoWolf Git SSH helper.

The launcher removes inherited `TMPDIR`, `DEN_FENCE_TMPDIR`, and
`DEN_FENCE_POLICY_FILE`. It sets the two temporary-directory variables to a
private per-launch scratch directory. It sets `DEN_FENCE_POLICY_FILE` to a
validated internal policy path.

Sandbox descendants inherit this internal read-only path. The mandatory macOS
hook uses it to invoke nested Fence. The policy file and its parent have
highest-precedence write denials, so sandbox processes cannot mutate the
policy. `DEN_FENCE_POLICY_FILE` is internal. Do not set or use it.

Other normal user variables remain available. These include Claude
authentication, locale, terminal, and editor variables. Den never enables
shell tracing while secrets are present.

Standard input, standard output, and standard error connect directly to Fence.
The launcher keeps the terminal foreground process group and pseudo-terminal
(PTY) behavior. A PTY is the terminal interface used by an interactive
program. Terminal resize and normal job-control behavior remain intact.

The launcher forwards `SIGINT`, `SIGTERM`, `SIGHUP`, and `SIGQUIT`. The shared
process group preserves `SIGWINCH`, `SIGTSTP`, and `SIGCONT` behavior. The
launcher returns the Claude or Fence status. Signal termination uses
`128 + signal`.

After Fence exits, Den removes its private policy and scratch directories.
Cleanup errors do not replace the child status. `SIGKILL` cannot run cleanup.
On a later launch, Den removes eligible stale, user-owned temporary directories
after they are more than 24 hours old.

## Resources and macOS hook

Den supplies no skills, plugins, Context Mode, CodeGraph, other MCP servers, or
non-security hooks. User-managed skills, plugins, hooks, and MCP servers in the
selected Claude configuration directory remain available. They run inside the
same Fence filesystem, command, process, and network policy.

On macOS, Den supplies one mandatory `den-fence` security hook. This
`PreToolUse` hook examines Claude Bash tool commands. It denies blocked
commands and sends allowed Bash commands through `fence -c` with the private
policy.
The hook does not inspect Claude native file tools. The outer whole-process
Fence boundary still applies to those tools and user resources.

User hooks cannot disable or replace `den-fence`. Unrelated user hooks remain
available. Den does not edit user settings to install the security hook.

Future resource bundles are outside this release. There is no usable
`resourceBundles`, `claudeResources`, `mkClaudeResourceBundle`, marketplace,
or resource-seed API.

## Network and filesystem limits

Unmatched outbound traffic is denied. The static allow list contains:

- `api.anthropic.com`, `*.anthropic.com`, `claude.ai`, and `*.claude.ai`
- `registry.npmjs.org` and `*.npmjs.org`
- `registry.yarnpkg.com`
- `pypi.org` and `files.pythonhosted.org`
- `crates.io`, `static.crates.io`, and `index.crates.io`
- `proxy.golang.org` and `sum.golang.org`
- `formulae.brew.sh`

Each launch also allows the exact validated RepoWolf hostname. Den blocks
`github.com`, `*.github.com`, `githubusercontent.com`,
`*.githubusercontent.com`, `gitlab.com`, `*.gitlab.com`, `bitbucket.org`, and
`*.bitbucket.org`. Deny rules take precedence over allow rules.

Den also blocks metadata services and selected telemetry, including
`169.254.169.254`, `metadata.google.internal`, `instance-data.ec2.internal`,
and `statsig.anthropic.com`.

Git rewrites `https://github.com/owner/repository.git` to the RepoWolf SSH
route. The controlled Git SSH helper handles supported Git smart-protocol
operations. The sandbox `gh` command is the credential-free RepoWolf client.
Direct web, API, archive, raw-content, release-asset, HTTPS, and SSH traffic to
denied Git hosts remains blocked.

The launch working tree and selected Claude state are writable. Nix store
closures are read-only. Fence denies normal credential locations, including
SSH, GnuPG, cloud, Kubernetes, Docker, registry, netrc, Git, and Cargo
credentials. Filesystem denials take precedence over grants and symbolic-link
paths.

These controls reduce access but do not make untrusted work safe. User
resources run with the sandbox's allowed access. Allowed network services can
receive data that the sandbox can read. Container sockets can escape the
boundary. Fence is not a VM-grade boundary.

## Troubleshooting

Do not print, log, or paste RepoWolf values while you troubleshoot.

### A RepoWolf variable is invalid

1. Load all three variables again from their approved source.
2. Make sure that the endpoint obeys every origin and hostname rule above.
3. Replace the token if its source cannot guarantee the exact `rw1_` format.
4. Start `claude` again.

The launcher names the invalid variable but never includes its value. Do not
use `echo`, `env`, shell tracing, or verbose secret-manager output to inspect
these values.

### The CA file is rejected

Use non-printing file tests:

```bash
test -n "${REPOWOLF_CA_FILE-}"
test -f "$REPOWOLF_CA_FILE"
test ! -L "$REPOWOLF_CA_FILE"
test -r "$REPOWOLF_CA_FILE"
claude
```

If a test fails, install the correct regular CA file and reset the variable.
Do not replace it with a symbolic link.

### A custom configuration directory is not private

1. Use an absolute path with an existing, protected parent.
2. Make sure that the invoking user owns the directory.
3. Set the directory mode to `0700`.
4. Remove non-owner ACL entries from the directory and its relevant ancestors.
5. Remove a symbolic final path component.
6. Choose a path that is separate from default state and credential paths.
7. Start `claude` again.

For environment-selected mode, set the mode on `CLAUDE_CONFIG_DIR`:

```bash
chmod 0700 "$CLAUDE_CONFIG_DIR"
claude
```

For a constructor or module `configDir`, use its configured absolute path:

```bash
chmod 0700 /absolute/config/path
claude
```

An ancestor needs the sticky exception only if another principal can write it
through group or other mode bits, or through an ACL. An ancestor writable only
by its owner does not need the sticky bit.

On Linux, use the host ACL administration tools to remove extra ACL grants. On
macOS, use the host ACL administration tools to remove inherited ACL grants.
Den fails closed, which means that it stops instead of using an unproven path.

### A Docker or Podman socket is not found

1. Make sure that the daemon already runs.
2. Select an existing local Unix socket with `socketPath` or its environment
   endpoint.
3. Make sure that the final target is a Unix socket.
4. For Podman, make sure that the invoking user owns the socket.
5. Start `claude` again.

Den does not start Docker, Podman, or a Podman machine. It does not accept TCP,
SSH, HTTP, `npipe`, or other remote endpoints.

### Fence reports a capability error

On Linux, Den requires Fence to report an available network namespace before
policy generation. Use a supported native host whose kernel permits the
required Fence feature. Do not bypass this requirement. Without it, direct
network isolation is unavailable.

On either platform, a Fence application error means that Fence failed to apply
the generated policy. Correct the reported host capability, filesystem, or
policy application problem. Then start `claude` again. Den does not fall back
to an unsandboxed process.

### Direct Git access fails

This result is expected for GitHub, GitLab, and Bitbucket endpoints. Use the
sandbox `gh` client or standard Git operations for GitHub. RepoWolf supports
that route. A tool that contacts GitHub directly with HTTP or SSH remains
blocked.

RepoWolf is not a general route for every Git host. This release does not add a
direct route to GitLab or Bitbucket.

### The sandbox blocks an expected host or file

Compare the target with the allow and deny lists above. Deny rules always win.
There is no user option to weaken the base security policy. Move required
state into the selected configuration directory only when it is not a
protected credential path.

### A container command reaches more than expected

Stop the daemon integration if the work is not trusted. Socket access can
escape Fence. On macOS, any non-empty `hostPorts` list allows all host
localhost ports. No configuration can narrow that Darwin behavior in this
release.

## Security glossary

- **ACL:** Extra filesystem permissions for named users and groups.
- **Canonical path:** A normalized path after existing symbolic links resolve.
- **Fail closed:** Stop the launch when a security requirement cannot be
  proven.
- **Origin:** A URL scheme, hostname, and optional port, without an application
  path.
- **Rootless:** A daemon that runs with user privileges instead of root
  privileges.
- **Unix socket:** A local filesystem endpoint for communication between
  processes.
