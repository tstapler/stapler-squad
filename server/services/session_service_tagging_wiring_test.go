package services

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func wiringTaggingRule(id string) domain.TaggingRuleData {
	return domain.TaggingRuleData{RuleID: id, Name: id, BranchPattern: "^zz-wiring/", OutputTag: "WiringTag",
		Enabled: true, Source: "user", CreatedAt: time.Now(), UpdatedAt: time.Now()}
}

func TestNewSessionService_should_LoadPersistedTaggingRulesIntoEngine_When_Starting(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	require.NoError(t, storage.UpsertTaggingRule(context.Background(), wiringTaggingRule("persisted")))

	svc := NewSessionService(storage, events.NewEventBus(100))
	t.Cleanup(func() { svc.Shutdown() })

	added, _, _ := svc.GetTaggingEngine().ApplyToFixpoint(classifier.SessionTaggingContext{Branch: "zz-wiring/x"})
	require.Len(t, added, 1)
	assert.Equal(t, "WiringTag", added[0].Tag)
}

// An undecodable required_tags column makes the startup load fail while writes still work.
func TestNewSessionService_should_StillPersistTaggingRules_When_StartupLoadFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sessions.db")
	repo, err := session.NewEntRepository(session.WithDatabasePath(path))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, repo.Close()) })
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	require.NoError(t, storage.UpsertTaggingRule(ctx, wiringTaggingRule("corrupt")))
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, db.Close()) })
	_, err = db.ExecContext(ctx, "UPDATE tagging_rules SET required_tags = 'not json'")
	require.NoError(t, err)
	_, err = storage.AllTaggingRules(ctx)
	require.Error(t, err, "precondition: the startup load must fail")

	svc := NewSessionService(storage, events.NewEventBus(100))
	t.Cleanup(func() { svc.Shutdown() })

	_, err = svc.UpsertTaggingRule(ctx, connect.NewRequest(&sessionv1.UpsertTaggingRuleRequest{
		Rule: &sessionv1.TaggingRuleProto{Id: "after-fallback", Name: "after", OutputTag: "After", Enabled: true},
	}))
	require.NoError(t, err)

	var persisted int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tagging_rules WHERE rule_id = 'after-fallback'").Scan(&persisted))
	assert.Equal(t, 1, persisted)
}
