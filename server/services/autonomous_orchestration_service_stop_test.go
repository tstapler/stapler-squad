package services

// Observability of every stop: the stuck row's context, the single log line and
// the notification body (Story 3.1.4).

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// recordsFor keeps only the captured log lines for one session title. captureLogs
// serializes capturers, not emitters, so other tests' lines can land in the buffer.
func recordsFor(buf *svctest.SyncLogBuffer, title string) []string {
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

func stallWith(reason domain.StopReason, turns int, tail string) session.AutonomousDriverOutcome {
	snap := svctest.IdleSnapshot
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
	f := newCharFixture(t, "stop-detail-redacted", session.SessionRoleWork, domain.BacklogStatusInProgress)
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
		nudge   string
	}{
		// An idle-prompt stall with the nudge flags off records that the nudge was disabled.
		{"stall", stallWith(domain.StopReasonOrchestratorError, 2, "TAILMARKER-xyz"), "level=WARN", "nudge_outcome=disabled"},
		{"done", session.AutonomousDriverOutcome{StopReason: domain.StopReasonDone, Turns: 3, RunID: 7}, "level=INFO", "nudge_outcome=n/a"},
		{"cancelled", session.AutonomousDriverOutcome{StopReason: domain.StopReasonCancelledExternal, RunID: 7}, "level=INFO", "nudge_outcome=n/a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title := "stall-" + strings.ReplaceAll(t.Name(), "/", "-")
			f := newCharFixture(t, title, session.SessionRoleWork, domain.BacklogStatusInProgress)
			f.svc.SetAutonomousStuckRespawner(newSignalingRespawner())
			buf := svctest.CaptureLogs(t)

			f.complete(tc.outcome)

			lines := stoppedLines(recordsFor(buf, title))
			require.Len(t, lines, 1)
			line := lines[0]
			assert.Contains(t, line, tc.level)
			for _, key := range []string{"item=", "role=work", "run_id=7", "stop_reason=", "state=", "turns=", "max_turns=",
				"last_activity_age=", "action=", "tail_bytes=", "resupervise=0/3", "resupervise_cap_hit=false"} {
				assert.Contains(t, line, key)
			}
			assert.Contains(t, line, tc.nudge)
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
	f := newCharFixture(t, title, session.SessionRoleWork, domain.BacklogStatusInProgress)
	f.svc.SetAutonomousStuckRespawner(newSignalingRespawner())
	buf := svctest.CaptureLogs(t)

	f.complete(stallWith(domain.StopReasonOrchestratorError, 2, "cloning with "+canaryGHP))

	assert.NotContains(t, f.openStuckRow(t).Context, canaryGHP)
	for _, m := range notificationMessages(f.drain()) {
		assert.NotContains(t, m, canaryGHP)
	}
	assert.NotContains(t, strings.Join(recordsFor(buf, title), "\n"), canaryGHP)
	assert.NotContains(t, buf.String(), canaryGHP)
}
