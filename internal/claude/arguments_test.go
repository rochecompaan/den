package claude

import (
	"reflect"
	"testing"
)

func TestValidateArgumentsRejectsReservedFlags(t *testing.T) {
	for _, argument := range []string{
		"--settings", "--settings=/tmp/user-settings.json",
		"--permission-mode", "--permission-mode=plan",
		"--dangerously-skip-permissions", "--dangerously-skip-permissions=true",
	} {
		t.Run(argument, func(t *testing.T) {
			if err := ValidateArguments([]string{argument}); err == nil {
				t.Fatalf("ValidateArguments(%q) accepted Den-owned flag", argument)
			}
		})
	}
}

func TestValidateArgumentsPreservesOrdinaryArguments(t *testing.T) {
	arguments := []string{
		"--plugin-dir", "/tmp/with spaces", "--mcp-config=", "--strict-mcp-config", "", "--bare",
	}
	want := append([]string(nil), arguments...)
	if err := ValidateArguments(arguments); err != nil {
		t.Fatalf("ValidateArguments() error = %v", err)
	}
	if !reflect.DeepEqual(arguments, want) {
		t.Fatalf("ValidateArguments() changed arguments to %#v, want %#v", arguments, want)
	}
}

func TestValidateDarwinArgumentsRejectsBareModes(t *testing.T) {
	for _, argument := range []string{"--bare", "--bare=true", "--bare=false"} {
		t.Run(argument, func(t *testing.T) {
			if err := ValidateDarwinArguments([]string{argument}); err == nil {
				t.Fatalf("ValidateDarwinArguments(%q) allowed mode that skips the mandatory hook", argument)
			}
		})
	}
}

func TestScrubDarwinEnvironmentRemovesClaudeCodeSimple(t *testing.T) {
	host := []string{"KEEP=value", "CLAUDE_CODE_SIMPLE=1", "CLAUDE_CODE_SIMPLE=0"}
	if got, want := ScrubDarwinEnvironment(host), []string{"KEEP=value"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ScrubDarwinEnvironment() = %#v, want %#v", got, want)
	}
}
