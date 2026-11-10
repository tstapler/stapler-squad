package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogflowmarker"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/flow"
)

// markerLimiterFor lazily builds the repo's shared marker limiter.
func (r *EntRepository) markerLimiterFor() *MarkerLimiter {
	r.markerLimiterOnce.Do(func() {
		r.markerLimiter = NewMarkerLimiter(r.markerClock)
	})
	return r.markerLimiter
}

func (r *EntRepository) markerNow() time.Time {
	if r.markerClock != nil {
		return r.markerClock().UTC()
	}
	return time.Now().UTC()
}

// RecordFlowMarker appends a server-decided marker. Best-effort: the limiter
// runs first (deduped decisions cost zero DB calls) and failures are logged,
// never returned, so a marker can never block or change PR-fix behavior.
// Never call it inside a transaction on this repo: the writer is capped at one
// connection (SetMaxOpenConns(1)) and it would self-deadlock. It publishes no
// item-changed events and is deliberately not exposed through any MCP/RPC handler.
func (r *EntRepository) RecordFlowMarker(ctx context.Context, itemID string, kind FlowMarkerKind, value string, prNumber int) {
	if kind == "" || value == "" {
		return
	}
	id, err := uuid.Parse(itemID)
	if err != nil {
		log.WarningLog().Printf("[RecordFlowMarker] invalid item id %q: %v", itemID, err)
		return
	}
	r.insertFlowMarker(ctx, id, kind, value, prNumber, fmt.Sprintf("%s|%s|%d", kind, id, prNumber), nil)
}

// RecordFlowMarkerForItemPR is RecordFlowMarker for callers that do not hold
// the item's PR number: the limiter (keyed per kind+item) runs first, and only
// an admitted marker pays for a pr_number-only projection read. Dedupe is per
// item, not per PR: a new PR on the same item inside the 30m interval records
// the same value only once (intended; it bounds writes on thrashing items).
// A failed PR read drops the marker and forgets the limiter entry so it retries.
func (r *EntRepository) RecordFlowMarkerForItemPR(ctx context.Context, itemID string, kind FlowMarkerKind, value string) {
	if kind == "" || value == "" {
		return
	}
	id, err := uuid.Parse(itemID)
	if err != nil {
		log.WarningLog().Printf("[RecordFlowMarker] invalid item id %q: %v", itemID, err)
		return
	}
	r.insertFlowMarker(ctx, id, kind, value, 0, fmt.Sprintf("%s|%s|item", kind, id), func() (int, error) {
		return r.Client.BacklogItem.Query().Where(backlogitem.ID(id)).Select(backlogitem.FieldPrNumber).Int(ctx)
	})
}

// insertFlowMarker runs the limiter, optionally resolves the PR number, then
// inserts; a failed read or insert forgets the limiter entry so the marker can retry.
func (r *EntRepository) insertFlowMarker(ctx context.Context, id uuid.UUID, kind FlowMarkerKind, value string, prNumber int, limiterKey string, resolvePR func() (int, error)) {
	limiter := r.markerLimiterFor()
	if !limiter.Allow(limiterKey, value) {
		return
	}
	if resolvePR != nil {
		pr, err := resolvePR()
		if err != nil {
			limiter.Forget(limiterKey, value)
			log.WarningLog().Printf("[RecordFlowMarker] pr_number read failed item=%s kind=%s value=%s: %v", id, kind, value, err)
			return
		}
		prNumber = pr
	}
	if _, err := r.Client.BacklogFlowMarker.Create().
		SetItemID(id).
		SetKind(string(kind)).
		SetValue(value).
		SetPrNumber(prNumber).
		SetCreatedAt(r.markerNow()).
		Save(ctx); err != nil {
		limiter.Forget(limiterKey, value)
		log.ErrorLog().Printf("[RecordFlowMarker] failed item=%s kind=%s value=%s: %v", id, kind, value, err)
	}
}

