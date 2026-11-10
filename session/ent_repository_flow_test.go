package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/dbobs"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/flow"
)

// newFlowTestRepo opens an on-disk repository (dbpool reader/writer split).
func newFlowTestRepo(t *testing.T) *EntRepository {
	t.Helper()
	repo, err := NewEntRepository(WithDatabasePath(filepath.Join(t.TempDir(), "sessions.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func seedFlowItem(t *testing.T, c *ent.Client, title string) uuid.UUID {
	t.Helper()
	item, err := c.BacklogItem.Create().SetTitle(title).SetStatus("idea").Save(context.Background())
	require.NoError(t, err)
	return item.ID
}

func seedFlowEvent(t *testing.T, c *ent.Client, item uuid.UUID, from, to string, at time.Time, note string) {
	t.Helper()
	cr := c.BacklogStatusEvent.Create().SetItemID(item).SetFromStatus(from).SetToStatus(to).
		SetTriggeredBy("agent").SetCreatedAt(at)
	if note != "" {
		cr.SetNote(note)
	}
	_, err := cr.Save(context.Background())
	require.NoError(t, err)
}

func TestEntFlowReader_should_FilterByParsedInstant_When_RowsCarryMixedOffsets(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	item := seedFlowItem(t, c, "mixed")
	pdt := time.FixedZone("", -7*3600)
	// 2026-10-05 20:00:00-07:00 is 2026-10-06T03:00Z (in range).
	seedFlowEvent(t, c, item, "ready", "in_progress", time.Date(2026, 10, 5, 20, 0, 0, 0, pdt), "a")
	// 2026-10-05 20:00:00+00:00 is 2026-10-05T20:00Z (before From 21:30Z).
	seedFlowEvent(t, c, item, "in_progress", "review", time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC), "b")

	r := NewEntFlowReader(c)
	rng := flow.EventRange{
		From: time.Date(2026, 10, 5, 21, 30, 0, 0, time.UTC),
		To:   time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}
	got, err := r.Events(context.Background(), rng)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "a", got[0].Note)
	require.Equal(t, time.Date(2026, 10, 6, 3, 0, 0, 0, time.UTC), got[0].At)
	require.Equal(t, time.UTC, got[0].At.Location())

	// The SQL prefilter is a superset: the raw fetch saw both rows.
	raw, err := r.fetchEvents(context.Background(), rng)
	require.NoError(t, err)
	require.Len(t, raw, 2)
}

func TestEntFlowReader_should_OrderByInstantThenSeq_When_RowsCarryMixedOffsets(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	item := seedFlowItem(t, c, "order")
	pdt := time.FixedZone("", -7*3600)
	// Text order puts the -07:00 row (10:30) first; instant order puts it last (17:30Z).
	seedFlowEvent(t, c, item, "ready", "in_progress", time.Date(2026, 8, 10, 10, 30, 0, 0, pdt), "later")
	seedFlowEvent(t, c, item, "idea", "ready", time.Date(2026, 8, 10, 17, 0, 0, 0, time.UTC), "earlier")

	got, err := NewEntFlowReader(c).Events(context.Background(), flow.EventRange{
		From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, []string{"earlier", "later"}, []string{got[0].Note, got[1].Note})
	require.Greater(t, got[0].Seq, int64(0))
}

func TestEntFlowReader_should_ReturnLatestPreRangeEvent_When_LastEventsBefore(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	a, b := seedFlowItem(t, c, "a"), seedFlowItem(t, c, "b")
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	seedFlowEvent(t, c, a, "idea", "ready", base, "a1")
	seedFlowEvent(t, c, a, "ready", "in_progress", base.Add(time.Hour), "a2")
	seedFlowEvent(t, c, a, "in_progress", "review", base.Add(48*time.Hour), "a-after")
	seedFlowEvent(t, c, b, "idea", "ready", base.Add(2*time.Hour), "b1")

	before := base.Add(24 * time.Hour)
	got, err := NewEntFlowReader(c).LastEventsBefore(context.Background(), before,
		[]flow.ItemID{flow.ItemID(a.String()), flow.ItemID(b.String()), flow.ItemID(uuid.NewString())})
	require.NoError(t, err)
	notes := map[flow.ItemID]string{}
	for _, e := range got {
		notes[e.ItemID] = e.Note
	}
	require.Equal(t, map[flow.ItemID]string{flow.ItemID(a.String()): "a2", flow.ItemID(b.String()): "b1"}, notes)
}

func TestEntFlowReader_should_ListOnlyNonTerminalItems_When_OpenItemIDs(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	open := seedFlowItem(t, c, "open")
	for _, st := range []string{"done", "archived"} {
		it := seedFlowItem(t, c, st)
		_, err := c.BacklogItem.UpdateOneID(it).SetStatus(st).Save(context.Background())
		require.NoError(t, err)
	}

	got, err := NewEntFlowReader(c).OpenItemIDs(context.Background())

	require.NoError(t, err)
	require.Equal(t, []flow.ItemID{flow.ItemID(open.String())}, got)
}

func TestEntFlowReader_should_ReturnTypedError_When_ContextCancelled(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewEntFlowReader(repo.GetEntClient()).Events(ctx, flow.EventRange{From: time.Now().Add(-time.Hour)})
	require.ErrorIs(t, err, context.Canceled)
	var re *FlowReadError
	require.ErrorAs(t, err, &re)
	require.Equal(t, "events", re.Op)
}

// Not parallel: asserts per-pool counters in the process-wide dbobs registry.
func TestEntFlowReader_should_LabelReadsReaderPool_When_PlainSelectsOnly(t *testing.T) {
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	item := seedFlowItem(t, c, "pool")
	seedFlowEvent(t, c, item, "idea", "ready", time.Now().UTC().Add(-time.Hour), "x")

	r := NewEntFlowReader(c)
	rng := flow.EventRange{From: time.Now().UTC().Add(-48 * time.Hour)}
	ids := []flow.ItemID{flow.ItemID(item.String())}
	ctx := context.Background()
	win, err := flow.NewFlowWindowBounds(rng.From, time.Now().UTC().Add(time.Hour))
	require.NoError(t, err)
	calls := map[string]func() error{
		"Events":           func() error { _, err := r.Events(ctx, rng); return err },
		"CountEvents":      func() error { _, err := r.CountEvents(ctx, rng); return err },
		"StreamEvents":     func() error { return r.StreamEvents(ctx, rng, func([]flow.Event) error { return nil }) },
		"LastEventsBefore": func() error { _, err := r.LastEventsBefore(ctx, time.Now().UTC(), ids); return err },
		"Verdicts":         func() error { _, err := r.Verdicts(ctx, win); return err },
		"AgentDoneClaims":  func() error { _, err := r.AgentDoneClaims(ctx, win); return err },
		"StuckIntervals":   func() error { _, err := r.StuckIntervals(ctx); return err },
		"Items":            func() error { _, err := r.Items(ctx, ids); return err },
		"OpenItemIDs":      func() error { _, err := r.OpenItemIDs(ctx); return err },
	}
	for name, call := range calls {
		beforeR, beforeW := poolQueries("ent-read"), poolQueries("ent")
		require.NoError(t, call(), name)
		require.Greater(t, poolQueries("ent-read"), beforeR, "%s must hit the reader pool", name)
		require.Equal(t, beforeW, poolQueries("ent"), "%s must not touch the writer pool", name)
	}
}

func poolQueries(name string) int64 {
	var n int64
	for _, s := range dbobs.Snapshot() {
		if s.Name == name {
			n += s.Queries
		}
	}
	return n
}
