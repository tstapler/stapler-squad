package services

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/rules"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/testutil"
)

// These tests pin NewSessionService's wiring of the server/services/rules leaf.

func newRulesWiringService(t *testing.T, storage *session.Storage) *SessionService {
	t.Helper()
	svc := NewSessionService(storage, events.NewEventBus(100))
	t.Cleanup(func() { svc.Shutdown() })
	return svc
}

func classifierRuleIDs(svc *SessionService) []string {
	current := svc.GetClassifier().Rules()
	ids := make([]string, 0, len(current))
	for _, r := range current {
		ids = append(ids, r.ID)
	}
	return ids
}

func TestNewSessionService_should_LoadStoredUserRulesIntoTheClassifier(t *testing.T) {
	svctest.WithFakeHomePath(t, t.TempDir())
	storage := svctest.CreateTestStorage(t)
	store, err := rules.NewRulesStore(storage)
	require.NoError(t, err)
	_, err = store.Upsert(context.Background(), domain.RuleSpec{
		ID: "stored-user-rule", Name: "stored", ToolName: "Bash", Decision: "deny", Source: "user", Enabled: true, Priority: 10,
	})
	require.NoError(t, err)

	svc := newRulesWiringService(t, storage)

	assert.Contains(t, classifierRuleIDs(svc), "stored-user-rule")
}

func TestNewSessionService_should_ServeRulesFromAnEmptyStore_When_TheStoredRulesCannotBeLoaded(t *testing.T) {
	svctest.WithFakeHomePath(t, t.TempDir())
	storage := svctest.CreateTestStorage(t)
	require.NoError(t, storage.GetEntClient().Close()) // Storage.Close is a no-op
	_, loadErr := rules.NewRulesStore(storage)
	require.Error(t, loadErr, "the fixture must make the load fail")

	svc := newRulesWiringService(t, storage)

	resp, err := svc.ListApprovalRules(t.Context(), connect.NewRequest(&sessionv1.ListApprovalRulesRequest{}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.Rules, "seed rules are still listed")
}

func TestNewSessionService_should_StartTheAnalyticsFlushLoop(t *testing.T) {
	svctest.WithFakeHomePath(t, t.TempDir())
	svc := newRulesWiringService(t, svctest.CreateTestStorage(t))

	svc.GetAnalyticsStore().Record(rules.AnalyticsEntry{SessionID: "flush-probe", ToolName: "Bash", Decision: "allow", RiskLevel: "low"})

	require.NoError(t, testutil.WaitForCondition(func() bool {
		entries, err := svc.GetAnalyticsStore().LoadWindow(context.Background(), time.Now().Add(-time.Hour))
		return err == nil && len(entries) == 1
	}, testutil.FastWaitConfig()))
}

func TestNewSessionService_should_ApplyReloadedClaudeSettingsRules_When_TheWatcherFires(t *testing.T) {
	home := t.TempDir()
	svctest.WithFakeHomePath(t, home)
	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"allow":["Bash(git log*)"]}}`)
	svc := newRulesWiringService(t, svctest.CreateTestStorage(t))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	svc.GetClaudeSettingsWatcher().Start(ctx)

	current := svc.GetClassifier().Rules()
	sources := make([]string, 0, len(current))
	for _, r := range current {
		sources = append(sources, r.Source)
	}
	assert.Contains(t, sources, "claude-settings")
	// The RPC reaches the watcher only through SetClaudeSettingsWatcher.
	_, err := svc.ReloadClaudeSettingsRules(t.Context(), connect.NewRequest(&sessionv1.ReloadClaudeSettingsRulesRequest{}))
	assert.NoError(t, err)
}

// A cancelled context stops the request before any model is called; without a prompt
// builder the handler answers Unimplemented instead.
func TestNewSessionService_should_EnableRuleGeneration_When_AnAIClientIsAvailable(t *testing.T) {
	svctest.WithFakeHomePath(t, t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-wiring")
	svc := newRulesWiringService(t, svctest.CreateTestStorage(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.GenerateSuggestedRule(ctx, connect.NewRequest(&sessionv1.GenerateSuggestedRuleRequest{
		Source: sessionv1.SuggestionSource_SUGGESTION_SOURCE_ANALYTICS_GAPS,
	}))

	require.Error(t, err)
	assert.NotEqual(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestStartSharedRulesReload_should_StartOnlyOutsideTestModeWithAHome(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		testMode bool
		homeErr  error
		want     []string
	}{
		"server":    {want: []string{"/h/.config/stapler-squad/shared_rules.yaml 3s"}},
		"test mode": {testMode: true},
		"no home":   {homeErr: errors.New("no home")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var started []string
			startSharedRulesReload(tc.testMode,
				func() (string, error) { return "/h", tc.homeErr },
				func(_ context.Context, path string, interval time.Duration) {
					started = append(started, path+" "+interval.String())
				})
			assert.Equal(t, tc.want, started)
		})
	}
}

// NewSessionService runs in test mode here, and the order only matters to a reconcile
// goroutine, so neither the arguments nor the order are observable: pin them in the source.
func TestNewSessionService_should_StartTheSharedRulesReloadAfterTheApprovalServiceIsSet(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("session_service.go")
	require.NoError(t, err)
	file, err := parser.ParseFile(token.NewFileSet(), "session_service.go", src, 0)
	require.NoError(t, err)
	var statements []string // unconditional statements of the constructor, in order
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "NewSessionServiceWithSearchEngine" {
			continue
		}
		for _, stmt := range fn.Body.List {
			if expr, ok := stmt.(*ast.ExprStmt); ok {
				statements = append(statements, types.ExprString(expr.X))
			}
		}
	}

	set := slices.Index(statements, "rulesSvc.SetApprovalService(approvalSvc)")
	start := slices.Index(statements,
		"startSharedRulesReload(config.IsTestMode(), os.UserHomeDir, rulesSvc.StartConfigFileRulesReload)")

	require.GreaterOrEqual(t, set, 0)
	assert.Greater(t, start, set)
}
