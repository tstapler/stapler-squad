package session

import (
	"context"
	"sort"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstatusevent"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
	"github.com/tstapler/stapler-squad/session/flow"
)

// maxOffsetSkew bounds the UTC offset of any stored created_at text (+/-14h),
// so subtracting it from a UTC bound yields a text-comparison superset.
const maxOffsetSkew = 15 * time.Hour

type flowEventRow struct {
	ItemID      uuid.UUID `json:"item_id"`
	FromStatus  string    `json:"from_status"`
	ToStatus    string    `json:"to_status"`
	TriggeredBy string    `json:"triggered_by"`
	Note        *string   `json:"note"`
	CreatedAt   time.Time `json:"created_at"`
	Seq         int64     `json:"seq"`
}

var flowEventColumns = []string{
	backlogstatusevent.FieldItemID, backlogstatusevent.FieldFromStatus, backlogstatusevent.FieldToStatus,
	backlogstatusevent.FieldTriggeredBy, backlogstatusevent.FieldNote, backlogstatusevent.FieldCreatedAt,
}

// selectFlowEvents projects the flow columns plus SQLite's rowid as Seq (ent's
// Select rejects non-schema columns, so rowid rides in as an aggregate-style expression).
func selectFlowEvents(q *ent.BacklogStatusEventQuery) *ent.BacklogStatusEventSelect {
	return q.Select(flowEventColumns...).Aggregate(func(s *sql.Selector) string {
		return sql.As("rowid", "seq")
	})
}

func (row flowEventRow) event() flow.Event {
	e := flow.Event{
		ItemID:      flow.ItemID(row.ItemID.String()),
		From:        domain.BacklogStatus(row.FromStatus),
		To:          domain.BacklogStatus(row.ToStatus),
		At:          row.CreatedAt.UTC(),
		TriggeredBy: row.TriggeredBy,
		Seq:         row.Seq,
	}
	if row.Note != nil {
		e.Note = *row.Note
	}
	return e
}

// prefilter returns the conservative text-comparison predicates: created_at
// is stored as local wall-clock text with a varying offset, so bounds are
// widened by maxOffsetSkew and Go applies the exact parsed-instant filter.
func prefilter(rng flow.EventRange) []predicate.BacklogStatusEvent {
	preds := []predicate.BacklogStatusEvent{
		backlogstatusevent.CreatedAtGTE(rng.From.UTC().Add(-maxOffsetSkew)),
	}
	if !rng.To.IsZero() {
		preds = append(preds, backlogstatusevent.CreatedAtLT(rng.To.UTC().Add(maxOffsetSkew)))
	}
	if ids := parseFlowItemIDs(rng.ItemIDs); len(rng.ItemIDs) > 0 {
		preds = append(preds, backlogstatusevent.ItemIDIn(ids...))
	}
	return preds
}

func inRange(rng flow.EventRange, at time.Time) bool {
	return !at.Before(rng.From) && (rng.To.IsZero() || at.Before(rng.To))
}

// parseFlowItemIDs drops ids that are not UUIDs: no such item can exist.
func parseFlowItemIDs(ids []flow.ItemID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if u, err := uuid.Parse(string(id)); err == nil {
			out = append(out, u)
		}
	}
	return out
}

// fetchEvents runs the prefilter query only (a superset of the exact range).
func (r *EntFlowReader) fetchEvents(ctx context.Context, rng flow.EventRange) ([]flow.Event, error) {
	var rows []flowEventRow
	err := selectFlowEvents(r.client.BacklogStatusEvent.Query().Where(prefilter(rng)...)).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]flow.Event, len(rows))
	for i, row := range rows {
		out[i] = row.event()
	}
	return out, nil
}

