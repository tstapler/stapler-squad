package tmux

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// ListAllPanes runs one `tmux list-panes -a -F format` against socket for a background poller:
// ok is false when no exec-gate slot is free right now (skip this tick) or tmux failed.
func ListAllPanes(socket, format string) (output []byte, ok bool) {
	resolved := ResolveSocket(socket)
	args := resolved.Args("list-panes", "-a", "-F", format)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	release, acquired := TryAcquireExecSlot(resolved.String())
	if !acquired {
		return nil, false
	}
	defer release()

	out, err := safeexec.CommandContext(ctx, Binary(), args...).Output()
	if err != nil {
		return nil, false
	}
	return out, true
}
