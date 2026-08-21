package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// newIsolatedDefaultsService creates a DefaultsService backed by a fresh temporary
// directory, preventing config state from leaking between tests.
func newIsolatedDefaultsService(t *testing.T) *DefaultsService {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	return NewDefaultsService()
}

// TestNewDefaultsServiceCalledOnlyViaHelper ensures no test in this file bypasses
// the isolation helper by calling NewDefaultsService() directly.
// Bare calls read/write the real user config dir, causing order-dependent failures.
//
// Pattern matched: lines that assign the result of NewDefaultsService() (i.e.
// `:= NewDefaultsService()`), which is the only form a direct test call takes.
// This excludes: the helper's own return statement, comment lines, and string
// literals inside this function that happen to contain the text.
func TestNewDefaultsServiceCalledOnlyViaHelper(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("defaults_service_test.go")
	require.NoError(t, err)

	// inSelf tracks whether the scanner is inside this function's body, so that
	// string literals referencing the pattern don't trigger a false positive.
	inSelf := false
	depth := 0

	var violations []string
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(line, "func TestNewDefaultsServiceCalledOnlyViaHelper(") {
			inSelf = true
		}
		if inSelf {
			depth += strings.Count(line, "{") - strings.Count(line, "}")
			if depth <= 0 && inSelf {
				inSelf = false
			}
			continue
		}

		// Only flag assignment-form calls: `svc := NewDefaultsService()`
		if !strings.Contains(trimmed, ":= NewDefaultsService()") {
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		violations = append(violations, fmt.Sprintf("  line %d: %s", i+1, trimmed))
	}
	if len(violations) > 0 {
		t.Errorf("bare NewDefaultsService() calls found — use newIsolatedDefaultsService(t) instead:\n%s",
			strings.Join(violations, "\n"))
	}
}

// TestGetSessionDefaults_ReturnsDefaults verifies that GetSessionDefaults returns a
// non-nil defaults payload without error on a fresh (empty) configuration.
func TestGetSessionDefaults_ReturnsDefaults(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.GetSessionDefaultsRequest{})
	resp, err := svc.GetSessionDefaults(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	assert.NotNil(t, resp.Msg.Defaults)
}

// TestResolveDefaults_NoPath verifies that ResolveDefaults with an empty working
// directory succeeds and returns a response (falls back to global defaults).
func TestResolveDefaults_NoPath(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.ResolveDefaultsRequest{
		WorkingDir:  "",
		ProfileName: "",
	})
	resp, err := svc.ResolveDefaults(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	// EnvVars must never be nil so the proto response is valid.
	assert.NotNil(t, resp.Msg.EnvVars)
}

// TestUpdateGlobalDefaults_UpdatesProgram verifies that calling UpdateGlobalDefaults
// with a program name persists it and returns the updated defaults.
func TestUpdateGlobalDefaults_UpdatesProgram(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
		Program: "aider",
	})
	resp, err := svc.UpdateGlobalDefaults(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Defaults)
	assert.Equal(t, "aider", resp.Msg.Defaults.Program)
}

// TestUpdateGlobalDefaults_SetsMaxConcurrentBacklogWorkItems verifies that the
// backlog work-item concurrency cap round-trips through UpdateGlobalDefaults /
// GetSessionDefaults, replacing what used to be a hardcoded constant.
func TestUpdateGlobalDefaults_SetsMaxConcurrentBacklogWorkItems(t *testing.T) {
	svc := newIsolatedDefaultsService(t)
	ctx := context.Background()

	resp, err := svc.UpdateGlobalDefaults(ctx, connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
		MaxConcurrentBacklogWorkItems: 5,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Defaults)
	assert.Equal(t, int32(5), resp.Msg.Defaults.MaxConcurrentBacklogWorkItems)

	getResp, err := svc.GetSessionDefaults(ctx, connect.NewRequest(&sessionv1.GetSessionDefaultsRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int32(5), getResp.Msg.Defaults.MaxConcurrentBacklogWorkItems)
}

// TestUpdateGlobalDefaults_RaisingLimitTriggersImmediateDequeue verifies that a
// successful UpdateGlobalDefaults call invokes the onGlobalDefaultsUpdated
// callback (wired in server/dependencies.go to an immediate backlog dequeue
// sweep), rather than requiring callers to wait for the next 60s reconcile tick.
func TestUpdateGlobalDefaults_RaisingLimitTriggersImmediateDequeue(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	called := make(chan struct{}, 1)
	svc.SetOnGlobalDefaultsUpdated(func() { called <- struct{}{} })

	_, err := svc.UpdateGlobalDefaults(context.Background(), connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
		MaxConcurrentBacklogWorkItems: 5,
	}))
	require.NoError(t, err)

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("onGlobalDefaultsUpdated callback was not invoked after UpdateGlobalDefaults")
	}
}

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

