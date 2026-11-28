package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// addCreatingSession inserts a Creating-status instance directly into storage
// (via AddInstance) and returns the corresponding live *session.Instance so a
// test can additionally exercise the in-memory actor path (TryForceStatusIfEpoch).
func addCreatingSession(t *testing.T, storage *session.Storage, title, uuid string) *instance.Instance {
	t.Helper()
	inst := &instance.Instance{
		Title:     title,
		UUID:      uuid,
		Path:      "/tmp/test",
		Status:    domain.Creating,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	return inst
}
