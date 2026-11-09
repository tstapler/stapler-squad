package session

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/flow"
)

var (
	_ flow.StuckSource        = (*EntFlowReader)(nil)
	_ flow.StuckCaptureSource = (*EntFlowReader)(nil)
)

// stuckEpisodeTableMissingLogged limits the missing-table warning to once per process.
var stuckEpisodeTableMissingLogged atomic.Bool

type stuckKey struct {
	item   uuid.UUID
	reason string
}

// stuckRows loads legacy state rows and episodes. A missing episode table
// (older database) yields no episodes, never an error, so the stuck section
// degrades to legacy-only instead of failing.
func (r *EntFlowReader) stuckRows(ctx context.Context) ([]*ent.BacklogStuckState, []*ent.BacklogStuckEpisode, error) {
	//nolint:entfullscan the stuck-state table is small (hundreds of rows) and every interval is needed to bound open and closed durations.
	states, err := r.client.BacklogStuckState.Query().All(ctx)
	if err != nil {
		return nil, nil, flowReadErr("stuck_intervals", err)
	}
	//nolint:entfullscan episodes are one row per stuck occurrence (hundreds); every one is needed to rebuild exact intervals.
	episodes, err := r.client.BacklogStuckEpisode.Query().All(ctx)
	if err != nil {
		if isMissingTableErr(err) {
			if stuckEpisodeTableMissingLogged.CompareAndSwap(false, true) {
				log.WarningLog().Printf("[EntFlowReader] backlog_stuck_episodes missing; stuck history is legacy-only: %v", err)
			}
			return states, nil, nil
		}
		return nil, nil, flowReadErr("stuck_episodes", err)
	}
	return states, episodes, nil
}

func isMissingTableErr(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "does not exist")
}

// StuckIntervals returns every recorded episode plus the legacy state rows
// not superseded by one. Per (item, reason), with E the earliest episode
// start: a legacy row first detected at or after E is dropped (the episode is
// authoritative); an earlier one is kept and its end clipped to E when it is
// open or ends later. Legacy intervals stay lower bounds (Episode=false). All
// times are normalized to UTC before comparison: MarkStuck wrote local time.
func (r *EntFlowReader) StuckIntervals(ctx context.Context) ([]flow.StuckInterval, error) {
	states, episodes, err := r.stuckRows(ctx)
	if err != nil {
		return nil, err
	}
	earliest := make(map[stuckKey]time.Time, len(episodes))
	out := make([]flow.StuckInterval, 0, len(states)+len(episodes))
	for _, ep := range episodes {
		opened := ep.OpenedAt.UTC()
		k := stuckKey{ep.ItemID, ep.Reason}
		if e, ok := earliest[k]; !ok || opened.Before(e) {
			earliest[k] = opened
		}
		iv := flow.StuckInterval{ItemID: flow.ItemID(ep.ItemID.String()), Reason: ep.Reason, FirstDetectedAt: opened, Episode: true}
		iv.ResolvedAt = utcPtr(ep.ResolvedAt)
		out = append(out, iv)
	}
	for _, row := range states {
		first := row.FirstDetectedAt.UTC()
		iv := flow.StuckInterval{ItemID: flow.ItemID(row.ItemID.String()), Reason: row.Reason, FirstDetectedAt: first, ResolvedAt: utcPtr(row.ResolvedAt)}
		if e, ok := earliest[stuckKey{row.ItemID, row.Reason}]; ok {
			if !first.Before(e) {
				continue
			}
			if iv.ResolvedAt == nil || iv.ResolvedAt.After(e) {
				iv.ResolvedAt = &e
			}
		}
		out = append(out, iv)
	}
	return out, nil
}

// StuckCapture reports when episode capture began and the open counts that
// should agree: open episodes versus open legacy rows first detected at or
// after capture began (earlier open rows are legacy history, not drift).
func (r *EntFlowReader) StuckCapture(ctx context.Context) (flow.StuckCapture, error) {
	states, episodes, err := r.stuckRows(ctx)
	if err != nil {
		return flow.StuckCapture{}, err
	}
	var c flow.StuckCapture
	for _, ep := range episodes {
		if opened := ep.OpenedAt.UTC(); c.CaptureSince.IsZero() || opened.Before(c.CaptureSince) {
			c.CaptureSince = opened
		}
		if ep.ResolvedAt == nil {
			c.OpenEpisodes++
		}
	}
	if c.CaptureSince.IsZero() {
		return c, nil
	}
	for _, row := range states {
		if row.ResolvedAt == nil && !row.FirstDetectedAt.UTC().Before(c.CaptureSince) {
			c.OpenStateRows++
		}
	}
	return c, nil
}
