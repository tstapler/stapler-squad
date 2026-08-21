package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
)

// defaultSyncInterval is the time between sync ticks.
const defaultSyncInterval = 15 * time.Minute

// syncSourceLocks serializes SyncOne calls per source ID across every
// SyncLoop instance in the process — the long-lived periodic loop and any
// short-lived loop constructed for a manual TriggerSync RPC both consult this
// same map. Without it, a manual sync racing the periodic tick for the same
// source could both miss the same not-yet-created item's external_id lookup
// and both attempt to create it.
var syncSourceLocks sync.Map // map[string]*sync.Mutex

// lockForSource returns the mutex serializing syncs for a given source ID,
// creating one on first use.
func lockForSource(sourceID string) *sync.Mutex {
	m, _ := syncSourceLocks.LoadOrStore(sourceID, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// SyncLoop drives periodic sync of all enabled ItemSources.
type SyncLoop struct {
	storage  *Storage
	registry *PluginRegistry
	interval time.Duration
	stopCh   chan struct{}
	keyFunc  func() ([]byte, error) // provides encryption key for decryption

	// workflowEngine evaluates backlog status transition guards for
	// GitHub-driven sync writes (Phase 2/3 backward sync). Defaults to
	// NewDefaultWorkflowEngine() so existing NewSyncLoop(...) call sites
	// continue to compile unchanged.
	workflowEngine WorkflowEngine
}

// NewSyncLoop creates a SyncLoop with the default interval and no key provider.
func NewSyncLoop(storage *Storage, registry *PluginRegistry) *SyncLoop {
	return &SyncLoop{
		storage:        storage,
		registry:       registry,
		interval:       defaultSyncInterval,
		stopCh:         make(chan struct{}),
		keyFunc:        nil,
		workflowEngine: NewDefaultWorkflowEngine(),
	}
}

// NewSyncLoopWithKeyProvider creates a SyncLoop with a key provider for decryption.
func NewSyncLoopWithKeyProvider(storage *Storage, registry *PluginRegistry, keyFunc func() ([]byte, error)) *SyncLoop {
	return &SyncLoop{
		storage:        storage,
		registry:       registry,
		interval:       defaultSyncInterval,
		stopCh:         make(chan struct{}),
		keyFunc:        keyFunc,
		workflowEngine: NewDefaultWorkflowEngine(),
	}
}

// Start runs the sync loop until ctx is cancelled or Stop is called.
func (sl *SyncLoop) Start(ctx context.Context) {
	ticker := time.NewTicker(sl.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sl.stopCh:
			return
		case <-ticker.C:
			sl.runAllSources(ctx)
		}
	}
}

// Stop gracefully shuts down the sync loop.
// Safe to call multiple times.
func (sl *SyncLoop) Stop() {
	select {
	case <-sl.stopCh:
		// Already closed, do nothing
	default:
		close(sl.stopCh)
	}
}

// runAllSources fetches all enabled sources and syncs each one.
func (sl *SyncLoop) runAllSources(ctx context.Context) {
	sources, err := sl.storage.ListItemSources(ctx)
	if err != nil {
		log.ErrorLog.Printf("[SyncLoop] ListItemSources error: %v", err)
		return
	}

	for i := range sources {
		src := &sources[i]
		if !src.Enabled {
			continue
		}
		// We need the raw ent.ItemSource for ent field access; call through the ent repo.
		entSrc, entErr := sl.storage.repo.GetItemSourceByID(ctx, src.ID)
		if entErr != nil {
			log.ErrorLog.Printf("[SyncLoop] GetItemSourceByID(%s) error: %v", src.ID, entErr)
			continue
		}
		if syncErr := sl.SyncOne(ctx, entSrc); syncErr != nil {
			log.ErrorLog.Printf("[SyncLoop] SyncOne(%s plugin=%s) error: %v", src.ID, src.PluginID, syncErr)
		}
	}
}

// DecryptConfigToken decrypts an encrypted token in config JSON if needed.
// If the config has "encrypted":true, it decrypts the token field using the provided key function.
// If decryption is not available or not needed, returns the raw config unchanged.
// Exported so package server/services (holding a *SyncLoop handle) can call it
// cross-package for the forward-sync subscriber.
func (sl *SyncLoop) DecryptConfigToken(raw string) (string, error) {
	if raw == "" {
		return raw, nil
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, nil // Not JSON; pass through as-is
	}

	encrypted, _ := cfg["encrypted"].(bool)
	if !encrypted {
		return raw, nil // Not encrypted; pass through
	}

	// Need to decrypt
	if sl.keyFunc == nil {
		return "", fmt.Errorf("config has encrypted token but no key provider available")
	}

	encToken, _ := cfg["token"].(string)
	if encToken == "" {
		return raw, nil // No token to decrypt
	}

	key, err := sl.keyFunc()
	if err != nil {
		return "", fmt.Errorf("get key: %w", err)
	}

	plainToken, err := DecryptToken(key, encToken)
	if err != nil {
		return "", fmt.Errorf("decrypt token: %w", err)
	}

	// Build decrypted config JSON (remove encrypted flag, replace token with plaintext)
	cfg["token"] = plainToken
	delete(cfg, "encrypted")

	decrypted, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("re-encode config: %w", err)
	}

	return string(decrypted), nil
}

