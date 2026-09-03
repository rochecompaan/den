package native

const nestedFenceContextFile = ".den-native-outer-context"

func nestedFenceWitnessCommand() string {
	return `set -eux
 context="$DEN_FENCE_TMPDIR/.den-native-outer-context"
 test -f "$context" || exit 1
 {
   IFS= read -r outer_parent || exit 1
   IFS= read -r outer_proxy || exit 1
   IFS= read -r outer_sandbox || exit 1
 } < "$context"
 test "$outer_sandbox" = 1 || exit 1
 test "${FENCE_SANDBOX:-}" = 1 || exit 1
 test -n "$outer_parent" || exit 1
 test -n "$outer_proxy" || exit 1
 test -n "${HTTP_PROXY:-}" || exit 1
 case "$HTTP_PROXY" in
   http://127.0.0.1:*|http://localhost:*) ;;
   *) exit 1 ;;
 esac
 "$DEN_NATIVE_CONTEXT_CURL" --silent --show-error --max-time 2 --output /dev/null --proxy "$HTTP_PROXY" http://den-native-witness.invalid/ || exit 1
 printf 'nested-proxy:%s\n' "$HTTP_PROXY"`
}

func temporaryDirectoryContainmentWitnessCommand() string {
	return `
 den_fence_tmpdir_real=$(cd "$DEN_FENCE_TMPDIR" && pwd -P) || exit 1
 den_claude_tmpdir_real=$(cd "$TMPDIR" && pwd -P) || exit 1
 case "$den_claude_tmpdir_real" in
   "$den_fence_tmpdir_real"|"$den_fence_tmpdir_real"/*) ;;
   *) exit 1 ;;
 esac
 unset den_fence_tmpdir_real den_claude_tmpdir_real`
}
