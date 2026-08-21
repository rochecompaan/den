{ pkgs }:

pkgs.writeShellScriptBin "den-process-fixture" ''
  set -eu
  case "''${1-}" in
    exit) exit "''${2-0}" ;;
    signal) kill -"''${2-TERM}" "$$" ;;
    *) cat ;;
  esac
''
