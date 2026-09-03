#define _DARWIN_C_SOURCE
#define _POSIX_C_SOURCE 200809L

#include "resolver_acl.h"

#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/acl.h>
#include <sys/stat.h>
#include <unistd.h>
#include <uuid/uuid.h>

static void require(int condition, const char *message) {
  if (!condition) {
    perror(message);
    exit(1);
  }
}

int main(void) {
  char directory[] = "/tmp/den-resolver-acl.XXXXXX";
  require(mkdtemp(directory) != NULL, "mkdtemp");
  require(chmod(directory, 0755) == 0, "chmod directory");
  int descriptor = open(directory, O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC);
  require(descriptor >= 0, "open directory");
  require(resolver_directory_acl_safe(descriptor, NULL) == 1, "ordinary mode-only ACL rejected");

  acl_t acl = acl_init(1);
  require(acl != NULL, "acl_init");
  acl_entry_t entry;
  require(acl_create_entry(&acl, &entry) == 0, "acl_create_entry");
  require(acl_set_tag_type(entry, ACL_EXTENDED_ALLOW) == 0, "acl_set_tag_type");
  uuid_t principal = {0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42,
                      0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42};
  require(acl_set_qualifier(entry, &principal) == 0, "acl_set_qualifier");
  acl_permset_t permissions;
  require(acl_get_permset(entry, &permissions) == 0, "acl_get_permset");
  require(acl_clear_perms(permissions) == 0, "acl_clear_perms");
  require(acl_add_perm(permissions, ACL_WRITE_DATA) == 0, "acl_add_perm write");
  require(acl_add_perm(permissions, ACL_DELETE_CHILD) == 0, "acl_add_perm delete");
  require(acl_set_permset(entry, permissions) == 0, "acl_set_permset");
  require(acl_set_fd_np(descriptor, acl, ACL_TYPE_EXTENDED) == 0, "acl_set_fd_np");
  require(resolver_directory_acl_safe(descriptor, NULL) == 0,
          "named write/delete extended ACL accepted");

  acl_t empty = acl_init(0);
  require(empty != NULL, "acl_init empty");
  require(acl_set_fd_np(descriptor, empty, ACL_TYPE_EXTENDED) == 0, "clear extended ACL");
  require(acl_free(empty) == 0, "acl_free empty");
  require(acl_free(acl) == 0, "acl_free");
  require(close(descriptor) == 0, "close directory");
  require(rmdir(directory) == 0, "rmdir");
  puts("Darwin resolver ACL tests passed");
  return 0;
}