// TestGetSessionDefaults_ResolvesStaleSessionDefaults verifies that on a fresh
// config with no stale_session key at all, GetSessionDefaults still returns the
// resolved (never-zero) server defaults: 30 minute threshold, notifications on.
func TestGetSessionDefaults_ResolvesStaleSessionDefaults(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	resp, err := svc.GetSessionDefaults(context.Background(), connect.NewRequest(&sessionv1.GetSessionDefaultsRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Defaults)
	assert.Equal(t, int32(30), resp.Msg.Defaults.StaleSessionThresholdMinutes)
	assert.True(t, resp.Msg.Defaults.StaleSessionNotifyEnabled)
}

// TestUpdateGlobalDefaults_ZeroStaleSessionThreshold_UsesServerDefault verifies the
// "0 means use the server default" convention (matching max_auto_rework_iterations
// and max_concurrent_backlog_work_items): sending 0 leaves the persisted config.json
// without an explicit override, and the response echoes the resolved default (30).
//
// The "ResetsAfterExplicitOverride" sub-case covers the bug this convention previously
// violated: cfg.StaleSession.ThresholdMinutes was only assigned when
// req.Msg.StaleSessionThresholdMinutes > 0, so once a user set an explicit override
// (e.g. 45), sending 0 to reset it back to "use default" was silently ignored and 45
// persisted forever. The fix makes the assignment unconditional, exactly matching the
// sibling MaxAutoReworkIterations/MaxConcurrentBacklogWorkItems fields two lines above,
// relying on ThresholdMinutesOrDefault() to resolve a stored 0 back to 30 at read time.
func TestUpdateGlobalDefaults_ZeroStaleSessionThreshold_UsesServerDefault(t *testing.T) {
	t.Run("StartsUnset", func(t *testing.T) {
		svc := newIsolatedDefaultsService(t)

		resp, err := svc.UpdateGlobalDefaults(context.Background(), connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
			StaleSessionThresholdMinutes: 0,
			StaleSessionNotifyEnabled:    true,
		}))
		require.NoError(t, err)
		require.NotNil(t, resp.Msg.Defaults)
		assert.Equal(t, int32(30), resp.Msg.Defaults.StaleSessionThresholdMinutes)

		configDir, err := config.GetConfigDir()
		require.NoError(t, err)
		data, err := os.ReadFile(filepath.Join(configDir, config.ConfigFileName))
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &raw))
		if staleSessionRaw, ok := raw["stale_session"]; ok {
			var staleSession map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(staleSessionRaw, &staleSession))
			if thresholdRaw, ok := staleSession["threshold_minutes"]; ok {
				assert.Equal(t, "0", string(thresholdRaw), "threshold_minutes must stay unset/0, not the resolved default")
			}
		}
	})

	t.Run("ResetsAfterExplicitOverride", func(t *testing.T) {
		svc := newIsolatedDefaultsService(t)

		// First set an explicit override away from the default.
		overrideResp, err := svc.UpdateGlobalDefaults(context.Background(), connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
			StaleSessionThresholdMinutes: 45,
			StaleSessionNotifyEnabled:    true,
		}))
		require.NoError(t, err)
		require.NotNil(t, overrideResp.Msg.Defaults)
		assert.Equal(t, int32(45), overrideResp.Msg.Defaults.StaleSessionThresholdMinutes)

		configDir, err := config.GetConfigDir()
		require.NoError(t, err)
		configPath := filepath.Join(configDir, config.ConfigFileName)

		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		var persistedAfterOverride config.Config
		require.NoError(t, json.Unmarshal(data, &persistedAfterOverride))
		require.Equal(t, 45, persistedAfterOverride.StaleSession.ThresholdMinutes, "override must persist before the reset")

		// Now send 0 to reset back to "use server default" — this must not be a no-op.
		resetResp, err := svc.UpdateGlobalDefaults(context.Background(), connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
			StaleSessionThresholdMinutes: 0,
			StaleSessionNotifyEnabled:    true,
		}))
		require.NoError(t, err)
		require.NotNil(t, resetResp.Msg.Defaults)
		assert.Equal(t, int32(30), resetResp.Msg.Defaults.StaleSessionThresholdMinutes, "response must echo the resolved default, not the stale 45 override")

		data, err = os.ReadFile(configPath)
		require.NoError(t, err)
		var persistedAfterReset config.Config
		require.NoError(t, json.Unmarshal(data, &persistedAfterReset))
		assert.Equal(t, 0, persistedAfterReset.StaleSession.ThresholdMinutes, "persisted threshold_minutes must be reset to 0, not left at 45")
	})
}

