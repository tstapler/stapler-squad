package backlogstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
)

// ResolveBacklogItemLookup resolves a caller-supplied BacklogItem identifier
// — either a legacy raw UUID string or a bl_-prefixed public id
// (BacklogItemID, see session/backlog_item_id.go) — to the row's underlying
// primary-key uuid.UUID. Every call site that touches a BacklogItem id
// string replaces its bare uuid.Parse(id) with b.ResolveBacklogItemLookup(ctx, id)
// and keeps its own subsequent error-wrapping line unchanged (Story 1.3,
// project_plans/backlog-deep-linking/implementation/plan.md task 1): the
// legacy-UUID branch below is exactly uuid.Parse, so every existing
// wrap/sentinel-error site (e.g. GetBacklogItem's ErrNotFound wrapping)
// behaves identically for a malformed string, and now also for a
// public_id that resolves to no row — both surface as the same error the
// call site already turns into "not found" today.
//
// A public_id lookup costs one extra indexed query (by the unique public_id
// column) to translate it to the row's id before the caller's existing
// id-based query runs; this keeps the public_id branch in exactly one place
// instead of duplicating it across every one of the ~30 call sites this
// story wires.
//
// Enumeration of every other uuid.Parse/uuid.UUID call site touching a raw
// id string in this repo (closing plan.md Story 1.3 Task 0, tracked as an
// open item in adversarial-review.md): server/services/*.go's other
// uuid.Parse calls (PipelineMode, Workflow, ItemSource ids) parse distinct
// entity types, not BacklogItem, so they're correctly left on the legacy
// path. Every consumer of a BacklogItem id string -- MCP tools
// (server/mcp/tools_backlog.go), the deep-link resolver
// (server/services/deep_link_resolver.go's resolveLocal), and the review/
// triage/lifecycle update paths -- funnels through Storage.GetBacklogItem or
// another EntRepository method that already calls ResolveBacklogItemLookup,
// so no other call site needs its own dual-ID branch.
func (b *Backlog) ResolveBacklogItemLookup(ctx context.Context, id string) (uuid.UUID, error) {
	if backlogdto.IsBacklogItemIDShape(id) {
		item, err := b.client.BacklogItem.Query().
			Where(backlogitem.PublicID(id)).
			Only(ctx)
		if err != nil {
			return uuid.UUID{}, err
		}
		return item.ID, nil
	}

	return uuid.Parse(id)
}

func (b *Backlog) backlogItemExists(ctx context.Context, itemID string) (bool, error) {
	id, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, err
	}
	return b.client.BacklogItem.Query().Where(backlogitem.ID(id)).Exist(ctx)
}
