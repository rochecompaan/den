{ pkgs, repowolfClient }:

let
  clientClosure = pkgs.closureInfo {
    rootPaths = [ repowolfClient ];
  };
in
pkgs.runCommand "repowolf-client-closure-check" { } ''
  set -eu

  test -f ${repowolfClient}/bin/repowolf-client
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
