// Package configdir securely selects Claude's writable configuration state.
package configdir

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
)

// Mode identifies whether Claude uses its default state or one custom directory.
type Mode string

const (
	Default Mode = "default"
	Custom  Mode = "custom"
)

// Dependencies contains immutable platform tools used to inspect filesystem ACLs.
type Dependencies struct {
	ACLProbe []string
}

// Selection is the validated state-directory choice and its rollback lifecycle.
type Selection struct {
	Mode               Mode
	CanonicalPath      string
	WritablePaths      []string
	DeniedDefaultPaths []string
	Device             uint64
	Inode              uint64
	Created            bool

	state *selectionState
}

type selectionState struct {
	mu         sync.Mutex
	path       string
	identity   fileIdentity
	acl        [sha256.Size]byte
	ancestors  []pathSnapshot
	deps       Dependencies
	ownerName  string
	ownerID    string
	created    bool
	committed  bool
	rolledBack bool
}

type pathSnapshot struct {
	path     string
	identity fileIdentity
	acl      [sha256.Size]byte
}

type fileIdentity struct {
	device uint64
	inode  uint64
	uid    uint32
	mode   fs.FileMode
}

var (
	errInvalid  = errors.New("custom configuration directory is invalid")
	errPrivate  = errors.New("custom configuration directory is not private")
	errOverlap  = errors.New("custom configuration directory overlaps a protected path")
	errChanged  = errors.New("custom configuration directory changed before launch")
	errRollback = errors.New("created configuration directory changed before rollback")
	errACL      = errors.New("configuration directory ACL validation failed")
)

// Select applies explicit, inherited, then default precedence and validates custom state.
func Select(explicit *string, inherited *string, home string, denied []string, deps Dependencies) (Selection, error) {
	if home == "" || !filepath.IsAbs(home) {
		return Selection{}, errInvalid
	}
	value := explicit
	if value == nil {
		value = inherited
	}
	if value == nil {
		return Selection{Mode: Default, WritablePaths: claudeDefaultPaths(home)}, nil
	}
	if *value == "" || !filepath.IsAbs(*value) {
		return Selection{}, errInvalid
	}
	return selectCustom(*value, home, denied, deps)
}

func selectCustom(value, home string, denied []string, deps Dependencies) (selection Selection, resultErr error) {
	canonical, exists, err := canonicalFinal(value)
	if err != nil {
		return Selection{}, errInvalid
	}
	if err := validateProtectedOverlap(canonical, home, denied); err != nil {
		return Selection{}, err
	}
	ownerUID := uint32(os.Getuid())
	ownerName, ownerID, err := invokingOwner(ownerUID)
	if err != nil || len(deps.ACLProbe) == 0 {
		return Selection{}, errACL
	}
	ancestors, err := captureAncestors(filepath.Dir(canonical), ownerUID, ownerName, ownerID, deps)
	if err != nil {
		return Selection{}, err
	}

	created := false
	if !exists {
		if err := os.Mkdir(canonical, 0o700); err != nil {
			return Selection{}, errInvalid
		}
		created = true
	}
	state := &selectionState{
		path: canonical, ancestors: ancestors, deps: deps,
		ownerName: ownerName, ownerID: ownerID, created: created,
	}
	if created {
		defer func() {
			if resultErr != nil {
				_ = rollbackCreated(state)
			}
		}()
	}

	identity, err := captureFinalIdentity(canonical, ownerUID)
	if err != nil {
		return Selection{}, err
	}
	state.identity = identity
	acl, err := captureFinalACL(canonical, ownerName, ownerID, deps)
	if err != nil {
		return Selection{}, err
	}
	state.acl = acl
	return Selection{
		Mode:               Custom,
		CanonicalPath:      canonical,
		WritablePaths:      []string{directoryPolicyPath(canonical)},
		DeniedDefaultPaths: claudeDefaultPaths(home),
		Device:             identity.device,
		Inode:              identity.inode,
		Created:            created,
		state:              state,
	}, nil
}

func invokingOwner(uid uint32) (string, string, error) {
	id := strconv.FormatUint(uint64(uid), 10)
	account, err := user.LookupId(id)
	if err != nil {
		return "", "", err
	}
	return account.Username, id, nil
}

func captureFinalIdentity(path string, ownerUID uint32) (fileIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil || info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fileIdentity{}, errInvalid
	}
	identity, ok := identityFromFileInfo(info)
	if !ok || identity.uid != ownerUID || !privateDirectoryMode(identity.mode) {
		return fileIdentity{}, errPrivate
	}
	return identity, nil
}

func captureFinalACL(path, ownerName, ownerID string, deps Dependencies) ([sha256.Size]byte, error) {
	acl, access, err := inspectACL(path, ownerName, ownerID, deps)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	if access.nonOwnerAny {
		return [sha256.Size]byte{}, errPrivate
	}
	return acl, nil
}

func privateDirectoryMode(mode fs.FileMode) bool {
	return mode.Perm() == 0o700 && mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0
}

func (s Selection) Revalidate() error {
	if s.Mode != Custom || s.state == nil {
		return nil
	}
	state := s.state
	state.mu.Lock()
	defer state.mu.Unlock()
	identity, err := captureFinalIdentity(state.path, state.identity.uid)
	if err != nil || identity != state.identity {
		return errChanged
	}
	acl, err := captureFinalACL(state.path, state.ownerName, state.ownerID, state.deps)
	if err != nil || acl != state.acl {
		return errChanged
	}
	ancestors, err := captureAncestors(filepath.Dir(state.path), state.identity.uid, state.ownerName, state.ownerID, state.deps)
	if err != nil || !sameSnapshots(ancestors, state.ancestors) {
		return errChanged
	}
	return nil
}

// Rollback removes only an unchanged directory created by this selection.
func (s Selection) Rollback() error {
	if s.state == nil {
		return nil
	}
	state := s.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.created || state.committed || state.rolledBack {
		return nil
	}
	if err := rollbackCreated(state); err != nil {
		return err
	}
	state.rolledBack = true
	return nil
}

func rollbackCreated(state *selectionState) error {
	info, err := os.Lstat(state.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errRollback
	}
	identity, ok := identityFromFileInfo(info)
	if !ok || state.identity != (fileIdentity{}) && identity != state.identity {
		return errRollback
	}
	if err := os.Remove(state.path); err != nil {
		return errRollback
	}
	return nil
}

// Commit transfers ownership of a created directory to the started Fence child.
func (s Selection) Commit() {
	if s.state == nil {
		return
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	s.state.committed = true
}

func sameSnapshots(left, right []pathSnapshot) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
