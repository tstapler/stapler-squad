package flowstore

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogflowmarker"
	"github.com/tstapler/stapler-squad/session/flow"
)

var _ flow.MarkerSource = (*Reader)(nil)

// Markers implements flow.MarkerSource over the system-written marker table
// only. It never reads notes, so agent text carrying marker-looking strings
// cannot appear in it.
func (r *Reader) Markers(ctx context.Context, from, to time.Time) ([]flow.Marker, error) {
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
func (r *Reader) MarkerCapture(ctx context.Context) (flow.MarkerCapture, error) {
	var c flow.MarkerCapture
	var err error
	if c.FirstOutcome, err = r.firstMarkerAt(ctx, flow.MarkerPRFixOutcome); err != nil {
		return flow.MarkerCapture{}, err
	}
	if c.FirstTrigger, err = r.firstMarkerAt(ctx, flow.MarkerPRFixTrigger); err != nil {
		return flow.MarkerCapture{}, err
	}
	if c.FirstNudge, err = r.firstMarkerAt(ctx, flow.MarkerManualNudge); err != nil {
		return flow.MarkerCapture{}, err
	}
	return c, nil
}

func (r *Reader) firstMarkerAt(ctx context.Context, kind flow.MarkerKind) (time.Time, error) {
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
func (r *Reader) CaptureSince(ctx context.Context) (time.Time, error) {
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
