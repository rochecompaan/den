// Package environment builds the controlled child-process environment for the launcher.
package environment

import (
	"path/filepath"
	"strings"
)

// Controlled contains values validated by the launcher or fixed by its manifest.
type Controlled struct {
	Endpoint      string
	Token         string
	CAFile        string
	ClientDir     string
	PathEntries   []string
	DockerHost    string
	ContainerHost string
	XDGRuntimeDir string
}

// Build replaces inherited Git and RepoWolf state with the controlled child environment.
func Build(host []string, controlled Controlled) []string {
	result := make([]string, 0, len(host)+15)
	seen := make(map[string]struct{}, len(host)+15)
	controlledEntries := controlledEntries(controlled)
	replacements := make(map[string]struct{}, len(controlledEntries))
	for _, entry := range controlledEntries {
		name, _, _ := strings.Cut(entry, "=")
		replacements[name] = struct{}{}
	}
	for _, entry := range host {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || scrub(name) {
			continue
		}
		if _, replaced := replacements[name]; replaced {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, entry)
	}
	return append(result, controlledEntries...)
}

func controlledEntries(controlled Controlled) []string {
	gitSSH := filepath.Join(controlled.ClientDir, "bin", "repowolf-git-ssh")
	result := []string{
		"PATH=" + strings.Join(controlled.PathEntries, ":"),
		"REPOWOLF_ENDPOINT=" + controlled.Endpoint,
		"REPOWOLF_TOKEN=" + controlled.Token,
		"REPOWOLF_CA_FILE=" + controlled.CAFile,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=" + gitSSH,
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0=url.git@github.com:.insteadOf",
		"GIT_CONFIG_VALUE_0=https://github.com/",
		"GIT_CONFIG_KEY_1=credential.helper",
		"GIT_CONFIG_VALUE_1=",
		"GIT_CONFIG_KEY_2=core.sshCommand",
		"GIT_CONFIG_VALUE_2=" + gitSSH,
	}
	if controlled.DockerHost != "" {
		result = append(result, "DOCKER_HOST="+controlled.DockerHost)
	}
	if controlled.ContainerHost != "" {
		result = append(result, "CONTAINER_HOST="+controlled.ContainerHost)
	}
	if controlled.XDGRuntimeDir != "" {
		result = append(result, "XDG_RUNTIME_DIR="+controlled.XDGRuntimeDir)
	}
	return result
}

func scrub(name string) bool {
	if name == "PATH" || strings.HasPrefix(name, "REPOWOLF_") || strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
		return true
	}
	switch name {
	case "GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN",
		"DOCKER_HOST", "CONTAINER_HOST",
		"SSH_AUTH_SOCK", "GIT_ASKPASS", "SSH_ASKPASS", "GIT_SSH", "GIT_SSH_COMMAND",
		"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
		"GIT_TERMINAL_PROMPT":
		return true
	default:
		return false
	}
}