// SyncByID looks up an ItemSource by ID and syncs it, regardless of its
// Enabled flag — unlike the periodic loop (runAllSources), which only syncs
// enabled sources, this is for an explicit manual/on-demand trigger where the
// caller already decided to sync this specific source.
func (sl *SyncLoop) SyncByID(ctx context.Context, sourceID string) error {
	entSrc, err := sl.storage.repo.GetItemSourceByID(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("SyncByID: %w", err)
	}
	return sl.SyncOne(ctx, entSrc)
}

// maxBackwardSyncPreviewSamples bounds the sample titles PreviewBackwardSyncImpact
// returns for display in the Settings confirmation dialog (Epic 4.4, Story 4.4.1).
const maxBackwardSyncPreviewSamples = 5

// PreviewBackwardSyncImpactByID looks up an ItemSource by ID and previews the
// impact of enabling backward sync for it — see PreviewBackwardSyncImpact.
func (sl *SyncLoop) PreviewBackwardSyncImpactByID(ctx context.Context, sourceID string) (itemCount int, sampleTitles []string, possiblyIncomplete bool, err error) {
	entSrc, err := sl.storage.repo.GetItemSourceByID(ctx, sourceID)
	if err != nil {
		return 0, nil, false, fmt.Errorf("PreviewBackwardSyncImpactByID: %w", err)
	}
	return sl.PreviewBackwardSyncImpact(ctx, entSrc)
}

