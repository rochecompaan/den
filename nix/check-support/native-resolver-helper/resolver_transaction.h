#ifndef DEN_NATIVE_RESOLVER_TRANSACTION_H
#define DEN_NATIVE_RESOLVER_TRANSACTION_H

#include <stdbool.h>
#include <sys/types.h>

struct resolver_identity {
  dev_t device;
  ino_t inode;
};

struct resolver_transaction {
  int root_fd;
  int etc_fd;
  int resolver_fd;
  int target_fd;
  bool resolver_created;
  bool target_created;
  struct resolver_identity resolver_identity;
  struct resolver_identity target_identity;
};

void resolver_transaction_init(struct resolver_transaction *transaction);
int resolver_transaction_begin(struct resolver_transaction *transaction,
                               const char *root, uid_t required_owner,
                               const char *contents);
int resolver_transaction_cleanup(struct resolver_transaction *transaction);

#endif
