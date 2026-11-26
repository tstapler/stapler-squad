package healthnotify

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/golden.txt")

var logTime = regexp.MustCompile(`time=\S+ `)

type goldenRecorder struct {
	t   *testing.T
	ch  <-chan *events.Event
	out strings.Builder
}

// step records every event published since the previous step. Publish sends to the
// subscriber channel before returning, so a non-blocking drain is complete.
func (r *goldenRecorder) step(name string) {
	r.t.Helper()
	fmt.Fprintf(&r.out, "== %s\n", name)
	for {
		select {
		case ev := <-r.ch:
			id := ev.NotificationID
			if _, err := uuid.Parse(id); err == nil {
				id = "<uuid>"
			}
			b, err := json.Marshal([]any{ev.Type, ev.SessionID, ev.Context, id, ev.NotificationType,
				ev.NotificationPriority, ev.NotificationTitle, ev.NotificationMessage, ev.NotificationMetadata})
			require.NoError(r.t, err)
			r.out.Write(append(b, '\n'))
		default:
			return
		}
	}
}

// TestHealthNotifiers_should_PublishTheRecordedEventsAndLogs_When_Scripted replays a script
// recorded from the pre-move code in server/services (origin/main 385c81549).
// Not parallel: it sets STAPLER_SQUAD_TEST_DIR and swaps the process-global logger.
func TestHealthNotifiers_should_PublishTheRecordedEventsAndLogs_When_Scripted(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	dir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	logs := &log.SyncBuffer{}
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })

	bus := events.NewEventBus(64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)
	r := &goldenRecorder{t: t, ch: ch}
	done, stop := context.WithCancel(context.Background())
	stop()

	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }
	a := newStaleTestInstance("alpha", "uuid-a", domain.Active, ago(40*time.Minute))
	b := newStaleTestInstance("bravo", "uuid-b", domain.Active, ago(11*time.Minute))
	c := newStaleTestInstance("charlie", "uuid-c", domain.Paused, ago(50*time.Minute))
	d := newStaleTestInstance("delta-no-uuid", "", domain.Active, ago(100*time.Hour))
	e := newStaleTestInstance("echo", "uuid-e", domain.Active, ago(9*time.Minute))
	insts := []*instance.Instance{a, b, c, d, e}

	writeStaleSessionConfig(t, dir, 30, false)
	stale := newGoldenStale(bus, insts...)
	stale.checkAll()
	r.step("stale: notify disabled")
	writeStaleSessionConfig(t, dir, 30, true)
	stale.Start(done)
	r.step("stale: Start with cancelled context")
	stale.checkAll()
	r.step("stale: same episode")
	setLastMeaningfulOutput(a, time.Now())
	stale.checkAll()
	r.step("stale: alpha recovered")
	setLastMeaningfulOutput(a, ago(31*time.Minute))
	stale.checkAll()
	r.step("stale: alpha stale again")
	d.ForceStatus(domain.Paused)
	stale.checkAll()
	d.ForceStatus(domain.Active)
	stale.checkAll()
	r.step("stale: delta paused then resumed")
	writeStaleSessionConfig(t, dir, 10, true)
	stale.checkAll()
	r.step("stale: threshold lowered to 10")
	newGoldenStale(nil, insts...).checkAll()
	r.step("stale: nil bus")

	setLastMeaningfulOutput(a, ago(30*time.Minute))
	ratio, ok := 0.0, true
	mem := newGoldenMemory(bus, insts...)
	mem.ratioFunc = func() (float64, bool) { return ratio, ok }
	for _, v := range []float64{0.5, 0.8999, 0.90, 0.95, 0.80, 0.75, 0.7499, 0.5, 0.91} {
		ratio = v
		mem.checkOnce()
		r.step(fmt.Sprintf("memory: ratio %v", v))
	}
	ratio, ok = 0.1, false
	mem.checkOnce()
	r.step("memory: unavailable")
	ratio, ok = 0.1, true
	mem.Start(done)
	r.step("memory: Start with cancelled context clears")

	idle := newGoldenMemory(bus, c, e)
	idle.ratioFunc = func() (float64, bool) { return 0.99, true }
	idle.checkOnce()
	r.step("memory: no idle candidates")
	fmt.Fprintf(&r.out, "no instances: %q\n", newGoldenMemory(bus).reclaimOptionsText())
	nilBus := newGoldenMemory(nil, insts...)
	nilBus.ratioFunc = func() (float64, bool) { return 0.99, true }
	nilBus.checkOnce()
	nilBus.ratioFunc = func() (float64, bool) { return 0.1, true }
	nilBus.checkOnce()
	r.step("memory: nil bus")

	r.out.WriteString("== logs\n")
	for _, line := range strings.SplitAfter(logs.String(), "\n") {
		if strings.Contains(line, "notifier") {
			r.out.WriteString(logTime.ReplaceAllString(line, ""))
		}
	}

	if *updateGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile("testdata/golden.txt", []byte(r.out.String()), 0o644))
	}
	want, err := os.ReadFile("testdata/golden.txt")
	require.NoError(t, err)
	assert.Equal(t, string(want), r.out.String())
}
