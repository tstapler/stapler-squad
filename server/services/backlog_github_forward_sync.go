package services

// backlog_github_forward_sync.go — EventBus subscriber implementing forward
// sync (AC3): closing the linked GitHub issue (or moving the linked Linear/JIRA
// issue to its target state, ADR-002) when a backlog item transitions to done,
// if the item's source has ForwardSyncEnabled. Covers plan.md's Phase 1 Epics 1.2 (subscriber) and 1.3 (bot comment, via PostIssueComment).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// issueCommenter posts the visible comment that accompanies every automated
// forward-sync action.
type issueCommenter interface {
	PostIssueComment(ctx context.Context, config session.PluginConfig, externalID string, body string) error
}

// externalIssueStateUpdater is implemented by ItemSourcePlugins whose done
// state is a named, per-workflow target (Linear workflow states, JIRA
// transitions) rather than GitHub's binary close. See ADR-002.
type externalIssueStateUpdater interface {
	UpdateIssueState(ctx context.Context, config session.PluginConfig, externalID string, targetState string) (time.Time, error)
	issueCommenter
}

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

// issueStateReader is an optional extension of externalIssueCloser: plugins
// implementing it let handleForwardSyncClose skip the close (and its comment)
// when the issue is already closed.
type issueStateReader interface {
	ReadIssueState(ctx context.Context, config session.PluginConfig, externalID string) (state string, updatedAt time.Time, err error)
}

// forwardSyncCloseTimeout bounds the state-read + close + comment sequence run
// on the subscriber goroutine.
const forwardSyncCloseTimeout = 30 * time.Second

// forwardSyncCloseComment is posted on the GitHub issue after an automated
// close, per the "no silent automated action" convention (pitfalls research
// §7 / AC3 Story 1.1.2).
const forwardSyncCloseComment = "Closed automatically — the linked backlog item was marked done in stapler-squad."

// forwardSyncCloseCommentFor appends the claiming host when known.
func forwardSyncCloseCommentFor(hostID string) string {
	if hostID == "" {
		return forwardSyncCloseComment
	}
	return strings.TrimSuffix(forwardSyncCloseComment, ".") + fmt.Sprintf(" on host `%s`.", hostID)
}

// forwardSyncCloseCommentWithPR appends the implementing PR when known.
func forwardSyncCloseCommentWithPR(hostID, prURL string) string {
	base := forwardSyncCloseCommentFor(hostID)
	if prURL == "" {
		return base
	}
	return strings.TrimSuffix(base, ".") + fmt.Sprintf(" Implemented in: %s.", prURL)
}

// forwardSyncStateComment is the equivalent comment for state-update trackers.
const forwardSyncStateComment = "Moved to %q automatically — the linked backlog item was marked done in stapler-squad."

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
func StartBacklogGitHubForwardSyncSubscriber(ctx context.Context, bus *events.EventBus, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage) <-chan struct{} {
	done := make(chan struct{})
	if bus == nil || registry == nil || syncLoop == nil || storage == nil {
		log.Warn("backlog_github_forward_sync: missing dependency, subscriber not started")
		close(done)
		return done
	}

	ch, _ := bus.Subscribe(ctx)
	go func() {
		defer close(done)
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
				if payload == nil || payload.Kind != events.BacklogChangeStatusTransition || payload.NewStatus != string(domain.BacklogStatusDone) {
					continue
				}
				handleForwardSyncClose(ctx, registry, syncLoop, storage, payload.Item)
			case <-ctx.Done():
				return
			}
		}
	}()
	return done
}

// sourcelessCloseDeps are the seams for items imported without an ItemSource
// (import_github_issue): the plugin that talks to GitHub and the keychain lookup.
type sourcelessCloseDeps struct {
	plugin   externalIssueCloser
	tokenFor func(host string) string
}

// githubIssuesPluginID is the registry ID of the GitHub issues plugin.
const githubIssuesPluginID = "github_issues"

// defaultSourcelessCloseDeps resolves the GitHub issues plugin from registry;
// plugin is nil when it is not registered or cannot close issues.
func defaultSourcelessCloseDeps(registry *session.PluginRegistry) sourcelessCloseDeps {
	deps := sourcelessCloseDeps{tokenFor: githubpkg.GetKeychainTokenForHost}
	if registry == nil {
		return deps
	}
	if p, ok := registry.Get(githubIssuesPluginID); ok {
		if closer, ok := p.(externalIssueCloser); ok {
			deps.plugin = closer
		}
	}
	return deps
}

