# Private ancestors for agent state

The approved change allows a group-writable projects directory beneath a private
home. The selected state directory remains private. Existing home and project
permissions remain unchanged.

A private boundary blocks access by other accounts. An access control list
(ACL) adds permission rules for users and groups. The common state-directory
validator applies these rules to Claude and Pi, including custom and default
state paths.

## Rules

1. Inspect ancestors from the filesystem root toward the selected state directory.
2. Before a private boundary, retain the existing checks for group, other, and
   ACL write access. Retain the sticky-directory exception for root or the
   invoking account.
3. A boundary must belong to the invoking account. Its exact mode is `0700`,
   without special permission bits. Its ACL grants no access to other accounts.
4. Every parent above the boundary must belong to root or the invoking account.
   A different owner can change its permissions or replace descendants.
5. Permit writable ancestors below that boundary. Do not change their permissions.
6. Retain every ancestor snapshot, including those below the boundary. Directory
   identity, ownership, mode, and ACL changes still fail revalidation.
7. Retain all final-state ownership, mode, ACL, overlap, and final-symlink rules.

The private boundary protects its descendants, not its own name. A private home
beneath an unsafe writable parent does not make that parent safe. Revalidation
supplements this protection and does not replace it.

## Verification

The Go suite and race suite each pass 637 tests and subtests across 16 packages.
The native Linux ACL test passes with the system `getfacl` command. The
`go vet ./...` command, Nix launcher check, and full `x86_64-linux` flake check pass.

The test that changes real ownership requires root and is skipped on this host.
Deterministic ownership-policy tests run without root. A temporary mutation
that removes the foreign-owner guard causes three of those cases to fail.
A native ACL-change test fails when ancestor ACL snapshots are omitted.

The Nix build reports foreign-owned ancestors. Physical tests that need
a trusted private boundary skip there, while permission-policy tests still run.
Unsafe-parent fixtures select an unprotected base rather than silently relying
on a private temporary directory.

The Darwin/arm64 configuration-directory tests compile. No Darwin runtime test
runs on this Linux host. Documentation uses direct whitespace checks rather
than tests that assert its text.
