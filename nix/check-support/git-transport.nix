{ pkgs }:

let
  fakeSSH = pkgs.writeShellScript "repowolf-git-ssh" ''
    set -eu
    if test "$1" = -G; then
      exit 1
    fi
    printf '%s\n' "$*" >> "$TMPDIR/transport.log"
    test "$1" = git@github.com
    case "$2" in
      "git-upload-pack 'owner/repo.git'") exec ${pkgs.gitMinimal}/bin/git-upload-pack "$TMPDIR/remotes/owner/repo.git" ;;
      "git-receive-pack 'owner/repo.git'") exec ${pkgs.gitMinimal}/bin/git-receive-pack "$TMPDIR/remotes/owner/repo.git" ;;
      *) exit 1 ;;
    esac
  '';
in
pkgs.runCommand "git-transport-check"
  {
    nativeBuildInputs = [ pkgs.coreutils pkgs.diffutils pkgs.gitMinimal ];
  }
  ''
    set -eu

    export HOME="$TMPDIR/home"
    mkdir -p "$HOME" "$TMPDIR/remotes/owner"
    remote="$TMPDIR/remotes/owner/repo.git"

    git init --bare "$remote" >/dev/null
    git init "$TMPDIR/source" >/dev/null
    git -C "$TMPDIR/source" config user.name test
    git -C "$TMPDIR/source" config user.email test@example.invalid
    touch "$TMPDIR/source/file"
    git -C "$TMPDIR/source" add file
    git -C "$TMPDIR/source" commit -m initial >/dev/null
    git -C "$TMPDIR/source" remote add origin "$remote"
    git -C "$TMPDIR/source" push origin HEAD:main >/dev/null
    git -C "$remote" symbolic-ref HEAD refs/heads/main

    cat > "$TMPDIR/global-helper" <<EOF
#!/bin/sh
touch "$TMPDIR/global-helper-ran"
EOF
    cat > "$TMPDIR/repo-helper" <<EOF
#!/bin/sh
touch "$TMPDIR/repo-helper-ran"
EOF
    chmod +x "$TMPDIR/global-helper" "$TMPDIR/repo-helper"
    cat > "$HOME/.gitconfig" <<EOF
[credential]
  helper = !$TMPDIR/global-helper
EOF

    export PATH="${pkgs.gitMinimal}/bin:${pkgs.coreutils}/bin:${pkgs.diffutils}/bin"
    export GIT_TERMINAL_PROMPT=0
    export GIT_SSH_COMMAND="${fakeSSH}"
    export GIT_CONFIG_COUNT=3
    export GIT_CONFIG_KEY_0=url.git@github.com:.insteadOf
    export GIT_CONFIG_VALUE_0=https://github.com/
    export GIT_CONFIG_KEY_1=credential.helper
    export GIT_CONFIG_VALUE_1=
    export GIT_CONFIG_KEY_2=core.sshCommand
    export GIT_CONFIG_VALUE_2="${fakeSSH}"
    unset GIT_ASKPASS SSH_ASKPASS GIT_SSH GIT_CONFIG_GLOBAL GIT_CONFIG_SYSTEM GIT_CONFIG_PARAMETERS

    git clone https://github.com/owner/repo.git "$TMPDIR/clone" >/dev/null
    git -C "$TMPDIR/clone" config credential.helper "!$TMPDIR/repo-helper"
    cp "$HOME/.gitconfig" "$TMPDIR/global-before"
    cp "$TMPDIR/clone/.git/config" "$TMPDIR/repo-before"

    git -C "$TMPDIR/clone" fetch origin >/dev/null
    git -C "$TMPDIR/clone" push origin HEAD:refs/heads/pushed >/dev/null
    printf 'protocol=https\nhost=github.com\n\n' | git credential fill >/dev/null 2>&1 || true

    test ! -e "$TMPDIR/global-helper-ran"
    test ! -e "$TMPDIR/repo-helper-ran"
    cmp "$TMPDIR/global-before" "$HOME/.gitconfig"
    cmp "$TMPDIR/repo-before" "$TMPDIR/clone/.git/config"
    transport_count="$(wc -l < "$TMPDIR/transport.log")"
    if test "$transport_count" -ne 3; then
      echo "Git invoked repowolf-git-ssh $transport_count times, want 3" >&2
      cat "$TMPDIR/transport.log" >&2
      exit 1
    fi
    if grep -v "^git@github.com git-\\(upload\\|receive\\)-pack 'owner/repo.git'$" "$TMPDIR/transport.log"; then
      echo "Git used an unexpected transport helper" >&2
      exit 1
    fi

    touch "$out"
  ''