// Events returns events inside rng, ordered by (At, Seq). The SQL prefilter is
// a superset; the exact filter and ordering use parsed instants (ADR-004).
func (r *EntFlowReader) Events(ctx context.Context, rng flow.EventRange) ([]flow.Event, error) {
	raw, err := r.fetchEvents(ctx, rng)
	if err != nil {
		return nil, flowReadErr("events", err)
	}
	out := raw[:0]
	for _, e := range raw {
		if inRange(rng, e.At) {
			out = append(out, e)
		}
	}
	sortEvents(out)
	return out, nil
}

// CountEvents returns an upper bound on len(Events(rng)): the prefilter count,
// which can exceed the exact count by rows within the offset skew of a bound.
// It decides chunk-or-refuse only, so the over-count is harmless.
func (r *EntFlowReader) CountEvents(ctx context.Context, rng flow.EventRange) (int, error) {
	n, err := r.client.BacklogStatusEvent.Query().Where(prefilter(rng)...).Count(ctx)
	return n, flowReadErr("count_events", err)
}

// LastEventsBefore returns, per item in ids, its latest event strictly before
// `before`, seeding each item's status at window start.
//
// It runs one GROUP BY item_id MAX(created_at) query and one fetch of the
// matching rows, then picks the latest by parsed instant in Go. Exact for
// ranges starting after 2026-08-18 (all rows +00:00); best-effort inside the
// older mixed-offset span, where the text MAX can pick a non-latest row.
func (r *EntFlowReader) LastEventsBefore(ctx context.Context, before time.Time, ids []flow.ItemID) ([]flow.Event, error) {
	uids := parseFlowItemIDs(ids)
	if len(uids) == 0 {
		return nil, nil
	}
	rows, err := r.latestRowsBefore(ctx, before, uids)
	if err != nil {
		return nil, flowReadErr("last_events_before", err)
	}
	latest := make(map[flow.ItemID]flow.Event, len(uids))
	for _, row := range rows {
		e := row.event()
		if !e.At.Before(before) {
			continue
		}
		if cur, ok := latest[e.ItemID]; !ok || eventLess(cur, e) {
			latest[e.ItemID] = e
		}
	}
	out := make([]flow.Event, 0, len(latest))
	for _, e := range latest {
		out = append(out, e)
	}
	sortEvents(out)
	return out, nil
}

func (r *EntFlowReader) latestRowsBefore(ctx context.Context, before time.Time, uids []uuid.UUID) ([]flowEventRow, error) {
	var maxes []struct {
		ItemID uuid.UUID `json:"item_id"`
		Max    string    `json:"max"`
	}
	base := []predicate.BacklogStatusEvent{
		backlogstatusevent.ItemIDIn(uids...),
		backlogstatusevent.CreatedAtLT(before.UTC()),
	}
	err := r.client.BacklogStatusEvent.Query().Where(base...).
		GroupBy(backlogstatusevent.FieldItemID).
		Aggregate(func(s *sql.Selector) string {
			return sql.As(sql.Max(s.C(backlogstatusevent.FieldCreatedAt)), "max")
		}).Scan(ctx, &maxes)
	if err != nil || len(maxes) == 0 {
		return nil, err
	}
	stamps := make([]time.Time, 0, len(maxes))
	for _, m := range maxes {
		// The driver reports MAX() as RFC3339 text; matching rows through ent's
		// own time encoding finds rows stored in UTC (exact after 2026-08-18).
		if t, perr := time.Parse(time.RFC3339Nano, m.Max); perr == nil {
			stamps = append(stamps, t)
		}
	}
	var rows []flowEventRow
	err = selectFlowEvents(r.client.BacklogStatusEvent.Query().Where(
		backlogstatusevent.ItemIDIn(uids...),
		backlogstatusevent.CreatedAtIn(stamps...),
	)).Scan(ctx, &rows)
	return rows, err
}

func eventLess(a, b flow.Event) bool {
	if !a.At.Equal(b.At) {
		return a.At.Before(b.At)
	}
	return a.Seq < b.Seq
}

func sortEvents(es []flow.Event) {
	sort.SliceStable(es, func(i, j int) bool { return eventLess(es[i], es[j]) })
}
