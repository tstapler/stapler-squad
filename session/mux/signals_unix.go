//go:build !windows

package mux

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyWinch registers ch to receive SIGWINCH (terminal resize) signals.
func notifyWinch(ch chan os.Signal) {
	signal.Notify(ch, syscall.SIGWINCH)
}
