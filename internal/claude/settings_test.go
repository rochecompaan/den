package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDarwinSettingsAllowsMissingAndUnrelatedHooks(t *testing.T) {
	scopes := testScopes(t)
	writeSettings(t, scopes.ProjectFile, `{
		"hooks": {"PreToolUse": [{"matcher": "Write", "hooks": [{"type": "command", "command": "echo user-hook"}]}]}
	}`)
	if err := ValidateDarwinSettings(scopes); err != nil {
		t.Fatalf("ValidateDarwinSettings() error = %v", err)
	}
}

func TestValidateDarwinSettingsRejectsHookSuppression(t *testing.T) {
	for name, settings := range map[string]string{
		"disable all hooks":  `{"disableAllHooks": true}`,
		"fence command":      `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "fence --claude-pre-tool-use"}]}]}}`,
		"policy environment": `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "echo $DEN_FENCE_POLICY_FILE"}]}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			scopes := testScopes(t)
			writeSettings(t, scopes.UserFile, settings)
			if err := ValidateDarwinSettings(scopes); err == nil {
				t.Fatal("ValidateDarwinSettings() allowed an attempt to suppress or replace den-fence")
			}
		})
	}
}

func TestValidateDarwinSettingsReportsScopeNotContentsForMalformedFile(t *testing.T) {
	scopes := testScopes(t)
	const contents = "malformed-secret-settings"
	writeSettings(t, scopes.ManagedFile, contents)
	err := ValidateDarwinSettings(scopes)
	if err == nil {
		t.Fatal("ValidateDarwinSettings() accepted malformed managed settings")
	}
	if !strings.Contains(err.Error(), "managed settings") {
		t.Fatalf("error = %q, want scope name", err)
	}
	if strings.Contains(err.Error(), contents) {
		t.Fatalf("error leaked settings contents: %q", err)
	}
	if !strings.Contains(err.Error(), "correct") {
		t.Fatalf("error = %q, want corrective action", err)
	}
}

func TestValidateDarwinSettingsRejectsSettingsChangedAfterValidation(t *testing.T) {
	scopes := testScopes(t)
	writeSettings(t, scopes.ProjectLocalFile, `{"hooks": {}}`)
	if err := ValidateDarwinSettings(scopes); err != nil {
		t.Fatalf("first ValidateDarwinSettings() error = %v", err)
	}
	writeSettings(t, scopes.ProjectLocalFile, `{"hooks": {"PreToolUse": []}}`)
	if err := RevalidateDarwinSettings(scopes); err == nil {
		t.Fatal("RevalidateDarwinSettings() accepted settings changed after validation")
	}
}

func TestDarwinScopesUseClaudeCode2158DocumentedScopeSet(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	workingDirectory := filepath.Join(t.TempDir(), "project")
	scopes := DarwinScopes(configDir, workingDirectory)
	if got, want := []string{scopes.UserFile, scopes.ProjectFile, scopes.ProjectLocalFile, scopes.ManagedFile}, []string{
		filepath.Join(configDir, "settings.json"),
		filepath.Join(workingDirectory, ".claude", "settings.json"),
		filepath.Join(workingDirectory, ".claude", "settings.local.json"),
		"/Library/Application Support/ClaudeCode/managed-settings.json",
	}; !equalStrings(got, want) {
		t.Fatalf("DarwinScopes() = %#v, want %#v", got, want)
	}
}

func testScopes(t *testing.T) Scopes {
	t.Helper()
	root := t.TempDir()
	return NewScopes(
		filepath.Join(root, "user", "settings.json"),
		filepath.Join(root, "project", ".claude", "settings.json"),
		filepath.Join(root, "project", ".claude", "settings.local.json"),
		filepath.Join(root, "managed-settings.json"),
	)
}

func writeSettings(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
