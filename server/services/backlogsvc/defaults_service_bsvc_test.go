package backlogsvc

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// TestUpdateGlobalDefaults_should_UpdateBacklogServiceLiveConfig_When_SharedConfigWired
// is the regression test for PR #199 review F1: BacklogService.cfg is a
// *config.Config pointer set once at process start, while UpdateGlobalDefaults
// previously called config.LoadConfig() again independently — a fresh read into
// a brand new struct — and saved that, never writing back into the pointer
// BacklogService actually reads MaxConcurrentBacklogWorkItemsOrDefault() from.
// Raising the cap via Settings therefore had zero runtime effect until a
// restart. With SetSharedBacklogConfig wiring BacklogService's own
// *config.Config + mutex into DefaultsService (mirroring
// server/dependencies.go's production wiring), a successful UpdateGlobalDefaults
// call must be immediately observable through that SAME instance — not just
// through a second, independent config.LoadConfig() call.
func TestUpdateGlobalDefaults_should_UpdateBacklogServiceLiveConfig_When_SharedConfigWired(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	// The exact instance BacklogService would hold in production (see
	// server/dependencies.go: cfg := config.LoadConfig(); ... NewBacklogService(..., cfg, ...)).
	cfg := config.LoadConfig()
	backlogSvc := NewBacklogService(nil, nil, cfg, nil, nil, nil)
	svc.SetSharedBacklogConfig(cfg, backlogSvc.ConfigMu())

	require.Equal(t, 2, cfg.MaxConcurrentBacklogWorkItemsOrDefault(), "sanity check: default cap before any update")

	_, err := svc.UpdateGlobalDefaults(context.Background(), connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
		MaxConcurrentBacklogWorkItems: 5,
	}))
	require.NoError(t, err)

	assert.Equal(t, 5, cfg.MaxConcurrentBacklogWorkItemsOrDefault(),
		"BacklogService's own live config instance must observe the raised cap immediately, with no restart or reload")
}
