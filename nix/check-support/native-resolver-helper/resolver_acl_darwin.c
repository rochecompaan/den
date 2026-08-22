#include "resolver_acl.h"

#include <errno.h>
#include <stddef.h>
#include <sys/acl.h>

int resolver_directory_acl_safe(int descriptor, void *context) {
  (void)context;
  acl_t acl = acl_get_fd_np(descriptor, ACL_TYPE_EXTENDED);
  if (acl == NULL) {
    return -1;
  }
  acl_entry_t entry;
  int entry_status = acl_get_entry(acl, ACL_FIRST_ENTRY, &entry);
  int saved_error = errno;
  if (acl_free(acl) != 0) {
    return -1;
  }
  if (entry_status < 0) {
    errno = saved_error;
    return -1;
  }
  return entry_status == 0 ? 1 : 0;
}
