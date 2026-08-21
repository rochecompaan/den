{ inputs, pkgs }:

let
  fakes = import ./fakes.nix { inherit inputs pkgs; };
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
    set -eu
    export CGO_ENABLED=0
    root=/private/tmp/den-task12-pure
    rm -rf "$root"
    mkdir -m 0700 -p "$root/home" "$root/worktree"
    export HOME="$root/home"
    cp -R "$src" "$root/source"
    chmod -R u+w "$root/source"
    (cd "$root/source" && go test ./internal/... ./cmd/... -count=1)

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

    run_sandbox "argument with spaces" "" --plugin-dir user-plugin --mcp-config user-mcp.json --strict-mcp-config
    test "$(grep -Fxc invoked "$root/fence.marker")" = 1
    if grep -Fq preflight "$root/fence.marker"; then exit 1; fi
    policyPath=$(sed -n 's/^policy=<\(.*\)>$/\1/p' "$root/fence.log")
    scratchPath=$(sed -n 's/^tmpdir=<\(.*\)>$/\1/p' "$root/fence.log")
    grep -Fqx "fence-tmpdir=<$scratchPath>" "$root/fence.log"
    grep -Fqx 'argv[0]=<--settings>' "$root/fence-argv.log"
    grep -Fqx "argv[1]=<$policyPath>" "$root/fence-argv.log"
    grep -Fqx 'argv[2]=<--expose-host-path>' "$root/fence-argv.log"
    grep -Fqx "argv[3]=<$REPOWOLF_CA_FILE>" "$root/fence-argv.log"
    grep -Fqx 'argv[4]=<-->' "$root/fence-argv.log"
    grep -Fqx 'argv[5]=<${fakes.fakeClaude}/bin/claude>' "$root/fence-argv.log"
    grep -Fqx 'argv[6]=<--dangerously-skip-permissions>' "$root/fence-argv.log"
    grep -Fqx 'argv[7]=<--settings>' "$root/fence-argv.log"
    grep -Fqx 'argv[8]=<${fakes.darwinSettings}>' "$root/fence-argv.log"
    grep -Fqx 'argv[9]=<argument with spaces>' "$root/fence-argv.log"
    test "$(grep -Fxc 'repowolf-git-ssh <upload> <github.com>' "$root/repowolf.log")" = 2
    test "$(grep -Fxc 'repowolf-git-ssh <receive> <github.com>' "$root/repowolf.log")" = 1
    git --git-dir="$root/git-remote.git" show-ref --verify --quiet refs/heads/pushed
    unset DEN_FAKE_GIT_ROOT DEN_FAKE_GIT_REMOTE
    if grep -Fq "$token" "$root/policy.json" "$root/fence.log" "$root/agent.log"; then exit 1; fi

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

    export CLAUDE_CODE_SIMPLE=1 DEN_FAKE_EXPECT_SIMPLE_SCRUB=1
    run_sandbox ordinary
    unset CLAUDE_CODE_SIMPLE DEN_FAKE_EXPECT_SIMPLE_SCRUB

    ptyRunner="$root/pty-runner"
    cat > "$ptyRunner" <<EOF
    #!${pkgs.bash}/bin/bash
    cd "$root/worktree"
    exec ${sandbox}/bin/claude
    EOF
    chmod 0700 "$ptyRunner"
    export DEN_PTY_LOGICAL_ROOT="$root"
    ${pkgs.python3}/bin/python ${./pty-driver.py} "$ptyRunner" "$root"
    unset DEN_PTY_LOGICAL_ROOT

    rm -rf "$root"
    touch "$out"
  ''