// TestUpdateGlobalDefaults_PersistsExplicitStaleSessionOverride is a real file-I/O
// integration test (matching this file's existing pattern, e.g.
// TestUpdateGlobalDefaults_should_UpdateBacklogServiceLiveConfig_When_SharedConfigWired):
// an explicit override must be persisted to the actual config.json on disk and echoed
// back resolved in the RPC response.
func TestUpdateGlobalDefaults_PersistsExplicitStaleSessionOverride(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	resp, err := svc.UpdateGlobalDefaults(context.Background(), connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
		StaleSessionThresholdMinutes: 45,
		StaleSessionNotifyEnabled:    false,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Defaults)
	assert.Equal(t, int32(45), resp.Msg.Defaults.StaleSessionThresholdMinutes)
	assert.False(t, resp.Msg.Defaults.StaleSessionNotifyEnabled)

	configDir, err := config.GetConfigDir()
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(configDir, config.ConfigFileName))
	require.NoError(t, err)

	var persisted config.Config
	require.NoError(t, json.Unmarshal(data, &persisted))
	assert.Equal(t, 45, persisted.StaleSession.ThresholdMinutes)
	require.NotNil(t, persisted.StaleSession.NotifyEnabled)
	assert.False(t, *persisted.StaleSession.NotifyEnabled)
}

