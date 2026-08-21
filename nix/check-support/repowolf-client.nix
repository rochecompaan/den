{ pkgs, repowolfClient }:

let
  clientClosure = pkgs.closureInfo {
    rootPaths = [ repowolfClient ];
  };
in
pkgs.runCommand "repowolf-client-closure-check" { } ''
  set -eu

  expected_bin_entries="$(printf '%s\n' gh repowolf-client repowolf-git-ssh)"
  actual_bin_entries="$(find ${repowolfClient}/bin -mindepth 1 -maxdepth 1 -printf '%f\n' | sort)"
  if test "$actual_bin_entries" != "$expected_bin_entries"; then
    echo "unexpected RepoWolf client bin entries:" >&2
    printf '%s\n' "$actual_bin_entries" >&2
    exit 1
  fi

  test -f ${repowolfClient}/bin/repowolf-client
  if ! test ! -L ${repowolfClient}/bin/repowolf-client; then
    echo "RepoWolf client executable must not be a symlink" >&2
    exit 1
  fi
  test -x ${repowolfClient}/bin/repowolf-client
  test "$(readlink ${repowolfClient}/bin/gh)" = repowolf-client
  test "$(readlink ${repowolfClient}/bin/repowolf-git-ssh)" = repowolf-client
  test ! -e ${repowolfClient}/bin/repowolf

  while IFS= read -r path; do
    case "''${path##*/}" in
      *openssh*|*github-cli*|*gh-*|*repowolf-server*|*private-key*|*credentials*)
        echo "forbidden path in RepoWolf client closure: $path" >&2
        exit 1
        ;;
    esac
  done < ${clientClosure}/store-paths

  touch "$out"
''
