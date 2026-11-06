package push

import (
	"context"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

type gateFunc func(string) bool

func (f gateFunc) AllowStatusChange(id string) bool { return f(id) }

func stoppedEvent(title string, hidden bool) *events.Event {
	return &events.Event{
		Type:          events.EventSessionUpdated,
		Session:       &session.Instance{ID: "id-" + title, Title: title, Status: session.Stopped, Hidden: hidden},
		UpdatedFields: []string{events.FieldStatus},
	}
}

// T-PS-01: a hidden session is dropped with the gate on (the default), visible
// still pushes, and gate off (the rollback) pushes both with the shadow counter
// recording the would-be drop. The event's Instance is Hidden and indexed as
// hidden: the gate is the only thing between it and the push.
func TestStatusChangePush_ShouldDeliverZeroHiddenOneVisible_WhenGateOnAndOneWhenOff(t *testing.T) {
	t.Parallel()
	run := func(flagOn bool) (hidden, visible int, g *deliverygate.Gate) {
		g = deliverygate.NewGate(
			deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
				return deliverygate.FlagSettings{Global: flagOn}, nil
			}),
		)
		g.Flags().Reload()
		g.Index().Replace([]deliverygate.Entry{
			{Title: "hidden-sess", Hidden: true, Kind: deliverygate.KindReview},
			{Title: "visible-sess"},
		})
		n := &mockNotifier{name: "rec"}
		ctx := context.Background()
		dedup := newDedupTracker(0)
		deliverEvent(ctx, stoppedEvent("hidden-sess", true), dedup, []Notifier{n}, g)
		hidden = n.CallCount()
		deliverEvent(ctx, stoppedEvent("visible-sess", false), dedup, []Notifier{n}, g)
		visible = n.CallCount() - hidden
		return hidden, visible, g
	}

	h, v, _ := run(true)
	assert.Equal(t, 0, h, "gate on: hidden Stopped push must be dropped")
	assert.Equal(t, 1, v, "gate on: visible Stopped push must pass")

	h, v, g := run(false)
	assert.Equal(t, 1, h, "gate off: hidden push delivers (shadow)")
	assert.Equal(t, 1, v)
	assert.EqualValues(t, 1, g.Metrics().Total(deliverygate.CounterWouldSuppress), "shadow suppression must be counted")
}

// A hidden instance the index does not know (the index/instance disagreement a
// stale seed would cause) fails open: it pushes, and the gate counts it as
// unresolved so the miss is visible. This is the residual risk of removing the
// push builder's own Instance.Hidden check.
func TestStatusChangePush_ShouldFailOpenAndCount_WhenHiddenInstanceMissingFromIndex(t *testing.T) {
	t.Parallel()
	g := deliverygate.NewGate(deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
		return deliverygate.FlagSettings{Global: true}, nil
	}))
	g.Flags().Reload()
	n := &mockNotifier{name: "rec"}
	deliverEvent(context.Background(), stoppedEvent("hidden-unindexed", true), newDedupTracker(0), []Notifier{n}, g)
	assert.Equal(t, 1, n.CallCount())
	assert.EqualValues(t, 1, g.Metrics().Value(deliverygate.CounterUnresolved, "routine"))
}

// T-PS-04: an unresolved session fails open.
func TestStatusChangePush_ShouldDeliverAndCount_WhenGateReportsUnresolved(t *testing.T) {
	t.Parallel()
	g := deliverygate.NewGate(deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
		return deliverygate.FlagSettings{Global: true}, nil
	}))
	g.Flags().Reload()
	n := &mockNotifier{name: "rec"}
	deliverEvent(context.Background(), stoppedEvent("never-indexed", false), newDedupTracker(0), []Notifier{n}, g)
	require.Equal(t, 1, n.CallCount())
	assert.EqualValues(t, 1, g.Metrics().Value(deliverygate.CounterUnresolved, "routine"))
}

func TestStatusChangePush_ShouldPassSessionUUIDToGate_WhenInstanceHasUUID(t *testing.T) {
	t.Parallel()
	var seen string
	n := &mockNotifier{name: "rec"}
	ev := stoppedEvent("t", false)
	ev.Session.UUID = "uuid-1"
	deliverEvent(context.Background(), ev, newDedupTracker(0), []Notifier{n}, gateFunc(func(id string) bool {
		seen = id
		return false
	}))
	assert.Equal(t, "uuid-1", seen)
	assert.Equal(t, 0, n.CallCount())
}

// The gate applies to the status-change path only: an inline notification the
// bus filter already vetted is not re-gated here.
func TestStatusChangePush_ShouldNotConsultGate_WhenEventIsInlineNotification(t *testing.T) {
	t.Parallel()
	called := false
	n := &mockNotifier{name: "rec"}
	ev := &events.Event{
		Type: events.EventNotification, NotificationPriority: priorityUrgent,
		NotificationTitle: "T", NotificationMessage: "M", NotificationID: "n1",
	}
	ev.Timestamp = time.Now()
	deliverEvent(context.Background(), ev, newDedupTracker(0), []Notifier{n}, gateFunc(func(string) bool {
		called = true
		return false
	}))
	assert.False(t, called)
	assert.Equal(t, 1, n.CallCount())
}

// T-PS-03: the push package's own sources do not import the delivery gate; the
// consumer-side interface is declared here and the services package wires it.
func TestPush_ShouldNotImportDeliveryGate_WhenImportGraphChecked(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		require.NoError(t, err)
		for _, imp := range parsed.Imports {
			assert.NotContains(t, imp.Path.Value, "server/deliverygate", "%s imports the gate", f)
		}
	}
}
