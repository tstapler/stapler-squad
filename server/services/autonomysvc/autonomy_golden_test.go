package autonomysvc

// Differential golden for the autonomous orchestration service: stop handling,
// stall recovery, stale-work inspection and dead-pane recording over a scenario
// matrix. Recorded from origin/main before the move; the body is unchanged by it.
// Regenerate with -update-autonomy-golden.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/autonomous"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

var updateAutonomyGolden = flag.Bool("update-autonomy-golden", false, "rewrite testdata/autonomy.golden")

type autonomyRespawner struct {
	called chan string
	opDone chan struct{} // closed once the op under test has returned
	err    error
}

func (r *autonomyRespawner) AutoRespawnAutonomousWork(_ context.Context, itemID string) error {
	r.called <- itemID
	if r.err == nil {
		return nil
	}
	// Production leaves the failure notice (this goroutine) unordered against the
	// stop notice (the caller's); holding the error fixes the recorded order.
	// Bounded so that a synchronous dispatch fails the golden instead of hanging.
	select {
	case <-r.opDone:
	case <-time.After(10 * time.Second):
	}
	return r.err
}

type autonomyNudger struct {
	err   error
	pre   func()
	calls int
}

func (n *autonomyNudger) NudgeStalledSession(_ context.Context, _ *instance.Instance, claim func() (bool, error)) error {
	n.calls++
	if n.pre != nil {
		n.pre()
	}
	if n.err != nil {
		return n.err
	}
	ok, err := claim()
	if err != nil {
		return err
	}
	if !ok {
		return autonomous.ErrNudgeSlotUnavailable
	}
	return nil
}

type autonomyScenario struct {
	name       string
	role       string // "" = unlinked
	itemStatus domain.BacklogStatus
	status     domain.Status
	archived   bool
	hidden     bool
	op         string // "complete" (default), "stale", "deadpane"
	outcome    autonomous.AutonomousDriverOutcome
	snapshot   domain.StopSnapshot
	diagnose   bool
	stall      bool
	nudgeErr   error
	restartErr error
	respawnErr error
	noRespawn  bool // no respawner wired
	noFinder   bool
	noStorage  bool
	stopDuring bool // StopDriverForSession while the nudge is in flight
	readoption bool
	autonomy   bool
	twice      bool // a second stall inside the nudge cooldown
}

func autonomySnapshot(state domain.ObservedState, tail string) domain.StopSnapshot {
	s := svctest.IdleSnapshot
	s.PaneTail = tail
	switch state {
	case domain.ObservedIdlePrompt:
	case domain.ObservedWorking:
		s.Bucket = domain.BucketActive
	case domain.ObservedNeedsApproval:
		s.Bucket = domain.BucketNeedsInput
	case domain.ObservedError:
		s.Bucket = domain.BucketError
	case domain.ObservedIdle:
		s.Bucket = domain.BucketIdle
	case domain.ObservedExited:
		s.PaneDead = true
	case domain.ObservedCrashed:
		s.PaneDead, s.ExitCode = true, 2
	case domain.ObservedUnknown:
		s.Bucket = domain.BucketUnknown
	}
	return s
}

func autonomyWork(name string) autonomyScenario {
	return autonomyScenario{
		name: name, role: instance.SessionRoleWork, itemStatus: domain.BacklogStatusInProgress, status: domain.Active, autonomy: true,
		outcome: autonomous.AutonomousDriverOutcome{Turns: 20, Stuck: true, StopReason: domain.StopReasonMaxTurns, Reason: "no DONE signal"},
	}
}

var autonomyStates = []domain.ObservedState{
	domain.ObservedIdlePrompt, domain.ObservedNeedsApproval, domain.ObservedIdle, domain.ObservedWorking,
	domain.ObservedError, domain.ObservedExited, domain.ObservedCrashed, domain.ObservedUnknown,
}