// PreviewBackwardSyncImpact reports how many already-imported items for
// source would immediately transition — per determineBackwardSyncTarget,
// ADR-002 — if backward sync were enabled for it right now. Used to gate the
// Settings UI's first-enable confirmation dialog (Epic 4.4, resolving
// Unresolved Question #3) so a user can see the blast radius of already-closed
// linked issues before opting in, rather than a silent bulk-archive on the
// same tick the toggle flips.
//
// Read-only: reuses the same decrypted-token/plugin-Fetch path SyncOne uses,
// but does not advance source.SyncCursor, does not record a SourceSyncEvent,
// and does not itself gate on source.BackwardSyncEnabled — the entire point
// is to preview what WOULD happen if it were enabled. Deliberately fetches
// with an empty cursor rather than source.SyncCursor: the preview needs the
// full current state of already-imported items' issues (a source may have
// been forward-syncing for a while before backward sync is ever considered,
// advancing the cursor well past issues that are now closed but haven't
// changed since).
//
// If the plugin implements PaginatedFetcher, the full result set is fetched
// across all pages (bounded by the plugin's own cap) rather than just the
// newest page — GitHub's Issues API sorts by `created` descending by
// default, so a single-page Fetch would silently miss older closed issues on
// repos with more than one page of history, undercounting the blast radius.
// possiblyIncomplete is true when the underlying fetch hit its page cap,
// meaning the count/titles returned are a lower bound, not exhaustive.
func (sl *SyncLoop) PreviewBackwardSyncImpact(ctx context.Context, source *ent.ItemSource) (itemCount int, sampleTitles []string, possiblyIncomplete bool, err error) {
	er := sl.storage.repo

	plugin, ok := sl.registry.Get(source.PluginID)
	if !ok {
		return 0, nil, false, fmt.Errorf("no plugin registered for plugin_id %q", source.PluginID)
	}

	decryptedConfig, err := sl.DecryptConfigToken(source.Config)
	if err != nil {
		return 0, nil, false, fmt.Errorf("decrypt config: %w", err)
	}

	cfg := PluginConfig{Raw: decryptedConfig}

	var items []ExternalItem
	if paginated, ok := plugin.(PaginatedFetcher); ok {
		var fetchErr error
		items, _, possiblyIncomplete, fetchErr = paginated.FetchAll(ctx, cfg, "")
		if fetchErr != nil {
			return 0, nil, false, fmt.Errorf("fetch: %w", fetchErr)
		}
	} else {
		var fetchErr error
		items, _, fetchErr = plugin.Fetch(ctx, cfg, "")
		if fetchErr != nil {
			return 0, nil, false, fmt.Errorf("fetch: %w", fetchErr)
		}
	}

	closedExternalIDs := make([]string, 0, len(items))
	for _, extItem := range items {
		if extItem.State == "closed" {
			closedExternalIDs = append(closedExternalIDs, extItem.ExternalID)
		}
	}

	// Batch the local-item lookup instead of one query per closed issue.
	existingByExternalID, lookupErr := er.GetBacklogItemsByExternalIDs(ctx, source.ID.String(), closedExternalIDs)
	if lookupErr != nil {
		return 0, nil, false, fmt.Errorf("batch lookup backlog items: %w", lookupErr)
	}

	for _, externalID := range closedExternalIDs {
		existing, found := existingByExternalID[externalID]
		if !found {
			// Not a locally-imported item — excluded rather than counted.
			continue
		}
		if _, ok := determineBackwardSyncTarget(BacklogStatus(existing.Status)); ok {
			itemCount++
			if len(sampleTitles) < maxBackwardSyncPreviewSamples {
				sampleTitles = append(sampleTitles, existing.Title)
			}
		}
	}

	return itemCount, sampleTitles, possiblyIncomplete, nil
}

