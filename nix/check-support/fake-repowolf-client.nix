{ pkgs }:

pkgs.runCommand "fake-repowolf-client" { } ''
  mkdir -p "$out/bin"
  cat > "$out/bin/repowolf-client" <<'EOF'
  #!${pkgs.bash}/bin/bash
  set -eu
  program=''${0##*/}
  case "$program" in
    gh)
      test -n "$REPOWOLF_ENDPOINT"
      test -n "$REPOWOLF_TOKEN"
      test -n "$REPOWOLF_CA_FILE"
      if test -n "''${DEN_FAKE_REPOWOLF_LOG-}"; then printf 'gh\n' >> "$DEN_FAKE_REPOWOLF_LOG"; fi
      exit 0
      ;;
    repowolf-git-ssh) ;;
    *) exit 91 ;;
  esac

  test -n "$REPOWOLF_ENDPOINT"
  test -n "$REPOWOLF_TOKEN"
  test -n "$REPOWOLF_CA_FILE"
  test -n "''${DEN_FAKE_GIT_REMOTE-}"
  case "$*" in *github.com*) ;; *) exit 92 ;; esac
  if test "''${1-}" = -G; then exit 1; fi
  command=''${!#}
  case "$command" in
    git-upload-pack*) operation=upload ;;
    git-receive-pack*) operation=receive ;;
    *) exit 93 ;;
  esac
  if test -n "''${DEN_FAKE_REPOWOLF_LOG-}"; then
    printf 'repowolf-git-ssh <%s> <github.com>\n' "$operation" >> "$DEN_FAKE_REPOWOLF_LOG"
  fi
  case "$operation" in
    upload) exec ${pkgs.gitMinimal}/bin/git-upload-pack "$DEN_FAKE_GIT_REMOTE" ;;
    receive) exec ${pkgs.gitMinimal}/bin/git-receive-pack "$DEN_FAKE_GIT_REMOTE" ;;
  esac
  EOF
  chmod 0555 "$out/bin/repowolf-client"
  ln -s repowolf-client "$out/bin/gh"
  ln -s repowolf-client "$out/bin/repowolf-git-ssh"
''
