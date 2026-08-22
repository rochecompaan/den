#define _DARWIN_C_SOURCE
#define _POSIX_C_SOURCE 200809L

#include "resolver_transaction.h"

#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

static const char *target_name = "registry.npmjs.org";
static const char *fixture_contents = "nameserver 127.0.0.1\nport 38415\n";

struct acl_fixture {
  int calls;
  int reject_call;
  int error_call;
};

static int fixture_acl_probe(int descriptor, void *context_pointer) {
  (void)descriptor;
  struct acl_fixture *context = context_pointer;
  context->calls++;
  if (context->calls == context->error_call) {
    errno = EIO;
    return -1;
  }
  return context->calls == context->reject_call ? 0 : 1;
}

static int begin_safe(struct resolver_transaction *transaction, const char *root) {
  struct acl_fixture context = {0};
  return resolver_transaction_begin(transaction, root, geteuid(), fixture_contents,
                                    fixture_acl_probe, &context);
}

static void fail(const char *message) {
  perror(message);
  exit(1);
}

static void require(bool condition, const char *message) {
  if (!condition) {
    fprintf(stderr, "%s\n", message);
    exit(1);
  }
}

static char *make_root(bool resolver_exists, mode_t resolver_mode) {
  char *root = strdup("/tmp/den-resolver-helper.XXXXXX");
  if (root == NULL || mkdtemp(root) == NULL) {
    fail("mkdtemp");
  }
  char etc[512];
  char resolver[512];
  snprintf(etc, sizeof(etc), "%s/etc", root);
  snprintf(resolver, sizeof(resolver), "%s/etc/resolver", root);
  if (mkdir(etc, 0755) != 0) {
    fail("mkdir etc");
  }
  if (resolver_exists && (mkdir(resolver, resolver_mode) != 0 || chmod(resolver, resolver_mode) != 0)) {
    fail("mkdir resolver");
  }
  return root;
}

static void remove_root(char *root) {
  char target[512];
  char resolver[512];
  char etc[512];
  snprintf(target, sizeof(target), "%s/etc/resolver/%s", root, target_name);
  snprintf(resolver, sizeof(resolver), "%s/etc/resolver", root);
  snprintf(etc, sizeof(etc), "%s/etc", root);
  (void)unlink(target);
  (void)rmdir(resolver);
  (void)rmdir(etc);
  (void)rmdir(root);
  free(root);
}

static void write_file(const char *path, const char *contents) {
  int descriptor = open(path, O_WRONLY | O_CREAT | O_EXCL, 0600);
  if (descriptor < 0) {
    fail("open test file");
  }
  size_t length = strlen(contents);
  if (write(descriptor, contents, length) != (ssize_t)length || close(descriptor) != 0) {
    fail("write test file");
  }
}

static void require_contents(const char *path, const char *expected) {
  char buffer[128] = {0};
  int descriptor = open(path, O_RDONLY | O_NOFOLLOW);
  if (descriptor < 0) {
    fail("open result");
  }
  ssize_t length = read(descriptor, buffer, sizeof(buffer) - 1);
  if (length < 0 || close(descriptor) != 0) {
    fail("read result");
  }
  require(strcmp(buffer, expected) == 0, "file contents changed unexpectedly");
}

static void test_owned_cleanup(void) {
  char *root = make_root(false, 0);
  struct resolver_transaction transaction;
  resolver_transaction_init(&transaction);
  require(begin_safe(&transaction, root) == 0,
          "owned transaction setup failed");
  require(resolver_transaction_cleanup(&transaction) == 0, "owned transaction cleanup failed");
  char resolver[512];
  snprintf(resolver, sizeof(resolver), "%s/etc/resolver", root);
  errno = 0;
  require(access(resolver, F_OK) != 0 && errno == ENOENT, "created resolver directory remains");
  remove_root(root);
}

static void test_existing_directory_remains(void) {
  char *root = make_root(true, 0755);
  struct resolver_transaction transaction;
  resolver_transaction_init(&transaction);
  require(begin_safe(&transaction, root) == 0,
          "existing-directory setup failed");
  require(resolver_transaction_cleanup(&transaction) == 0, "existing-directory cleanup failed");
  char resolver[512];
  snprintf(resolver, sizeof(resolver), "%s/etc/resolver", root);
  struct stat status;
  require(lstat(resolver, &status) == 0 && S_ISDIR(status.st_mode), "existing resolver directory was removed");
  remove_root(root);
}

