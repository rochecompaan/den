{ inputs, pkgs }:

let
  fakes = import ./fakes.nix { inherit inputs pkgs; };
  aclDiagnosticProbe = pkgs.writeShellScriptBin "den-acl-diagnostic-probe" ''
    set -eu
    if test -z "''${DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG-}"; then
      exec /bin/ls "$@"
    fi

    stdout_file=$(${pkgs.coreutils}/bin/mktemp)
    stderr_file=$(${pkgs.coreutils}/bin/mktemp)
    id_stderr_file=$(${pkgs.coreutils}/bin/mktemp)
    cleanup() {
      ${pkgs.coreutils}/bin/rm -f "$stdout_file" "$stderr_file" "$id_stderr_file"
    }
    trap cleanup EXIT

    status=0
    /bin/ls "$@" > "$stdout_file" 2> "$stderr_file" || status=$?
    owner_name_status=0
    owner_name=$(${pkgs.coreutils}/bin/id -un 2> "$id_stderr_file") || owner_name_status=$?
    owner_uid_status=0
    owner_uid=$(${pkgs.coreutils}/bin/id -u 2>> "$id_stderr_file") || owner_uid_status=$?
    target="''${!#}"

    export DEN_ACL_PROBE_STATUS="$status"
    export DEN_ACL_PROBE_TARGET="$target"
    export DEN_ACL_PROBE_OWNER_NAME="$owner_name"
    export DEN_ACL_PROBE_OWNER_NAME_STATUS="$owner_name_status"
    export DEN_ACL_PROBE_OWNER_UID="$owner_uid"
    export DEN_ACL_PROBE_OWNER_UID_STATUS="$owner_uid_status"
    ${pkgs.python3}/bin/python3 - "$stdout_file" "$stderr_file" "$id_stderr_file" <<'PY'
    import os
    import re
    import sys

    stdout_path, stderr_path, id_stderr_path = sys.argv[1:]
    native_root = os.environ.get("DEN_NATIVE_HOST_ROOT", "")
    original = os.environ.get("DEN_CONFIGDIR_ACL_ORIGINAL_PATH", "")
    target = os.environ["DEN_ACL_PROBE_TARGET"]
    owner_name = os.environ["DEN_ACL_PROBE_OWNER_NAME"]

    def scrub(value):
        for path, replacement in sorted(
            (
                (native_root, "<native-host-root>"),
                (original, "<original-path>"),
                (target, "<probe-path>"),
            ),
            key=lambda item: len(item[0]),
            reverse=True,
        ):
            if path:
                value = value.replace(path, replacement)
        value = re.sub(
            r"^([bcdlps-][^\s]*\s+\d+\s+)(\S+)(\s+)(\S+)",
            lambda match: match.group(1)
            + ("<invoking-user>" if owner_name and match.group(2) == owner_name else "<other-user>")
            + match.group(3)
            + "<group>",
            value,
            flags=re.MULTILINE,
        )
        value = re.sub(
            r"user:([^\s]+)",
            lambda match: "user:<invoking-user>"
            if owner_name and match.group(1) == owner_name
            else "user:<other-user>",
            value,
        )
        value = re.sub(r"group:([^\s]+)", "group:<group>", value)
        value = re.sub(r"/private/tmp/[^\s]+", "<private-tmp-path>", value)
        if owner_name:
            value = value.replace(owner_name, "<invoking-user>")
        return value

    target_kind = "directory-handle" if target.startswith("/dev/fd/") else "path"
    with open(os.environ["DEN_CONFIGDIR_ACL_DIAGNOSTIC_LOG"], "a", encoding="utf-8") as log:
        log.write(
            "acl-probe command=/bin/ls -lde target=<%s> exit=%s\n"
            % (target_kind, os.environ["DEN_ACL_PROBE_STATUS"])
        )
        log.write(
            "acl-probe id-u exit=%s value=%s; id-un exit=%s value=%s\n"
            % (
                os.environ["DEN_ACL_PROBE_OWNER_UID_STATUS"],
                "<resolved>" if os.environ["DEN_ACL_PROBE_OWNER_UID"] else "<unresolved>",
                os.environ["DEN_ACL_PROBE_OWNER_NAME_STATUS"],
                "<invoking-user>" if owner_name else "<unresolved>",
            )
        )
        for label, path in (
            ("stdout", stdout_path),
            ("stderr", stderr_path),
            ("id-stderr", id_stderr_path),
        ):
            with open(path, "r", encoding="utf-8", errors="replace") as source:
                content = scrub(source.read())
            log.write("acl-probe %s:\n%s" % (label, content or "<empty>\n"))

    PY

    ${pkgs.coreutils}/bin/cat "$stdout_file"
    ${pkgs.coreutils}/bin/cat "$stderr_file" >&2
    exit "$status"
  '';
  inheritedSandbox = fakes.mkSandbox { };
  explicitSandbox = fakes.mkSandbox {
    configDir = "/private/tmp/den-claude-startup-runtime-placeholder";
  };
in
pkgs.writeShellApplication {
  name = "claude-startup";
  runtimeInputs = [ pkgs.coreutils pkgs.gitMinimal pkgs.gnugrep pkgs.jq ];
  derivationArgs = {
    passthru.denHostFixturePlatform = "darwin";
  };
  text = ''
    export DEN_CLAUDE_STARTUP_INHERITED_MANIFEST=${inheritedSandbox.denManifest}
    export DEN_CLAUDE_STARTUP_EXPLICIT_MANIFEST=${explicitSandbox.denManifest}
    export DEN_CLAUDE_STARTUP_LAUNCHER=${fakes.launcher}/bin/den-launcher
    export DEN_CLAUDE_STARTUP_ACL_PROBE=${aclDiagnosticProbe}/bin/den-acl-diagnostic-probe
    ${builtins.readFile ./claude-startup-runtime-manifest.sh}
    ${builtins.readFile ./claude-startup-darwin.sh}
  '';
}
