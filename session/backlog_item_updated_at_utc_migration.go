package session

// backlog_item_updated_at_utc_migration.go — one-time idempotent migration
// that normalizes BacklogItem.updated_at to UTC for rows written before the
// fix in session/ent/schema/backlog_item.go (Default/UpdateDefault changed
// from bare time.Now, which returns Local, to time.Now().UTC()).
//
// The SQLite driver (modernc.org/sqlite, formerly mattn/go-sqlite3) binds a
// time.Time by formatting it as TEXT in the value's OWN Location (absent a
// _time_format DSN override, both drivers fall back to time.Time.String(),
// e.g. "2006-01-02 15:04:05.999999999 -0700 MST"), so pre-existing rows
// still carry a Local offset suffix (e.g. "-0700") while every row written
// after the fix carries "+0000". Two concrete problems this causes until
// every row is touched at least once:
//  1. BINARY-collated TEXT sort (ORDER BY updated_at, used by the default
//     backlog list/queue sort) does not respect chronological order across
//     mixed offset suffixes — an old row can sort out of place relative to
//     a new one representing an earlier instant.
//  2. A protobuf-Timestamp-derived CAS precondition (always UTC —
//     timestamppb.Timestamp.AsTime()) can never byte-match a Local-formatted
//     stored value, so the first manual-override attempt against an
//     untouched pre-fix row fails with a confusing (but non-corrupting —
//     CAS-safe, caller just reloads and retries) precondition error.
//
// Re-saving each row's UpdatedAt via .UTC() preserves the exact same
// instant (time.Time.UTC() only changes the Location used for
// formatting/comparison-by-driver, not the underlying instant) while
// normalizing the stored TEXT representation. Mirrors status_remap.go's
// one-time-idempotent-migration-at-startup pattern.

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// runBacklogItemUpdatedAtUTCBackfill re-saves every BacklogItem whose
// stored updated_at isn't already UTC, normalizing it in place. Idempotent:
// rows already in UTC (including freshly-created ones, and rows already
// migrated by a prior run) are skipped — a no-op call costs one query. Safe
// to call on a fresh/empty database. Best-effort per row: a single row's
// save failure is logged and does not abort the rest, mirroring
// SetBacklogItemPRAndTransition's secondary-write discipline.
func runBacklogItemUpdatedAtUTCBackfill(ctx context.Context, er *EntRepository) error {
	items, err := er.client.BacklogItem.Query().All(ctx)
	if err != nil {
		// Table may not exist yet (fresh DB before schema.Create) — ignore,
		// mirroring runStatusRemap's same defensive posture.
		return nil //nolint:nilerr
	}

	var migrated int
	for _, item := range items {
		if item.UpdatedAt.Location() == time.UTC {
			continue
		}
		if _, saveErr := er.client.BacklogItem.UpdateOneID(item.ID).
			SetUpdatedAt(item.UpdatedAt.UTC()).
			Save(ctx); saveErr != nil {
			log.WarningLog().Printf("[Migration] backlog item updated_at UTC backfill: item=%s: %v", item.ID, saveErr)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		log.InfoLog().Printf("[Migration] backlog item updated_at UTC backfill: normalized %d row(s)", migrated)
	}
	return nil
}
