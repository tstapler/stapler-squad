package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// TestCreateSession_should_SetInstanceExtraArgs_When_RequestIncludesExtraArgs verifies that
// extra_args on the request (as sent by a preset-prefilled Omnibar submission) reaches the
// live instance's ExtraArgs field, mirroring the existing EnvVars threading test.
func TestCreateSession_should_SetInstanceExtraArgs_When_RequestIncludesExtraArgs(t *testing.T) {
	storage := createTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:     "extra-args-test-session",
		Path:      t.TempDir(),
		Program:   "ssh",
		ExtraArgs: []string{"-t", "host", "true"},
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst, "instance should be in the live poller immediately after CreateSession")
	require.Equal(t, []string{"-t", "host", "true"}, inst.ExtraArgs)
}

// TestCreateSession_should_LeaveInstanceExtraArgsEmpty_When_RequestOmitsExtraArgs is the
// backward-compatibility guard: an old client (or any non-preset flow) that never sets
// extra_args must produce zero behavior change.
func TestCreateSession_should_LeaveInstanceExtraArgsEmpty_When_RequestOmitsExtraArgs(t *testing.T) {
	storage := createTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   "no-extra-args-test-session",
		Path:    t.TempDir(),
		Program: "claude",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	require.Empty(t, inst.ExtraArgs)
}

// TestCreateSession_should_ComposeProfileCLIFlagsBeforePresetExtraArgs_When_BothPresent is the
// architecture-review.md CONCERNS remediation: a profile's resolved cli_flags and a preset's
// extra_args are two independently-correct code paths that visibly compose into a single launch
// command (buildLaunchCommand's CLIFlags loop runs before the ExtraArgs loop). This proves the
// handler sets both fields on the created Instance without one clobbering the other, so their
// composition at launch time (profile flags first, preset args last) is the intended, tested
// behavior rather than an untested emergent property.
func TestCreateSession_should_ComposeProfileCLIFlagsBeforePresetExtraArgs_When_BothPresent(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.SessionDefaults.Profiles = map[string]config.ProfileDefaults{
		"my-profile": {Name: "my-profile", CLIFlags: "--verbose"},
	}
	require.NoError(t, config.SaveConfig(cfg))

	storage := createTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:     "profile-plus-preset-test-session",
		Path:      t.TempDir(),
		Program:   "ssh",
		Profile:   "my-profile",
		ExtraArgs: []string{"-t", "host", "true"},
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	require.Equal(t, "--verbose", inst.CLIFlags, "profile-resolved cli_flags must still reach the instance")
	require.Equal(t, []string{"-t", "host", "true"}, inst.ExtraArgs, "preset extra_args must still reach the instance")
}
