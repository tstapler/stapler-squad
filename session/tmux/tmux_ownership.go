package tmux

// tmux_ownership.go — verifying that a tmux pane found by name actually
// belongs to the Instance that's about to reuse, attach to, or kill it. See
// backlog item ce71ad1a: a stale pane left behind under a name a new,
// unrelated Instance also resolves to was silently reattached to, delivering
// one Instance's prompts into another's live session.

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// ReadSessionOwnerUUID reads back the STAPLER_SESSION_UUID env var stamped on
// a tmux session at creation (instance_tmux.go's buildExtraEnv) via `tmux
// show-environment`, the same read-back mechanism orphan_sweep.go and
// workspace_peers.go already use to identify sessions. For callers with no
// TmuxSession object to hand (e.g. server/services' KillTmuxSessionByTitle,
// which only has a title/name).
//
// Returns ("", err) whenever the marker can't be confirmed present: no such
// session, no server running, or the variable was simply never set on an
// existing session (a normal tmux exit-1 case, not evidence of anything).
// Callers must treat any non-nil err as "ownership unverifiable" and act
// conservatively -- never as an implicit "no owner, safe to proceed."
//
// socket should come from ResolveSocket (or Socket("") for the default
// server), matching every other package-level tmux invocation's socket
// handling.
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

// verifyExistingSessionOwner checks that a tmux session already confirmed to
// exist under t.sanitizedName actually belongs to this TmuxSession's expected
// owner, before a caller reattaches to or reuses it. Returns true when either
// there's nothing to verify against (expectedOwnerUUID is empty) or the
// pane's STAPLER_SESSION_UUID marker matches. A present-but-different or
// altogether absent marker is treated as untrusted, not grandfathered in --
// ce71ad1a's whole incident was exactly a name match without this check.
//
// Goes through t.cmdExec (not ReadSessionOwnerUUID's raw safeexec) so the
// existing reuse-path unit tests can mock it without a real tmux binary.
func (t *TmuxSession) verifyExistingSessionOwner(ctx context.Context) bool {
	want := t.expectedOwnerUUID()
	if want == "" {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, ownerTmuxCmdTimeout)
	defer cancel()
	cmd := t.buildTmuxCommandContext(ctx, "show-environment", "-t", t.sanitizedName, "STAPLER_SESSION_UUID")
	out, err := t.cmdExec.Output(cmd)
	if err != nil {
		log.Warn("verifyExistingSessionOwner: could not read pane owner marker, refusing to reuse",
			"session", t.sanitizedName, "want", want, "err", err)
		return false
	}
	got := strings.TrimPrefix(strings.TrimSpace(string(out)), "STAPLER_SESSION_UUID=")
	if got != want {
		log.Warn("verifyExistingSessionOwner: pane owner mismatch, refusing to reuse stale session",
			"session", t.sanitizedName, "want", want, "got", got)
		return false
	}
	return true
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