// TestUpsertProfile_EmptyName verifies that UpsertProfile with an empty profile name
// returns CodeInvalidArgument.
func TestUpsertProfile_EmptyName(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertProfileRequest{
		Profile: &sessionv1.ProfileDefaultsProto{
			Name:    "",
			Program: "claude",
		},
	})
	_, err := svc.UpsertProfile(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertProfile_NilProfile verifies that UpsertProfile with a nil profile
// returns CodeInvalidArgument.
func TestUpsertProfile_NilProfile(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertProfileRequest{
		Profile: nil,
	})
	_, err := svc.UpsertProfile(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertProfile_CreatesProfile verifies that a valid name + program succeeds and
// the response echoes back the created profile.
func TestUpsertProfile_CreatesProfile(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertProfileRequest{
		Profile: &sessionv1.ProfileDefaultsProto{
			Name:    "work",
			Program: "claude",
		},
	})
	resp, err := svc.UpsertProfile(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Profile)
	assert.Equal(t, "work", resp.Msg.Profile.Name)
	assert.Equal(t, "claude", resp.Msg.Profile.Program)
}

// TestDeleteProfile_NotFound verifies that deleting a non-existent profile returns
// CodeNotFound.
func TestDeleteProfile_NotFound(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.DeleteProfileRequest{
		Name: "no-such-profile",
	})
	_, err := svc.DeleteProfile(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestDeleteProfile_EmptyName verifies that deleting with an empty name returns
// CodeInvalidArgument.
func TestDeleteProfile_EmptyName(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.DeleteProfileRequest{
		Name: "",
	})
	_, err := svc.DeleteProfile(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestDeleteProfile_Success verifies that upserting a profile and then deleting it
// succeeds.
func TestDeleteProfile_Success(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	// First create the profile.
	upsertReq := connect.NewRequest(&sessionv1.UpsertProfileRequest{
		Profile: &sessionv1.ProfileDefaultsProto{
			Name:    "temp-profile",
			Program: "aider",
		},
	})
	_, err := svc.UpsertProfile(context.Background(), upsertReq)
	require.NoError(t, err)

	// Now delete it.
	deleteReq := connect.NewRequest(&sessionv1.DeleteProfileRequest{
		Name: "temp-profile",
	})
	_, err = svc.DeleteProfile(context.Background(), deleteReq)
	require.NoError(t, err)
}

// TestUpsertDirectoryRule_EmptyPath verifies that UpsertDirectoryRule with an empty
// path returns CodeInvalidArgument.
func TestUpsertDirectoryRule_EmptyPath(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertDirectoryRuleRequest{
		Rule: &sessionv1.DirectoryRuleProto{
			Path: "",
		},
	})
	_, err := svc.UpsertDirectoryRule(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertDirectoryRule_NilRule verifies that UpsertDirectoryRule with a nil rule
// returns CodeInvalidArgument.
func TestUpsertDirectoryRule_NilRule(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertDirectoryRuleRequest{
		Rule: nil,
	})
	_, err := svc.UpsertDirectoryRule(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertDirectoryRule_ValidPath verifies that a rule with a valid path is created
// successfully and the response echoes back the path.
func TestUpsertDirectoryRule_ValidPath(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertDirectoryRuleRequest{
		Rule: &sessionv1.DirectoryRuleProto{
			Path:    "/home/user/projects",
			Profile: "work",
		},
	})
	resp, err := svc.UpsertDirectoryRule(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Rule)
	assert.Equal(t, "/home/user/projects", resp.Msg.Rule.Path)
	assert.Equal(t, "work", resp.Msg.Rule.Profile)
}

// TestDeleteDirectoryRule_NotFound verifies that deleting a non-existent directory
// rule returns CodeNotFound.
func TestDeleteDirectoryRule_NotFound(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.DeleteDirectoryRuleRequest{
		Path: "/nonexistent/path",
	})
	_, err := svc.DeleteDirectoryRule(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestDeleteDirectoryRule_EmptyPath verifies that deleting with an empty path returns
// CodeInvalidArgument.
func TestDeleteDirectoryRule_EmptyPath(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.DeleteDirectoryRuleRequest{
		Path: "",
	})
	_, err := svc.DeleteDirectoryRule(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestListAliases_ReturnsEmptyList_WhenNoAliasesConfigured verifies that ListAliases
// returns a non-nil empty slice when no aliases are configured.
func TestListAliases_ReturnsEmptyList_WhenNoAliasesConfigured(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.ListAliasesRequest{})
	resp, err := svc.ListAliases(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	// With no aliases configured, Aliases should be an empty (non-nil) slice.
	assert.NotNil(t, resp.Msg.Aliases)
}

// TestListAliases_ReturnsAllAliases_WhenConfigHasAliases verifies that ListAliases
// returns all configured aliases with correct field mapping.
func TestListAliases_ReturnsAllAliases_WhenConfigHasAliases(t *testing.T) {
	// newIsolatedDefaultsService sets STAPLER_SQUAD_TEST_DIR to a fresh TempDir.
	// Read the env var back to know where to write the pre-populated config file.
	svc := newIsolatedDefaultsService(t)
	tmpDir := os.Getenv("STAPLER_SQUAD_TEST_DIR")

	cfg := &config.Config{}
	cfg.SessionDefaults.Aliases = []config.AliasConfig{
		{Name: "proj-a", Path: "/home/user/proj-a"},
		{Name: "proj-b", Path: "/home/user/proj-b", Group: "work"},
	}
	cfgBytes, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "config.json"), cfgBytes, 0o600))

	req := connect.NewRequest(&sessionv1.ListAliasesRequest{})
	resp, err := svc.ListAliases(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Aliases)
	require.Len(t, resp.Msg.Aliases, 2)

	names := make([]string, len(resp.Msg.Aliases))
	for i, a := range resp.Msg.Aliases {
		names[i] = a.Name
	}
	assert.Contains(t, names, "proj-a")
	assert.Contains(t, names, "proj-b")
}

// TestDeleteDirectoryRule_Success verifies that upserting a directory rule and then
// deleting it succeeds.
func TestDeleteDirectoryRule_Success(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	// Create the rule first.
	upsertReq := connect.NewRequest(&sessionv1.UpsertDirectoryRuleRequest{
		Rule: &sessionv1.DirectoryRuleProto{
			Path:    "/tmp/test-project",
			Profile: "default",
		},
	})
	_, err := svc.UpsertDirectoryRule(context.Background(), upsertReq)
	require.NoError(t, err)

	// Delete it.
	deleteReq := connect.NewRequest(&sessionv1.DeleteDirectoryRuleRequest{
		Path: "/tmp/test-project",
	})
	_, err = svc.DeleteDirectoryRule(context.Background(), deleteReq)
	require.NoError(t, err)
}

// TestUpsertAlias_NilAlias verifies that UpsertAlias with a nil alias returns
// CodeInvalidArgument.
func TestUpsertAlias_NilAlias(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: nil,
	})
	_, err := svc.UpsertAlias(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertAlias_EmptyName verifies that UpsertAlias with an empty alias name
// returns CodeInvalidArgument.
func TestUpsertAlias_EmptyName(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{Name: ""},
	})
	_, err := svc.UpsertAlias(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertAlias_InvalidName verifies that UpsertAlias with a name containing a
// space returns CodeInvalidArgument with a message referencing the regex pattern.
func TestUpsertAlias_InvalidName(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{Name: "my project"},
	})
	_, err := svc.UpsertAlias(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertAlias_CreatesAlias verifies that a valid new alias is appended to the
// config and the response echoes back the alias with correct fields.
func TestUpsertAlias_CreatesAlias(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{Name: "myproj", Path: "~/code"},
	})
	resp, err := svc.UpsertAlias(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Alias)
	assert.Equal(t, "myproj", resp.Msg.Alias.Name)
	assert.Equal(t, "~/code", resp.Msg.Alias.Path)

	cfg := config.LoadConfig()
	require.Len(t, cfg.SessionDefaults.Aliases, 1)
	assert.Equal(t, "myproj", cfg.SessionDefaults.Aliases[0].Name)
	assert.Equal(t, "~/code", cfg.SessionDefaults.Aliases[0].Path)
}

// TestUpsertAlias_UpdatesExistingAlias verifies that upserting an alias with the
// same name overwrites the existing entry (no duplicate) and updates fields.
func TestUpsertAlias_UpdatesExistingAlias(t *testing.T) {
	svc := newIsolatedDefaultsService(t)
	ctx := context.Background()

	// Pre-populate with an alias.
	_, err := svc.UpsertAlias(ctx, connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{Name: "myproj", Description: "old"},
	}))
	require.NoError(t, err)

	// Upsert the same name with a new description.
	resp, err := svc.UpsertAlias(ctx, connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{Name: "myproj", Description: "new"},
	}))
	require.NoError(t, err)
	assert.Equal(t, "new", resp.Msg.Alias.Description)

	cfg := config.LoadConfig()
	require.Len(t, cfg.SessionDefaults.Aliases, 1)
	assert.Equal(t, "new", cfg.SessionDefaults.Aliases[0].Description)
}

