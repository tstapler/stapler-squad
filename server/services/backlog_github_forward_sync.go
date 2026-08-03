package services

// backlog_github_forward_sync.go — EventBus subscriber implementing forward
// sync (AC3): closing the linked GitHub issue when a backlog item transitions
// to done, if the item's source has ForwardSyncEnabled. Covers plan.md's
// Phase 1 Epics 1.2 (subscriber) and 1.3 (bot comment, via PostIssueComment).

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// externalIssueCloser is implemented by ItemSourcePlugins that support
// closing their linked external issue and leaving a comment explaining the
// automated action. Currently only *session.GitHubIssuesPlugin implements
// this; *session.GitHubPRsPlugin does not, so the subscriber's type assertion
// cleanly no-ops for PR-backed sources (interface-pollution guard — see
// TestForwardSyncSubscriber_NoOpWhenPluginDoesNotImplementCloser).
type externalIssueCloser interface {
	CloseIssue(ctx context.Context, config session.PluginConfig, externalID string, existingLabels []string, closeLabel string) (time.Time, error)
	PostIssueComment(ctx context.Context, config session.PluginConfig, externalID string, body string) error
}

// forwardSyncCloseComment is posted on the GitHub issue after an automated
// close, per the "no silent automated action" convention (pitfalls research
// §7 / AC3 Story 1.1.2).
const forwardSyncCloseComment = "Closed automatically — the linked backlog item was marked done in stapler-squad."

