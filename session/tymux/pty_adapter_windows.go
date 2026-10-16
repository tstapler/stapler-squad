//go:build windows

package tymux

import (
	"errors"
	"os"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// errPTYAdapterUnavailableOnWindows is returned by newTymuxPTYAdapter: the
// real implementation (pty_adapter.go) depends on syscall.Socketpair, which
// Windows' syscall package doesn't provide, and tymuxd itself has no
// Windows release to begin with (docs/reference/bundling-tymuxd.md) — so a
// tymux-backed session's GetPTY() has nothing to bridge to on this
// platform regardless.
var errPTYAdapterUnavailableOnWindows = errors.New("tymux: PTY adapter unavailable on windows")

// tymuxPTYAdapter is an empty stand-in on windows — see
// errPTYAdapterUnavailableOnWindows.
type tymuxPTYAdapter struct{}

func newTymuxPTYAdapter(_ *ClientFanout, _ func(*v1.AttachRequest) error) (*tymuxPTYAdapter, error) {
	return nil, errPTYAdapterUnavailableOnWindows
}

func (a *tymuxPTYAdapter) close() {}

func (a *tymuxPTYAdapter) file() *os.File { return nil }
