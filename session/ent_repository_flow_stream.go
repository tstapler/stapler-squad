package session

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/ent/backlogstatusevent"
	"github.com/tstapler/stapler-squad/session/flow"
)

// StreamEvents delivers the events of rng in item batches of rng.ChunkItems
// (default flow.DefaultReaderLimits), so a window above the row ceiling is read
// without holding every row at once. Each batch holds all in-range events of
// its items, ordered by (At, Seq); batches arrive in item-id order. Every batch
// query is its own plain SELECT on the reader pool with a rng.BatchTimeout
// budget, so a slow batch is cancelled at the driver and fails with a
// *flow.BatchError. An error from fn stops the stream and is returned as is.
func (r *EntFlowReader) StreamEvents(ctx context.Context, rng flow.EventRange, fn func(batch []flow.Event) error) error {
	def := flow.DefaultReaderLimits()
	chunk, timeout := rng.ChunkItems, rng.BatchTimeout
	if chunk <= 0 {
		chunk = def.ChunkItems
	}
	if timeout <= 0 {
		timeout = def.BatchTimeout
	}
	ids, err := r.distinctItems(ctx, rng)
	if err != nil {
		return flowReadErr("stream_events", err)
	}
	batches := (len(ids) + chunk - 1) / chunk
	for n, group := range slices.Collect(slices.Chunk(ids, chunk)) {
		if err := ctx.Err(); err != nil {
			return flowReadErr("stream_events", err)
		}
		events, err := r.readBatch(ctx, rng, group, timeout)
		if err != nil {
			return flowReadErr("stream_events", &flow.BatchError{Batch: n + 1, Batches: batches, Err: err})
		}
		if len(events) == 0 {
			continue
		}
		if err := fn(events); err != nil {
			return err
		}
	}
	return nil
}

// distinctItems lists the item ids the prefilter matches, in id order.
func (r *EntFlowReader) distinctItems(ctx context.Context, rng flow.EventRange) ([]flow.ItemID, error) {
	var rows []struct {
		ItemID uuid.UUID `json:"item_id"`
	}
	err := r.client.BacklogStatusEvent.Query().Where(prefilter(rng)...).
		GroupBy(backlogstatusevent.FieldItemID).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	ids := make([]flow.ItemID, len(rows))
	for i, row := range rows {
		ids[i] = flow.ItemID(row.ItemID.String())
	}
	slices.Sort(ids)
	return ids, nil
}

func (r *EntFlowReader) readBatch(ctx context.Context, rng flow.EventRange, ids []flow.ItemID, timeout time.Duration) ([]flow.Event, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	batchRng := rng
	batchRng.ItemIDs = ids
	raw, err := r.fetchEvents(ctx, batchRng)
	if err != nil {
		return nil, err
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
