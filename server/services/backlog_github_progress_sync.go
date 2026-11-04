package services

// backlog_github_progress_sync.go — EventBus subscriber that comments on the
// linked GitHub issue when a backlog item starts work (in_progress) or opens a
// PR (review). Gated by the github_issue_progress_comments flag; the done-close
// path lives in backlog_github_forward_sync.go and is not gated by it.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/config"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// githubIssueProgressCommentsFlagName gates the in_progress and review comments.
const githubIssueProgressCommentsFlagName = "github_issue_progress_comments"

// GitHubIssueProgressCommentsEnabled reads the flag live, so a Settings toggle
// applies without a restart.
func GitHubIssueProgressCommentsEnabled() bool {
	return config.LoadConfig().GetFeatureFlagWithDefault(githubIssueProgressCommentsFlagName, featureFlagDefault(githubIssueProgressCommentsFlagName))
}

// issueProgressCommenter posts a comment and returns the issue's post-comment
// updated_at (zero if it could not be read back).
type issueProgressCommenter interface {
	PostProgressComment(ctx context.Context, config session.PluginConfig, externalID, body string) (time.Time, error)
}

// StartBacklogGitHubProgressSyncSubscriber starts the progress-comment
// subscriber. flagEnabled is read per event; registry/syncLoop are the same
// instances the forward-sync subscriber receives.
func StartBacklogGitHubProgressSyncSubscriber(ctx context.Context, bus *events.EventBus, flagEnabled func() bool, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage) <-chan struct{} {
	done := make(chan struct{})
	if bus == nil || flagEnabled == nil || registry == nil || syncLoop == nil || storage == nil {
		log.Warn("backlog_github_progress_sync: missing dependency, subscriber not started")
		close(done)
		return done
	}

	ch, _ := bus.Subscribe(ctx)
	go func() {
		defer close(done)
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
				if payload == nil || payload.Kind != events.BacklogChangeStatusTransition {
					continue
				}
				if payload.NewStatus != string(session.BacklogStatusInProgress) && payload.NewStatus != string(session.BacklogStatusReview) {
					continue
				}
				if !flagEnabled() {
					continue
				}
				handleProgressComment(ctx, registry, syncLoop, storage, payload.Item, payload.NewStatus)
			case <-ctx.Done():
				return
			}
		}
	}()
	return done
}

// handleProgressComment posts the comment for newStatus. Failures are logged
// and swallowed: this runs from a subscriber goroutine and a missing token or
// GitHub outage must never fail the transition that triggered it. Best-effort:
// the dedup status is persisted after the post, so a persist failure can duplicate.
func handleProgressComment(ctx context.Context, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage, item *session.BacklogItemData, newStatus string) {
	if item == nil || item.ID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, progressCommentTimeout)
	defer cancel()
	// Re-fetch: the event's snapshot can predate pr_url being written and lacks the source edge.
	current, err := storage.GetBacklogItem(ctx, item.ID)
	if err != nil || current.ExternalURL == "" {
		return
	}
	if progressCommentAlreadyPosted(current.GitHubLastProgressCommentStatus, newStatus) {
		return
	}

	var body string
	switch newStatus {
	case string(session.BacklogStatusInProgress):
		body = progressStartedComment(current.ClaimingHostID)
	case string(session.BacklogStatusReview):
		if current.PrURL == "" {
			return
		}
		body = progressReviewComment(current.PrURL)
	default:
		return
	}

	poster, cfg, externalID, ok := resolveProgressTarget(ctx, registry, syncLoop, storage, current)
	if !ok {
		return
	}
	issueUpdatedAt, err := poster.PostProgressComment(ctx, cfg, externalID, body)
	if err != nil {
		log.Debug("backlog_github_progress_sync: post failed, skip", "item", current.ID, "status", newStatus, "err", err)
		return
	}

	// A zero updatedAt means the read-back failed: record the dedup status but
	// leave the watermark, since a guessed one could skip real remote edits.
	update := session.BacklogItemUpdate{GitHubLastProgressCommentStatus: &newStatus}
	if !issueUpdatedAt.IsZero() {
		update.GitHubSyncedIssueUpdatedAt = &issueUpdatedAt
	}
	if _, err := storage.UpdateBacklogItem(ctx, current.ID, update, nil); err != nil {
		log.Warn("backlog_github_progress_sync: failed to persist watermark", "item", current.ID, "err", err)
	}
}

// progressCommentAlreadyPosted: in_progress is the first comment in an item's
// life, so any recorded status means it was already announced (a rework cycle
// review -> in_progress must not re-announce); review only dedups against itself.
func progressCommentAlreadyPosted(lastPosted, newStatus string) bool {
	if lastPosted == "" {
		return false
	}
	return newStatus == string(session.BacklogStatusInProgress) || lastPosted == newStatus
}

// progressCommentTimeout bounds the network I/O done on the subscriber goroutine.
const progressCommentTimeout = 15 * time.Second

// progressReviewComment links the PR only when prURL is a canonical github.com
// pull URL, so a malformed stored value is never echoed into a public comment.
func progressReviewComment(prURL string) string {
	const base = "A pull request has been opened for this issue"
	if !strings.HasPrefix(prURL, "https://github.com/") {
		return base + "."
	}
	ref, err := githubpkg.ParseGitHubRefWithHosts(prURL, nil)
	if err != nil || ref.Type != githubpkg.RefTypePR {
		return base + "."
	}
	return fmt.Sprintf("%s: %s", base, prURL)
}

func progressStartedComment(hostID string) string {
	if hostID == "" {
		return "Work has started on this issue in stapler-squad."
	}
	return fmt.Sprintf("Work has started on this issue in stapler-squad on host `%s`.", hostID)
}

// resolveProgressTarget finds where to post for item: its ItemSource's plugin
// config when it has one, otherwise (MCP/web-imported items with no SourceID) a
// github.com config derived from ExternalURL, whose token the plugin takes from
// the keychain (these trust external_url as-is, and GHE hosts are skipped since
// no host list is passed to the parser). ok is false when the item has no postable GitHub issue.
func resolveProgressTarget(ctx context.Context, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage, item *session.BacklogItemData) (poster issueProgressCommenter, cfg session.PluginConfig, externalID string, ok bool) {
	if item.SourceID != "" && item.ExternalID != "" {
		source, err := storage.GetItemSourceByID(ctx, item.SourceID)
		if err != nil {
			return nil, cfg, "", false
		}
		plugin, found := registry.Get(source.PluginID)
		p, isPoster := plugin.(issueProgressCommenter)
		if !found || !isPoster {
			return nil, cfg, "", false
		}
		raw, err := syncLoop.DecryptConfigToken(source.Config)
		if err != nil {
			log.Warn("backlog_github_progress_sync: decrypt token failed", "source_id", source.ID, "err", err)
			return nil, cfg, "", false
		}
		return p, session.PluginConfig{Raw: raw}, item.ExternalID, true
	}

	ref, err := githubpkg.ParseGitHubRefWithHosts(item.ExternalURL, nil)
	if err != nil || ref.Type != githubpkg.RefTypeIssue {
		return nil, cfg, "", false
	}
	raw, err := json.Marshal(map[string]string{"host": ref.Host, "owner": ref.Owner, "repo": ref.Repo})
	if err != nil {
		return nil, cfg, "", false
	}
	return &session.GitHubIssuesPlugin{}, session.PluginConfig{Raw: string(raw)}, fmt.Sprint(ref.IssueNumber), true
}
