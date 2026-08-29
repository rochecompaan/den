#include <errno.h>
#include <inttypes.h>
#include <limits.h>
#include <membership.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdio.h>
#include <sys/acl.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

struct named_bit {
  int bit;
  const char *name;
};

/* Keep this order stable: the launcher digests these canonical lines. */
static const struct named_bit permissions[] = {
    {ACL_DELETE, "delete"},
    {ACL_READ_ATTRIBUTES, "readattr"},
    {ACL_WRITE_ATTRIBUTES, "writeattr"},
    {ACL_READ_EXTATTRIBUTES, "readextattr"},
    {ACL_WRITE_EXTATTRIBUTES, "writeextattr"},
    {ACL_READ_SECURITY, "readsecurity"},
    {ACL_WRITE_SECURITY, "writesecurity"},
    {ACL_CHANGE_OWNER, "chown"},
    {ACL_LIST_DIRECTORY, "list"},
    {ACL_ADD_FILE, "add_file"},
    {ACL_SEARCH, "search"},
    {ACL_ADD_SUBDIRECTORY, "add_subdirectory"},
    {ACL_DELETE_CHILD, "delete_child"},
};

static const struct named_bit flags[] = {
    {ACL_ENTRY_FILE_INHERIT, "file_inherit"},
    {ACL_ENTRY_DIRECTORY_INHERIT, "directory_inherit"},
    {ACL_ENTRY_LIMIT_INHERIT, "limit_inherit"},
    {ACL_ENTRY_ONLY_INHERIT, "only_inherit"},
    {ACL_ENTRY_INHERITED, "inherited"},
};

static int append_token(char *buffer, size_t capacity, size_t *used,
                        const char *token) {
  int written = snprintf(buffer + *used, capacity - *used, "%s%s",
                         *used == 0 ? "" : ",", token);
  if (written < 0 || (size_t)written >= capacity - *used) {
    return -1;
  }
  *used += (size_t)written;
  return 0;
}

static int parse_fd_argument(const char *argument, int *descriptor,
                             bool *is_fd_argument) {
  static const char prefix[] = "/dev/fd/";
  size_t index = 0;
  unsigned int value = 0;

  while (index < sizeof(prefix) - 1 && argument[index] == prefix[index]) {
    index++;
  }
  if (index != sizeof(prefix) - 1) {
    *is_fd_argument = false;
    return 0;
  }
  if (argument[index] == '\0') {
    return -1;
  }
  for (; argument[index] != '\0'; index++) {
    unsigned int digit;
    if (argument[index] < '0' || argument[index] > '9') {
      *is_fd_argument = false;
      return 0;
    }
    digit = (unsigned int)(argument[index] - '0');
    if (value > ((unsigned int)INT_MAX - digit) / 10U) {
      return -1;
    }
    value = value * 10U + digit;
  }
  *descriptor = (int)value;
  *is_fd_argument = true;
  return 0;
}

static int append_permissions(acl_entry_t entry, char *buffer,
                              size_t capacity, size_t *used) {
  acl_permset_t permission_set;
  acl_permset_mask_t maximal_mask;
  acl_permset_mask_t known_mask = ACL_SYNCHRONIZE;
  size_t index;

  if (acl_get_permset(entry, &permission_set) != 0 ||
      acl_maximal_permset_mask_np(&maximal_mask) != 0) {
    return -1;
  }
  for (index = 0; index < sizeof(permissions) / sizeof(permissions[0]);
       index++) {
    known_mask |= (acl_permset_mask_t)permissions[index].bit;
    int present = acl_get_perm_np(permission_set, permissions[index].bit);
    if (present != 0 && present != 1) {
      return -1;
    }
    if (present == 1 &&
        append_token(buffer, capacity, used, permissions[index].name) != 0) {
      return -1;
    }
  }
  if ((maximal_mask & ~known_mask) != 0) {
    return -1;
  }
  {
    int synchronize = acl_get_perm_np(permission_set, ACL_SYNCHRONIZE);
    if (synchronize != 0) {
      return -1;
    }
  }
  return 0;
}