// SyncOne fetches and upserts items for a single ItemSource. Concurrent calls
// for the same source (e.g. a manual TriggerSync racing the periodic tick)
// are serialized via a per-source lock — see syncSourceLocks.
func (sl *SyncLoop) SyncOne(ctx context.Context, source *ent.ItemSource) error {
	mu := lockForSource(source.ID.String())
	mu.Lock()
	defer mu.Unlock()

	start := time.Now()

	er := sl.storage.repo

	plugin, ok := sl.registry.Get(source.PluginID)
	if !ok {
		return fmt.Errorf("no plugin registered for plugin_id %q", source.PluginID)
	}

	// Decrypt config if needed before passing to plugin
	decryptedConfig, err := sl.DecryptConfigToken(source.Config)
	if err != nil {
		return fmt.Errorf("decrypt config: %w", err)
	}

	cfg := PluginConfig{Raw: decryptedConfig}
	cursor := source.SyncCursor

	items, newCursor, fetchErr := plugin.Fetch(ctx, cfg, cursor)
	if fetchErr != nil {
		// Record the failed attempt so it's visible in sync history instead of
		// vanishing silently — GetSyncHistory would otherwise show nothing for
		// a sync run that failed outright.
		if evErr := er.CreateSourceSyncEvent(ctx, source.ID.String(), cursor, 0, 0, 0, 1, fetchErr.Error(), start, time.Now()); evErr != nil {
			log.ErrorLog.Printf("[SyncLoop] CreateSourceSyncEvent(%s) error: %v", source.ID, evErr)
		}
		return fmt.Errorf("fetch: %w", fetchErr)
	}

	var created, updated, skipped, errored int

	for _, extItem := range items {
		data := plugin.MapToBacklogItem(extItem, source.ID.String())

		// Check if an item with this external_id already exists for this source.
		// external_id (e.g. a GitHub issue/PR number) is only unique within its
		// source — two different repos can both have an issue #1 — so the lookup
		// must never match across sources.
		existing, lookupErr := er.GetBacklogItemByExternalID(ctx, source.ID.String(), extItem.ExternalID)
		if lookupErr != nil && !errors.Is(lookupErr, ErrNotFound) {
			log.ErrorLog.Printf("[SyncLoop] GetBacklogItemByExternalID(%s) error: %v", extItem.ExternalID, lookupErr)
			errored++
			continue
		}

		if errors.Is(lookupErr, ErrNotFound) || existing == nil {
			// New item — create it.
			if _, createErr := sl.storage.CreateBacklogItem(ctx, data); createErr != nil {
				log.ErrorLog.Printf("[SyncLoop] CreateBacklogItem external_id=%s error: %v", extItem.ExternalID, createErr)
				errored++
				continue
			}
			created++
			continue
		}

		// Existing item — apply local-wins: only update fields not in UserModifiedFields.
		modifiedFields := ParseUserModifiedFields(existing.UserModifiedFields)

		update := BacklogItemUpdate{}
		anyField := false
		// anyChange tracks whether the closed/reopened blocks below already
		// accounted for this item this tick (via their own updated/errored/
		// skipped increment, or — for the reopened log-only case — the
		// updated++ alongside its watermark write). Those blocks bypass
		// anyField/BacklogItemUpdate entirely, so without this flag the
		// generic `if !anyField { skipped++ }` fallback below would ALSO fire
		// for the same item, double-counting it across created+updated+
		// skipped+errored and breaking the SourceSyncEvent aggregate's
		// partition-of-item-count invariant.
		anyChange := false

		if !ContainsModifiedField(modifiedFields, "title") {
			update.Title = &data.Title
			anyField = true
		}
		if !ContainsModifiedField(modifiedFields, "description") {
			update.Description = &data.Description
			anyField = true
		}
		if !ContainsModifiedField(modifiedFields, "priority") {
			update.Priority = &data.Priority
			anyField = true
		}
		// Status is always local-wins once user_modified_status_at is set.
		// Status transitions are only done via TransitionBacklogItemStatus — no update here.

		// Epic 2.3 (backward sync, AC4 part 3): Labels are gated on BOTH the
		// per-source BackwardSyncEnabled opt-in AND local-wins via
		// UserModifiedFields — see the 2026-08-03 validation-pass correction on
		// Task 2.3.1a: without the BackwardSyncEnabled gate, Labels would sync
		// unconditionally regardless of the per-source opt-in, unlike the
		// status blocks below (Epic 2.1/2.2), which already gate on it.
		if source.BackwardSyncEnabled && !ContainsModifiedField(modifiedFields, "labels") {
			update.Labels = &data.Labels
			anyField = true
		}

		// Epic 2.4 (AC6): ExternalURL is backfilled unconditionally — never
		// gated by BackwardSyncEnabled or UserModifiedFields, per ADR-001
		// Decision 1. This is a deliberate asymmetry with the Labels block
		// above: an item's provenance link is not something a user "edits" in
		// the same sense as content fields, so it always gets filled in once
		// known. Kept structurally independent of the gated blocks.
		if existing.ExternalURL == "" && data.ExternalURL != "" {
			update.ExternalURL = &data.ExternalURL
			anyField = true
		}

		// Epic 2.1/2.2 (AC4, ADR-002) + Phase 3 (AC7, ADR-003): backward-sync
		// status handling for closed/reopened issues. Status transitions bypass
		// BacklogItemUpdate/anyField entirely — they use
		// TransitionBacklogItemStatus, a different call, per the "Status is
		// always local-wins" comment above, which this block supersedes with
		// real logic for the GitHub-driven case. These blocks do their own
		// updated++/skipped++/errored++ accounting alongside the field-update
		// counters above/below — both are legitimate, independent counts of
		// different kinds of change for the same item in the same tick.
		if source.BackwardSyncEnabled && extItem.State == "closed" {
			// extItem.IssueUpdatedAt can be the zero time.Time if GitHub's
			// updated_at failed to parse (see IssueUpdatedAt's assignment in
			// backlog_plugin_github.go's Fetch) or was never populated. A zero
			// value is never After() a real watermark, so treating it as a
			// normal timestamp would either (a) make alreadyReconciled
			// short-circuit forever even against a real, older watermark, or
			// (b) persist a zero watermark that then suppresses all future
			// reprocessing. Skip the loop-prevention/watermark logic entirely
			// this tick rather than risk either.
			if extItem.IssueUpdatedAt.IsZero() {
				log.WarningLog.Printf("[SyncLoop] backward-sync skip item=%s: GitHub issue_updated_at is missing/unparseable (zero time) — skipping loop-prevention/watermark logic this tick", existing.ID)
			} else {
				alreadyReconciled := existing.GithubSyncedIssueUpdatedAt != nil && !extItem.IssueUpdatedAt.After(*existing.GithubSyncedIssueUpdatedAt)
				if !alreadyReconciled {
					anyChange = true
					advanceWatermark := true
					if target, ok := determineBackwardSyncTarget(BacklogStatus(existing.Status)); ok {
						guardInput := BacklogItemTransitionInput{
							Status:            BacklogStatus(existing.Status),
							AcCriteria:        AcCriteriaJSON(existing.AcceptanceCriteria),
							PlanApproved:      existing.PlanApproved,
							SkipPlanning:      existing.SkipPlanning,
							PlanArtifactsPath: existing.PlanArtifactsPath,
						}
						if GuardedTransitionAllowed(sl.workflowEngine, guardInput, target) {
							if _, transErr := sl.storage.TransitionBacklogItemStatus(ctx, existing.ID.String(), target, nil, TriggeredByGitHubSync); transErr != nil { //nolint:silenttransition retried next sync tick — advanceWatermark stays false so alreadyReconciled won't suppress reprocessing; errored++ also surfaces via CreateSourceSyncEvent's aggregate count
								log.WarningLog.Printf("[SyncLoop] backward-sync transition failed item=%s: %v", existing.ID, transErr)
								errored++
								advanceWatermark = false
							} else {
								updated++
							}
						} else {
							log.InfoLog.Printf("[SyncLoop] backward-sync skip item=%s status=%s (no valid target for closed issue)", existing.ID, existing.Status)
							skipped++
						}
					} else {
						log.InfoLog.Printf("[SyncLoop] backward-sync skip item=%s status=%s (mid-flight or terminal, no auto-archive)", existing.ID, existing.Status)
						skipped++
						// Nothing changed locally — don't advance the watermark, or a
						// later manual revert to a pre-work status (with no further
						// GitHub-side change) would have alreadyReconciled short-circuit
						// before determineBackwardSyncTarget is even consulted again,
						// permanently suppressing an otherwise-legitimate auto-archive.
						// Same fix pattern as the transition-failure branch above.
						advanceWatermark = false
					}
					if advanceWatermark {
						watermark := extItem.IssueUpdatedAt
						if _, wmErr := sl.storage.UpdateBacklogItem(ctx, existing.ID.String(), BacklogItemUpdate{GitHubSyncedIssueUpdatedAt: &watermark}, nil); wmErr != nil {
							log.WarningLog.Printf("[SyncLoop] backward-sync watermark update failed item=%s: %v", existing.ID, wmErr)
						}
					}
				}
			}
		}

		if source.BackwardSyncEnabled && extItem.State == "open" && (existing.Status == string(BacklogStatusArchived) || existing.Status == string(BacklogStatusDone)) {
			// See the zero-time guard comment in the closed-issue block above —
			// same rationale applies here.
			if extItem.IssueUpdatedAt.IsZero() {
				log.WarningLog.Printf("[SyncLoop] backward-sync skip item=%s: GitHub issue_updated_at is missing/unparseable (zero time) — skipping loop-prevention/watermark logic this tick", existing.ID)
			} else {
				alreadyLogged := existing.GithubSyncedIssueUpdatedAt != nil && !extItem.IssueUpdatedAt.After(*existing.GithubSyncedIssueUpdatedAt)
				if !alreadyLogged {
					log.InfoLog.Printf("[SyncLoop] GitHub issue reopened; backlog item=%s is %s — reopen manually to re-triage (no automatic action taken)", existing.ID, existing.Status)
					watermark := extItem.IssueUpdatedAt
					if _, wmErr := sl.storage.UpdateBacklogItem(ctx, existing.ID.String(), BacklogItemUpdate{GitHubSyncedIssueUpdatedAt: &watermark}, nil); wmErr != nil {
						log.WarningLog.Printf("[SyncLoop] backward-sync watermark update failed item=%s: %v", existing.ID, wmErr)
					} else {
						// This is a real (if content-free) change to the item's
						// sync state — count it as updated rather than letting it
						// fall into the generic skipped++ fallback below, which is
						// reserved for "nothing happened this tick".
						anyChange = true
						updated++
					}
				}
			}
		}

		if !anyField && !anyChange {
			skipped++
			continue
		}
		if !anyField {
			// anyChange is true: the closed/reopened block above already
			// recorded this item's outcome (updated/errored/skipped) — there
			// are no content fields to apply via BacklogItemUpdate below.
			continue
		}

		if _, updateErr := sl.storage.UpdateBacklogItem(ctx, existing.ID.String(), update, nil); updateErr != nil {
			log.ErrorLog.Printf("[SyncLoop] UpdateBacklogItem %s error: %v", existing.ID, updateErr)
			errored++
			continue
		}
		updated++
	}

	// Advance the cursor and record the SourceSyncEvent atomically — see
	// FinishSourceSync's doc comment for why these must not be separate writes.
	now := time.Now()
	if finishErr := er.FinishSourceSync(ctx, source.ID.String(), newCursor, created, updated, skipped, errored, start, now); finishErr != nil {
		log.ErrorLog.Printf("[SyncLoop] FinishSourceSync(%s) error: %v", source.ID, finishErr)
	}

	log.InfoLog.Printf("[SyncLoop] source=%s plugin=%s created=%d updated=%d skipped=%d errored=%d",
		source.ID, source.PluginID, created, updated, skipped, errored)
	return nil
}