// ListFlowMarkers returns markers of kind with since <= created_at < until, oldest first.
func (r *EntRepository) ListFlowMarkers(ctx context.Context, kind FlowMarkerKind, since, until time.Time) ([]FlowMarkerData, error) {
	rows, err := r.Client.BacklogFlowMarker.Query().
		Where(backlogflowmarker.Kind(string(kind)),
			backlogflowmarker.CreatedAtGTE(since.UTC()),
			backlogflowmarker.CreatedAtLT(until.UTC())).
		Order(ent.Asc(backlogflowmarker.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list flow markers: %w", err)
	}
	return toFlowMarkerData(rows), nil
}

// ListFlowMarkersForItem is ListFlowMarkers scoped to one item.
func (r *EntRepository) ListFlowMarkersForItem(ctx context.Context, itemID string, kind FlowMarkerKind, since, until time.Time) ([]FlowMarkerData, error) {
	id, err := uuid.Parse(itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}
	rows, err := r.Client.BacklogFlowMarker.Query().
		Where(backlogflowmarker.ItemID(id),
			backlogflowmarker.Kind(string(kind)),
			backlogflowmarker.CreatedAtGTE(since.UTC()),
			backlogflowmarker.CreatedAtLT(until.UTC())).
		Order(ent.Asc(backlogflowmarker.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list flow markers for item: %w", err)
	}
	return toFlowMarkerData(rows), nil
}

func toFlowMarkerData(rows []*ent.BacklogFlowMarker) []FlowMarkerData {
	out := make([]FlowMarkerData, 0, len(rows))
	for _, m := range rows {
		out = append(out, FlowMarkerData{
			ItemID: m.ItemID.String(), Kind: FlowMarkerKind(m.Kind), Value: m.Value,
			PRNumber: m.PrNumber, CreatedAt: m.CreatedAt,
		})
	}
	return out
}

var _ flow.MarkerSource = (*EntFlowReader)(nil)

// Markers implements flow.MarkerSource over the system-written marker table
// only. It never reads notes, so agent text carrying marker-looking strings
// cannot appear in it.
func (r *EntFlowReader) Markers(ctx context.Context, from, to time.Time) ([]flow.Marker, error) {
	rows, err := r.client.BacklogFlowMarker.Query().
		Where(backlogflowmarker.CreatedAtGTE(from.UTC()), backlogflowmarker.CreatedAtLT(to.UTC())).
		Order(ent.Asc(backlogflowmarker.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		if isMissingTableErr(err) {
			return nil, nil
		}
		return nil, flowReadErr("flow_markers", err)
	}
	out := make([]flow.Marker, len(rows))
	for i, m := range rows {
		out[i] = flow.Marker{
			Item: flow.ItemID(m.ItemID.String()), Kind: flow.MarkerKind(m.Kind), Value: m.Value,
			PRNumber: m.PrNumber, At: m.CreatedAt.UTC(),
		}
	}
	return out, nil
}

// MarkerCapture reports when each marker kind was first written; a missing
// marker table reads as no capture.
func (r *EntFlowReader) MarkerCapture(ctx context.Context) (flow.MarkerCapture, error) {
	var c flow.MarkerCapture
	var err error
	if c.FirstOutcome, err = r.firstMarkerAt(ctx, FlowMarkerPRFixOutcome); err != nil {
		return flow.MarkerCapture{}, err
	}
	if c.FirstTrigger, err = r.firstMarkerAt(ctx, FlowMarkerPRFixTrigger); err != nil {
		return flow.MarkerCapture{}, err
	}
	if c.FirstNudge, err = r.firstMarkerAt(ctx, FlowMarkerManualNudge); err != nil {
		return flow.MarkerCapture{}, err
	}
	return c, nil
}

func (r *EntFlowReader) firstMarkerAt(ctx context.Context, kind FlowMarkerKind) (time.Time, error) {
	first, err := r.client.BacklogFlowMarker.Query().
		Where(backlogflowmarker.Kind(string(kind))).
		Order(ent.Asc(backlogflowmarker.FieldCreatedAt)).
		First(ctx)
	switch {
	case ent.IsNotFound(err), err != nil && isMissingTableErr(err):
		return time.Time{}, nil
	case err != nil:
		return time.Time{}, flowReadErr("flow_marker_capture", err)
	}
	return first.CreatedAt.UTC(), nil
}

// CaptureSince is the earliest of the first stuck episode and the first
// pr_fix_trigger, pr_fix_outcome and manual_nudge marker; zero when none exist. It is the date
// before which capture-dependent metrics are unavailable or lower bounds.
func (r *EntFlowReader) CaptureSince(ctx context.Context) (time.Time, error) {
	sc, err := r.StuckCapture(ctx)
	if err != nil {
		return time.Time{}, err
	}
	mc, err := r.MarkerCapture(ctx)
	if err != nil {
		return time.Time{}, err
	}
	var earliest time.Time
	for _, t := range []time.Time{sc.CaptureSince, mc.FirstTrigger, mc.FirstOutcome, mc.FirstNudge} {
		if !t.IsZero() && (earliest.IsZero() || t.Before(earliest)) {
			earliest = t
		}
	}
	return earliest, nil
}

// SetMarkerClock overrides the clock behind flow-marker timestamps and rate limiting; call it
// before the first marker is recorded (tests).
func (r *EntRepository) SetMarkerClock(now func() time.Time) { r.markerClock = now }
