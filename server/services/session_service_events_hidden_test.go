package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// T-CR-05: PermanentlyFailed already has a bus producer (EventBusNotifier.Notify
// with type ERROR and item_id = the session UUID). With the gate on it is
// delivered exactly once, and the resolver still classifies the session as
// hidden by UUID although the item_id stamp is present.
func TestPermanentlyFailed_ShouldDeliverOneErrorAndResolveHidden_WhenItemIDEqualsUUID(t *testing.T) {
	e := newCrashEnv(t, true)
	gate := e.svc.DeliveryGate()
	gate.Index().Replace([]deliverygate.Entry{
		{UUID: "uuid-hidden-1", Title: "review:abc", TmuxName: "ssq_review_abc", Hidden: true, Kind: deliverygate.KindReview},
	})

	n := &EventBusNotifier{Bus: e.bus}
	n.Notify("uuid-hidden-1", "Session gave up after repeated failures", "failed 3 attempts",
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR), true, true)
	// A routine event from the same hidden session is dropped, proving the
	// session was resolved as hidden (not delivered through fail-open).
	n.Notify("uuid-hidden-1", "done", "finished",
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), false, false)

	got := e.collect(t)
	errs := notificationsOf(got, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR))
	require.Len(t, errs, 1)
	assert.Empty(t, notificationsOf(got, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE)))
	assert.EqualValues(t, 1, gate.Metrics().Value(deliverygate.CounterHiddenDelivered, "bus", "failure", "review"))
	assert.EqualValues(t, 0, gate.Metrics().Total(deliverygate.CounterUnresolved))
}

// A hidden crash is delivered through the gate (failure class); a hidden
// Stopped exit publishes no notification at all.
func TestHiddenCrash_ShouldDeliverFailureThroughTheGate_WhenGateOn(t *testing.T) {
	e := newCrashEnv(t, true)
	gate := e.svc.DeliveryGate()
	gate.Index().Replace([]deliverygate.Entry{
		{UUID: "uuid-hidden-2", Title: "diag:xyz", TmuxName: "ssq_diag_xyz", Hidden: true, Kind: deliverygate.KindDiagnose},
	})

	e.svc.publishCrash(crashedSnap("uuid-hidden-2", "diag:xyz", "pane exited 137", testNow))
	got := notificationsOf(e.collect(t), failureType)
	require.Len(t, got, 1)
	assert.EqualValues(t, 1, gate.Metrics().Value(deliverygate.CounterHiddenDelivered, "bus", "failure", "diagnose"))
}

// T-MX-13: constructing the gated service performs no lookups (zero index
// misses and zero unresolved deliveries before LoadInstances), and once the
// startup seed has run, restore-time producers for a hidden session resolve
// against the index instead of missing it.
func TestStartupWindow_ShouldCountZeroIndexMissesBeforeLoadInstances_WhenNoSessionProducersRun(t *testing.T) {
	e := newCrashEnv(t, true)
	gate := e.svc.DeliveryGate()
	require.NotNil(t, gate)
	assert.EqualValues(t, 0, gate.Metrics().Value(deliverygate.CounterIndexMiss), "no lookups before any producer runs")
	assert.EqualValues(t, 0, gate.Metrics().Total(deliverygate.CounterUnresolved))

	hidden := &session.Instance{Title: "restored-review", UUID: "restored-review-uuid", Hidden: true, Tags: []string{"backlog:review"}}
	gate.SeedFromInstances([]*session.Instance{hidden}) // what BuildRuntimeDeps does after LoadInstances

	e.svc.onColdRestoreLostHistory(hidden)
	e.svc.onRateLimitDetected(hidden, hidden.UUID, time.Time{})
	e.bus.Publish(events.NewNotificationEvent(hidden.UUID, hidden.Title, "restore-routine",
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), 2, "t", "m", nil))
	e.collect(t)

	assert.EqualValues(t, 0, gate.Metrics().Value(deliverygate.CounterIndexMiss), "seeded index resolves every restore-time key")
	assert.EqualValues(t, 0, gate.Metrics().Total(deliverygate.CounterUnresolved))
}

// T-MX-08: every producer that can emit a FAILURE for a hidden session is
// enumerated here with the test that proves its delivery through the gate. A
// named test that no longer exists fails the guard.
func TestHiddenFailureProducers_ShouldEachHaveTest_WhenTableEnumerated(t *testing.T) {
	producers := []struct {
		name string
		test string // proves delivery
	}{
		{name: "hook Stop task_failed (main agent)",
			test: "TestHookEvents_ShouldYieldZeroRowsForHiddenPostToolErrorAndOneFailureForMainStop_WhenGateOn"},
		{name: "autonomous driver 'Autonomous fix stuck'",
			test: "TestAutonomous_ShouldDeliverStuckFailureAndDropCompleteInfo_WhenHiddenAndGateOn"},
		{name: "rate-limit recovery failure",
			test: "TestHiddenSessionEvents_ShouldDeliverOnlyRecoveryFailure_WhenGateOn"},
		{name: "capacity guardrail hard stop",
			test: "TestCapacityGuardrailStop_ShouldDeliverForHiddenSession_WhenGateOn"},
		{name: "PermanentlyFailed ERROR",
			test: "TestPermanentlyFailed_ShouldDeliverOneErrorAndResolveHidden_WhenItemIDEqualsUUID"},
		{name: "session crash FAILURE",
			test: "TestHiddenCrash_ShouldDeliverFailureThroughTheGate_WhenGateOn"},
	}

	have := map[string]bool{}
	fset := token.NewFileSet()
	require.NoError(t, filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				have[fn.Name.Name] = true
			}
		}
		return nil
	}))

	for _, p := range producers {
		assert.True(t, have[p.test], "%s: test %q not found in server/services", p.name, p.test)
	}
}
