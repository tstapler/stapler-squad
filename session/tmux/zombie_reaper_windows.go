//go:build windows

package tmux

import (
	"context"
	"time"
)

// StartZombieReaper is a no-op on Windows; zombie processes do not exist on
// Windows because the OS always tracks child process lifetimes differently.
func StartZombieReaper(_ context.Context, _ time.Duration, _ func(string, ...any)) {}

// reapZombieChildren is a no-op on Windows.
func reapZombieChildren() int { return 0 }
