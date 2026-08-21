{ inputs, pkgs }:

let
  fakes = import ./fakes.nix { inherit inputs pkgs; };
  sandbox = fakes.mkSandbox { };
  explicitSandbox = fakes.mkSandbox { configDir = "/tmp/den-task12-explicit"; };
in
pkgs.runCommand "pure-launcher"
  {
    src = ../..;
    nativeBuildInputs = [ pkgs.go pkgs.jq pkgs.gitMinimal pkgs.util-linux pkgs.procps ];
  }
  ''
    set -eu
    export CGO_ENABLED=0
    export HOME="$TMPDIR/go-home"
    mkdir -p "$HOME"
    cp -R "$src" source
    chmod -R u+w source

    # The complete deterministic Go suite is part of this pure gate. It covers
    # validation matrices and race/error seams that cannot be induced safely by
    # a black-box shell process.
    (cd source && go test ./internal/... ./cmd/... -count=1)

    namespaceTmp="$TMPDIR/namespace-tmp"
    rootHost="$namespaceTmp/den-task12-pure"
    root=/tmp/den-task12-pure
    rm -rf "$namespaceTmp"
    mkdir -m 1777 "$namespaceTmp"
    mkdir -m 0755 "$namespaceTmp/etc"
    for file in passwd group hosts nsswitch.conf services protocols; do
      if test -e "/etc/$file"; then cp "/etc/$file" "$namespaceTmp/etc/$file"; else : > "$namespaceTmp/etc/$file"; fi
    done
    printf 'nameserver 127.0.0.1\n' > "$namespaceTmp/etc/resolv.conf"
    mkdir -m 0700 -p "$rootHost/home" "$rootHost/worktree" "$namespaceTmp/den-task12-explicit"
    cd "$rootHost/worktree"
    printf 'fixture-ca\n' > "$rootHost/ca.pem"
    chmod 0400 "$rootHost/ca.pem"
    token=rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
    export HOME="$root/home"
    export REPOWOLF_ENDPOINT=https://broker.example.test/
    export REPOWOLF_TOKEN="$token"
    export REPOWOLF_CA_FILE="$root/ca.pem"
    export DEN_FAKE_AGENT_LOG="$root/agent.log"
    export DEN_FAKE_FENCE_LOG="$root/fence.log"
    export DEN_FAKE_POLICY_COPY="$root/policy.json"
    export DEN_FAKE_REPOWOLF_LOG="$root/repowolf.log"
    export DEN_FAKE_EXPECT_AUTH=claude-auth-fixture
    export CLAUDE_CODE_OAUTH_TOKEN=claude-auth-fixture
    namespace_run() {
      ${pkgs.util-linux}/bin/unshare -Ur -m ${pkgs.bash}/bin/bash -c '
        ${pkgs.util-linux}/bin/mount --bind "$1" /tmp
        ${pkgs.util-linux}/bin/mount --bind "$1/etc" /etc
        cd /tmp/den-task12-pure/worktree
        shift
        exec "$@"
      ' den-task12 "$namespaceTmp" "$@"
    }
    run_sandbox() { namespace_run ${sandbox}/bin/claude "$@"; }
    run_explicit() { namespace_run ${explicitSandbox}/bin/claude "$@"; }

    # Seed every inherited injection that the launcher must remove.
    export GH_TOKEN=redacted GITHUB_TOKEN=redacted GH_ENTERPRISE_TOKEN=redacted
    export GITHUB_ENTERPRISE_TOKEN=redacted SSH_AUTH_SOCK=/invalid/agent
    export GIT_ASKPASS=/invalid/askpass SSH_ASKPASS=/invalid/ssh-askpass
    export GIT_SSH=/invalid/ssh GIT_SSH_COMMAND=/invalid/ssh-command
    export GIT_CONFIG_GLOBAL=/invalid/global GIT_CONFIG_SYSTEM=/invalid/system
    export GIT_CONFIG_PARAMETERS="'credential.helper'='/invalid helper' 'http.extraHeader'='Authorization: redacted'"
    export GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=credential.helper
    export GIT_CONFIG_VALUE_0=/invalid/helper GIT_CONFIG_KEY_1=http.extraHeader
    export GIT_CONFIG_VALUE_1='Authorization: redacted'
    export TMPDIR=/invalid/tmp DEN_FENCE_TMPDIR=/invalid/fence-tmp

    run_sandbox "argument with spaces" "" --plugin-dir user-plugin \
      --mcp-config user-mcp.json --strict-mcp-config

    grep -Fqx 'arg[0]=<--dangerously-skip-permissions>' "$rootHost/agent.log"
    grep -Fqx 'arg[1]=<argument with spaces>' "$rootHost/agent.log"
    grep -Fqx 'arg[2]=<>' "$rootHost/agent.log"
    grep -Fqx 'arg[3]=<--plugin-dir>' "$rootHost/agent.log"
    grep -Fqx 'arg[5]=<--mcp-config>' "$rootHost/agent.log"
    grep -Fqx 'arg[7]=<--strict-mcp-config>' "$rootHost/agent.log"
    grep -Fqx 'policy-mode=400' "$rootHost/fence.log"
    grep -Fqx 'separate-scratch=yes' "$rootHost/fence.log"
    grep -Fqx 'tmpdir-controlled=yes' "$rootHost/fence.log"

    jq -e --arg broker broker.example.test --arg home "$HOME" '
      .allowPty == true and
      (.network.allowedDomains | index($broker)) != null and
      (.network.allowedDomains | index("api.anthropic.com")) != null and
      (.network.allowedDomains | index("registry.npmjs.org")) != null and
      (.network.deniedDomains | index("github.com")) != null and
      (.network.deniedDomains | index("gitlab.com")) != null and
      (.network.deniedDomains | index("bitbucket.org")) != null and
      (.network.deniedDomains | index("169.254.169.254")) != null and
      (.network.deniedDomains | index("statsig.anthropic.com")) != null and
      (.network.allowedDomains | all(. != "*.github.com" and . != "*.gitlab.com" and . != "*.bitbucket.org")) and
      (.filesystem.allowWrite | index($home + "/.claude")) != null and
      (.filesystem.allowWrite | index($home + "/.claude.json")) != null and
      (.filesystem.allowWrite | index($home + "/.config/claude")) != null
    ' "$rootHost/policy.json"
    echo task12-policy-ok >&2
    if grep -Fq "$token" "$rootHost/policy.json" "$rootHost/fence.log" "$rootHost/agent.log"; then
      echo 'a generated artifact disclosed the fake token' >&2
      exit 1
    fi

    # PATH starts with RepoWolf, then Fence, gitMinimal, Bash, Coreutils, and launcher.
    expected_path='${fakes.fakeRepoWolfClient}/bin:${fakes.fakeFence}/bin:${pkgs.gitMinimal}/bin:${pkgs.bash}/bin:${pkgs.coreutils}/bin:${fakes.launcher}/bin:${fakes.fakeClaude}/bin'
    grep -Fqx "path=$expected_path" "$rootHost/agent.log"
    grep -Fqx "git-ssh=${fakes.fakeRepoWolfClient}/bin/repowolf-git-ssh" "$rootHost/agent.log"
    grep -Fqx 'repowolf-git-ssh <clone> <github.com> <git-upload-pack>' "$rootHost/repowolf.log"
    grep -Fqx 'repowolf-git-ssh <fetch> <github.com> <git-upload-pack>' "$rootHost/repowolf.log"
    grep -Fqx 'repowolf-git-ssh <push> <github.com> <git-receive-pack>' "$rootHost/repowolf.log"
    grep -Fqx 'gh <repo> <view>' "$rootHost/repowolf.log"
    echo task12-environment-ok >&2
    unset GH_TOKEN GITHUB_TOKEN GH_ENTERPRISE_TOKEN GITHUB_ENTERPRISE_TOKEN
    unset SSH_AUTH_SOCK GIT_ASKPASS SSH_ASKPASS GIT_SSH GIT_SSH_COMMAND
    unset GIT_CONFIG_GLOBAL GIT_CONFIG_SYSTEM GIT_CONFIG_PARAMETERS
    unset GIT_CONFIG_COUNT GIT_CONFIG_KEY_0 GIT_CONFIG_VALUE_0 GIT_CONFIG_KEY_1 GIT_CONFIG_VALUE_1

    # Explicit configDir outranks inherited state; inherited state outranks fallback.
    chmod 0700 "$namespaceTmp/den-task12-explicit"
    export CLAUDE_CONFIG_DIR="$root/inherited-unused"
    export DEN_FAKE_STATE_MODE=custom
    run_explicit
    test -e "$namespaceTmp/den-task12-explicit/fake-state"
    test ! -e "$rootHost/inherited-unused"
    echo task12-precedence-ok >&2
    unset CLAUDE_CONFIG_DIR
    unset DEN_FAKE_STATE_MODE

    expect_failure() {
      if "$@" > "$rootHost/rejected.out" 2> "$rootHost/rejected.err"; then
        echo 'launcher accepted a rejected input' >&2
        exit 1
      fi
      test ! -s "$rootHost/rejected.out"
      if grep -Fq "$token" "$rootHost/rejected.err"; then
        echo 'rejection disclosed token' >&2
        exit 1
      fi
    }

    # Missing/empty RepoWolf values and public security-flag forms fail before Fence.
    saved_endpoint=$REPOWOLF_ENDPOINT
    unset REPOWOLF_ENDPOINT
    expect_failure run_sandbox
    export REPOWOLF_ENDPOINT=
    expect_failure run_sandbox
    export REPOWOLF_ENDPOINT=$saved_endpoint
    for argument in --settings --settings=x --permission-mode --permission-mode=x \
      --dangerously-skip-permissions --dangerously-skip-permissions=x; do
      expect_failure run_sandbox "$argument"
    done
    echo task12-rejections-ok >&2

    # Linux preserves --bare and CLAUDE_CODE_SIMPLE as ordinary user choices.
    export CLAUDE_CODE_SIMPLE=1
    run_sandbox --bare --bare=value
    grep -Fqx 'arg[1]=<--bare>' "$rootHost/agent.log"
    grep -Fqx 'arg[2]=<--bare=value>' "$rootHost/agent.log"
    echo task12-linux-arguments-ok >&2
    unset CLAUDE_CODE_SIMPLE

    # Stdio, nonzero status, signal status/forwarding, and PTY travel through
    # the package wrapper and both fake process boundaries.
    export DEN_FAKE_PROCESS_MODE=stdio
    printf 'stream-fixture\n' | run_sandbox > "$rootHost/stdout" 2> "$rootHost/stderr"
    grep -Fqx 'stdout:stream-fixture' "$rootHost/stdout"
    grep -Fqx 'stderr:stream-fixture' "$rootHost/stderr"
    echo task12-stdio-ok >&2
    unset DEN_FAKE_PROCESS_MODE
    export DEN_FAKE_EXIT_CODE=23
    set +e
    run_sandbox
    status=$?
    set -e
    test "$status" = 23
    echo task12-status-ok >&2
    unset DEN_FAKE_EXIT_CODE

    export DEN_FAKE_PROCESS_MODE=self-signal
    set +e
    run_sandbox
    status=$?
    set -e
    test "$status" = 143
    export DEN_FAKE_PROCESS_MODE=wait-signal
    export DEN_FAKE_PROCESS_PID_FILE="$root/process.pid"
    export DEN_FAKE_READY_FILE="$root/ready"
    export DEN_FAKE_SIGNAL_LOG="$root/signal"
    namespace_run ${sandbox}/bin/claude &
    namespace_pid=$!
    for _ in $(seq 1 100); do test -e "$rootHost/ready" && break; sleep 0.05; done
    test -s "$rootHost/process.pid"
    child_pid=$(cat "$rootHost/process.pid")
    launcher_pid=$(${pkgs.procps}/bin/ps -o ppid= -p "$child_pid" | tr -d ' ')
    kill -TERM "$launcher_pid"
    set +e
    wait "$namespace_pid"
    status=$?
    set -e
    test "$status" = 42
    grep -Fqx TERM "$rootHost/signal"
    echo task12-signal-ok >&2
    unset DEN_FAKE_PROCESS_MODE DEN_FAKE_PROCESS_PID_FILE DEN_FAKE_READY_FILE DEN_FAKE_SIGNAL_LOG

    # Wrapper success cleanup is checked here; the same gate's deterministic
    # tempdir tests simulate SIGKILL staleness and every cleanup exit class.
    uid=0
    run_sandbox
    test -z "$(find "$namespaceTmp/den-$uid" -mindepth 1 -maxdepth 1 -type d \( -name 'policy-*' -o -name 'scratch-*' \) -print -quit)"
    echo task12-cleanup-ok >&2

    # Neither repository nor user Git configuration changes across fake flows.
    git config --file "$rootHost/home/.gitconfig" user.name fixture
    git init -q
    git config user.email fixture@example.test
    before_global=$(sha256sum "$rootHost/home/.gitconfig")
    before_local=$(sha256sum .git/config)
    run_sandbox
    test "$(sha256sum "$rootHost/home/.gitconfig")" = "$before_global"
    test "$(sha256sum .git/config)" = "$before_local"

    touch "$out"
  ''
