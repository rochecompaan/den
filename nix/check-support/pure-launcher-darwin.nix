{ inputs, pkgs }:

let
  fakes = import ./fakes.nix { inherit inputs pkgs; };
  den-launcher = import ../packages/den-launcher.nix { inherit pkgs; };
  sandbox = fakes.mkSandbox { };
  relativeExplicitSandbox = fakes.overrideManifest {
    name = "claude-relative-explicit";
    package = sandbox;
    filter = ''.explicitConfigDir = "relative-config"'';
  };
in
pkgs.runCommand "pure-launcher"
  {
    src = ../..;
    nativeBuildInputs = [ pkgs.go pkgs.jq pkgs.gitMinimal pkgs.python3 ];
  }
  ''
    set -ETeu
    current_phase=initialization
    last_command=
    last_line=
    failure_command=
    failure_line=

    capture_command() {
      local line=''${BASH_LINENO[0]}
      if [[ $line == 1 || ''${FUNCNAME[1]:-} == report_failure ]]; then
        return 0
      fi
      last_line=$line
      last_command=$BASH_COMMAND
    }

    capture_failure() {
      local status=$1
      if [[ -z $failure_line ]]; then
        failure_line=$2
        failure_command=$3
      fi
      return "$status"
    }

    report_failure() {
      local status=$1
      trap - DEBUG ERR EXIT
      if (( status != 0 )); then
        local line=''${failure_line:-$last_line}
        local command=''${failure_command:-$last_command}
        printf 'pure-launcher failure: phase=%s line=%s status=%s command=%q\n' \
          "$current_phase" "$line" "$status" "$command" >&2
      fi
      exit "$status"
    }

    set_phase() {
      current_phase=$1
      printf 'pure-launcher phase: %s\n' "$current_phase" >&2
    }

    trap capture_command DEBUG
    trap 'capture_failure "$?" "$LINENO" "$BASH_COMMAND"' ERR
    trap 'report_failure "$?"' EXIT
    set_phase "source-setup-and-go-tests"
    export CGO_ENABLED=0
    root=/private/tmp/den-task12-pure
    rm -rf "$root"
    mkdir -m 0700 -p "$root/home" "$root/worktree"
    export HOME="$root/home"
    cp -R "$src" "$root/source"
    chmod -R u+w "$root/source"
    (cd "$root/source" && ln -s ${den-launcher.goModules} vendor && go test -mod=vendor ./internal/... ./cmd/... -count=1)

    set_phase "git-and-credential-fixture-setup"
    cd "$root/worktree"
    printf fixture-ca > "$root/ca.pem"
    chmod 0400 "$root/ca.pem"
    git init -q --bare "$root/git-remote.git"
    git init -q "$root/git-seed"
    printf seed > "$root/git-seed/seed"
    git -C "$root/git-seed" add seed
    git -C "$root/git-seed" -c user.name=fixture -c user.email=fixture@example.test commit -qm seed
    git -C "$root/git-seed" push -q "$root/git-remote.git" HEAD:refs/heads/main
    git --git-dir="$root/git-remote.git" symbolic-ref HEAD refs/heads/main

    token=rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
    export REPOWOLF_ENDPOINT=https://broker.example.test/
    export REPOWOLF_TOKEN="$token"
    export REPOWOLF_CA_FILE="$root/ca.pem"
    export DEN_FAKE_AGENT_LOG="$root/agent.log"
    export DEN_FAKE_FENCE_LOG="$root/fence.log"
    export DEN_FAKE_FENCE_MARKER="$root/fence.marker"
    export DEN_FAKE_FENCE_ARGV_LOG="$root/fence-argv.log"
    export DEN_FAKE_POLICY_COPY="$root/policy.json"
    export DEN_FAKE_REPOWOLF_LOG="$root/repowolf.log"
    export DEN_FAKE_GIT_REMOTE="$root/git-remote.git"
    export DEN_FAKE_GIT_ROOT="$root/git-flow"
    export DEN_FAKE_EXPECT_AUTH=claude-auth-fixture
    export CLAUDE_CODE_OAUTH_TOKEN=claude-auth-fixture
    run_sandbox() { (cd "$root/worktree" && ${sandbox}/bin/claude "$@"); }

    set_phase "normal-launcher-execution"
    run_sandbox "argument with spaces" "" --plugin-dir user-plugin --mcp-config user-mcp.json --strict-mcp-config
    set_phase "success-path-evidence-assertions"
    test "$(grep -Fxc invoked "$root/fence.marker")" = 1
    if grep -Fq preflight "$root/fence.marker"; then exit 1; fi
    policyPath=$(sed -n 's/^policy=<\(.*\)>$/\1/p' "$root/fence.log")
    scratchPath=$(sed -n 's/^tmpdir=<\(.*\)>$/\1/p' "$root/fence.log")
    grep -Fqx "fence-tmpdir=<$scratchPath>" "$root/fence.log"
    grep -Fqx 'argv[0]=<--settings>' "$root/fence-argv.log"
    grep -Fqx "argv[1]=<$policyPath>" "$root/fence-argv.log"
    grep -Fqx 'argv[2]=<--expose-host-path>' "$root/fence-argv.log"
    preparedCA=$(sed -n 's/^argv\[3\]=<\(.*\)>$/\1/p' "$root/fence-argv.log")
    test "$preparedCA" != "$REPOWOLF_CA_FILE"
    test "$(basename "$preparedCA")" = repowolf-ca.pem
    grep -Fqx 'argv[4]=<-->' "$root/fence-argv.log"
    grep -Fqx 'argv[5]=<${fakes.fakeClaude}/bin/claude>' "$root/fence-argv.log"
    grep -Fqx 'argv[6]=<--dangerously-skip-permissions>' "$root/fence-argv.log"
    grep -Fqx 'argv[7]=<--settings>' "$root/fence-argv.log"
    grep -Fqx 'argv[8]=<${fakes.darwinSettings}>' "$root/fence-argv.log"
    grep -Fqx 'argv[9]=<argument with spaces>' "$root/fence-argv.log"
    grep -Fqx 'argv[10]=<>' "$root/fence-argv.log"
    grep -Fqx 'argv[11]=<--plugin-dir>' "$root/fence-argv.log"
    grep -Fqx 'argv[12]=<user-plugin>' "$root/fence-argv.log"
    grep -Fqx 'argv[13]=<--mcp-config>' "$root/fence-argv.log"
    grep -Fqx 'argv[14]=<user-mcp.json>' "$root/fence-argv.log"
    grep -Fqx 'argv[15]=<--strict-mcp-config>' "$root/fence-argv.log"
    test "$(grep -c '^argv\[' "$root/fence-argv.log")" = 16
    if grep -q '^argv\[16\]=' "$root/fence-argv.log"; then exit 1; fi
    test "$(grep -Fxc 'repowolf-git-ssh <upload> <github.com>' "$root/repowolf.log")" = 2
    test "$(grep -Fxc 'repowolf-git-ssh <receive> <github.com>' "$root/repowolf.log")" = 1
    git --git-dir="$root/git-remote.git" show-ref --verify --quiet refs/heads/pushed
    unset DEN_FAKE_GIT_ROOT DEN_FAKE_GIT_REMOTE
    if grep -Fq "$token" "$root/policy.json" "$root/fence.log" "$root/agent.log"; then exit 1; fi

    set_phase "early-rejection-cases"
    expect_early_failure() {
      rm -f "$root/fence.marker" "$root/policy.json" "$root/agent.log" "$root/fence.log" "$root/fence-argv.log" "$root/rejected.out" "$root/rejected.err"
      if "$@" > "$root/rejected.out" 2> "$root/rejected.err"; then exit 1; fi
      test ! -s "$root/rejected.out"
      test ! -e "$root/fence.marker"
      test ! -e "$root/policy.json"
      test ! -e "$root/agent.log"
      test ! -e "$root/fence-argv.log"
      if grep -Fq "$token" "$root/rejected.err"; then exit 1; fi
    }

    saved_endpoint=$REPOWOLF_ENDPOINT
    saved_token=$REPOWOLF_TOKEN
    saved_ca=$REPOWOLF_CA_FILE
    unset REPOWOLF_ENDPOINT; expect_early_failure run_sandbox
    export REPOWOLF_ENDPOINT=; expect_early_failure run_sandbox
    export REPOWOLF_ENDPOINT=$saved_endpoint
    unset REPOWOLF_TOKEN; expect_early_failure run_sandbox
    export REPOWOLF_TOKEN=; expect_early_failure run_sandbox
    export REPOWOLF_TOKEN=$saved_token
    unset REPOWOLF_CA_FILE; expect_early_failure run_sandbox
    export REPOWOLF_CA_FILE=; expect_early_failure run_sandbox
    export REPOWOLF_CA_FILE=$saved_ca
    export CLAUDE_CONFIG_DIR=relative-config; expect_early_failure run_sandbox
    unset CLAUDE_CONFIG_DIR
    expect_early_failure ${relativeExplicitSandbox}/bin/claude
    for argument in --settings --settings=x --permission-mode --permission-mode=x \
      --dangerously-skip-permissions --dangerously-skip-permissions=x --bare --bare=value; do
      expect_early_failure run_sandbox "$argument"
    done

    mkdir -p "$HOME/.claude"
    printf '{"disableAllHooks":true}\n' > "$HOME/.claude/settings.json"
    expect_early_failure run_sandbox
    printf '{"hooks":{"PreToolUse":[{"hooks":[{"command":"fence --claude-pre-tool-use"}]}]}}\n' > "$HOME/.claude/settings.json"
    expect_early_failure run_sandbox
    rm "$HOME/.claude/settings.json"

    set_phase "simple-mode"
    export CLAUDE_CODE_SIMPLE=1 DEN_FAKE_EXPECT_SIMPLE_SCRUB=1
    run_sandbox ordinary
    unset CLAUDE_CODE_SIMPLE DEN_FAKE_EXPECT_SIMPLE_SCRUB

    set_phase "pty-execution"
    ptyRunner="$root/pty-runner"
    cat > "$ptyRunner" <<EOF
    #!${pkgs.bash}/bin/bash
    printf 'pty-stage: runner-entered\n' >&2
    cd "$root/worktree"
    printf 'pty-stage: runner-cd-ok\n' >&2
    exec ${sandbox}/bin/claude
    EOF
    chmod 0700 "$ptyRunner"
    export DEN_PTY_LOGICAL_ROOT="$root"
    ${pkgs.python3}/bin/python ${./pty-driver.py} "$ptyRunner" "$root"
    unset DEN_PTY_LOGICAL_ROOT

    set_phase "cleanup-and-completion"
    rm -rf "$root"
    touch "$out"
  ''