func autonomyMatrixScenarios() []autonomyScenario {
	var out []autonomyScenario
	roles := []string{"", instance.SessionRoleTriage, instance.SessionRoleWork, instance.SessionRoleReview, "mystery"}
	statuses := []domain.BacklogStatus{domain.BacklogStatusIdea, domain.BacklogStatusReady, domain.BacklogStatusInProgress, domain.BacklogStatusReview}
	outcomes := []autonomous.AutonomousDriverOutcome{
		{Done: true, Turns: 3, Reason: "all good", PRUrl: "https://example.invalid/pr/1", StopReason: domain.StopReasonDone},
		{Turns: 20, Stuck: true, StopReason: domain.StopReasonMaxTurns},
		{Turns: 2, StopReason: domain.StopReasonCancelledExternal},
		{Turns: 2, StopReason: domain.StopReasonCancelledShutdown},
	}
	for _, role := range roles {
		for _, st := range statuses {
			if role == "" && st != domain.BacklogStatusInProgress {
				continue
			}
			for _, o := range outcomes {
				s := autonomyWork(fmt.Sprintf("matrix/%s/%s/%s", role, st, o.StopReason))
				s.role, s.itemStatus, s.outcome = role, st, o
				s.snapshot = autonomySnapshot(domain.ObservedWorking, "working on it")
				out = append(out, s)
			}
		}
	}
	for _, reason := range append(slices.Clone(domain.AllStopReasons), "") {
		for _, state := range autonomyStates {
			s := autonomyWork(fmt.Sprintf("reason/%s/%s", reason, state))
			s.outcome.StopReason = reason
			s.snapshot = autonomySnapshot(state, "line one\nline two")
			s.diagnose, s.stall = true, true
			out = append(out, s)
		}
	}
	return out
}

func autonomyNudgeScenarios(op string) []autonomyScenario {
	out := make([]autonomyScenario, 0, 64)
	nudgeErrs := []struct {
		name string
		err  error
	}{
		{"nil", nil}, {"uncertain", domain.ErrNudgeSubmitUncertain}, {"canceled", context.Canceled},
		{"deadline", context.DeadlineExceeded}, {"notactive", autonomous.ErrNudgeSessionNotActive},
		{"claimfailed", autonomous.ErrNudgeClaimFailed}, {"notidle", autonomous.ErrNudgeTargetNotIdle},
		{"leasebusy", instance.ErrLeaseBusy}, {"slot", autonomous.ErrNudgeSlotUnavailable},
		{"other", errors.New("pane belongs to someone else")},
	}
	base := func(name string) autonomyScenario {
		s := autonomyWork(op + "/" + name)
		s.op, s.diagnose, s.stall = op, true, true
		s.outcome.StopReason = domain.StopReasonOrchestratorError // max_turns never nudges (ADR-001 row 4)
		s.snapshot = autonomySnapshot(domain.ObservedIdlePrompt, "> ")
		return s
	}
	for _, e := range nudgeErrs {
		s := base("nudgeerr/" + e.name)
		s.nudgeErr = e.err
		out = append(out, s)
	}
	for _, d := range []bool{false, true} {
		for _, st := range []bool{false, true} {
			s := base(fmt.Sprintf("flags/%v-%v", d, st))
			s.diagnose, s.stall = d, st
			out = append(out, s)
		}
	}
	for _, state := range autonomyStates {
		s := base("state/" + string(state))
		s.snapshot = autonomySnapshot(state, "tail")
		out = append(out, s)
	}
	variants := map[string]func(*autonomyScenario){
		"archived":    func(s *autonomyScenario) { s.archived = true },
		"paused":      func(s *autonomyScenario) { s.status = domain.Paused },
		"restartfail": func(s *autonomyScenario) { s.restartErr = errors.New("no driver") },
		"norespawner": func(s *autonomyScenario) {
			s.noRespawn, s.snapshot = true, autonomySnapshot(domain.ObservedWorking, "busy")
		},
		"respawnerr": func(s *autonomyScenario) {
			s.respawnErr, s.snapshot = errors.New("spawn refused"), autonomySnapshot(domain.ObservedWorking, "busy")
		},
		"stopduring":  func(s *autonomyScenario) { s.stopDuring = true },
		"autonomyoff": func(s *autonomyScenario) { s.autonomy = false },
		"hidden":      func(s *autonomyScenario) { s.hidden = true },
		"movedon":     func(s *autonomyScenario) { s.itemStatus = domain.BacklogStatusReview },
		"nofinder":    func(s *autonomyScenario) { s.noFinder = true },
		"nostorage":   func(s *autonomyScenario) { s.noStorage = true },
		"twice":       func(s *autonomyScenario) { s.twice = true },
		"secrettail": func(s *autonomyScenario) {
			s.snapshot = autonomySnapshot(domain.ObservedIdlePrompt, "export API_TOKEN=abcdef0123456789abcdef")
		},
	}
	names := make([]string, 0, len(variants))
	for n := range variants {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		s := base("variant/" + n)
		variants[n](&s)
		out = append(out, s)
	}
	return out
}