// forwardSyncAction is everything the done-transition needs to close one item's
// external issue, whichever path resolved it. sourceID is empty for
// sourceless (MCP/web-imported) items, which have no ItemSource to record
// failures against.
type forwardSyncAction struct {
	poster         issueCommenter
	reader         issueStateReader // nil: no already-closed pre-check
	update         func(ctx context.Context, config session.PluginConfig) (time.Time, error)
	config         session.PluginConfig
	externalID     string
	comment        string
	failureSubject string
	sourceID       string
}

// handleForwardSyncClose closes item's linked GitHub issue and leaves a bot
// comment. Items with an ItemSource need ForwardSyncEnabled and a plugin that
// supports closing; items imported via import_github_issue (ExternalURL, no
// SourceID) are closed using the keychain token for the issue's host.
// Failures are logged (and recorded via storage.RecordSourceSyncFailure when
// there is a source) rather than propagated: this runs from an EventBus
// subscriber goroutine, and the item has already reached done.
func handleForwardSyncClose(ctx context.Context, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage, item *backlogdto.BacklogItemData) {
	handleForwardSyncCloseWith(ctx, registry, syncLoop, storage, item, defaultSourcelessCloseDeps(registry))
}

func handleForwardSyncCloseWith(ctx context.Context, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage, item *backlogdto.BacklogItemData, sourceless sourcelessCloseDeps) {
	if item == nil || item.ID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, forwardSyncCloseTimeout)
	defer cancel()

	// Re-fetch by ID rather than trusting item.SourceID/ExternalID/Labels off
	// the event payload directly: EntRepository.TransitionBacklogItemStatus
	// reloads the item via a plain BacklogItem.Get (no .WithSource()) before
	// publishing, so the payload's Item snapshot can have an empty SourceID
	// even for a genuinely source-linked item.
	current, err := storage.GetBacklogItem(ctx, item.ID)
	if err != nil {
		log.Debug("backlog_github_forward_sync: item lookup failed, skip", "item", item.ID, "err", err)
		return
	}

	// Only items with no ItemSource at all take the sourceless path: a
	// source-linked item must honor its source's ForwardSyncEnabled opt-in.
	var action *forwardSyncAction
	switch {
	case current.SourceID == "":
		action = resolveSourcelessCloseAction(current, sourceless)
	case current.ExternalID != "":
		action = resolveSourceCloseAction(ctx, registry, syncLoop, storage, current)
	}
	if action == nil {
		return
	}

	if action.reader != nil {
		state, stateUpdatedAt, stateErr := action.reader.ReadIssueState(ctx, action.config, action.externalID)
		if stateErr != nil {
			log.Debug("backlog_github_forward_sync: read issue state failed, proceeding with close", "item", current.ID, "err", stateErr)
		} else if state == "closed" {
			log.Debug("backlog_github_forward_sync: issue already closed, skipping close", "item", current.ID, "external_url", current.ExternalURL)
			persistForwardSyncWatermark(storage, current.ID, stateUpdatedAt)
			return
		}
	}

	issueUpdatedAt, closeErr := action.update(ctx, action.config)
	if closeErr != nil {
		log.Warn("backlog_github_forward_sync: "+action.failureSubject+" failed", "item", current.ID, "err", closeErr)
		if action.sourceID != "" {
			if recErr := storage.RecordSourceSyncFailure(ctx, action.sourceID, fmt.Sprintf("forward-sync %s failed for item %s: %v", action.failureSubject, current.ID, closeErr)); recErr != nil {
				log.Error("backlog_github_forward_sync: failed to record sync failure", "source_id", action.sourceID, "err", recErr)
			}
		}
		return
	}

	if commentErr := action.poster.PostIssueComment(ctx, action.config, action.externalID, action.comment); commentErr != nil {
		// The close itself already succeeded — a failed follow-up comment is
		// best-effort and must not block persisting the watermark below.
		log.Warn("backlog_github_forward_sync: post comment failed", "item", current.ID, "err", commentErr)
	}
	if action.sourceID == "" {
		log.Info("backlog_github_forward_sync: closed MCP-imported issue", "item", current.ID, "external_url", current.ExternalURL)
	}
	persistForwardSyncWatermark(storage, current.ID, issueUpdatedAt)
}

// persistForwardSyncWatermark advances the loop-prevention watermark using
// GitHub's own timestamp, not local wall-clock — see ADR-003 and pre-mortem P1 #1;
// local time is only the fallback when no response timestamp was available.
// It deliberately uses context.Background(): the GitHub write has already
// happened, so a cancelled subscriber ctx must not drop the watermark.
func persistForwardSyncWatermark(storage *session.Storage, itemID string, issueUpdatedAt time.Time) {
	watermark := issueUpdatedAt
	if watermark.IsZero() {
		watermark = time.Now().UTC()
	}
	if _, err := storage.UpdateBacklogItem(context.Background(), itemID, backlogdto.BacklogItemUpdate{GitHubSyncedIssueUpdatedAt: &watermark}, nil); err != nil {
		log.Warn("backlog_github_forward_sync: failed to persist watermark", "item", itemID, "err", err)
	}
}

