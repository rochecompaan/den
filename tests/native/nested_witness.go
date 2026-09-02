package native

const nestedFenceContextFile = ".den-native-outer-context"

func nestedFenceWitnessCommand() string {
	return `set -eu
 context="$DEN_FENCE_TMPDIR/.den-native-outer-context"
 test -f "$context"
 {
   IFS= read -r outer_parent
   IFS= read -r outer_proxy
   IFS= read -r outer_sandbox
 } < "$context"
 test "$outer_sandbox" = 1
 test "${FENCE_SANDBOX:-}" = 1
 test -n "$outer_parent"
 test -n "$outer_proxy"
 test -n "${HTTP_PROXY:-}"
 test "$outer_proxy" != "$HTTP_PROXY"
 case "$HTTP_PROXY" in
   http://127.0.0.1:*|http://localhost:*) ;;
   *) exit 1 ;;
 esac
 "$DEN_NATIVE_CONTEXT_CURL" --silent --show-error --max-time 2 --output /dev/null --proxy "$HTTP_PROXY" http://den-native-witness.invalid/
 printf 'nested-proxy:%s\n' "$HTTP_PROXY"`
}