// TestUpsertAlias_CaseInsensitiveDuplicate verifies that upserting an alias whose
// name differs only in case from an existing alias overwrites in-place (no duplicate).
func TestUpsertAlias_CaseInsensitiveDuplicate(t *testing.T) {
	svc := newIsolatedDefaultsService(t)
	ctx := context.Background()

	// Pre-populate with mixed-case alias.
	_, err := svc.UpsertAlias(ctx, connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{Name: "MyProj"},
	}))
	require.NoError(t, err)

	// Upsert with lowercase name — should overwrite, not append.
	_, err = svc.UpsertAlias(ctx, connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{Name: "myproj"},
	}))
	require.NoError(t, err)

	cfg := config.LoadConfig()
	require.Len(t, cfg.SessionDefaults.Aliases, 1, "case-insensitive duplicate should overwrite, not append")
}

// TestUpsertAlias_AllFieldsRoundTrip verifies that all AliasProto fields are persisted
// and round-tripped correctly through UpsertAlias and config.LoadConfig.
func TestUpsertAlias_AllFieldsRoundTrip(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{
			Name:        "fullproj",
			Path:        "~/code",
			Group:       "work",
			Description: "my desc",
			Profile:     "my-profile",
			Program:     "aider",
			AutoYes:     true,
			Tags:        []string{"backend", "infra"},
			EnvVars:     map[string]string{"FOO": "bar"},
			CliFlags:    "--verbose",
		},
	})
	resp, err := svc.UpsertAlias(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Alias)
	assert.Equal(t, "fullproj", resp.Msg.Alias.Name)
	assert.Equal(t, "~/code", resp.Msg.Alias.Path)
	assert.Equal(t, "work", resp.Msg.Alias.Group)
	assert.Equal(t, "my desc", resp.Msg.Alias.Description)
	assert.Equal(t, "my-profile", resp.Msg.Alias.Profile)
	assert.Equal(t, "aider", resp.Msg.Alias.Program)
	assert.True(t, resp.Msg.Alias.AutoYes)
	assert.ElementsMatch(t, []string{"backend", "infra"}, resp.Msg.Alias.Tags)
	assert.Equal(t, map[string]string{"FOO": "bar"}, resp.Msg.Alias.EnvVars)
	assert.Equal(t, "--verbose", resp.Msg.Alias.CliFlags)

	cfg := config.LoadConfig()
	require.Len(t, cfg.SessionDefaults.Aliases, 1)
	a := cfg.SessionDefaults.Aliases[0]
	assert.Equal(t, "fullproj", a.Name)
	assert.Equal(t, "~/code", a.Path)
	assert.Equal(t, "work", a.Group)
	assert.Equal(t, "my desc", a.Description)
	assert.Equal(t, "my-profile", a.Profile)
	assert.Equal(t, "aider", a.Program)
	assert.True(t, a.AutoYes)
	assert.ElementsMatch(t, []string{"backend", "infra"}, a.Tags)
	assert.Equal(t, map[string]string{"FOO": "bar"}, a.EnvVars)
	assert.Equal(t, "--verbose", a.CLIFlags)
}

