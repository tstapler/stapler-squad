// Package diagnose contains test fixtures for the requirearchivedcheck
// analyzer, resolved via analysistest's GOPATH-style testdata overlay at the
// real path prefix (session/diagnose) this analyzer is scoped to. Instance,
// SessionStopper, and their methods stand in for the real session.Instance
// (session/instance_state.go's IsArchived) and
// server/services.SessionService's ArchiveSessionByUUID/KillTmuxPaneOnly, so
// this fixture package doesn't have to import the real (much larger)
// session/server modules.
package diagnose

import "context"

// Instance stands in for the real *session.Instance.
type Instance struct {
	SessionUUID string
}

// IsArchived stands in for the real (*session.Instance).IsArchived.
func (i *Instance) IsArchived() bool { return false }

// SessionStopper stands in for a real archive/kill/revive-capable dependency
// (e.g. server/services.SessionService, used as BacklogService.SessionStopper).
type SessionStopper struct{}

func (s *SessionStopper) ArchiveSessionByUUID(ctx context.Context, sessionUUID string) error {
	return nil
}
func (s *SessionStopper) KillTmuxPaneOnly(ctx context.Context, sessionUUID string) error {
	return nil
}
func (s *SessionStopper) ResumeSession(ctx context.Context, sessionUUID string) error {
	return nil
}

// BAD1: archives via a SessionUUID-shaped argument rooted in inst, with no
// preceding IsArchived() check anywhere in the function.
func bad1(ctx context.Context, stopper *SessionStopper, inst *Instance) {
	_ = stopper.ArchiveSessionByUUID(ctx, inst.SessionUUID) // want `ArchiveSessionByUUID called without a preceding same-function \.IsArchived\(\) check`
}

// BAD2: same violation via KillTmuxPaneOnly.
func bad2(ctx context.Context, stopper *SessionStopper, inst *Instance) {
	_ = stopper.KillTmuxPaneOnly(ctx, inst.SessionUUID) // want `KillTmuxPaneOnly called without a preceding same-function \.IsArchived\(\) check`
}

// BAD3: the IsArchived() check is present, but on a different Instance than
// the one actually archived — must still be flagged.
func bad3(ctx context.Context, stopper *SessionStopper, inst, other *Instance) {
	_ = inst.IsArchived()
	_ = stopper.ArchiveSessionByUUID(ctx, other.SessionUUID) // want `ArchiveSessionByUUID called without a preceding same-function \.IsArchived\(\) check`
}

// GOOD1: a preceding same-function IsArchived() check on the same receiver
// guards the archive call.
func good1(ctx context.Context, stopper *SessionStopper, inst *Instance) {
	if inst.IsArchived() {
		return
	}
	_ = stopper.ArchiveSessionByUUID(ctx, inst.SessionUUID)
}

// GOOD2: same guard pattern for ResumeSession, called directly on Instance
// as the receiver rather than via a SessionUUID argument.
func good2(ctx context.Context, inst *Instance, stopper *SessionStopper) {
	if !inst.IsArchived() {
		_ = stopper.ResumeSession(ctx, inst.SessionUUID)
	}
}

// GOOD3: a //nolint directive suppresses the finding with a justification.
func good3(ctx context.Context, stopper *SessionStopper, inst *Instance) {
	_ = stopper.ArchiveSessionByUUID(ctx, inst.SessionUUID) //nolint:requirearchivedcheck test fixture, not a real call
}
