#define _DARWIN_C_SOURCE
#define _POSIX_C_SOURCE 200809L

#include "resolver_transaction.h"

#include <errno.h>
#include <fcntl.h>
#include <stdint.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

static const char *resolver_name = "resolver";
static const char *target_name = "registry.npmjs.org";

static struct resolver_identity identity_of(const struct stat *status) {
  return (struct resolver_identity){.device = status->st_dev, .inode = status->st_ino};
}

static bool same_identity(struct resolver_identity left, struct resolver_identity right) {
  return left.device == right.device && left.inode == right.inode;
}

static bool safe_directory(const struct stat *status, uid_t required_owner) {
  return S_ISDIR(status->st_mode) && status->st_uid == required_owner &&
         (status->st_mode & (S_IWGRP | S_IWOTH)) == 0;
}

static int write_all(int descriptor, const char *contents, size_t length) {
  size_t offset = 0;
  while (offset < length) {
    ssize_t written = write(descriptor, contents + offset, length - offset);
    if (written < 0) {
      if (errno == EINTR) {
        continue;
      }
      return -1;
    }
    if (written == 0) {
      errno = EIO;
      return -1;
    }
    offset += (size_t)written;
  }
  return 0;
}

static int verify_contents(int descriptor, const char *expected, size_t length) {
  char buffer[128];
  if (length > sizeof(buffer) || lseek(descriptor, 0, SEEK_SET) < 0) {
    return -1;
  }
  size_t offset = 0;
  while (offset < length) {
    ssize_t count = read(descriptor, buffer + offset, length - offset);
    if (count < 0) {
      if (errno == EINTR) {
        continue;
      }
      return -1;
    }
    if (count == 0) {
      errno = EIO;
      return -1;
    }
    offset += (size_t)count;
  }
  char extra;
  if (read(descriptor, &extra, 1) != 0 || memcmp(buffer, expected, length) != 0) {
    errno = EIO;
    return -1;
  }
  return 0;
}

static int close_descriptors(struct resolver_transaction *transaction) {
  int close_error = 0;
  int *descriptors[] = {&transaction->target_fd, &transaction->resolver_fd,
                        &transaction->etc_fd, &transaction->root_fd};
  for (size_t index = 0; index < sizeof(descriptors) / sizeof(descriptors[0]); ++index) {
    if (*descriptors[index] >= 0 && close(*descriptors[index]) != 0 && close_error == 0) {
      close_error = errno;
    }
    *descriptors[index] = -1;
  }
  return close_error;
}

void resolver_transaction_init(struct resolver_transaction *transaction) {
  memset(transaction, 0, sizeof(*transaction));
  transaction->root_fd = -1;
  transaction->etc_fd = -1;
  transaction->resolver_fd = -1;
  transaction->target_fd = -1;
}

static int fail_begin(struct resolver_transaction *transaction, int original_error) {
  (void)resolver_transaction_cleanup(transaction);
  errno = original_error;
  return -1;
}

int resolver_transaction_begin(struct resolver_transaction *transaction,
                               const char *root, uid_t required_owner,
                               const char *contents) {
  resolver_transaction_init(transaction);
  int directory_flags = O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW;
  transaction->root_fd = open(root, directory_flags);
  if (transaction->root_fd < 0) {
    return -1;
  }
  transaction->etc_fd = openat(transaction->root_fd, "etc", directory_flags);
  if (transaction->etc_fd < 0) {
    return fail_begin(transaction, errno);
  }
  struct stat status;
  if (fstat(transaction->etc_fd, &status) != 0) {
    return fail_begin(transaction, errno);
  }
  if (!safe_directory(&status, required_owner)) {
    return fail_begin(transaction, EPERM);
  }

  if (mkdirat(transaction->etc_fd, resolver_name, 0755) == 0) {
    transaction->resolver_created = true;
  } else if (errno != EEXIST) {
    return fail_begin(transaction, errno);
  }
  transaction->resolver_fd = openat(transaction->etc_fd, resolver_name, directory_flags);
  if (transaction->resolver_fd < 0) {
    return fail_begin(transaction, errno);
  }
  if (fstat(transaction->resolver_fd, &status) != 0) {
    return fail_begin(transaction, errno);
  }
  if (!safe_directory(&status, required_owner)) {
    return fail_begin(transaction, EPERM);
  }
  transaction->resolver_identity = identity_of(&status);

  transaction->target_fd = openat(transaction->resolver_fd, target_name,
                                  O_RDWR | O_CREAT | O_EXCL | O_NOFOLLOW | O_CLOEXEC, 0644);
  if (transaction->target_fd < 0) {
    return fail_begin(transaction, errno);
  }
  transaction->target_created = true;
  if (fstat(transaction->target_fd, &status) != 0) {
    return fail_begin(transaction, errno);
  }
  if (!S_ISREG(status.st_mode) || status.st_uid != required_owner) {
    return fail_begin(transaction, EPERM);
  }
  transaction->target_identity = identity_of(&status);

  size_t length = strlen(contents);
  if (write_all(transaction->target_fd, contents, length) != 0 ||
      fchmod(transaction->target_fd, 0644) != 0 || fsync(transaction->target_fd) != 0 ||
      verify_contents(transaction->target_fd, contents, length) != 0 ||
      fsync(transaction->resolver_fd) != 0) {
    return fail_begin(transaction, errno);
  }
  return 0;
}

static int named_identity(int directory, const char *name, struct resolver_identity *identity,
                          mode_t *mode) {
  struct stat status;
  if (fstatat(directory, name, &status, AT_SYMLINK_NOFOLLOW) != 0) {
    return -1;
  }
  *identity = identity_of(&status);
  if (mode != NULL) {
    *mode = status.st_mode;
  }
  return 0;
}

int resolver_transaction_cleanup(struct resolver_transaction *transaction) {
  int cleanup_error = 0;
  if (transaction->target_created) {
    struct resolver_identity named;
    if (named_identity(transaction->resolver_fd, target_name, &named, NULL) != 0 ||
        !same_identity(named, transaction->target_identity)) {
      cleanup_error = ESTALE;
    } else if (unlinkat(transaction->resolver_fd, target_name, 0) != 0) {
      cleanup_error = errno;
    } else {
      struct stat absent;
      if (fstatat(transaction->resolver_fd, target_name, &absent, AT_SYMLINK_NOFOLLOW) == 0 ||
          errno != ENOENT || fsync(transaction->resolver_fd) != 0) {
        cleanup_error = errno == 0 ? EIO : errno;
      }
    }
  }

  if (transaction->resolver_created && cleanup_error == 0) {
    struct resolver_identity named;
    mode_t mode = 0;
    if (named_identity(transaction->etc_fd, resolver_name, &named, &mode) != 0 ||
        !S_ISDIR(mode) || !same_identity(named, transaction->resolver_identity)) {
      cleanup_error = ESTALE;
    } else if (unlinkat(transaction->etc_fd, resolver_name, AT_REMOVEDIR) != 0) {
      cleanup_error = errno;
    } else {
      struct stat absent;
      if (fstatat(transaction->etc_fd, resolver_name, &absent, AT_SYMLINK_NOFOLLOW) == 0 ||
          errno != ENOENT || fsync(transaction->etc_fd) != 0) {
        cleanup_error = errno == 0 ? EIO : errno;
      }
    }
  }

  int close_error = close_descriptors(transaction);
  if (cleanup_error == 0 && close_error != 0) {
    cleanup_error = close_error;
  }
  transaction->target_created = false;
  transaction->resolver_created = false;
  if (cleanup_error != 0) {
    errno = cleanup_error;
    return -1;
  }
  return 0;
}