// resolveSourceCloseAction handles items ingested from an ItemSource. nil means
// nothing to do.
func resolveSourceCloseAction(ctx context.Context, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage, current *backlogdto.BacklogItemData) *forwardSyncAction {
	source, err := storage.GetItemSourceByID(ctx, current.SourceID)
	if err != nil {
		log.Debug("backlog_github_forward_sync: source lookup failed, skip", "item", current.ID, "source_id", current.SourceID, "err", err)
		return nil
	}
	if !source.ForwardSyncEnabled {
		return nil
	}
	plugin, ok := registry.Get(source.PluginID)
	if !ok {
		log.Debug("backlog_github_forward_sync: no plugin registered, skip", "plugin_id", source.PluginID, "item", current.ID)
		return nil
	}

	action := &forwardSyncAction{externalID: current.ExternalID, sourceID: source.ID}
	switch p := plugin.(type) {
	case externalIssueCloser:
		action.poster, action.comment, action.failureSubject = p, forwardSyncCloseCommentWithPR(current.ClaimingHostID, current.PrURL), "close"
		action.reader, _ = plugin.(issueStateReader)
		action.update = func(ctx context.Context, config session.PluginConfig) (time.Time, error) {
			return p.CloseIssue(ctx, config, current.ExternalID, current.Labels, source.ForwardSyncCloseLabel)
		}
	case externalIssueStateUpdater:
		// ForwardSyncCloseLabel doubles as the target workflow state/transition name.
		if source.ForwardSyncCloseLabel == "" {
			msg := fmt.Sprintf("forward-sync target state not configured for item %s", current.ID)
			log.Warn("backlog_github_forward_sync: "+msg, "source_id", source.ID)
			if recErr := storage.RecordSourceSyncFailure(ctx, source.ID, msg); recErr != nil {
				log.Error("backlog_github_forward_sync: failed to record sync failure", "source_id", source.ID, "err", recErr)
			}
			return nil
		}
		action.poster, action.comment, action.failureSubject = p, fmt.Sprintf(forwardSyncStateComment, source.ForwardSyncCloseLabel), "state update"
		action.update = func(ctx context.Context, config session.PluginConfig) (time.Time, error) {
			return p.UpdateIssueState(ctx, config, current.ExternalID, source.ForwardSyncCloseLabel)
		}
	default:
		log.Info("backlog_github_forward_sync: plugin does not support forward sync, skip", "plugin_id", source.PluginID, "item", current.ID)
		return nil
	}

	decryptedConfig, err := syncLoop.DecryptConfigToken(source.Config)
	if err != nil {
		log.Warn("backlog_github_forward_sync: decrypt token failed", "source_id", source.ID, "err", err)
		return nil
	}
	action.config = session.PluginConfig{Raw: decryptedConfig}
	return action
}

// resolveSourcelessCloseAction handles MCP/web-imported items: a github.com
// issue URL and no ItemSource. Mirrors resolveProgressTarget — GHE hosts are
// skipped (no host list is passed to the parser). nil means nothing to do.
func resolveSourcelessCloseAction(current *backlogdto.BacklogItemData, deps sourcelessCloseDeps) *forwardSyncAction {
	if current.ExternalURL == "" || deps.plugin == nil {
		return nil
	}
	ref, err := githubpkg.ParseGitHubRefWithHosts(current.ExternalURL, nil)
	if err != nil || ref.Type != githubpkg.RefTypeIssue {
		return nil
	}
	if deps.tokenFor(ref.Host) == "" {
		log.Warn("backlog_github_forward_sync: no token for MCP-imported item, skip", "host", ref.Host, "item", current.ID)
		return nil
	}
	raw, err := json.Marshal(map[string]string{"host": ref.Host, "owner": ref.Owner, "repo": ref.Repo})
	if err != nil {
		return nil
	}
	externalID := fmt.Sprint(ref.IssueNumber)
	action := &forwardSyncAction{
		poster:         deps.plugin,
		config:         session.PluginConfig{Raw: string(raw)},
		externalID:     externalID,
		comment:        forwardSyncCloseCommentWithPR(current.ClaimingHostID, current.PrURL),
		failureSubject: "close",
	}
	action.reader, _ = deps.plugin.(issueStateReader)
	action.update = func(ctx context.Context, config session.PluginConfig) (time.Time, error) {
		return deps.plugin.CloseIssue(ctx, config, externalID, current.Labels, "")
	}
	return action
}
