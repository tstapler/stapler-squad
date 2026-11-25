package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// NewCore must hand the real storage to the peer-help and owner-action tools
// through newIdentityGatedModules. Not parallel: it sets a process-global wake
// deps provider and the config directory.
func TestNewCore_should_ReachTheStorageFromRequestOwnerAction_When_StorageIsWired(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"feature_flags":{"agent_wake_triggers":true}}`), 0o600))
	t.Setenv("STAPLER_SQUAD_TEST_DIR", dir)
	deps := &WakeToolDeps{Backend: newFakeWakeBackend(), Owners: fakeOwnerLookup{facts: map[string]wakes.OwnerFacts{}}}
	SetWakeToolDeps(func() *WakeToolDeps { return deps })
	t.Cleanup(func() { SetWakeToolDeps(nil) })

	storage := newTestGuidanceStorage(t)
	item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "unlinked", Status: string(domain.BacklogStatusReady)})
	require.NoError(t, err)
	s := NewCore(&stubStore{}, nil, nil, storage, nil, nil, nil, nil, nil)
	tool := s.GetTool(toolRequestOwnerAction)
	require.NotNil(t, tool)

	res, err := tool.Handler(as(uuid.NewString()), makeToolReq(ownerActionArgs(item.ID)))
	require.NoError(t, err)

	assert.Equal(t, ErrPermissionDenied, resultCode(parseResult(t, res)), "an existing item this session is not linked to proves the storage reached the handler")
}
