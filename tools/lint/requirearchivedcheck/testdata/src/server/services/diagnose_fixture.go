// Package services contains test fixtures for the requirearchivedcheck
// analyzer, resolved via analysistest's GOPATH-style testdata overlay at the
// real path prefix (server/services) this analyzer is scoped to — but only
// for files whose basename starts with diagnose_. Instance and
// SessionStopper stand in for the real session.Instance and
// server/services.SessionService (see the session/diagnose fixture package's
// doc comment for the full rationale).
package services

import "context"

// Instance stands in for the real *session.Instance.
type Instance struct {
	SessionUUID string
}

// IsArchived stands in for the real (*session.Instance).IsArchived.
func (i *Instance) IsArchived() bool { return false }

// SessionStopper stands in for the real server/services.SessionService.
type SessionStopper struct{}

func (s *SessionStopper) ArchiveSessionByUUID(ctx context.Context, sessionUUID string) error {
	return nil
}

// BAD: this file's name starts with diagnose_, so it IS in scope — flagged
// just like session/diagnose/fixture.go's bad1.
func archiveFromDiagnose(ctx context.Context, stopper *SessionStopper, inst *Instance) {
	_ = stopper.ArchiveSessionByUUID(ctx, inst.SessionUUID) // want `ArchiveSessionByUUID called without a preceding same-function \.IsArchived\(\) check`
}