static int append_flags(acl_entry_t entry, char *buffer, size_t capacity,
                        size_t *used) {
  acl_flagset_t flag_set;
  size_t index;

  if (acl_get_flagset_np(entry, &flag_set) != 0) {
    return -1;
  }
  for (index = 0; index < sizeof(flags) / sizeof(flags[0]); index++) {
    int present = acl_get_flag_np(flag_set, flags[index].bit);
    if (present != 0 && present != 1) {
      return -1;
    }
    if (present == 1 && append_token(buffer, capacity, used, flags[index].name) != 0) {
      return -1;
    }
  }
  return 0;
}

static int print_entry(acl_entry_t entry, unsigned int index) {
  char rights[512] = "";
  size_t used = 0;
  acl_tag_t tag;
  void *qualifier = NULL;
  uuid_t *uuid;
  id_t identifier;
  int identifier_type;
  const char *principal;
  const char *action;
  int result = -1;

  if (acl_get_tag_type(entry, &tag) != 0) {
    goto cleanup;
  }
  if (tag == ACL_EXTENDED_ALLOW) {
    action = "allow";
  } else if (tag == ACL_EXTENDED_DENY) {
    action = "deny";
  } else {
    goto cleanup;
  }

  qualifier = acl_get_qualifier(entry);
  if (qualifier == NULL) {
    goto cleanup;
  }
  uuid = qualifier;
  if (mbr_uuid_to_id(*uuid, &identifier, &identifier_type) != 0 ||
      identifier < 0) {
    goto cleanup;
  }
  if (identifier_type == ID_TYPE_UID) {
    principal = "user";
  } else if (identifier_type == ID_TYPE_GID) {
    principal = "group";
  } else {
    goto cleanup;
  }

  if (append_permissions(entry, rights, sizeof(rights), &used) != 0 ||
      append_flags(entry, rights, sizeof(rights), &used) != 0) {
    goto cleanup;
  }
  if (used == 0 && append_token(rights, sizeof(rights), &used, "none") != 0) {
    goto cleanup;
  }
  if (dprintf(STDOUT_FILENO, "%u: %s:%" PRIuMAX " %s %s\n", index,
              principal, (uintmax_t)identifier, action, rights) < 0) {
    goto cleanup;
  }
  result = 0;

cleanup:
  if (qualifier != NULL && acl_free(qualifier) != 0) {
    result = -1;
  }
  return result;
}

int main(int argc, char *argv[]) {
  acl_t acl = NULL;
  struct stat status;
  int descriptor = -1;
  int entry_status;
  unsigned int index = 0;
  bool fd_mode = false;
  int result = 1;

  if (argc != 2 || parse_fd_argument(argv[1], &descriptor, &fd_mode) != 0) {
    goto cleanup;
  }
  if (fd_mode) {
    if (fstat(descriptor, &status) != 0 || !S_ISDIR(status.st_mode)) {
      goto cleanup;
    }
    acl = acl_get_fd_np(descriptor, ACL_TYPE_EXTENDED);
  } else {
    if (argv[1][0] != '/' || lstat(argv[1], &status) != 0 ||
        S_ISLNK(status.st_mode) || !S_ISDIR(status.st_mode)) {
      goto cleanup;
    }
    acl = acl_get_file(argv[1], ACL_TYPE_EXTENDED);
  }
  if (acl == NULL) {
    if (errno == ENOENT) {
      result = 0;
    }
    goto cleanup;
  }

  {
    acl_entry_t entry;
    entry_status = acl_get_entry(acl, ACL_FIRST_ENTRY, &entry);
    while (entry_status == 0) {
      if (print_entry(entry, index) != 0 || index == UINT_MAX) {
        goto cleanup;
      }
      index++;
      entry_status = acl_get_entry(acl, ACL_NEXT_ENTRY, &entry);
    }
  }
  if (entry_status == 1) {
    result = 0;
  }

cleanup:
  if (acl != NULL && acl_free(acl) != 0) {
    result = 1;
  }
  return result;
}