static void test_preexisting_target_refused(void) {
  char *root = make_root(true, 0755);
  char target[512];
  snprintf(target, sizeof(target), "%s/etc/resolver/%s", root, target_name);
  write_file(target, "foreign\n");
  struct resolver_transaction transaction;
  resolver_transaction_init(&transaction);
  require(begin_safe(&transaction, root) != 0,
          "pre-existing resolver target was accepted");
  require_contents(target, "foreign\n");
  require(resolver_transaction_cleanup(&transaction) == 0, "failed setup cleanup failed");
  require_contents(target, "foreign\n");
  remove_root(root);
}

static void test_replacement_refused(void) {
  char *root = make_root(true, 0755);
  struct resolver_transaction transaction;
  resolver_transaction_init(&transaction);
  require(begin_safe(&transaction, root) == 0,
          "replacement setup failed");
  require(unlinkat(transaction.resolver_fd, target_name, 0) == 0, "unlink owned target failed");
  int replacement = openat(transaction.resolver_fd, target_name,
                           O_WRONLY | O_CREAT | O_EXCL | O_NOFOLLOW, 0600);
  require(replacement >= 0, "create replacement failed");
  require(write(replacement, "foreign\n", 8) == 8 && close(replacement) == 0,
          "write replacement failed");
  require(resolver_transaction_cleanup(&transaction) != 0, "replacement cleanup unexpectedly succeeded");
  char target[512];
  snprintf(target, sizeof(target), "%s/etc/resolver/%s", root, target_name);
  require_contents(target, "foreign\n");
  remove_root(root);
}

static void test_directory_replacement_refused(void) {
  char *root = make_root(false, 0);
  struct resolver_transaction transaction;
  resolver_transaction_init(&transaction);
  require(begin_safe(&transaction, root) == 0,
          "directory replacement setup failed");
  require(unlinkat(transaction.resolver_fd, target_name, 0) == 0, "unlink target before directory replacement failed");
  require(unlinkat(transaction.etc_fd, "resolver", AT_REMOVEDIR) == 0, "unlink owned resolver directory failed");
  require(mkdirat(transaction.etc_fd, "resolver", 0755) == 0, "create replacement resolver directory failed");
  require(resolver_transaction_cleanup(&transaction) != 0, "directory replacement cleanup unexpectedly succeeded");
  char resolver[512];
  snprintf(resolver, sizeof(resolver), "%s/etc/resolver", root);
  struct stat status;
  require(lstat(resolver, &status) == 0 && S_ISDIR(status.st_mode), "replacement resolver directory was removed");
  remove_root(root);
}

static void test_extended_directory_acl_refused(void) {
  for (int rejected_directory = 1; rejected_directory <= 2; ++rejected_directory) {
    char *root = make_root(true, 0755);
    struct resolver_transaction transaction;
    resolver_transaction_init(&transaction);
    struct acl_fixture context = {.reject_call = rejected_directory};
    require(resolver_transaction_begin(&transaction, root, geteuid(), fixture_contents,
                                       fixture_acl_probe, &context) != 0,
            "named unprivileged write/delete ACL was accepted");
    require(resolver_transaction_cleanup(&transaction) == 0, "ACL rejection cleanup failed");
    char target[512];
    snprintf(target, sizeof(target), "%s/etc/resolver/%s", root, target_name);
    errno = 0;
    require(access(target, F_OK) != 0 && errno == ENOENT,
            "target was installed before extended ACL rejection");
    remove_root(root);
  }
}

static void test_acl_probe_error_refused(void) {
  char *root = make_root(true, 0755);
  struct resolver_transaction transaction;
  resolver_transaction_init(&transaction);
  struct acl_fixture context = {.error_call = 1};
  require(resolver_transaction_begin(&transaction, root, geteuid(), fixture_contents,
                                     fixture_acl_probe, &context) != 0,
          "directory ACL probe error was accepted");
  require(resolver_transaction_cleanup(&transaction) == 0, "ACL probe error cleanup failed");
  remove_root(root);
}

static void test_live_root_is_private(void) {
  require(strcmp(DEN_RESOLVER_LIVE_ROOT, "/private") == 0,
          "live Darwin resolver root must bypass the /etc symlink");
}

static void test_unsafe_directory_refused(void) {
  char *root = make_root(true, 0777);
  struct resolver_transaction transaction;
  resolver_transaction_init(&transaction);
  require(begin_safe(&transaction, root) != 0,
          "writable resolver directory was accepted");
  require(resolver_transaction_cleanup(&transaction) == 0, "unsafe-directory failed setup cleanup failed");
  remove_root(root);
}

int main(void) {
  test_owned_cleanup();
  test_existing_directory_remains();
  test_preexisting_target_refused();
  test_replacement_refused();
  test_directory_replacement_refused();
  test_extended_directory_acl_refused();
  test_acl_probe_error_refused();
  test_live_root_is_private();
  test_unsafe_directory_refused();
  puts("resolver transaction tests passed");
  return 0;
}
