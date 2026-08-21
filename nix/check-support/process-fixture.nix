{ pkgs }:

pkgs.writeShellScriptBin "den-process-fixture" ''
  set -eu
  case "''${1-}" in
    exit) exit "''${2-0}" ;;
    signal) kill -"''${2-TERM}" "$$" ;;
    pty)
      test -t 0
      pgrp="$(${pkgs.procps}/bin/ps -o pgrp= -p $$ | ${pkgs.coreutils}/bin/tr -d ' ')"
      tpgid="$(${pkgs.procps}/bin/ps -o tpgid= -p $$ | ${pkgs.coreutils}/bin/tr -d ' ')"
      test "$pgrp" = "$tpgid"
      printf 'pty-ok\n'
      ;;
    job-control)
      : "''${DEN_PROCESS_PID_FILE:?}"
      : "''${DEN_PROCESS_SIGNAL_FILE:?}"
      printf '%s' "$$" > "$DEN_PROCESS_PID_FILE"
      trap 'printf W >> "$DEN_PROCESS_SIGNAL_FILE"' WINCH
      trap 'printf T >> "$DEN_PROCESS_SIGNAL_FILE"' TSTP
      trap 'printf C >> "$DEN_PROCESS_SIGNAL_FILE"; exit 0' CONT
      while :; do sleep 1; done
      ;;
    *) cat ;;
  esac
''
