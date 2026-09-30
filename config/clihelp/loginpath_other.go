//go:build windows

package clihelp

import (
	"context"
	"errors"
)

func runShellScript(context.Context, string, string) (string, error) {
	return "", errors.New("login PATH derivation is not supported on windows")
}
