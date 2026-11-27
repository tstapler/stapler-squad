package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	serverevents "github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Boots the real server once and drives each seam between it and the workflow,
// review-queue, checkpoint and wake leaves, so dropping a wiring line fails here.
func TestWireDepsIntoServer_ConnectsWorkflowReviewCheckpointAndWakeLeaves(t *testing.T) {
	keyring.MockInit()
	envtest.NewIsolatedStateDir(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("STAPLER_SQUAD_TMUX_SOCKET", "ssq-wiring-"+uuid.NewString()[:8])
	cfg := config.LoadConfig()
	cfg.FeatureFlags = map[string]bool{"agent_wake_triggers": true, "notification_inline_actions": true}
	require.NoError(t, config.SaveConfig(cfg))

	deps, err := BuildDependencies()
	require.NoError(t, err)
	srv := newShutdownOnCleanupServer(t, "localhost:0", deps)
	ctx := context.Background()
	svc := deps.SessionService

	t.Run("workflow service", func(t *testing.T) {
		subCtx, stopSub := context.WithCancel(ctx)
		defer stopSub()
		events, subID := deps.EventBus.Subscribe(subCtx)
		require.NotEmpty(t, subID)
		created, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
			Slug: "wiring", Name: "wiring", Command: "c", TargetDirectory: t.TempDir(),
		}))
		require.NoError(t, err, "SetWorkflowService")
		id := created.Msg.Workflow.Id
		requireEvent(t, events, serverevents.EventWorkflowChanged)

		attDir, err := config.LoadConfig().WorkflowAttachmentDirOrDefault()
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(attDir, 0o750))
		outside := filepath.Join(t.TempDir(), "x.png")
		require.NoError(t, os.WriteFile(outside, []byte("x"), 0o600))
		_, err = svc.RunWorkflow(ctx, connect.NewRequest(&sessionv1.RunWorkflowRequest{Id: id, AttachmentPaths: []string{outside}}))
		require.ErrorContains(t, err, "outside the attachment directory", "scheduler and SetAttachmentDir")

		workflowID := uuid.MustParse(id)
		require.NoError(t, deps.TriggerFireEventRepo.Create(ctx, session.TriggerFireEventInput{WorkflowID: &workflowID, Outcome: "fired_failed"}))
		fired, err := svc.ListTriggerFireEvents(ctx, connect.NewRequest(&sessionv1.ListTriggerFireEventsRequest{WorkflowId: id}))
		require.NoError(t, err)
		assert.Len(t, fired.Msg.Events, 1, "SetTriggerFireEventRepo")

		_, err = svc.ArchiveWorkflowSessions(ctx, connect.NewRequest(&sessionv1.ArchiveWorkflowSessionsRequest{WorkflowId: id}))
		require.NoError(t, err, "workflow service storage")
	})

	t.Run("reactive queue and scrollback", func(t *testing.T) {
		watchCtx, cancel := context.WithCancel(ctx)
		cancel()
		require.NoError(t, svc.WatchReviewQueue(watchCtx, connect.NewRequest(&sessionv1.WatchReviewQueueRequest{}), nil), "SetReactiveQueueManager")

		inst := &instance.Instance{Title: "wiring-cp", UUID: uuid.NewString(), Path: t.TempDir(), Status: domain.Paused, Program: "claude"}
		inst.SetStartedForTest(true)
		deps.ReviewQueuePoller.AddInstance(inst)
		t.Cleanup(func() { deps.ReviewQueuePoller.RemoveInstance(inst.Title) })
		require.NoError(t, deps.ScrollbackManager.AppendOutput(inst.Title, []byte("line\n")))
		want := deps.ScrollbackManager.CurrentSequence(inst.Title)
		require.NotZero(t, want)
		cp, err := svc.CreateCheckpoint(ctx, connect.NewRequest(&sessionv1.CreateCheckpointRequest{SessionId: inst.Title, Label: "l"}))
		require.NoError(t, err)
		assert.EqualValues(t, want, cp.Msg.Checkpoint.ScrollbackSeq, "SetScrollbackManager")
	})

	t.Run("wake trigger service", func(t *testing.T) {
		ts := httptest.NewServer(srv.mux)
		t.Cleanup(ts.Close)
		wake := sessionv1connect.NewWakeTriggerServiceClient(http.DefaultClient, ts.URL+"/api")
		owner := &instance.Instance{Title: "wiring-owner", UUID: uuid.NewString(), Path: t.TempDir(), Status: domain.Paused, Program: "claude"}
		require.NoError(t, deps.Storage.AddInstance(owner))

		reminded, err := wake.CreateOperatorReminder(ctx, connect.NewRequest(&sessionv1.CreateOperatorReminderRequest{
			SessionUuid: owner.UUID, FireAt: timestamppb.New(time.Now().Add(time.Hour)), Label: "look",
		}))
		require.NoError(t, err, "handler registration")
		require.Equal(t, sessionv1.ReminderOutcome_REMINDER_OUTCOME_CREATED, reminded.Msg.Outcome, "SetBackend")

		action, err := domain.NewWakeAction(domain.WakeActionSteerText, "late")
		require.NoError(t, err)
		past := time.Now().Add(-time.Hour)
		_, _, err = deps.Storage.CreateWakeTrigger(ctx, domain.CreateWakeTriggerInput{
			OwnerSessionUUID: "overdue-owner", Spec: domain.AtSpec{At: past}, Action: action, NextFireAt: &past, ExpiresAt: &past,
		})
		require.NoError(t, err)

		listed, err := wake.ListWakeTriggers(ctx, connect.NewRequest(&sessionv1.ListWakeTriggersRequest{}))
		require.NoError(t, err)
		assert.True(t, listed.Msg.Enabled, "Enabled")
		byOwner := map[string]*sessionv1.WakeTriggerInfo{}
		for _, tr := range listed.Msg.Triggers {
			byOwner[tr.OwnerSessionUuid] = tr
		}
		require.Contains(t, byOwner, owner.UUID, "Store")
		assert.True(t, byOwner[owner.UUID].OwnerExists, "Owners")
		assert.Equal(t, "wiring-owner", byOwner[owner.UUID].OwnerTitle, "Titles")
		require.Contains(t, byOwner, "overdue-owner")
		assert.Equal(t, "missed", byOwner["overdue-owner"].Status, "ExpireOverdue")

		invoke := func(kind sessionv1.ActionKind, key sessionv1.ActionTargetKey, target string) *sessionv1.InvokeNotificationActionResponse {
			res, err := svc.InvokeNotificationAction(ctx, connect.NewRequest(&sessionv1.InvokeNotificationActionRequest{
				ActionKind: kind, TargetKeyType: key, TargetKey: target, IdempotencyKey: uuid.NewString(),
			}))
			require.NoError(t, err)
			return res.Msg
		}
		cancelled := invoke(sessionv1.ActionKind_ACTION_KIND_CANCEL_WAKE, sessionv1.ActionTargetKey_ACTION_TARGET_KEY_WAKE_TRIGGER_ID, reminded.Msg.Trigger.Id)
		assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, cancelled.Outcome, "Cancels: %v", cancelled)
		peer := invoke(sessionv1.ActionKind_ACTION_KIND_CANCEL_PEER_HELP, sessionv1.ActionTargetKey_ACTION_TARGET_KEY_GUIDANCE_REQUEST_ID, uuid.NewString())
		assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, peer.Outcome, "Guidance: %v", peer)
		peers, err := wake.ListPeerHelpRequests(ctx, connect.NewRequest(&sessionv1.ListPeerHelpRequestsRequest{}))
		require.NoError(t, err)
		assert.Empty(t, peers.Msg.Requests)
	})
}

func requireEvent(t *testing.T, events <-chan *serverevents.Event, want serverevents.EventType) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Type == want {
				return
			}
		case <-deadline:
			t.Fatalf("no %s event published", want)
		}
	}
}
