package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/session"
)

// fakeJulesKeyManager is a fake julesKeyManager, letting Story 2.4.1 tests
// exercise has_api_key without a real OS keychain.
type fakeJulesKeyManager struct {
	key    jules.JulesAPIKey
	hasKey bool
}

func (f *fakeJulesKeyManager) APIKey(_ context.Context) (jules.JulesAPIKey, error) {
	if !f.hasKey {
		return "", jules.ErrJulesNotConfigured
	}
	return f.key, nil
}

func (f *fakeJulesKeyManager) SetJulesAPIKey(_ context.Context, key jules.JulesAPIKey) error {
	f.key = key
	f.hasKey = true
	return nil
}

func (f *fakeJulesKeyManager) DeleteJulesAPIKey(_ context.Context) error {
	f.key = ""
	f.hasKey = false
	return nil
}

// fakeJulesPollerStorage is a minimal fake session.julesPollerStorage (an
// unexported interface — satisfied structurally) so a real
// *session.JulesSessionPoller can be exercised here without a real
// *session.Storage. Only ListOpenJulesItemSessions/GetItemSessionBySessionUUID
// carry test-controlled data; every write is a no-op success.
type fakeJulesPollerStorage struct {
	entries []session.ItemSessionBacklogEntry
	summary session.ItemSessionSummary
}

func (f *fakeJulesPollerStorage) ListOpenJulesItemSessions(_ context.Context) ([]session.ItemSessionBacklogEntry, error) {
	return f.entries, nil
}

func (f *fakeJulesPollerStorage) GetItemSessionBySessionUUID(_ context.Context, _ string) (session.ItemSessionSummary, error) {
	return f.summary, nil
}

func (f *fakeJulesPollerStorage) TouchItemSessionProgress(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (f *fakeJulesPollerStorage) UpdateItemSessionEndedWithReason(_ context.Context, _ string, _ time.Time, _ string) error {
	return nil
}

func (f *fakeJulesPollerStorage) AppendProgressNote(_ context.Context, _ string, _ int, _, _ string) error {
	return nil
}

func (f *fakeJulesPollerStorage) SetBacklogItemPRAndTransition(_ context.Context, _ *session.BacklogItemData, _ string, _ int, _ string, _ *session.PRReassignmentGuard) error {
	return nil
}

func (f *fakeJulesPollerStorage) TransitionBacklogItemStatus(_ context.Context, _ string, _ session.BacklogStatus, _ *session.BacklogItemPrecondition, _ string) (*session.BacklogItemData, error) {
	return &session.BacklogItemData{}, nil
}

// fakeJulesPollStatusClient is a fake session.julesStatusClient whose
// GetSession outcome flips at runtime via setFail, driving
// JulesSessionPoller's auth-reconnect state machine (Story 2.3.4) for
// TestJulesConfigService_GetJulesConfig_should_ReflectPollerAuthReconnectRequiredLive_When_PollerFlagToggles.
type fakeJulesPollStatusClient struct {
	fail chan bool
	cur  bool
}

func newFakeJulesPollStatusClient(initialFail bool) *fakeJulesPollStatusClient {
	c := &fakeJulesPollStatusClient{fail: make(chan bool, 1), cur: initialFail}
	c.fail <- initialFail
	return c
}

func (f *fakeJulesPollStatusClient) IsLimited() bool { return false }

func (f *fakeJulesPollStatusClient) GetSession(_ context.Context, _ jules.JulesSessionName) (*jules.JulesSession, error) {
	select {
	case v := <-f.fail:
		f.cur = v
		f.fail <- v
	default:
	}
	if f.cur {
		return nil, jules.ErrJulesNotConfigured
	}
	return &jules.JulesSession{Name: "sessions/xyz", State: jules.JulesStateQueued}, nil
}

func (f *fakeJulesPollStatusClient) setFail(v bool) {
	<-f.fail
	f.fail <- v
}

func TestJulesConfigService_GetJulesConfig_should_NeverReturnKeyMaterial_When_KeyStored(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	const key = "AIzaSyD-EXAMPLE"
	svc := NewJulesConfigService(&fakeJulesKeyManager{hasKey: true, key: key}, nil, nil)

	resp, err := svc.GetJulesConfig(context.Background(), connect.NewRequest(&sessionv1.GetJulesConfigRequest{}))
	require.NoError(t, err)

	assert.True(t, resp.Msg.Config.HasApiKey)
	// JulesConfigProto structurally has no field that could hold the key, but
	// assert the serialized form too, matching this codebase's existing
	// never-leak-a-secret test convention (slack_config_service_test.go).
	assert.NotContains(t, resp.Msg.Config.String(), key)
	assert.NotContains(t, resp.Msg.Config.String(), "AIzaSyD")
}

func TestJulesConfigService_UpdateJulesConfig_should_WriteKeychainNotConfigJSON_When_APIKeyProvided(t *testing.T) {
	keyring.MockInit()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	const newKey = "AIzaSyD-NEW"

	keys := jules.NewKeyringTokenSource()
	svc := NewJulesConfigService(keys, nil, nil)

	resp, err := svc.UpdateJulesConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateJulesConfigRequest{
		ApiKey:  newKey,
		Enabled: true,
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Config.Enabled)
	assert.True(t, resp.Msg.Config.HasApiKey)

	stored, err := keys.APIKey(context.Background())
	require.NoError(t, err)
	assert.Equal(t, jules.JulesAPIKey(newKey), stored, "KeyringTokenSource must hold the new key")

	configDir, err := config.GetConfigDir()
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(configDir, config.ConfigFileName))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), newKey, "config.json must never contain the api key")
}

