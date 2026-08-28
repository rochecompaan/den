#include "resolver_acl.h"

#include <errno.h>
#include <stddef.h>
#include <sys/acl.h>

int resolver_directory_acl_safe(int descriptor, void *context) {
  (void)context;
  acl_t acl = acl_get_fd_np(descriptor, ACL_TYPE_EXTENDED);
  if (acl == NULL) {
    /* macOS reports an object without an extended ACL via ENOENT. */
    return errno == ENOENT ? 1 : -1;
  }
  int count = acl_entry_count_np(acl);
  int saved_error = errno;
  if (acl_free(acl) != 0) {
    return -1;
  }
  if (count < 0) {
    errno = saved_error;
    return -1;
  }
  return count == 0 ? 1 : 0;
}
