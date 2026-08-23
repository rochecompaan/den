# shellcheck shell=bash

den_validate_claude_startup_manifest() {
  if [[ $# -ne 2 ]]; then
    printf 'usage: den_validate_claude_startup_manifest BASE RUNTIME\n' >&2
    return 2
  fi
  local base=$1 runtime=$2
  jq -e -n --slurpfile base "$base" --slurpfile runtime "$runtime" '
    ($base | length == 1) and
    ($runtime | length == 1) and
    ($base[0] | has("explicitConfigDir") and has("aclProbe")) and
    ($runtime[0] | has("explicitConfigDir") and has("aclProbe")) and
    (($base[0] | del(.explicitConfigDir, .aclProbe)) ==
     ($runtime[0] | del(.explicitConfigDir, .aclProbe)))
  ' >/dev/null
}

den_adapt_claude_startup_manifest() {
  if [[ $# -ne 4 ]]; then
    printf 'usage: den_adapt_claude_startup_manifest BASE OUTPUT MODE CONFIG_DIR\n' >&2
    return 2
  fi
  local base=$1 output=$2 mode=$3 config_dir=$4
  : "${DEN_CLAUDE_STARTUP_ACL_PROBE:?Darwin ACL probe is required}"
  [[ $DEN_CLAUDE_STARTUP_ACL_PROBE == /* ]] || return 2

  (
    umask 077
    case "$mode" in
      inherited)
        [[ -z $config_dir ]] || return 2
        jq -e '.explicitConfigDir == null' "$base" >/dev/null || return 2
        jq --arg probe "$DEN_CLAUDE_STARTUP_ACL_PROBE" \
          '.aclProbe = [$probe, "-lde"]' "$base" > "$output" || return 2
        ;;
      explicit)
        [[ $config_dir == /* ]] || return 2
        jq --arg probe "$DEN_CLAUDE_STARTUP_ACL_PROBE" --arg config "$config_dir" \
          '.aclProbe = [$probe, "-lde"] | .explicitConfigDir = $config' \
          "$base" > "$output" || return 2
        ;;
      *)
        printf 'unknown manifest adaptation mode: %s\n' "$mode" >&2
        return 2
        ;;
    esac

    chmod 0600 "$output" || return 2
    den_validate_claude_startup_manifest "$base" "$output"
  )
}
