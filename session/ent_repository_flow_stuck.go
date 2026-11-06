package session

import (
	"context"

	"github.com/tstapler/stapler-squad/session/flow"
)

// StuckIntervals returns every stuck row, resolved ones included (a nil
// ResolvedAt means still stuck). Rows are legacy state rows, so closed
// durations are a lower bound until episode capture (ADR-003).
func (r *EntFlowReader) StuckIntervals(ctx context.Context) ([]flow.StuckInterval, error) {
	//nolint:entfullscan the stuck-state table is small (hundreds of rows) and every interval is needed to bound open and closed durations.
	rows, err := r.client.BacklogStuckState.Query().All(ctx)
	if err != nil {
		return nil, flowReadErr("stuck_intervals", err)
	}
	out := make([]flow.StuckInterval, len(rows))
	for i, row := range rows {
		iv := flow.StuckInterval{
			ItemID:          flow.ItemID(row.ItemID.String()),
			Reason:          row.Reason,
			FirstDetectedAt: row.FirstDetectedAt.UTC(),
		}
		if row.ResolvedAt != nil {
			t := row.ResolvedAt.UTC()
			iv.ResolvedAt = &t
		}
		out[i] = iv
	}
	return out, nil
}
