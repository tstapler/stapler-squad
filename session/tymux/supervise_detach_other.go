//go:build !darwin

package tymux

import "os/exec"

func detachFromSession(*exec.Cmd) {}
