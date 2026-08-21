// Package claude validates Claude-specific launcher inputs.
package claude

import (
	"errors"
	"strings"
)

var reservedFlags = []string{
	"--settings",
	"--permission-mode",
	"--dangerously-skip-permissions",
}

// ValidateArguments rejects user arguments that could override Den's settings.
func ValidateArguments(arguments []string) error {
	for _, argument := range arguments {
		for _, flag := range reservedFlags {
			if argument == flag || strings.HasPrefix(argument, flag+"=") {
				return errors.New("Claude argument conflicts with a Den-owned security flag; remove it")
			}
		}
	}
	return nil
}

// ValidateDarwinArguments rejects Claude modes that bypass the macOS hook.
func ValidateDarwinArguments(arguments []string) error {
	for _, argument := range arguments {
		if argument == "--bare" || strings.HasPrefix(argument, "--bare=") {
			return errors.New("Claude --bare bypasses the mandatory macOS security hook; remove it")
		}
	}
	return nil
}

// ScrubDarwinEnvironment removes the Claude mode that bypasses macOS hooks.
func ScrubDarwinEnvironment(host []string) []string {
	result := make([]string, 0, len(host))
	for _, entry := range host {
		name, _, ok := strings.Cut(entry, "=")
		if ok && name == "CLAUDE_CODE_SIMPLE" {
			continue
		}
		result = append(result, entry)
	}
	return result
}
