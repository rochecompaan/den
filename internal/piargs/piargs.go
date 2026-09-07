// Package piargs validates Pi 0.84.4 launcher arguments.
package piargs

import (
	"errors"
	"reflect"
	"strings"
)

var reservedFlags = []string{
	"--session-dir", "--session", "--fork", "--export",
	"--extension", "-e", "--skill", "--prompt-template", "--theme",
}

var reservedCommands = []string{"install", "remove", "uninstall", "update", "list", "config"}

// Validate verifies the immutable manifest table and user arguments.
func Validate(manifestFlags, manifestCommands, userArgs []string) error {
	if !reflect.DeepEqual(manifestFlags, reservedFlags) || !reflect.DeepEqual(manifestCommands, reservedCommands) {
		return errors.New("manifest Pi argument policy is invalid")
	}
	if len(userArgs) > 0 && userArgs[0] != "--" && isReservedCommand(userArgs[0]) {
		return errors.New("Pi package commands are disabled by Den")
	}
	for index, argument := range userArgs {
		switch argument {
		case "--session-dir", "--export", "--extension", "-e", "--skill", "--prompt-template", "--theme":
			return errors.New("Pi argument conflicts with a Den-owned input; remove it")
		case "--session", "--fork":
			if index+1 >= len(userArgs) || (!isPartialID(userArgs[index+1]) && !isUUID(userArgs[index+1])) {
				return errors.New("Pi session must be a hexadecimal ID")
			}
		case "--session-id":
			if index+1 >= len(userArgs) || !isUUID(userArgs[index+1]) {
				return errors.New("Pi session ID must be a UUID")
			}
		}
		if hasReservedEquals(argument) || hasReservedShortForm(argument) {
			return errors.New("Pi argument conflicts with a Den-owned input; remove it")
		}
	}
	return nil
}

func isReservedCommand(value string) bool {
	for _, command := range reservedCommands {
		if value == command {
			return true
		}
	}
	return false
}

func hasReservedEquals(argument string) bool {
	for _, flag := range []string{"--session-dir", "--session", "--fork", "--export", "--session-id", "--extension", "--skill", "--prompt-template", "--theme"} {
		if strings.HasPrefix(argument, flag+"=") {
			return true
		}
	}
	return false
}

func hasReservedShortForm(argument string) bool {
	if !strings.HasPrefix(argument, "-") || strings.HasPrefix(argument, "--") {
		return false
	}
	if argument == "-c" || argument == "-r" || argument == "-a" || argument == "-na" || argument == "-ne" || argument == "-ns" || argument == "-np" {
		return false
	}
	return strings.Contains(argument[1:], "e")
}

func isPartialID(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') && !(character >= 'A' && character <= 'F') {
			return false
		}
	}
	return true
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') && !(character >= 'A' && character <= 'F') {
			return false
		}
	}
	return true
}
