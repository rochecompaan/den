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
	ACLProbe       []string
	ProtectedHomes []string
}

// Selection is the validated state-directory choice and its rollback lifecycle.
type Selection struct {
	Mode               Mode
	CanonicalPath      string
	WritablePaths      []string
	DeniedDefaultPaths []string
	ProtectedPaths     []string
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
	probe      aclProbe
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
	if len(deps.ProtectedHomes) == 0 {
		deps.ProtectedHomes = []string{home}
	}
	value := explicit
	if value == nil {
		value = inherited
	}
	if value == nil {
		protected, err := expandProtectedPatterns(denied, deps.ProtectedHomes)
		if err != nil {
			return Selection{}, errInvalid
		}
		return Selection{Mode: Default, WritablePaths: claudeDefaultPaths(home), ProtectedPaths: protected}, nil
	}
	if *value == "" || !filepath.IsAbs(*value) {
		return Selection{}, errInvalid
	}
	return selectCustom(*value, home, denied, deps)
}

func selectCustom(value, home string, denied []string, deps Dependencies) (selection Selection, resultErr error) {
	probe, err := snapshotACLProbe(deps.ACLProbe)
	if err != nil {
		return Selection{}, err
	}
	canonical, exists, err := canonicalFinal(value)
	if err != nil {
		return Selection{}, errInvalid
	}
	protected, err := expandProtectedPatterns(denied, deps.ProtectedHomes)
	if err != nil {
		return Selection{}, errInvalid
	}
	for _, protectedHome := range deps.ProtectedHomes {
		if err := validateProtectedOverlapForHome(canonical, home, protectedHome, denied); err != nil {
			return Selection{}, err
		}
	}
	ownerUID := uint32(os.Getuid())
	ownerName, ownerID, err := invokingOwner(ownerUID)
	if err != nil {
		return Selection{}, errACL
	}
	ancestors, err := captureAncestors(filepath.Dir(canonical), ownerUID, ownerName, ownerID, probe)
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
		path: canonical, ancestors: ancestors, probe: probe,
		ownerName: ownerName, ownerID: ownerID, created: created,
	}
	if created {
		defer func() {
			if resultErr != nil {
				_ = rollbackCreated(state)
			}
		}()
	}

	identity, acl, err := captureFinalState(canonical, ownerUID, ownerName, ownerID, probe)
	state.identity = identity
	if err != nil {
		return Selection{}, err
	}
	state.acl = acl
	return Selection{
		Mode:               Custom,
		CanonicalPath:      canonical,
		WritablePaths:      []string{directoryPolicyPath(canonical)},
		DeniedDefaultPaths: claudeDefaultPaths(home),
		ProtectedPaths:     protected,
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

func captureFinalState(path string, ownerUID uint32, ownerName, ownerID string, probe aclProbe) (fileIdentity, [sha256.Size]byte, error) {
	file, err := openDirectoryHandle(path)
	if err != nil {
		return fileIdentity{}, [sha256.Size]byte{}, errInvalid
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info == nil || !info.IsDir() {
		return fileIdentity{}, [sha256.Size]byte{}, errInvalid
	}
	identity, ok := identityFromFileInfo(info)
	if !ok || identity.uid != ownerUID || !privateDirectoryMode(identity.mode) || !directoryHandleWritable(file) {
		return fileIdentity{}, [sha256.Size]byte{}, errPrivate
	}
	acl, access, err := inspectACL(path, ownerName, ownerID, probe)
	if err != nil {
		return identity, [sha256.Size]byte{}, err
	}
	if access.nonOwnerAny || access.ownerWriteDenied {
		return identity, [sha256.Size]byte{}, errPrivate
	}
	current, err := os.Lstat(path)
	if err != nil || current == nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 {
		return identity, [sha256.Size]byte{}, errChanged
	}
	currentIdentity, ok := identityFromFileInfo(current)
	if !ok || currentIdentity != identity {
		return identity, [sha256.Size]byte{}, errChanged
	}
	return identity, acl, nil
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
	identity, acl, err := captureFinalState(state.path, state.identity.uid, state.ownerName, state.ownerID, state.probe)
	if err != nil || identity != state.identity || acl != state.acl {
		return errChanged
	}
	ancestors, err := captureAncestors(filepath.Dir(state.path), state.identity.uid, state.ownerName, state.ownerID, state.probe)
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
	if state.identity == (fileIdentity{}) {
		return errRollback
	}
	identity, ok := identityFromFileInfo(info)
	if !ok || identity != state.identity {
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
