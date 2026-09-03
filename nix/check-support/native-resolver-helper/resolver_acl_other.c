#include "resolver_acl.h"

int resolver_directory_acl_safe(int descriptor, void *context) {
  (void)descriptor;
  (void)context;
  return 1;
}
