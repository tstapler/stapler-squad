package escapedb

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/require"

	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/ent"
)

// openForTest opens a fresh escape-events database under dir and closes it with the test.
func openForTest(ctx context.Context, t *testing.T, dir string) *ent.Client {
	t.Helper()
	client, _ := openStoreForTest(ctx, t, dir)
	return client
}

// openStoreForTest is openForTest plus the raw driver the writer and rollups run on.
func openStoreForTest(ctx context.Context, t *testing.T, dir string) (*ent.Client, dialect.Driver) {
	t.Helper()
	client, db, err := Open(ctx, dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client, db.Driver()
}

// testBase is local time, as time.Now() is in production; June avoids DST transitions in the
// text-compared wall_time column on any machine zone.
var testBase = time.Date(2026, 6, 1, 0, 30, 0, 0, time.Local)

// genEvents builds n events across two sessions, two sequence types, two stages and some
// mangled outcomes, spaced step apart from base.
func genEvents(base time.Time, n int, step time.Duration) []pkganalytics.EscapeEventRecord {
	out := make([]pkganalytics.EscapeEventRecord, 0, n)
	for i := 0; i < n; i++ {
		ev := pkganalytics.EscapeEventRecord{
			SessionID: []string{"a", "b"}[i%2], ProjectPath: "/p/" + []string{"a", "b"}[i%2],
			Stage:        []pkganalytics.Stage{pkganalytics.StagePTYRead, pkganalytics.StageTransport}[(i/2)%2],
			SequenceType: []string{"SGR", "Cursor"}[(i/3)%2], ByteLen: 3 + i%5,
			WallTime: base.Add(time.Duration(i) * step), SessionSeq: int64(i),
		}
		if i%7 == 0 {
			ev.Mangled, ev.MangleType = true, "stripped"
		}
		out = append(out, ev)
	}
	return out
}

// truthAggregate counts events in the closed range [start, end] the slow, obvious way.
func truthAggregate(events []pkganalytics.EscapeEventRecord, sessionID string, start, end *time.Time) map[AggregateRow]int64 {
	out := make(map[AggregateRow]int64)
	for _, e := range events {
		if (sessionID != "" && e.SessionID != sessionID) || (start != nil && e.WallTime.Before(*start)) || (end != nil && e.WallTime.After(*end)) {
			continue
		}
		out[AggregateRow{SessionID: e.SessionID, ProjectPath: e.ProjectPath, SequenceType: e.SequenceType,
			Stage: string(e.Stage), Mangled: e.Mangled, MangleType: e.MangleType}]++
	}
	return out
}

func aggregateMap(rows []AggregateRow) map[AggregateRow]int64 {
	out := make(map[AggregateRow]int64, len(rows))
	for _, r := range rows {
		n := r.Count
		r.Count = 0
		out[r] += n
	}
	return out
}