func autonomyScenarios() []autonomyScenario {
	out := autonomyMatrixScenarios()
	out = append(out, autonomyNudgeScenarios("complete")...)
	out = append(out, autonomyNudgeScenarios("stale")...)
	for _, readoption := range []bool{false, true} {
		s := autonomyWork(fmt.Sprintf("readoption/%v", readoption))
		s.readoption, s.outcome.StopReason = readoption, domain.StopReasonCancelledShutdown
		out = append(out, s)
	}
	for _, role := range []string{"", instance.SessionRoleWork, instance.SessionRoleReview} {
		for _, state := range autonomyStates {
			s := autonomyWork(fmt.Sprintf("deadpane/%s/%s", role, state))
			s.op, s.role = "deadpane", role
			s.snapshot = autonomySnapshot(state, "last words\nauthorization: bearer abcdefgh12345678\nhttps://u:p@example.invalid/x")
			out = append(out, s)
		}
	}
	return out
}

var autonomyUUIDRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func runAutonomyScenario(t *testing.T, sc autonomyScenario, idx int) string {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := events.NewEventBus(100)
	svc := NewAutonomousOrchestrationService(nil, bus)
	svc.SetLifecycleContext(ctx)
	if !sc.noStorage {
		svc.SetStorageGetter(func() *session.Storage { return storage })
	}

	title := fmt.Sprintf("golden-%d", idx)
	inst := &instance.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test", Status: sc.status, Program: "claude",
		AutonomousMode: sc.autonomy, Hidden: sc.hidden, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if sc.archived {
		at := time.Now()
		inst.ArchivedAt = &at
	}
	require.NoError(t, storage.AddInstance(inst))
	if !sc.noFinder {
		svc.SetInstanceFinder(func(string) *instance.Instance { return inst })
	}

	var itemID string
	if sc.role != "" {
		item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: title + " item", Status: string(sc.itemStatus)})
		require.NoError(t, err)
		itemID = item.ID
		_, err = storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: itemID, SessionUUID: inst.UUID, SessionRole: sc.role})
		require.NoError(t, err)
	}

	respawner := &autonomyRespawner{called: make(chan string, 4), opDone: make(chan struct{}), err: sc.respawnErr}
	releaseRespawner := sync.OnceFunc(func() { close(respawner.opDone) })
	defer releaseRespawner()
	if !sc.noRespawn {
		svc.SetAutonomousStuckRespawner(respawner)
	}
	nudger := &autonomyNudger{err: sc.nudgeErr}
	if sc.stopDuring {
		nudger.pre = func() { svc.StopDriverForSession(title) }
	}
	restarts := 0
	svc.SetStallNudger(nudger)
	svc.SetNudgeFlagReaders(func() bool { return sc.diagnose }, func() bool { return sc.stall })
	svc.SetDriverRestarter(func(*instance.Instance, time.Duration) error {
		restarts++
		return sc.restartErr
	})
	svc.SetReadoptionEnabled(func() bool { return sc.readoption })
	svc.staleSnapshot = func(*instance.Instance) domain.StopSnapshot { return sc.snapshot }

	ch, _ := bus.Subscribe(ctx)
	var out strings.Builder
	fmt.Fprintf(&out, "=== %s\n", sc.name)

	act := func() {
		switch sc.op {
		case "stale":
			item, err := storage.GetBacklogItem(ctx, itemID)
			require.NoError(t, err)
			fmt.Fprintf(&out, "disposition=%d\n", svc.InspectStaleWork(ctx, inst.UUID, item))
		case "deadpane":
			svc.RecordDeadPane(inst.UUID, title, sc.snapshot, domain.ClassifyObserved(sc.snapshot))
		default:
			o := sc.outcome
			o.Snapshot = sc.snapshot
			svc.OnAutonomousDriverComplete(title, o)
		}
	}
	act()
	if sc.twice {
		act()
	}
	releaseRespawner()

	if itemID != "" {
		sessions, err := storage.ListItemSessions(ctx, itemID)
		require.NoError(t, err)
		ended := len(sessions) == 1 && sessions[0].EndedAt != nil
		// Only the respawn path ends a work ItemSession here, and it dispatches async.
		if ended && sc.role == instance.SessionRoleWork && sc.op != "stale" && !sc.noRespawn && !sc.twice {
			select {
			case <-respawner.called:
				out.WriteString("respawned\n")
				if sc.respawnErr != nil {
					require.Eventually(t, func() bool { return len(ch) >= 3 }, 10*time.Second, 5*time.Millisecond)
				}
			case <-time.After(10 * time.Second):
				out.WriteString("respawn missing\n")
			}
		}
		item, err := storage.GetBacklogItem(ctx, itemID)
		require.NoError(t, err)
		fmt.Fprintf(&out, "item status=%s ended=%v\n", item.Status, ended)
	}
	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	for _, r := range rows {
		fmt.Fprintf(&out, "row reason=%s notified=%v attempts=%d nudges=%d context=%q\n",
			r.Reason, r.NotifiedAt != nil, r.RemediationAttempts, r.DiagnoseNudgeCount, r.Context)
	}
	snap := inst.Snapshot()
	fmt.Fprintf(&out, "instance mode=%v outcome=%q stop=%q nudges=%d restarts=%d\n",
		snap.Autonomous.AutonomousMode, snap.Autonomous.AutonomousOutcome, snap.Autonomous.AutonomousStopReason, nudger.calls, restarts)
	for done := false; !done; {
		select {
		case ev := <-ch:
			keys := make([]string, 0, len(ev.NotificationMetadata))
			for k := range ev.NotificationMetadata {
				keys = append(keys, k+"="+ev.NotificationMetadata[k])
			}
			slices.Sort(keys)
			fmt.Fprintf(&out, "event %s session=%q fields=%v id=%q type=%d prio=%d title=%q msg=%q meta=%v\n",
				ev.Type, ev.SessionID, ev.UpdatedFields, ev.NotificationID, ev.NotificationType, ev.NotificationPriority,
				ev.NotificationTitle, ev.NotificationMessage, keys)
		default:
			done = true
		}
	}
	text := out.String()
	if itemID != "" {
		text = strings.ReplaceAll(text, itemID, "<item>")
	}
	return autonomyUUIDRe.ReplaceAllString(text, "<id>")
}

func TestAutonomyGolden(t *testing.T) {
	var out strings.Builder
	for i, sc := range autonomyScenarios() {
		out.WriteString(runAutonomyScenario(t, sc, i))
	}
	const path = "testdata/autonomy.golden"
	if *updateAutonomyGolden {
		require.NoError(t, os.WriteFile(path, []byte(out.String()), 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), out.String())
}