// determineBackwardSyncTarget implements ADR-002's policy: a closed GitHub
// issue maps to BacklogStatusArchived for pre-work statuses only. It never
// targets "done" (would require porting HasUnshippedCode/OverallOutcome
// computation cross-package and risks conflating "closed" with "shipped").
// Returns ok=false when there is no valid target under this policy (item is
// already done/archived, or mid-flight in_progress/review/pr_pending).
func determineBackwardSyncTarget(current BacklogStatus) (target BacklogStatus, ok bool) {
	switch current {
	case BacklogStatusIdea, BacklogStatusRefining, BacklogStatusReady, BacklogStatusQueued:
		return BacklogStatusArchived, true
	default:
		return "", false
	}
}

// ParseUserModifiedFields deserializes UserModifiedFields JSON (e.g. ["title","description"]).
func ParseUserModifiedFields(raw string) []string {
	if raw == "" {
		return nil
	}
	var fields []string
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil
	}
	return fields
}

// ContainsModifiedField returns true if name is in the fields slice.
func ContainsModifiedField(fields []string, name string) bool {
	for _, f := range fields {
		if f == name {
			return true
		}
	}
	return false
}

// MergeUserModifiedFields adds newFields to the existing JSON-encoded set of
// user-modified field names, deduplicating, and returns the re-serialized JSON.
func MergeUserModifiedFields(raw string, newFields ...string) (string, error) {
	existing := ParseUserModifiedFields(raw)

	seen := make(map[string]bool, len(existing)+len(newFields))
	merged := make([]string, 0, len(existing)+len(newFields))
	for _, f := range existing {
		if !seen[f] {
			seen[f] = true
			merged = append(merged, f)
		}
	}
	for _, f := range newFields {
		if !seen[f] {
			seen[f] = true
			merged = append(merged, f)
		}
	}

	out, err := json.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("failed to marshal user modified fields: %w", err)
	}
	return string(out), nil
}
