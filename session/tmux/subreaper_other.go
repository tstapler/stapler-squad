//go:build !linux

package tmux

// SetSubreaper is a no-op on non-Linux platforms.
//
// Darwin: orphaned children are reparented to launchd, which reaps them
// promptly. There is no prctl(PR_SET_CHILD_SUBREAPER) equivalent on macOS.
//
// Windows: zombie processes do not exist; the OS tracks child lifetimes
// differently.
func SetSubreaper() error { return nil }
