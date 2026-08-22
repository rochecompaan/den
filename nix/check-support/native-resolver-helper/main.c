#define _POSIX_C_SOURCE 200809L

#include "resolver_transaction.h"

#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

static volatile sig_atomic_t received_signal;

static void handle_signal(int signal_number) {
  received_signal = signal_number;
}

static int install_signal_handlers(void) {
  struct sigaction action;
  memset(&action, 0, sizeof(action));
  action.sa_handler = handle_signal;
  if (sigemptyset(&action.sa_mask) != 0) {
    return -1;
  }
  for (int signal_number = SIGHUP; signal_number <= SIGTERM; ++signal_number) {
    if ((signal_number == SIGHUP || signal_number == SIGINT || signal_number == SIGTERM) &&
        sigaction(signal_number, &action, NULL) != 0) {
      return -1;
    }
  }
  return 0;
}

static int write_ready(void) {
  static const char ready[] = "READY\n";
  size_t offset = 0;
  while (offset < sizeof(ready) - 1) {
    ssize_t written = write(STDOUT_FILENO, ready + offset, sizeof(ready) - 1 - offset);
    if (written < 0) {
      if (errno == EINTR && received_signal == 0) {
        continue;
      }
      return -1;
    }
    offset += (size_t)written;
  }
  return 0;
}

int main(int argument_count, char **arguments) {
  (void)arguments;
  static const char contents[] = "nameserver 127.0.0.1\nport 38415\n";
  if (argument_count != 1 || geteuid() != 0) {
    fprintf(stderr, "resolver helper requires no arguments and effective uid 0\n");
    return 2;
  }
  if (install_signal_handlers() != 0) {
    perror("resolver helper signal setup");
    return 1;
  }

  struct resolver_transaction transaction;
  resolver_transaction_init(&transaction);
  if (resolver_transaction_begin(&transaction, "/", 0, contents) != 0) {
    perror("resolver helper setup");
    return 1;
  }
  if (write_ready() != 0) {
    perror("resolver helper readiness");
    (void)resolver_transaction_cleanup(&transaction);
    return 1;
  }

  int input_error = 0;
  char buffer[64];
  while (received_signal == 0) {
    ssize_t count = read(STDIN_FILENO, buffer, sizeof(buffer));
    if (count > 0) {
      continue;
    }
    if (count == 0) {
      break;
    }
    if (errno == EINTR) {
      continue;
    }
    input_error = errno;
    break;
  }

  if (resolver_transaction_cleanup(&transaction) != 0) {
    perror("resolver helper cleanup");
    return 1;
  }
  if (input_error != 0) {
    errno = input_error;
    perror("resolver helper control pipe");
    return 1;
  }
  if (received_signal != 0) {
    return 128 + received_signal;
  }
  return 0;
}
