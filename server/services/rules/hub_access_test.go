package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestNewEmptyRulesStore_should_AcceptRules_When_NothingWasLoaded(t *testing.T) {
	t.Parallel()
	store := NewEmptyRulesStore(svctest.CreateTestStorage(t))
	assert.Empty(t, store.All())

	_, err := store.Upsert(t.Context(), domain.RuleSpec{ID: "r1", Name: "r1", ToolName: "Bash", Decision: "deny", Source: "user"})

	require.NoError(t, err)
	assert.Len(t, store.All(), 1)
}

func TestDisableAntigravityExport_should_ClearTheExporter(t *testing.T) {
	t.Parallel()
	svc := NewRulesService(nil, nil, nil, nil, nil, nil)
	require.NotNil(t, svc.exportAntigravity)

	svc.DisableAntigravityExport()

	assert.Nil(t, svc.exportAntigravity)
}