// StartBacklogGitHubForwardSyncSubscriber subscribes to the EventBus and
// closes the GitHub issue linked to a backlog item when that item transitions
// to done, if the item's source has ForwardSyncEnabled. Mirrors
// analytics.StartAnalyticsSubscriber's skeleton (server/analytics/subscriber.go).
//
// registry and syncLoop are threaded in as separate parameters — rather than
// deriving registry from syncLoop.Registry(), as plan.md's original sketch
// assumed — because *session.SyncLoop has no exported registry accessor and
// deps.SyncLoop (server/dependencies.go) is always nil in the current
// dependency graph (the live periodic SyncLoop is owned internally by
// session.BacklogController). Callers should pass
// deps.BacklogService.Registry() and deps.BacklogService.SyncLoopForForwardSync()
// instead — see server.go's wiring — which share the same plugin registry and
// key provider TriggerSync already uses.
func StartBacklogGitHubForwardSyncSubscriber(ctx context.Context, bus *events.EventBus, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage) {
	if bus == nil || registry == nil || syncLoop == nil || storage == nil {
		log.Warn("backlog_github_forward_sync: missing dependency, subscriber not started")
		return
	}

	ch, _ := bus.Subscribe(ctx)
	go func() {
		log.Info("backlog_github_forward_sync: subscriber started")
		defer log.Info("backlog_github_forward_sync: subscriber stopped")
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return
				}
				if event == nil || event.Type != events.EventBacklogItemChanged {
					continue
				}
				payload := event.BacklogItemPayload
				if payload == nil || payload.Kind != events.BacklogChangeStatusTransition || payload.NewStatus != string(session.BacklogStatusDone) {
					continue
				}
				handleForwardSyncClose(ctx, registry, syncLoop, storage, payload.Item)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// handleForwardSyncClose closes item's linked GitHub issue and leaves a bot
// comment, if the item's source has ForwardSyncEnabled and its plugin
// supports closing issues. Failures are logged and recorded via
// storage.RecordSourceSyncFailure rather than propagated — this runs from an
// EventBus subscriber goroutine with no caller to return an error to.
//
// Pre-mortem P2 #5 (skip close+comment, log instead, when the issue is
// already known closed) is deliberately NOT implemented here:
// session.BacklogItemData has no stored external-state field (only
// ExternalURL/ExternalID), so cheaply checking "is this issue already closed"
// would require either a schema change (out of scope — Phase 0 already
// landed) or an extra live GitHub API call per done-transition. Per plan.md's
// explicit allowance, this P2 refinement is deferred to a follow-up rather
// than blocking Phase 1.
func handleForwardSyncClose(ctx context.Context, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage, item *session.BacklogItemData) {
	if item == nil || item.ID == "" {
		return
	}

	// Re-fetch by ID rather than trusting item.SourceID/ExternalID/Labels off
	// the event payload directly: EntRepository.TransitionBacklogItemStatus
	// reloads the item via a plain BacklogItem.Get (no .WithSource()) before
	// publishing, so the payload's Item snapshot can have an empty SourceID
	// even for a genuinely source-linked item — GetBacklogItem's query does
	// eager-load the Source edge, so this decouples forward-sync correctness
	// from that reload detail instead of silently no-op'ing on every
	// done-transition (discovered via TestForwardSyncSubscriber_* failing
	// against a real ent-backed Storage; see PR notes).
	current, err := storage.GetBacklogItem(ctx, item.ID)
	if err != nil {
		log.Debug("backlog_github_forward_sync: item lookup failed, skip", "item", item.ID, "err", err)
		return
	}
	if current.SourceID == "" || current.ExternalID == "" {
		return // locally-created item, nothing to sync
	}

	source, err := storage.GetItemSourceByID(ctx, current.SourceID)
	if err != nil {
		log.Debug("backlog_github_forward_sync: source lookup failed, skip", "item", current.ID, "source_id", current.SourceID, "err", err)
		return
	}
	if !source.ForwardSyncEnabled {
		return
	}

	plugin, ok := registry.Get(source.PluginID)
	if !ok {
		log.Debug("backlog_github_forward_sync: no plugin registered, skip", "plugin_id", source.PluginID, "item", current.ID)
		return
	}
	closer, ok := plugin.(externalIssueCloser)
	if !ok {
		log.Info("backlog_github_forward_sync: plugin does not support closing issues, skip", "plugin_id", source.PluginID, "item", current.ID)
		return
	}

	decryptedConfig, err := syncLoop.DecryptConfigToken(source.Config)
	if err != nil {
		log.Warn("backlog_github_forward_sync: decrypt token failed", "source_id", source.ID, "err", err)
		return
	}
	config := session.PluginConfig{Raw: decryptedConfig}

	issueUpdatedAt, closeErr := closer.CloseIssue(ctx, config, current.ExternalID, current.Labels, source.ForwardSyncCloseLabel)
	if closeErr != nil {
		log.Warn("backlog_github_forward_sync: close issue failed", "item", current.ID, "err", closeErr)
		if recErr := storage.RecordSourceSyncFailure(ctx, source.ID, fmt.Sprintf("forward-sync close failed for item %s: %v", current.ID, closeErr)); recErr != nil {
			log.Error("backlog_github_forward_sync: failed to record sync failure", "source_id", source.ID, "err", recErr)
		}
		return
	}

	if commentErr := closer.PostIssueComment(ctx, config, current.ExternalID, forwardSyncCloseComment); commentErr != nil {
		// The close itself already succeeded — a failed follow-up comment is
		// best-effort and must not block persisting the loop-prevention
		// watermark below.
		log.Warn("backlog_github_forward_sync: post comment failed", "item", current.ID, "err", commentErr)
	}

	// Advance the loop-prevention watermark using GitHub's own timestamp, not
	// local wall-clock — see ADR-003 and pre-mortem P1 #1. Only fall back to
	// local time in the narrow case CloseIssue couldn't parse a response.
	watermark := issueUpdatedAt
	if watermark.IsZero() {
		watermark = time.Now().UTC()
	}
	if _, err := storage.UpdateBacklogItem(ctx, current.ID, session.BacklogItemUpdate{GitHubSyncedIssueUpdatedAt: &watermark}, nil); err != nil {
		log.Warn("backlog_github_forward_sync: failed to persist watermark", "item", current.ID, "err", err)
	}
}
