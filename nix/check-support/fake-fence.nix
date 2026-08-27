{ pkgs }:

pkgs.writeShellScriptBin "fence" ''
  set -eu
  if test -n "''${DEN_FAKE_FENCE_MARKER-}"; then
    printf 'invoked\n' >> "$DEN_FAKE_FENCE_MARKER"
  fi
  if test "''${1-}" = --linux-features; then
    if test -n "''${DEN_FAKE_FENCE_MARKER-}"; then
      printf 'preflight\n' >> "$DEN_FAKE_FENCE_MARKER"
    fi
    printf 'Feature  Purpose  Status  Detail\n'
    printf 'Network namespace  direct network isolation  ok  available\n'
    exit 0
  fi

  if test -n "''${DEN_FAKE_FENCE_ARGV_LOG-}"; then
    : > "$DEN_FAKE_FENCE_ARGV_LOG"
    index=0
    for argument in "$@"; do
      printf 'argv[%s]=<%s>\n' "$index" "$argument" >> "$DEN_FAKE_FENCE_ARGV_LOG"
      index=$((index + 1))
    done
  fi

  policy=
  while test "$#" -gt 0 && test "$1" != --; do
    if test "$1" = --settings; then
      policy=$2
      shift 2
    else
      shift
    fi
  done
  test -n "$policy"
  test "$(''${STAT:-${pkgs.coreutils}/bin/stat} -c %a "$policy")" = 400
  test "$TMPDIR" = "$DEN_FENCE_TMPDIR"
  test "$TMPDIR" != "$(dirname "$policy")"
  test "$DEN_FENCE_POLICY_FILE" = "$policy"
  if test -n "''${DEN_FAKE_POLICY_COPY-}"; then
    rm -f "$DEN_FAKE_POLICY_COPY"
    ${pkgs.coreutils}/bin/cp "$policy" "$DEN_FAKE_POLICY_COPY"
  fi
  if test -n "''${DEN_FAKE_FENCE_LOG-}"; then
    {
      printf 'policy-mode=400\n'
      printf 'separate-scratch=yes\n'
      printf 'tmpdir=<%s>\n' "$TMPDIR"
      printf 'fence-tmpdir=<%s>\n' "$DEN_FENCE_TMPDIR"
      printf 'policy=<%s>\n' "$DEN_FENCE_POLICY_FILE"
    } > "$DEN_FAKE_FENCE_LOG"
  fi
  test "''${1-}" = --
  shift
  exec "$@"
''
