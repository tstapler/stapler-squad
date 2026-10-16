package tmux

// tmux_ownership.go — verifying that a tmux pane found by name actually
// belongs to the Instance that's about to reuse, attach to, or kill it. See
// backlog item ce71ad1a: a stale pane left behind under a name a new,
// unrelated Instance also resolves to was silently reattached to, delivering
// one Instance's prompts into another's live session.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// ReadSessionOwnerUUID reads back the STAPLER_SESSION_UUID stamped on a tmux
// session at creation, for callers with only a name. Any non-nil err means
// ownership is unverifiable and must be treated conservatively, never as
// "no owner".
func ReadSessionOwnerUUID(ctx context.Context, socket Socket, name string) (string, error) {
	args := socket.Args("show-environment", "-t", name, "STAPLER_SESSION_UUID")
	out, err := safeexec.CommandContext(ctx, ResolveClientForSocket(socket.String()), args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("show-environment %s: %w (output: %s)", name, err, bytes.TrimSpace(out))
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "STAPLER_SESSION_UUID="), nil
}

// ownerTmuxCmdTimeout bounds the tmux calls on the start/reattach path so a
// hung tmux server can't stall session creation or restore.
const ownerTmuxCmdTimeout = 5 * time.Second

// expectedOwnerUUID returns the STAPLER_SESSION_UUID this TmuxSession was
// configured to own (set via SetExtraEnv by instance_tmux.go's
// wireTmuxSession), or "" when this TmuxSession has no owner expectation to
// verify against -- e.g. NewTmuxSessionFromExisting's external-mux-discovery
// wrapping, which legitimately never sets it.
func (t *TmuxSession) expectedOwnerUUID() string {
	for _, kv := range t.extraEnv {
		if v, ok := strings.CutPrefix(kv, "STAPLER_SESSION_UUID="); ok {
			return v
		}
	}
	return ""
}

type ownerVerdict int

const (
	ownerMatch        ownerVerdict = iota // reuse the pane
	ownerForeign                          // marker absent or a different UUID: safe to replace
	ownerUnverifiable                     // couldn't read the marker (timeout, tmux hiccup): don't touch the pane
)

// existingSessionOwner classifies the pane already living under t.sanitizedName.
// An absent marker is foreign (ce71ad1a); only a failed read is unverifiable.
func (t *TmuxSession) existingSessionOwner(ctx context.Context) ownerVerdict {
	want := t.expectedOwnerUUID()
	if want == "" {
		return ownerMatch
	}
	ctx, cancel := context.WithTimeout(ctx, ownerTmuxCmdTimeout)
	defer cancel()
	cmd := t.buildTmuxCommandContext(ctx, "show-environment", "-t", t.sanitizedName, "STAPLER_SESSION_UUID")
	out, err := t.cmdExec.Output(cmd)
	if err != nil {
		detail := err.Error() + string(out)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			detail += string(exitErr.Stderr)
		}
		if strings.Contains(detail, "unknown variable") {
			log.Warn("existingSessionOwner: pane has no owner marker", "session", t.sanitizedName, "want", want)
			return ownerForeign
		}
		log.Warn("existingSessionOwner: could not read pane owner marker", "session", t.sanitizedName, "want", want, "err", err)
		return ownerUnverifiable
	}
	got := strings.TrimPrefix(strings.TrimSpace(string(out)), "STAPLER_SESSION_UUID=")
	if got != want {
		log.Warn("existingSessionOwner: pane owner mismatch", "session", t.sanitizedName, "want", want, "got", got)
		return ownerForeign
	}
	return ownerMatch
}

// verifyExistingSessionOwner reports whether the existing pane is safe to
// attach to: true only on a confirmed match.
func (t *TmuxSession) verifyExistingSessionOwner(ctx context.Context) bool {
	return t.existingSessionOwner(ctx) == ownerMatch
}

// reuseOrReplaceExisting decides what to do with a pane found under our name:
// reuse=true on a match; a foreign pane is killed so the caller recreates it;
// an unverifiable one is left alone and returned as an error.
func (t *TmuxSession) reuseOrReplaceExisting() (reuse bool, err error) {
	switch t.existingSessionOwner(context.Background()) {
	case ownerMatch:
		return true, nil
	case ownerForeign:
		_ = t.killMismatchedOwnerSession() // failure surfaces as a duplicate-session error on recreate
		return false, nil
	default:
		return false, fmt.Errorf("tmux session %q owner could not be verified; refusing to reuse or kill it", t.sanitizedName)
	}
}

// killMismatchedOwnerSession kills a stale tmux session whose owner marker
// didn't match ours, so the subsequent recreateMissingSession's `new-session`
// doesn't collide with it (ce71ad1a).
func (t *TmuxSession) killMismatchedOwnerSession() error {
	ctx, cancel := context.WithTimeout(context.Background(), ownerTmuxCmdTimeout)
	defer cancel()
	cmd := t.buildTmuxCommandContext(ctx, "kill-session", "-t", t.sanitizedName)
	err := t.cmdExec.Run(cmd)
	if err != nil {
		log.Warn("killMismatchedOwnerSession: failed to kill stale tmux session before recreate",
			"session", t.sanitizedName, "err", err)
	}
	t.invalidateExistsCache()
	return err
}
