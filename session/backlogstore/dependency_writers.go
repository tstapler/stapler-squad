package backlogstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogitemdependency"
)

// AddBacklogItemDependency records that edge.BlockedID may not be
// dequeued/started until edge.BlockerID reaches a resolved status — see
// UnresolvedBlockerItemIDs for what counts as resolved (done or archived).
// Adding an already-existing pair is a no-op (upsert against the unique
// (blocker_id, blocked_id) index). Returns ErrDependencyCycle if the new
// edge would create a cycle, including the degenerate self-dependency case
// (BlockerID == BlockedID).
func (b *Backlog) AddBacklogItemDependency(ctx context.Context, edge backlogdto.BacklogItemDependencyEdge) error {
	blockerID, err := b.ResolveBacklogItemLookup(ctx, edge.BlockerID)
	if err != nil {
		return fmt.Errorf("%w: invalid blocker id %q: %v", domain.ErrNotFound, edge.BlockerID, err)
	}
	blockedID, err := b.ResolveBacklogItemLookup(ctx, edge.BlockedID)
	if err != nil {
		return fmt.Errorf("%w: invalid blocked id %q: %v", domain.ErrNotFound, edge.BlockedID, err)
	}
	if blockerID == blockedID {
		return backlogdto.ErrDependencyCycle
	}

	// The existence check, cycle check, and insert must all observe the same
	// consistent snapshot of the dependency graph and commit atomically —
	// otherwise two concurrent calls adding opposite-direction edges can each
	// pass the cycle check before either commits, together closing a cycle
	// the check was supposed to prevent (TOCTOU race). Run everything inside
	// one transaction, following the pattern established by MarkStuck above.
	tx, err := b.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("add backlog item dependency: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	existingIDs, err := tx.BacklogItem.Query().
		Where(backlogitem.IDIn(blockerID, blockedID)).
		IDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify backlog items exist for dependency %s -> %s: %w", edge.BlockerID, edge.BlockedID, err)
	}
	found := make(map[uuid.UUID]bool, len(existingIDs))
	for _, id := range existingIDs {
		found[id] = true
	}
	if !found[blockerID] {
		return fmt.Errorf("%w: blocker item %s does not exist", domain.ErrNotFound, edge.BlockerID)
	}
	if !found[blockedID] {
		return fmt.Errorf("%w: blocked item %s does not exist", domain.ErrNotFound, edge.BlockedID)
	}

	wouldCycle, err := dependencyReachable(ctx, tx.BacklogItemDependency, blockedID, blockerID)
	if err != nil {
		return fmt.Errorf("failed to check backlog item dependency cycle: %w", err)
	}
	if wouldCycle {
		return backlogdto.ErrDependencyCycle
	}

	err = tx.BacklogItemDependency.Create().
		SetBlockerID(blockerID).
		SetBlockedID(blockedID).
		OnConflictColumns(backlogitemdependency.FieldBlockerID, backlogitemdependency.FieldBlockedID).
		DoNothing().
		Exec(ctx)
	// On SQLite, DO NOTHING on a conflicting row leaves the RETURNING clause
	// with no row to return, which the driver surfaces as sql.ErrNoRows
	// rather than a constraint error — treat it the same as the no-op it is.
	if err != nil && !ent.IsConstraintError(err) && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to add backlog item dependency %s -> %s: %w", edge.BlockerID, edge.BlockedID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("add backlog item dependency: commit: %w", err)
	}
	return nil
}

// RemoveBacklogItemDependency deletes the blocker->blocked edge. Idempotent:
// a missing edge is a no-op. Both items must exist (ErrNotFound otherwise) so
// a typo'd ID is not silently reported as success. Removing an edge cannot
// create a cycle, so no graph check is needed.
func (b *Backlog) RemoveBacklogItemDependency(ctx context.Context, edge backlogdto.BacklogItemDependencyEdge) error {
	blockerID, err := b.ResolveBacklogItemLookup(ctx, edge.BlockerID)
	if err != nil {
		return fmt.Errorf("%w: invalid blocker id %q: %v", domain.ErrNotFound, edge.BlockerID, err)
	}
	blockedID, err := b.ResolveBacklogItemLookup(ctx, edge.BlockedID)
	if err != nil {
		return fmt.Errorf("%w: invalid blocked id %q: %v", domain.ErrNotFound, edge.BlockedID, err)
	}
	existingIDs, err := b.client.BacklogItem.Query().
		Where(backlogitem.IDIn(blockerID, blockedID)).
		IDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify backlog items exist for dependency %s -> %s: %w", edge.BlockerID, edge.BlockedID, err)
	}
	found := make(map[uuid.UUID]bool, len(existingIDs))
	for _, id := range existingIDs {
		found[id] = true
	}
	if !found[blockerID] {
		return fmt.Errorf("%w: blocker item %s does not exist", domain.ErrNotFound, edge.BlockerID)
	}
	if !found[blockedID] {
		return fmt.Errorf("%w: blocked item %s does not exist", domain.ErrNotFound, edge.BlockedID)
	}
	if _, err := b.client.BacklogItemDependency.Delete().
		Where(
			backlogitemdependency.BlockerIDEQ(blockerID),
			backlogitemdependency.BlockedIDEQ(blockedID),
		).
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to remove backlog item dependency %s -> %s: %w", edge.BlockerID, edge.BlockedID, err)
	}
	return nil
}

// dependencyReachable reports whether target is reachable from start by
// following existing blocker->blocked edges forward (a breadth-first
// traversal of "what does this item block, transitively"). Used to detect
// whether adding a new blocker->blocked edge would close a cycle: the new
// edge closes a cycle exactly when blocked (start) can already reach blocker
// (target) through existing edges. Takes the BacklogItemDependency client
// explicitly (rather than reading b.client) so callers can pass a
// transaction's client and have the traversal observe that transaction's
// consistent snapshot instead of racing concurrent commits.
func dependencyReachable(ctx context.Context, client *ent.BacklogItemDependencyClient, start, target uuid.UUID) (bool, error) {
	visited := map[uuid.UUID]bool{start: true}
	frontier := []uuid.UUID{start}
	for len(frontier) > 0 {
		rows, err := client.Query().
			Where(backlogitemdependency.BlockerIDIn(frontier...)).
			All(ctx)
		if err != nil {
			return false, err
		}
		var next []uuid.UUID
		for _, row := range rows {
			if row.BlockedID == target {
				return true, nil
			}
			if !visited[row.BlockedID] {
				visited[row.BlockedID] = true
				next = append(next, row.BlockedID)
			}
		}
		frontier = next
	}
	return false, nil
}
