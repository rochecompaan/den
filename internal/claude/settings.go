package claude

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const managedSettingsFile = "/Library/Application Support/ClaudeCode/managed-settings.json"

// Scopes names the Claude Code 2.1.158 settings files that apply on macOS.
type Scopes struct {
	UserFile         string
	ProjectFile      string
	ProjectLocalFile string
	ManagedFile      string

	state *settingsState
}

type settingsState struct {
	mu        sync.Mutex
	snapshots []settingsSnapshot
	validated bool
}

type settingsSnapshot struct {
	present bool
	digest  [sha256.Size]byte
}

// NewScopes creates one stateful set of settings files for validation.
func NewScopes(userFile, projectFile, projectLocalFile, managedFile string) Scopes {
	return Scopes{
		UserFile: userFile, ProjectFile: projectFile, ProjectLocalFile: projectLocalFile, ManagedFile: managedFile,
		state: &settingsState{},
	}
}

// DarwinScopes returns every settings scope documented by Claude Code 2.1.158.
func DarwinScopes(configDir, workingDirectory string) Scopes {
	return NewScopes(
		filepath.Join(configDir, "settings.json"),
		filepath.Join(workingDirectory, ".claude", "settings.json"),
		filepath.Join(workingDirectory, ".claude", "settings.local.json"),
		managedSettingsFile,
	)
}

// ValidateDarwinSettings rejects macOS settings that disable or replace Den's hook.
func ValidateDarwinSettings(scopes Scopes) error {
	return validateDarwinSettings(scopes, false)
}

// RevalidateDarwinSettings fails closed when a previously checked scope changed.
func RevalidateDarwinSettings(scopes Scopes) error {
	return validateDarwinSettings(scopes, true)
}

func validateDarwinSettings(scopes Scopes, requirePriorValidation bool) error {
	files, err := scopes.files()
	if err != nil {
		return err
	}
	if scopes.state == nil {
		return errors.New("Claude settings scopes are invalid; use the documented Claude Code 2.1.158 scopes")
	}

	scopes.state.mu.Lock()
	defer scopes.state.mu.Unlock()
	if requirePriorValidation && !scopes.state.validated {
		return errors.New("Claude settings were not validated before revalidation")
	}
	snapshots := make([]settingsSnapshot, 0, len(files))
	for _, file := range files {
		snapshot, err := validateSettingsFile(file.path, file.name)
		if err != nil {
			return err
		}
		snapshots = append(snapshots, snapshot)
	}
	if scopes.state.validated && !sameSnapshots(scopes.state.snapshots, snapshots) {
		return errors.New("Claude settings changed after validation; restore the validated settings before starting Fence")
	}
	scopes.state.snapshots = snapshots
	scopes.state.validated = true
	return nil
}

type settingsFile struct {
	name string
	path string
}

func (s Scopes) files() ([]settingsFile, error) {
	files := []settingsFile{
		{"user settings", s.UserFile},
		{"project settings", s.ProjectFile},
		{"project local settings", s.ProjectLocalFile},
		{"managed settings", s.ManagedFile},
	}
	for _, file := range files {
		if !filepath.IsAbs(file.path) {
			return nil, errors.New("Claude settings scopes are invalid; use the documented Claude Code 2.1.158 scopes")
		}
	}
	return files, nil
}

func validateSettingsFile(path, name string) (settingsSnapshot, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return settingsSnapshot{}, nil
	}
	if err != nil {
		return settingsSnapshot{}, settingsError(name)
	}
	if err := validateSettings(contents); err != nil {
		return settingsSnapshot{}, settingsError(name)
	}
	return settingsSnapshot{present: true, digest: sha256.Sum256(contents)}, nil
}

func validateSettings(contents []byte) error {
	var settings struct {
		DisableAllHooks *bool           `json:"disableAllHooks"`
		Hooks           json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(contents, &settings); err != nil {
		return err
	}
	if settings.DisableAllHooks != nil && *settings.DisableAllHooks {
		return errors.New("hooks disabled")
	}
	if len(settings.Hooks) == 0 || string(settings.Hooks) == "null" {
		return nil
	}
	var hooks map[string]any
	if err := json.Unmarshal(settings.Hooks, &hooks); err != nil {
		return err
	}
	if hasFenceReplacement(hooks) {
		return errors.New("hook replacement")
	}
	return nil
}

func hasFenceReplacement(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "command" {
				command, ok := child.(string)
				if ok && (strings.Contains(command, "--claude-pre-tool-use") || strings.Contains(command, "DEN_FENCE_POLICY_FILE")) {
					return true
				}
			}
			if hasFenceReplacement(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if hasFenceReplacement(child) {
				return true
			}
		}
	}
	return false
}

func settingsError(scope string) error {
	return fmt.Errorf("invalid %s; correct its JSON or remove the file", scope)
}

func sameSnapshots(left, right []settingsSnapshot) bool {
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
