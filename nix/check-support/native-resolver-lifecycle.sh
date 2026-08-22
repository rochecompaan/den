# shellcheck shell=bash

resolver_helper_pid=
resolver_helper_input=
resolver_helper_output=

start_resolver_helper() {
  if [[ $# -eq 0 || -n $resolver_helper_pid ]]; then
    return 2
  fi
  coproc DEN_RESOLVER_HELPER_PROCESS { exec "$@"; }
  resolver_helper_pid=$DEN_RESOLVER_HELPER_PROCESS_PID
  resolver_helper_output=${DEN_RESOLVER_HELPER_PROCESS[0]}
  resolver_helper_input=${DEN_RESOLVER_HELPER_PROCESS[1]}

  local ready=
  if ! IFS= read -r ready <&"$resolver_helper_output" || [[ $ready != READY ]]; then
    printf 'native resolver helper failed before readiness\n' >&2
    if ! stop_resolver_helper; then
      printf 'native resolver helper also failed during readiness cleanup\n' >&2
    fi
    return 1
  fi
  if ! exec {resolver_helper_output}<&-; then
    printf 'native resolver helper readiness pipe close failed\n' >&2
    if ! stop_resolver_helper; then
      printf 'native resolver helper also failed during readiness cleanup\n' >&2
    fi
    return 1
  fi
  resolver_helper_output=
}

stop_resolver_helper() {
  local status=0
  if [[ -n $resolver_helper_input ]]; then
    if ! exec {resolver_helper_input}>&-; then
      printf 'native resolver helper control pipe close failed\n' >&2
      status=1
    fi
    resolver_helper_input=
  fi
  if [[ -n $resolver_helper_output ]]; then
    if ! exec {resolver_helper_output}<&-; then
      printf 'native resolver helper readiness pipe close failed\n' >&2
      status=1
    fi
    resolver_helper_output=
  fi
  if [[ -n $resolver_helper_pid ]]; then
    local wait_status=0
    if wait "$resolver_helper_pid"; then
      wait_status=0
    else
      wait_status=$?
    fi
    resolver_helper_pid=
    if (( wait_status != 0 )); then
      status=$wait_status
    fi
  fi
  return "$status"
}

resolver_lifecycle_status() {
  local primary_status=${1:?primary status required}
  local helper_status=${2:?helper status required}
  if (( primary_status != 0 )); then
    return "$primary_status"
  fi
  if (( helper_status != 0 )); then
    return 1
  fi
  return 0
}
