package environment

import (
	"reflect"
	"testing"
)

func TestBuildScrubsCredentialsAndRestoresOnlyControlledRepoWolfValues(t *testing.T) {
	host := []string{
		"HOME=/home/test",
		"LANG=C.UTF-8",
		"CLAUDE_CODE_OAUTH_TOKEN=claude-token",
		"EDITOR=vi",
		"PATH=/host/bin",
		"GH_TOKEN=github-secret",
		"GITHUB_TOKEN=github-secret",
		"GH_ENTERPRISE_TOKEN=github-secret",
		"GITHUB_ENTERPRISE_TOKEN=github-secret",
		"SSH_AUTH_SOCK=/secret/ssh.sock",
		"GIT_ASKPASS=/secret/askpass",
		"SSH_ASKPASS=/secret/ssh-askpass",
		"GIT_SSH=/secret/ssh",
		"GIT_SSH_COMMAND=/secret/ssh-command",
		"GIT_CONFIG_GLOBAL=/secret/gitconfig",
		"GIT_CONFIG_SYSTEM=/secret/gitconfig",
		"GIT_CONFIG_PARAMETERS=credential.helper=secret",
		"GIT_CONFIG_COUNT=99",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=/secret/helper",
		"GIT_CONFIG_KEY_99=credential.helper",
		"GIT_CONFIG_VALUE_99=/secret/helper",
		"REPOWOLF_ENDPOINT=https://unvalidated.example.test",
		"REPOWOLF_TOKEN=unvalidated-token",
		"REPOWOLF_CA_FILE=/unvalidated/ca.pem",
		"REPOWOLF_SERVER_NAME=unvalidated-server",
		"REPOWOLF_FUTURE_OVERRIDE=unvalidated-value",
	}
	controlled := Controlled{
		Endpoint:    "https://broker.example.test/",
		Token:       "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		CAFile:      "/canonical/ca.pem",
		ClientDir:   "/nix/store/repowolf-client",
		PathEntries: []string{"/nix/store/git/bin", "/nix/store/coreutils/bin"},
	}

	got := entries(Build(host, controlled))
	want := map[string]string{
		"HOME":                    "/home/test",
		"LANG":                    "C.UTF-8",
		"CLAUDE_CODE_OAUTH_TOKEN": "claude-token",
		"EDITOR":                  "vi",
		"PATH":                    "/nix/store/git/bin:/nix/store/coreutils/bin",
		"REPOWOLF_ENDPOINT":       controlled.Endpoint,
		"REPOWOLF_TOKEN":          controlled.Token,
		"REPOWOLF_CA_FILE":        controlled.CAFile,
		"GIT_TERMINAL_PROMPT":     "0",
		"GIT_SSH_COMMAND":         "/nix/store/repowolf-client/bin/repowolf-git-ssh",
		"GIT_CONFIG_COUNT":        "3",
		"GIT_CONFIG_KEY_0":        "url.git@github.com:.insteadOf",
		"GIT_CONFIG_VALUE_0":      "https://github.com/",
		"GIT_CONFIG_KEY_1":        "credential.helper",
		"GIT_CONFIG_VALUE_1":      "",
		"GIT_CONFIG_KEY_2":        "core.sshCommand",
		"GIT_CONFIG_VALUE_2":      "/nix/store/repowolf-client/bin/repowolf-git-ssh",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Build() entries = %#v, want %#v", got, want)
	}
}

func TestBuildDoesNotMutateHostAndProducesNoDuplicateNames(t *testing.T) {
	host := []string{"PATH=/host", "KEEP=value", "KEEP=last", "REPOWOLF_SERVER_NAME=blocked"}
	original := append([]string(nil), host...)
	got := Build(host, Controlled{
		Endpoint:    "https://broker.example.test",
		Token:       "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		CAFile:      "/ca.pem",
		ClientDir:   "/client",
		PathEntries: []string{"/path"},
	})
	if !reflect.DeepEqual(host, original) {
		t.Fatalf("Build() mutated host: got %#v, want %#v", host, original)
	}
	if len(entries(got)) != len(got) {
		t.Fatalf("Build() produced duplicate names: %#v", got)
	}
}

func entries(environment []string) map[string]string {
	result := make(map[string]string, len(environment))
	for _, entry := range environment {
		for index := 0; index < len(entry); index++ {
			if entry[index] == '=' {
				result[entry[:index]] = entry[index+1:]
				break
			}
		}
	}
	return result
}
