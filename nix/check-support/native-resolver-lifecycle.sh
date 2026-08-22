# shellcheck shell=bash

resolver_helper_pid=
resolver_helper_input=
resolver_helper_output=
resolver_deferred_signal=0
resolver_signal_deferral_active=false

resolver_lifecycle_test_hook() {
  :
}

resolver_defer_signals() {
  if $resolver_signal_deferral_active; then
    return 2
  fi
  resolver_deferred_signal=0
  resolver_signal_deferral_active=true
  trap 'resolver_deferred_signal=129' HUP
  trap 'resolver_deferred_signal=130' INT
  trap 'resolver_deferred_signal=143' TERM
}

resolver_restore_signals() {
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 143' TERM
  resolver_signal_deferral_active=false
}

resolver_transition_status() {
  local operation_status=${1:?operation status required}
  resolver_restore_signals
  local signal_status=$resolver_deferred_signal
  if (( signal_status != 0 )); then
    if [[ -n $resolver_helper_pid ]]; then
      if ! stop_resolver_helper; then
        printf 'native resolver helper failed while completing deferred signal cleanup\n' >&2
      fi
    fi
    return "$signal_status"
  fi
  return "$operation_status"
}

resolver_stop_helper_deferred() {
  local status=0
  local wait_status=0
  resolver_lifecycle_test_hook stop_before_close
  if [[ -n $resolver_helper_input ]]; then
    if ! exec {resolver_helper_input}>&-; then
      printf 'native resolver helper control pipe close failed\n' >&2
      status=1
    fi
    resolver_helper_input=
  fi
  resolver_lifecycle_test_hook stop_after_input_close

  if [[ -n $resolver_helper_pid ]]; then
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
  if [[ -n $resolver_helper_output ]]; then
    if ! exec {resolver_helper_output}<&-; then
      printf 'native resolver helper readiness pipe close failed\n' >&2
      status=1
    fi
    resolver_helper_output=
  fi
  resolver_lifecycle_test_hook stop_after_state_clear
  return "$status"
}

stop_resolver_helper() {
  resolver_defer_signals
  local operation_status=0
  if resolver_stop_helper_deferred; then
    operation_status=0
  else
    operation_status=$?
  fi
  resolver_transition_status "$operation_status"
}

start_resolver_helper() {
  if [[ $# -eq 0 || -n $resolver_helper_pid ]]; then
    return 2
  fi
  resolver_defer_signals
  local operation_status=0
  local ready=

  resolver_lifecycle_test_hook start_before_coproc
  if (( resolver_deferred_signal == 0 )); then
    if coproc DEN_RESOLVER_HELPER_PROCESS { exec "$@"; }; then
      resolver_lifecycle_test_hook start_after_coproc
      resolver_helper_pid=${DEN_RESOLVER_HELPER_PROCESS_PID-}
      resolver_helper_output=${DEN_RESOLVER_HELPER_PROCESS[0]-}
      resolver_helper_input=${DEN_RESOLVER_HELPER_PROCESS[1]-}
      resolver_lifecycle_test_hook start_after_state_record
    else
      operation_status=$?
    fi
  fi

  if (( operation_status == 0 && resolver_deferred_signal == 0 )); then
    if [[ -z $resolver_helper_pid || -z $resolver_helper_output || -z $resolver_helper_input ]]; then
      operation_status=1
    elif IFS= read -r ready <&"$resolver_helper_output" && [[ $ready == READY ]]; then
      resolver_lifecycle_test_hook start_after_readiness
    else
      printf 'native resolver helper failed before readiness\n' >&2
      operation_status=1
    fi
  fi

  if (( operation_status != 0 || resolver_deferred_signal != 0 )); then
    local stop_status=0
    if resolver_stop_helper_deferred; then
      stop_status=0
    else
      stop_status=$?
    fi
    if (( operation_status == 0 && stop_status != 0 )); then
      operation_status=$stop_status
    fi
  elif [[ -n $resolver_helper_output ]]; then
    if ! exec {resolver_helper_output}<&-; then
      printf 'native resolver helper readiness pipe close failed\n' >&2
      operation_status=1
      if ! resolver_stop_helper_deferred; then
        printf 'native resolver helper also failed during readiness cleanup\n' >&2
      fi
    fi
    resolver_helper_output=
    resolver_lifecycle_test_hook start_after_readiness_pipe_close
  fi

  resolver_transition_status "$operation_status"
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
