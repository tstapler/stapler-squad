package flow

import (
	"errors"
	"fmt"
	"time"
)

// ReaderLimits bounds how much of the event table one computation reads.
type ReaderLimits struct {
	// MaxWindowRows is the event count above which the read is chunked by item.
	MaxWindowRows int
	// HardRowCeiling is the event count above which the read is refused.
	HardRowCeiling int
	// ChunkItems is the number of items per streamed batch.
	ChunkItems int
	// BatchTimeout cancels one batch query at the driver.
	BatchTimeout time.Duration
}

// DefaultReaderLimits returns the plan defaults (60k / 1M / 25 items / 5s).
func DefaultReaderLimits() ReaderLimits {
	return ReaderLimits{MaxWindowRows: 60_000, HardRowCeiling: 1_000_000, ChunkItems: 25, BatchTimeout: 5 * time.Second}
}

// withDefaults fills zero fields from DefaultReaderLimits.
func (l ReaderLimits) withDefaults() ReaderLimits {
	d := DefaultReaderLimits()
	if l.MaxWindowRows <= 0 {
		l.MaxWindowRows = d.MaxWindowRows
	}
	if l.HardRowCeiling <= 0 {
		l.HardRowCeiling = d.HardRowCeiling
	}
	if l.ChunkItems <= 0 {
		l.ChunkItems = d.ChunkItems
	}
	if l.BatchTimeout <= 0 {
		l.BatchTimeout = d.BatchTimeout
	}
	return l
}

// ErrWindowTooLarge is matched (errors.Is) by a refusal to read a window whose
// event count exceeds ReaderLimits.HardRowCeiling.
var ErrWindowTooLarge = errors.New("flow: window too large")

// WindowTooLargeError carries the refused row count and the ceiling.
type WindowTooLargeError struct {
	Rows, Limit int
}

func (e *WindowTooLargeError) Error() string {
	return fmt.Sprintf("flow: window too large (%d rows, limit %d)", e.Rows, e.Limit)
}

func (e *WindowTooLargeError) Is(target error) bool { return target == ErrWindowTooLarge }

// BatchError names the streamed batch whose query failed or timed out.
type BatchError struct {
	Batch, Batches int
	Err            error
}

func (e *BatchError) Error() string {
	return fmt.Sprintf("flow: event batch %d of %d: %v", e.Batch, e.Batches, e.Err)
}

func (e *BatchError) Unwrap() error { return e.Err }
