package services

// Observability of every stop: the stuck row's context, the single log line and
// the notification body (Story 3.1.4).

import (
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// recordsFor keeps only the captured log lines for one session title. captureLogs
// serializes capturers, not emitters, so other tests' lines can land in the buffer.
func recordsFor(buf *syncLogBuffer, title string) []string {
	var out []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "session="+title+" ") || strings.HasSuffix(line, "session="+title) {
			out = append(out, line)
		}
	}
	return out
}

func stoppedLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.Contains(l, "[AutonomousDriver] stopped") {
			out = append(out, l)
		}
	}
	return out
}

var idleSnapshot = domain.StopSnapshot{
	CapturedAt:       time.Date(2026, 10, 13, 12, 3, 40, 0, time.UTC),
	LastActivityAt:   time.Date(2026, 10, 13, 12, 3, 10, 0, time.UTC),
	Bucket:           domain.BucketSafeIdle,
	TmuxAlive:        true,
	ControllerActive: true,
}

func stallWith(reason domain.StopReason, turns int, tail string) session.AutonomousDriverOutcome {
	snap := idleSnapshot
	snap.PaneTail = tail
	return session.AutonomousDriverOutcome{StopReason: reason, Turns: turns, Snapshot: snap, RunID: 7}
}

func notificationMessages(evs []*events.Event) []string {
	var out []string
	for _, ev := range evs {
		if ev.Type == events.EventNotification {
			out = append(out, ev.NotificationMessage)
		}
	}
	return out
}

// OBS-1: plain summary first, key=value part after, secret never stored.
func TestOnAutonomousDriverComplete_StopDetailRedacted(t *testing.T) {
	f := newCharFixture(t, "stop-detail-redacted", session.SessionRoleWork, session.BacklogStatusInProgress)
	f.svc.SetAutonomousStuckRespawner(newSignalingRespawner())

	f.complete(stallWith(domain.StopReasonOrchestratorError, 2, "building...\npassword=hunter2\n"))

	ctx := f.openStuckRow(t).Context
	assert.True(t, strings.HasPrefix(ctx, "The autonomous run hit an internal error and stopped. The session was waiting at its prompt. "), ctx)
	assert.Contains(t, ctx, " | stop_reason=orchestrator_error state=idle_prompt action=respawned turns=2 ")
	assert.Contains(t, ctx, "last_activity=2026-10-13T12:03:10Z")
	assert.NotContains(t, ctx, "hunter2")
	assert.Contains(t, ctx, "tail=[tail withheld: secret pattern detected]")
}

// OBS-2: exactly one stopped line per stop, no tail, level by class.
func TestOnAutonomousDriverComplete_LogsExactlyOneStoppedLine(t *testing.T) {
	cases := []struct {
		name    string
		outcome session.AutonomousDriverOutcome
		level   string
	}{
		{"stall", stallWith(domain.StopReasonOrchestratorError, 2, "TAILMARKER-xyz"), "level=WARN"},
		{"done", session.AutonomousDriverOutcome{StopReason: domain.StopReasonDone, Turns: 3, RunID: 7}, "level=INFO"},
		{"cancelled", session.AutonomousDriverOutcome{StopReason: domain.StopReasonCancelledExternal, RunID: 7}, "level=INFO"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title := "stall-" + strings.ReplaceAll(t.Name(), "/", "-")
			f := newCharFixture(t, title, session.SessionRoleWork, session.BacklogStatusInProgress)
			f.svc.SetAutonomousStuckRespawner(newSignalingRespawner())
			buf := captureLogs(t)

			f.complete(tc.outcome)

			lines := stoppedLines(recordsFor(buf, title))
			require.Len(t, lines, 1)
			line := lines[0]
			assert.Contains(t, line, tc.level)
			for _, key := range []string{"item=", "role=work", "run_id=7", "stop_reason=", "state=", "turns=", "max_turns=",
				"last_activity_age=", "action=", "tail_bytes=", "resupervise=0/3", "nudge_outcome=n/a", "resupervise_cap_hit=false"} {
				assert.Contains(t, line, key)
			}
			assert.NotContains(t, buf.String(), "TAILMARKER-xyz", "the tail is never logged")
		})
	}
}

// OBS-5: no "stopped after N turns" wording; golden body for a startup timeout.
func TestNotification_StartupTimeoutNeverSaysStoppedAfterZeroTurns(t *testing.T) {
	f := newUnlinkedCharFixture(t, "stop-notify-startup")

	f.complete(stallWith(domain.StopReasonStartupTimeout, 0, ""))

	msgs := notificationMessages(f.drain())
	require.Len(t, msgs, 1)
	assert.NotContains(t, msgs[0], "stopped after")
	assert.Equal(t, "Session 'stop-notify-startup': The autonomous run gave up waiting for the session to become ready. "+
		"The session was waiting at its prompt. Open the session to review what was accomplished and give the next instruction.", msgs[0])
}

// OBS-6: a canary secret in the tail is absent from row, notification and logs.
func TestCanary_AbsentFromRowNotificationAndLogs(t *testing.T) {
	title := "stall-canary"
	f := newCharFixture(t, title, session.SessionRoleWork, session.BacklogStatusInProgress)
	f.svc.SetAutonomousStuckRespawner(newSignalingRespawner())
	buf := captureLogs(t)

	f.complete(stallWith(domain.StopReasonOrchestratorError, 2, "cloning with "+canaryGHP))

	assert.NotContains(t, f.openStuckRow(t).Context, canaryGHP)
	for _, m := range notificationMessages(f.drain()) {
		assert.NotContains(t, m, canaryGHP)
	}
	assert.NotContains(t, strings.Join(recordsFor(buf, title), "\n"), canaryGHP)
	assert.NotContains(t, buf.String(), canaryGHP)
}

// OBS-7: the operator read path returns the detail unchanged.
func TestStuckRowContext_ReturnedByListStuckBacklogItems(t *testing.T) {
	f := newCharFixture(t, "stop-detail-rpc", session.SessionRoleWork, session.BacklogStatusInProgress)
	svc := NewBacklogService(f.storage, nil, nil, nil, nil, nil)
	ctx := t.Context()
	detail := detailFor(idleSnapshot, 2, domain.StopReasonOrchestratorError)

	applied, err := f.storage.MarkStuck(ctx, f.itemID, domain.StuckReasonAutonomousStuck, session.BacklogStatusInProgress, detail)
	require.NoError(t, err)
	require.True(t, applied)

	resp, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Items, 1)
	assert.Equal(t, detail, resp.Msg.Items[0].Context)
}
