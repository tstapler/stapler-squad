package backlogstore

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
)

// backlogItemPublicIDBackfillLockFileName is the flock coordination file
// guarding BackfillBacklogItemPublicIDs against two processes racing to
// mint different public_ids for the same row.
const backlogItemPublicIDBackfillLockFileName = "backlog_item_public_id_backfill.lock"

// backlogItemPublicIDBackfillLockTimeout bounds how long
// BackfillBacklogItemPublicIDs waits to acquire its flock before giving up.
// Mirrors hostRegistryLockTimeout (session/hostclaim/host_registry.go).
const backlogItemPublicIDBackfillLockTimeout = 5 * time.Second

// backfillLockFilePath returns the filesystem path to use for a
// flock-guarded cross-process backfill lock named name, derived from
// the DbPath port. Returns "" when there is no real on-disk directory to
// coordinate on: a "file:" URI DSN (e.g. the shared-cache in-memory
// database some tests use) or an EntRepository constructed via
// NewEntRepositoryFromClient with no dbPath set at all. Callers must treat
// "" as "skip locking" — there is no filesystem location to coordinate on,
// and no plausible second OS process to race against in either case.
func (b *Backlog) backfillLockFilePath(name string) string {
	// A bare ":memory:" DSN has no directory; filepath.Dir would resolve it to the
	// process cwd and litter the working tree with a lock file.
	dbPath := b.dbPath()
	if dbPath == "" || dbPath == ":memory:" || strings.HasPrefix(dbPath, "file:") {
		return ""
	}
	return filepath.Join(filepath.Dir(dbPath), name)
}

// dbPath reads the database path at call time; a Backlog built without the port
// (or over NewEntRepositoryFromClient, which has no path) reports "".
func (b *Backlog) dbPath() string {
	if b.dbPathFn == nil {
		return ""
	}
	return b.dbPathFn()
}

// BackfillBacklogItemPublicIDs assigns a freshly minted BacklogItemID to
// every pre-existing BacklogItem row that predates this feature and so has
// no public_id (Story 1.4,
// project_plans/backlog-deep-linking/implementation/plan.md). Idempotent:
// rows that already have a public_id are excluded by the query itself
// (backlogitem.PublicIDIsNil(), per session/ent/schema/backlog_item.go's
// documented NULL-vs-empty-string representation), so re-running against an
// already-backfilled dataset touches no rows. Guarded by a flock-based
// exclusive lock — see backfillLockFilePath — so two instances racing on the
// same state dir can't each assign a different id to the same row; when no
// real lock path is available the backfill still runs, unguarded, since
// there's no second process to race against in that case.
func (b *Backlog) BackfillBacklogItemPublicIDs(ctx context.Context) error {
	lockPath := b.backfillLockFilePath(backlogItemPublicIDBackfillLockFileName)
	if lockPath == "" {
		return b.backfillBacklogItemPublicIDsLocked(ctx)
	}

	fl := flock.New(lockPath)
	lockCtx, cancel := context.WithTimeout(ctx, backlogItemPublicIDBackfillLockTimeout)
	defer cancel()
	locked, err := fl.TryLockContext(lockCtx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire backlog item public id backfill lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire backlog item public id backfill lock within timeout")
	}
	defer func() { _ = fl.Unlock() }()

	return b.backfillBacklogItemPublicIDsLocked(ctx)
}

// backfillBacklogItemPublicIDsLocked does the actual query-and-save work for
// BackfillBacklogItemPublicIDs. Split out so the exported method can wrap it
// with (or, when no real lock path exists, skip) the flock guard without
// duplicating the query/save logic.
func (b *Backlog) backfillBacklogItemPublicIDsLocked(ctx context.Context) error {
	rows, err := b.client.BacklogItem.Query().
		Where(backlogitem.PublicIDIsNil()).
		All(ctx)
	if err != nil {
		// Table may not exist yet (fresh DB before schema.Create) — ignore,
		// mirroring migrations.PRURLBackfill's same defensive posture.
		return nil //nolint:nilerr
	}

	var migrated int
	for _, row := range rows {
		// Confirm absence per row via the centralizing accessor (not a
		// second raw == "" check) per plan.md's Story 1.4 note — guards
		// against a row that was assigned a public_id (e.g. by a
		// concurrent CreateBacklogItem call) between the query above and
		// this loop iteration.
		data := &backlogdto.BacklogItemData{PublicIDRaw: row.PublicID}
		if _, ok := data.PublicID(); ok {
			continue
		}

		newID, err := backlogdto.NewBacklogItemID()
		if err != nil {
			return fmt.Errorf("failed to mint backlog item public id for row %s: %w", row.ID, err)
		}
		if _, saveErr := b.client.BacklogItem.UpdateOneID(row.ID).
			SetPublicID(newID.String()).
			Save(ctx); saveErr != nil {
			log.WarningLog().Printf("[Migration] backlog item public id backfill: item=%s: %v", row.ID, saveErr)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		log.InfoLog().Printf("[Migration] backlog item public id backfill: populated %d row(s)", migrated)
	}
	return nil
}

// BackfillMissingPRNumbers finds pr_pending items with a pr_url but no
// pr_number (pr_number == 0) and parses the number out of the URL. Such items
// are otherwise permanently invisible to FindPRPendingItems' PrNumberGT(0)
// filter, so ReconcilePRPending never polls them — a real stuck-forever case
// found via manual QA against live data, not something the loop itself would
// ever have surfaced. Best-effort: a URL that doesn't match is left as-is and
// logged, not treated as fatal.
func (b *Backlog) BackfillMissingPRNumbers(ctx context.Context) (int, error) {
	items, err := b.client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(domain.BacklogStatusPRPending)),
			backlogitem.PrNumberEQ(0),
			backlogitem.PrURLNotNil(),
			backlogitem.PrURLNEQ(""),
		).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to query items with missing pr_number: %w", err)
	}

	backfilled := 0
	for _, item := range items {
		m := backlogdto.PrNumberFromURLRe.FindStringSubmatch(item.PrURL)
		if m == nil {
			continue
		}
		num, convErr := strconv.Atoi(m[1])
		if convErr != nil || num <= 0 {
			continue
		}
		updated, updateErr := b.client.BacklogItem.UpdateOneID(item.ID).SetPrNumber(num).Save(ctx)
		if updateErr != nil {
			return backfilled, fmt.Errorf("failed to backfill pr_number for item %s: %w", item.ID, updateErr)
		}
		backfilled++

		// Best-effort publish: never blocks or fails the backfill itself. Found
		// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
		// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
		result := BacklogItemToData(updated)
		b.PublishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"prNumber"},
		})
	}
	return backfilled, nil
}