// TestUpsertAlias_WhitespaceOnlyName verifies that a whitespace-only name returns
// CodeInvalidArgument (treated as empty after trimming).
func TestUpsertAlias_WhitespaceOnlyName(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{Name: "   "},
	})
	_, err := svc.UpsertAlias(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertAlias_TrimsName verifies that a name with surrounding whitespace is trimmed
// before validation and storage, and the trimmed name is returned.
func TestUpsertAlias_TrimsName(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{Name: "  myproj  "},
	})
	resp, err := svc.UpsertAlias(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Alias)
	assert.Equal(t, "myproj", resp.Msg.Alias.Name)

	cfg := config.LoadConfig()
	require.Len(t, cfg.SessionDefaults.Aliases, 1)
	assert.Equal(t, "myproj", cfg.SessionDefaults.Aliases[0].Name)
}

// TestDeleteAlias_EmptyName verifies that DeleteAlias with an empty name returns
// CodeInvalidArgument.
func TestDeleteAlias_EmptyName(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.DeleteAliasRequest{Name: ""})
	_, err := svc.DeleteAlias(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestDeleteAlias_NotFound verifies that deleting a non-existent alias (with an
// empty config) returns CodeNotFound and leaves the config unchanged.
func TestDeleteAlias_NotFound(t *testing.T) {
	svc := newIsolatedDefaultsService(t)

	req := connect.NewRequest(&sessionv1.DeleteAliasRequest{Name: "nonexistent"})
	_, err := svc.DeleteAlias(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())

	cfg := config.LoadConfig()
	assert.Empty(t, cfg.SessionDefaults.Aliases)
}

// TestDeleteAlias_DeletesAlias verifies that deleting the middle alias from a
// three-alias config leaves exactly the two surrounding aliases intact.
func TestDeleteAlias_DeletesAlias(t *testing.T) {
	svc := newIsolatedDefaultsService(t)
	ctx := context.Background()

	// Pre-populate three aliases.
	for _, name := range []string{"first", "middle", "last"} {
		_, err := svc.UpsertAlias(ctx, connect.NewRequest(&sessionv1.UpsertAliasRequest{
			Alias: &sessionv1.AliasProto{Name: name},
		}))
		require.NoError(t, err)
	}

	// Delete the middle one.
	_, err := svc.DeleteAlias(ctx, connect.NewRequest(&sessionv1.DeleteAliasRequest{Name: "middle"}))
	require.NoError(t, err)

	cfg := config.LoadConfig()
	require.Len(t, cfg.SessionDefaults.Aliases, 2)
	names := []string{cfg.SessionDefaults.Aliases[0].Name, cfg.SessionDefaults.Aliases[1].Name}
	assert.Contains(t, names, "first")
	assert.Contains(t, names, "last")
	assert.NotContains(t, names, "middle")
}

// TestDeleteAlias_CaseInsensitive verifies that DeleteAlias matches by name
// case-insensitively, consistent with UpsertAlias.
func TestDeleteAlias_CaseInsensitive(t *testing.T) {
	svc := newIsolatedDefaultsService(t)
	ctx := context.Background()

	_, err := svc.UpsertAlias(ctx, connect.NewRequest(&sessionv1.UpsertAliasRequest{
		Alias: &sessionv1.AliasProto{Name: "MyAlias"},
	}))
	require.NoError(t, err)

	// Delete using lowercase variant — should succeed.
	_, err = svc.DeleteAlias(ctx, connect.NewRequest(&sessionv1.DeleteAliasRequest{Name: "myalias"}))
	require.NoError(t, err)

	cfg := config.LoadConfig()
	assert.Empty(t, cfg.SessionDefaults.Aliases)
}
