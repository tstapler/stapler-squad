//go:build windows

package mux

import "os"

// notifyWinch is a no-op on Windows where SIGWINCH does not exist.
func notifyWinch(_ chan os.Signal) {}
