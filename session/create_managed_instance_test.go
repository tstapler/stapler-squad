package session

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateManagedInstance_CreatesPersistedInstance_When_ParamsValid is the
// primary happy-path test for Story 1.2.0a's extracted domain function: given
// a Directory-mode session pointed at a path that already exists (via
// t.TempDir()), CreateManagedInstance must construct an *Instance, persist it
// via Storage.AddInstance so it round-trips through LoadInstances, register it
// in the live-handle Registry, and -- critically -- must NOT start tmux/the
// underlying process (no instance.Start() call), since that remains the
// caller's responsibility.
func TestCreateManagedInstance_CreatesPersistedInstance_When_ParamsValid(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	registry := NewRegistry(storage, nil)

	dir := t.TempDir()

	instance, err := CreateManagedInstance(context.Background(), CreateManagedInstanceParams{
		Options: CreateInstanceOptions{
			Title:       "cmi-happy-path",
			Path:        dir,
			Program:     "claude",
			SessionType: SessionTypeDirectory,
		},
		Storage:  storage,
		Registry: registry,
	})
	require.NoError(t, err)
	require.NotNil(t, instance)
	assert.Equal(t, "cmi-happy-path", instance.Title)
	assert.False(t, instance.started.Load(), "CreateManagedInstance must not start the instance")

	// Persisted: round-trips through LoadInstances.
	loaded, err := storage.ListInstanceData()
	require.NoError(t, err)
	found := false
	for _, d := range loaded {
		if d.GetStableID() == instance.GetStableID() {
			found = true
		}
	}
	assert.True(t, found, "instance must be persisted via Storage.AddInstance")

	// Registered: live handle acquirable from the registry.
	live, release, acquireErr := registry.Acquire(instance.GetStableID())
	require.NoError(t, acquireErr, "expected instance to be registered in the live-handle registry")
	defer release()
	assert.NotNil(t, live)
}

// TestCreateManagedInstance_ReturnsErrPathNotExist_When_DirectoryMissingAndNotCreateIfMissing
// covers the pre-flight path-existence check that used to live inline in
// SessionService.CreateSession: a Directory-mode session pointed at a
// nonexistent path with CreateIfMissing unset must fail fast with
// ErrPathNotExist (checked via errors.Is, per the sentinel-error contract the
// RPC handler relies on to reconstruct the original connect.CodeNotFound).
func TestCreateManagedInstance_ReturnsErrPathNotExist_When_DirectoryMissingAndNotCreateIfMissing(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	dir := t.TempDir()
	missingPath := dir + "/does-not-exist"

	instance, err := CreateManagedInstance(context.Background(), CreateManagedInstanceParams{
		Options: CreateInstanceOptions{
			Title:       "cmi-missing-path",
			Path:        missingPath,
			Program:     "claude",
			SessionType: SessionTypeDirectory,
		},
		Storage:         storage,
		CreateIfMissing: false,
	})
	require.Error(t, err)
	assert.Nil(t, instance)
	assert.True(t, errors.Is(err, ErrPathNotExist), "expected ErrPathNotExist, got %v", err)
}

// TestCreateManagedInstance_ReturnsErrResumePathNotExist_When_ResumingMissingPath
// covers the resume-specific variant of the same guard: a missing path with a
// non-empty ResumeID means the original project directory is gone, which the
// handler maps to a different user-facing message via errors.Is(err,
// ErrResumePathNotExist).
func TestCreateManagedInstance_ReturnsErrResumePathNotExist_When_ResumingMissingPath(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	dir := t.TempDir()
	missingPath := dir + "/does-not-exist"

	instance, err := CreateManagedInstance(context.Background(), CreateManagedInstanceParams{
		Options: CreateInstanceOptions{
			Title:       "cmi-missing-resume-path",
			Path:        missingPath,
			Program:     "claude",
			SessionType: SessionTypeDirectory,
		},
		Storage:  storage,
		ResumeID: "abc-123",
	})
	require.Error(t, err)
	assert.Nil(t, instance)
	assert.True(t, errors.Is(err, ErrResumePathNotExist), "expected ErrResumePathNotExist, got %v", err)
}