func TestJulesConfigService_TestJulesConnection_should_NameUnconnectedRepo_When_SourceNotInListSources(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	repoPath := newTestJulesRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	registry := jules.NewJulesSourceRegistry(&fakeSourceLister{
		sources: []jules.JulesSource{{Name: "sources/github-tstapler-dotfiles", ID: "src-1"}},
	})
	svc := NewJulesConfigService(nil, registry, nil)

	resp, err := svc.TestJulesConnection(context.Background(), connect.NewRequest(&sessionv1.TestJulesConnectionRequest{
		RepoPath: repoPath,
	}))
	require.NoError(t, err)

	assert.False(t, resp.Msg.Ok)
	assert.Contains(t, resp.Msg.Message, "tstapler/stapler-squad")
	assert.Contains(t, resp.Msg.Message, "jules.google.com")
}

func TestJulesConfigService_GetJulesConfig_should_ReflectPollerAuthReconnectRequiredLive_When_PollerFlagToggles(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	client := newFakeJulesPollStatusClient(true)
	storage := &fakeJulesPollerStorage{
		entries: []session.ItemSessionBacklogEntry{{
			SessionUUID: "jules-sessions/xyz",
			SessionRole: session.SessionRoleJulesWork,
			ItemID:      "item-1",
		}},
		summary: session.ItemSessionSummary{ID: "is-1", SessionUUID: "jules-sessions/xyz", Role: session.SessionRoleJulesWork},
	}
	pollerCfg := session.JulesSessionPollerConfig{
		PollInterval:  5 * time.Millisecond,
		CallTimeout:   time.Second,
		MaxSessionAge: time.Hour,
	}
	poller := session.NewJulesSessionPoller(client, storage, pollerCfg)
	svc := NewJulesConfigService(nil, nil, poller)

	poller.Start(t.Context())
	t.Cleanup(poller.Stop)

	require.Eventually(t, poller.AuthReconnectRequired, time.Second, 5*time.Millisecond,
		"poller must observe the 401/403 and set AuthReconnectRequired")

	resp, err := svc.GetJulesConfig(context.Background(), connect.NewRequest(&sessionv1.GetJulesConfigRequest{}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Config.AuthReconnectRequired)

	client.setFail(false)
	require.Eventually(t, func() bool { return !poller.AuthReconnectRequired() }, time.Second, 5*time.Millisecond,
		"poller must clear AuthReconnectRequired on its next successful tick")

	resp2, err := svc.GetJulesConfig(context.Background(), connect.NewRequest(&sessionv1.GetJulesConfigRequest{}))
	require.NoError(t, err)
	assert.False(t, resp2.Msg.Config.AuthReconnectRequired)
}

func TestJulesConfigService_GetJulesConfig_should_ReturnAuthReconnectRequiredFalse_When_PollerDependencyIsNil(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	svc := NewJulesConfigService(nil, nil, nil)

	resp, err := svc.GetJulesConfig(context.Background(), connect.NewRequest(&sessionv1.GetJulesConfigRequest{}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Config.AuthReconnectRequired)
}

func TestJulesConfigService_ConfirmEgressConsent_should_AppendAndPersistRepo_When_RepoNotAlreadyAcknowledged(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	svc := NewJulesConfigService(nil, nil, nil)
	const repoPath = "/home/tstapler/code/github.com/tstapler/stapler-squad"

	resp, err := svc.ConfirmEgressConsent(context.Background(), connect.NewRequest(&sessionv1.ConfirmEgressConsentRequest{RepoPath: repoPath}))
	require.NoError(t, err)
	assert.Equal(t, []string{repoPath}, resp.Msg.EgressAcknowledgedRepos)

	cfg := config.LoadConfig()
	assert.Contains(t, cfg.Jules.EgressAcknowledgedRepos, repoPath)
}

func TestJulesConfigService_ConfirmEgressConsent_should_AvoidDuplicateEntry_When_RepoAlreadyAcknowledged(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	svc := NewJulesConfigService(nil, nil, nil)
	const repoPath = "/home/tstapler/code/github.com/tstapler/stapler-squad"

	_, err := svc.ConfirmEgressConsent(context.Background(), connect.NewRequest(&sessionv1.ConfirmEgressConsentRequest{RepoPath: repoPath}))
	require.NoError(t, err)

	resp, err := svc.ConfirmEgressConsent(context.Background(), connect.NewRequest(&sessionv1.ConfirmEgressConsentRequest{RepoPath: repoPath}))
	require.NoError(t, err)
	assert.Equal(t, []string{repoPath}, resp.Msg.EgressAcknowledgedRepos, "must not gain a duplicate entry")
}

func TestJulesConfigService_ConfirmEgressConsent_should_ReturnInvalidArgument_When_RepoPathEmpty(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	svc := NewJulesConfigService(nil, nil, nil)

	_, err := svc.ConfirmEgressConsent(context.Background(), connect.NewRequest(&sessionv1.ConfirmEgressConsentRequest{RepoPath: ""}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	cfg := config.LoadConfig()
	assert.Empty(t, cfg.Jules.EgressAcknowledgedRepos)
}

func TestJulesConfigService_RevokeEgressConsent_should_RemoveOnlyTargetRepo_When_MultipleRepoAcknowledged(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	svc := NewJulesConfigService(nil, nil, nil)
	const repoA = "/home/tstapler/code/github.com/tstapler/stapler-squad"
	const repoB = "/home/tstapler/code/github.com/tstapler/dotfiles"

	_, err := svc.ConfirmEgressConsent(context.Background(), connect.NewRequest(&sessionv1.ConfirmEgressConsentRequest{RepoPath: repoA}))
	require.NoError(t, err)
	_, err = svc.ConfirmEgressConsent(context.Background(), connect.NewRequest(&sessionv1.ConfirmEgressConsentRequest{RepoPath: repoB}))
	require.NoError(t, err)

	resp, err := svc.RevokeEgressConsent(context.Background(), connect.NewRequest(&sessionv1.RevokeEgressConsentRequest{RepoPath: repoA}))
	require.NoError(t, err)
	assert.Equal(t, []string{repoB}, resp.Msg.EgressAcknowledgedRepos, "only repoA should be removed")

	cfg := config.LoadConfig()
	assert.NotContains(t, cfg.Jules.EgressAcknowledgedRepos, repoA)
	assert.Contains(t, cfg.Jules.EgressAcknowledgedRepos, repoB)
}

func TestJulesConfigService_RevokeEgressConsent_should_BeIdempotent_When_RepoNotAcknowledged(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	svc := NewJulesConfigService(nil, nil, nil)
	const repoPath = "/home/tstapler/code/github.com/tstapler/stapler-squad"

	resp, err := svc.RevokeEgressConsent(context.Background(), connect.NewRequest(&sessionv1.RevokeEgressConsentRequest{RepoPath: repoPath}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.EgressAcknowledgedRepos)
}

func TestJulesConfigService_RevokeEgressConsent_should_ReturnInvalidArgument_When_RepoPathEmpty(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	svc := NewJulesConfigService(nil, nil, nil)

	_, err := svc.RevokeEgressConsent(context.Background(), connect.NewRequest(&sessionv1.RevokeEgressConsentRequest{RepoPath: ""}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestJulesDispatchService_should_NeverCallConfirmEgressConsentMutation_When_SourceScanned
// is Story 2.4.2's cannot-self-grant proof (mirrors Story 2.2.3's read-only
// checkEgressConsent proof): a whole-file static scan of
// jules_dispatch_service.go for any call into the mutation path
// ConfirmEgressConsent uses (JulesConfigService.ConfirmEgressConsent itself,
// or a direct EgressAcknowledgedRepos append) — zero occurrences expected.
func TestJulesDispatchService_should_NeverCallConfirmEgressConsentMutation_When_SourceScanned(t *testing.T) {
	src, err := os.ReadFile("jules_dispatch_service.go")
	require.NoError(t, err)
	text := string(src)

	assert.NotContains(t, text, "ConfirmEgressConsent(",
		"jules_dispatch_service.go must never call the ConfirmEgressConsent mutation RPC")
	assert.NotContains(t, text, "EgressAcknowledgedRepos = append",
		"jules_dispatch_service.go must never append to EgressAcknowledgedRepos directly — only ConfirmEgressConsent may")
}
