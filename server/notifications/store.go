package notifications

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
)

const (
	// MaxNotifications is the maximum number of notifications to retain.
	MaxNotifications = 500
	// MaxNotificationAge is the maximum age of a notification before it is pruned.
	MaxNotificationAge = 7 * 24 * time.Hour

	// notifTypeApprovalNeeded is the NOTIFICATION_TYPE_APPROVAL_NEEDED enum value from
	// session/v1/types.proto (value = 1). APPROVAL_NEEDED notifications ARE deduplicated
	// by (sessionID, notificationType) like all other types, but with one special step:
	// when collapsing into an existing unread record the record ID is updated to the
	// incoming approval UUID. This preserves the SetMetadata correlation requirement
	// (outcome-stamping uses the notification record ID to find the record) while
	// preventing multiple "x3" cards for a session that issues several approval requests.
	notifTypeApprovalNeeded = int32(1)
	// notifTypeAutoApproved matches NOTIFICATION_TYPE_AUTO_APPROVED = 13 in types.proto.
	notifTypeAutoApproved = int32(13)

	// The following mirror session/v1/types.proto's NotificationType enum values
	// that notificationMapping.ts's ACTIONABLE_TYPES classifies as "needs a
	// decision" (approval_needed, question, error, task_failed, warning) — see
	// IsActionableType below.
	notifTypeInputRequired    = int32(2) // NOTIFICATION_TYPE_INPUT_REQUIRED (maps to UI "question")
	notifTypeConfirmationNeed = int32(3) // NOTIFICATION_TYPE_CONFIRMATION_NEEDED (maps to UI "approval_needed")
	notifTypeError            = int32(7) // NOTIFICATION_TYPE_ERROR
	notifTypeWarning          = int32(8) // NOTIFICATION_TYPE_WARNING
	notifTypeFailure          = int32(9) // NOTIFICATION_TYPE_FAILURE (maps to UI "error")

	// priorityUrgent/priorityHigh mirror sessionv1.NotificationPriority's URGENT(4)/HIGH(3)
	// values as raw ints, matching the mirror-constant pattern server/push/
	// trigger_constants.go already uses to avoid a proto import here.
	priorityUrgent = int32(4)
	priorityHigh   = int32(3)
)

// IsActionableType reports whether t is one of the backend NotificationType values
// that map to a UI type NeedsDecisionSection/NotificationPanel treat as "needs a
// decision" (notificationMapping.ts's ACTIONABLE_TYPES: approval_needed, question,
// error, task_failed, warning). Mirrors that TS set; there is no single
// cross-language source of truth, so a change to one must be mirrored in the other
// (same accepted pattern as NotificationRecord.IsReconciled()).
func IsActionableType(t int32) bool {
	switch t {
	case notifTypeApprovalNeeded, notifTypeConfirmationNeed, notifTypeInputRequired,
		notifTypeError, notifTypeFailure, notifTypeWarning:
		return true
	default:
		return false
	}
}

// NotificationRecord is the persisted representation of a notification event.
type NotificationRecord struct {
	ID               string            `json:"id"`
	SessionID        string            `json:"session_id"`
	SessionName      string            `json:"session_name"`
	NotificationType int32             `json:"notification_type"`
	Priority         int32             `json:"priority"`
	Title            string            `json:"title"`
	Message          string            `json:"message"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	IsRead           bool              `json:"is_read"`
	ReadAt           *time.Time        `json:"read_at,omitempty"`
	// OccurrenceCount tracks how many deduplicated occurrences this record represents.
	// A value of 0 (zero-value from old JSON) should be treated as 1 by consumers.
	OccurrenceCount int `json:"occurrence_count,omitempty"`
	// LastOccurredAt is the timestamp of the most recent occurrence. May differ from
	// CreatedAt which tracks the first occurrence.
	LastOccurredAt *time.Time `json:"last_occurred_at,omitempty"`
	// SessionScoped marks this record as originating from a specific, real session
	// (as opposed to a backlog-item-level or global notification whose SessionID field
	// is overloaded to carry a backlog item ID for coalescing purposes). Only records
	// with SessionScoped==true and no Metadata["item_id"] are eligible for orphan pruning
	// via PruneOrphaned — see events.MetadataKeySessionScoped.
	SessionScoped bool `json:"session_scoped,omitempty"`
}

// IsReconciled reports whether this record's approval was resolved by
// rule-reconciliation rather than a live human decision. Single source of
// truth for the "reconciled" metadata key's value convention.
func (r *NotificationRecord) IsReconciled() bool {
	return r.Metadata["reconciled"] == "true"
}

// notificationsFile is the JSON file format for persisted notifications.
type notificationsFile struct {
	Version       int                   `json:"version"`
	UpdatedAt     time.Time             `json:"updated_at"`
	Notifications []*NotificationRecord `json:"notifications"`
}

// ListOptions controls filtering and pagination for List operations.
type ListOptions struct {
	Limit      int
	Offset     int
	TypeFilter *int32
	SessionID  string
	UnreadOnly bool
}

// NotificationHistoryStore persists notification records to a JSON file
// with in-memory caching for fast reads.
type NotificationHistoryStore struct {
	filePath string
	mu       sync.RWMutex
	records  []*NotificationRecord

	// existenceChecker, when non-nil, is called at most once per orphanPruneInterval
	// from enforceRetention to batch-fetch the set of currently-existing session IDs
	// for orphan pruning. See SetSessionExistenceLookup and PruneOrphaned.
	existenceChecker func() map[string]struct{}
	// lastOrphanPruneAt tracks the last time the orphan-pruning sweep ran, gating it
	// to at most once per orphanPruneInterval even though enforceRetention runs on
	// every Append().
	lastOrphanPruneAt time.Time
}

// orphanPruneInterval is the minimum time between orphan-pruning sweeps triggered
// from enforceRetention. This decouples the (relatively expensive, batch-fetch)
// existence check from firing on every single Append().
const orphanPruneInterval = 1 * time.Minute

// NewNotificationHistoryStore creates a new store, loading existing data from disk.
// If the file does not exist or is corrupted, the store starts empty.
func NewNotificationHistoryStore(filePath string) (*NotificationHistoryStore, error) {
	// Ensure the parent directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create notifications directory: %w", err)
	}

	store := &NotificationHistoryStore{
		filePath: filePath,
		records:  make([]*NotificationRecord, 0),
	}

	if err := store.loadFromDisk(); err != nil {
		log.Warn("NotificationStore failed to load from disk, starting empty", "err", err)
		store.records = make([]*NotificationRecord, 0)
	}

	// Enforce retention limits on load in case file was manually edited
	store.enforceRetention()

	// Deduplicate existing records that were persisted before dedup logic was added.
	// This consolidates duplicates (regardless of read state) into single records
	// with accurate counts.
	if err := store.deduplicateExisting(); err != nil {
		log.Warn("NotificationStore failed to deduplicate existing records", "err", err)
	}

	return store, nil
}

// Append adds a notification record, enforces retention limits, and persists to disk.
// If a record with the same (sessionID, notificationType) already exists, the existing
// record is updated in place (occurrence count incremented, metadata refreshed, moved
// to front) instead of inserting a new record, regardless of whether the existing
// record was already read -- except AUTO_APPROVED records, which stay read across
// recurrence since they never surface as an active alert.
//
// For APPROVAL_NEEDED notifications the record ID is also updated to the incoming
// approval UUID so that SetMetadata outcome-stamping (which looks up by record ID)
// continues to correlate with the most recent approval for this session.
func (s *NotificationHistoryStore) Append(record *NotificationRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for exact duplicates by ID (idempotency guard)
	for _, existing := range s.records {
		if existing.ID == record.ID {
			return nil // Already exists, skip
		}
	}

	// Check for a duplicate by (sessionID, notificationType) and collapse.
	if existing := s.findDuplicate(record.SessionID, record.NotificationType); existing != nil {
		// For APPROVAL_NEEDED: update the record ID to the new approval UUID so that
		// SetMetadata("newApprovalID", ...) can still find this record after collapse.
		if record.NotificationType == notifTypeApprovalNeeded {
			existing.ID = record.ID
		}
		// Update existing record with latest data
		existing.OccurrenceCount++
		existing.LastOccurredAt = &record.CreatedAt
		existing.Message = record.Message
		existing.Metadata = record.Metadata
		existing.Title = record.Title
		// A recurrence means the event happened again -- surface it as unread again,
		// except for AUTO_APPROVED, which stays silently pre-read across recurrence.
		if record.NotificationType != notifTypeAutoApproved {
			existing.IsRead = false
			existing.ReadAt = nil
		}
		// Sweep retention on the collapse path too, not just the new-record path below --
		// otherwise a frequently-recurring record only gets swept when some other,
		// unrelated new notification happens to arrive.
		s.enforceRetention()
		// Move updated record to front (newest-first ordering)
		s.moveToFront(existing)
		return s.saveToDisk()
	}

	// No duplicate found -- insert as new record with count=1
	record.OccurrenceCount = 1
	now := record.CreatedAt
	record.LastOccurredAt = &now
	s.records = append([]*NotificationRecord{record}, s.records...)

	s.enforceRetention()

	return s.saveToDisk()
}

// AppendAutoApproved writes a silent NOTIFICATION_TYPE_AUTO_APPROVED record directly to
// history without publishing to the event bus. The record is immediately marked as read so
// it never appears in the active notification feed — only in the auto-handled history view.
func (s *NotificationHistoryStore) AppendAutoApproved(sessionID, sessionName, toolName, filePath, ruleID, ruleName, ruleSource, decision string) error {
	title := "Auto-" + decision + "d: " + toolName
	msg := filePath
	if msg == "" {
		msg = toolName
	}
	now := time.Now()
	record := &NotificationRecord{
		ID:               uuid.New().String(),
		SessionID:        sessionID,
		SessionName:      sessionName,
		NotificationType: notifTypeAutoApproved,
		Priority:         int32(1), // low — no interrupt
		Title:            title,
		Message:          msg,
		IsRead:           true, // pre-read: never surfaces as unread alert
		CreatedAt:        now,
		Metadata: map[string]string{
			"approval_decision":      decision,
			"tool_name":              toolName,
			"classifier_rule_id":     ruleID,
			"classifier_rule_name":   ruleName,
			"classifier_rule_source": ruleSource,
		},
	}
	return s.Append(record)
}

// findDuplicate scans s.records for a record matching the given
// (sessionID, notificationType) key, regardless of read state. Returns nil if no
// match is found. Must be called with the write lock held.
func (s *NotificationHistoryStore) findDuplicate(sessionID string, notifType int32) *NotificationRecord {
	for _, r := range s.records {
		if r.SessionID == sessionID && r.NotificationType == notifType {
			return r
		}
	}
	return nil
}

// moveToFront removes the given record from its current position in s.records
// and prepends it to the front. Must be called with the write lock held.
func (s *NotificationHistoryStore) moveToFront(record *NotificationRecord) {
	for i, r := range s.records {
		if r == record {
			// Remove from current position
			s.records = append(s.records[:i], s.records[i+1:]...)
			// Prepend to front
			s.records = append([]*NotificationRecord{record}, s.records...)
			return
		}
	}
}

// List returns a paginated, filtered slice of notification records and the total count.
func (s *NotificationHistoryStore) List(opts ListOptions) ([]*NotificationRecord, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Apply filters
	var filtered []*NotificationRecord
	for _, r := range s.records {
		if opts.TypeFilter != nil && r.NotificationType != *opts.TypeFilter {
			continue
		}
		if opts.SessionID != "" && r.SessionID != opts.SessionID {
			continue
		}
		if opts.UnreadOnly && r.IsRead {
			continue
		}
		filtered = append(filtered, r)
	}

	totalCount := len(filtered)

	// Apply pagination
	limit := opts.Limit
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > MaxNotifications {
		limit = MaxNotifications
	}

	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(filtered) {
		return []*NotificationRecord{}, totalCount, nil
	}

	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[offset:end], totalCount, nil
}

// MarkRead marks specific notifications as read. If ids is empty, marks all as read.
// Returns the number of records that were marked.
func (s *NotificationHistoryStore) MarkRead(ids []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0

	if len(ids) == 0 {
		// Mark all as read
		for _, r := range s.records {
			if !r.IsRead {
				r.IsRead = true
				r.ReadAt = &now
				count++
			}
		}
	} else {
		// Mark specific IDs
		idSet := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			idSet[id] = struct{}{}
		}
		for _, r := range s.records {
			if _, ok := idSet[r.ID]; ok && !r.IsRead {
				r.IsRead = true
				r.ReadAt = &now
				count++
			}
		}
	}

	if count > 0 {
		if err := s.saveToDisk(); err != nil {
			return count, err
		}
	}

	return count, nil
}

// Clear removes notifications. If before is nil, clears all. Otherwise clears
// notifications created before the given time. Returns the number cleared.
// Clear deletes notification records — every record when before is nil, or
// only those created before the given cutoff otherwise. It never removes a
// record that is unread and actionable (Task 3.1.5c), regardless of the
// before-timestamp cutoff: a still-pending approval_needed/question/error/
// task_failed/warning decision must only leave the store by being resolved,
// never by "Clear all"/"Clear history" on the Notifications page or the
// header bell dropdown. Returns the number of records actually removed.
func (s *NotificationHistoryStore) Clear(before *time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	originalLen := len(s.records)

	var kept []*NotificationRecord
	for _, r := range s.records {
		if !r.IsRead && IsActionableType(r.NotificationType) {
			kept = append(kept, r) // never delete a still-pending decision
			continue
		}
		if before == nil {
			continue // "clear everything" still respects the guard above
		}
		if !r.CreatedAt.Before(*before) {
			kept = append(kept, r)
		}
	}
	s.records = kept

	cleared := originalLen - len(s.records)
	if cleared > 0 {
		if err := s.saveToDisk(); err != nil {
			return cleared, err
		}
	}

	return cleared, nil
}

// SetSessionExistenceLookup registers the batch-fetch function used by the orphan-pruning
// sweep in enforceRetention to determine which session IDs currently exist. Pass nil to
// disable orphan pruning.
func (s *NotificationHistoryStore) SetSessionExistenceLookup(fn func() map[string]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.existenceChecker = fn
}

// PruneOrphaned removes records that are positively marked session-scoped
// (SessionScoped==true), carry no item_id (Metadata["item_id"] == ""), and whose
// SessionID is absent from existingSessionIDs()'s returned set. existingSessionIDs is
// called exactly ONCE per call (a single batch fetch), never once per record. Returns
// the number of records removed.
func (s *NotificationHistoryStore) PruneOrphaned(existingSessionIDs func() map[string]struct{}) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := s.pruneOrphanedRecords(existingSessionIDs)
	if removed > 0 {
		if err := s.saveToDisk(); err != nil {
			return removed, err
		}
	}
	return removed, nil
}

// pruneOrphanedRecords assumes s.mu is already held by the caller. Calls existingSessionIDs()
// exactly once (a single batch fetch) and checks each candidate record via in-memory map
// membership.
func (s *NotificationHistoryStore) pruneOrphanedRecords(existingSessionIDs func() map[string]struct{}) int {
	if existingSessionIDs == nil {
		return 0
	}
	existing := existingSessionIDs()
	if existing == nil {
		// Not ready to judge existence this pass — treat as "prune nothing," never as
		// "nothing exists" (a nil map is a distinct sentinel from a real, merely-empty map).
		return 0
	}
	var kept []*NotificationRecord
	removed := 0
	for _, r := range s.records {
		if r.SessionScoped && r.Metadata["item_id"] == "" {
			if _, ok := existing[r.SessionID]; !ok {
				removed++
				continue
			}
		}
		kept = append(kept, r)
	}
	s.records = kept
	return removed
}

// SetMetadata updates a single metadata key on the notification record with the given ID.
// A no-op (not an error) if the record does not exist, since the notification may have
// been pruned or the approval pre-dates history tracking.
func (s *NotificationHistoryStore) SetMetadata(id, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.records {
		if r.ID == id {
			if r.Metadata == nil {
				r.Metadata = make(map[string]string)
			}
			r.Metadata[key] = value
			return s.saveToDisk()
		}
	}
	return nil // record not found — silently ignored
}

// GetByID returns a copy of the record with the given ID, if present. The
// returned record (including its Metadata map) is safe to read after this
// call returns without racing concurrent SetMetadata calls on the same ID —
// it does not alias the store's live record or its Metadata map.
func (s *NotificationHistoryStore) GetByID(id string) (*NotificationRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.records {
		if r.ID == id {
			cp := *r
			if r.Metadata != nil {
				cp.Metadata = make(map[string]string, len(r.Metadata))
				for k, v := range r.Metadata {
					cp.Metadata[k] = v
				}
			}
			return &cp, true
		}
	}
	return nil, false
}

// GetUnreadCount returns the number of unread notifications. After server-side
// deduplication (Task 1.2), each unread record represents a distinct
// (sessionID, notificationType) group, so this count reflects the number of
// deduplicated unread groups -- not raw event occurrences.
func (s *NotificationHistoryStore) GetUnreadCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, r := range s.records {
		if !r.IsRead {
			count++
		}
	}
	return count
}

// deduplicateExisting consolidates duplicate records regardless of read state with the
// same (sessionID, notificationType) into the newest one. This runs once on startup to
// clean up pre-dedup persisted data. Idempotent -- safe to run multiple times.
func (s *NotificationHistoryStore) deduplicateExisting() error {
	// Group records by (sessionID, notificationType), regardless of read state.
	type dedupKey struct {
		sessionID string
		notifType int32
	}
	groups := make(map[dedupKey][]*NotificationRecord)
	for _, r := range s.records {
		key := dedupKey{sessionID: r.SessionID, notifType: r.NotificationType}
		groups[key] = append(groups[key], r)
	}

	// Find groups with duplicates
	needsSave := false
	toRemove := make(map[*NotificationRecord]bool)
	for _, group := range groups {
		if len(group) <= 1 {
			continue
		}
		needsSave = true
		// Records are in newest-first order (from s.records). The first in the group
		// is the newest -- keep it and merge the rest into it.
		keeper := group[0]
		totalCount := keeper.OccurrenceCount
		if totalCount == 0 {
			totalCount = 1 // Backward compat: old records with 0 count represent 1
		}
		for _, dup := range group[1:] {
			dupCount := dup.OccurrenceCount
			if dupCount == 0 {
				dupCount = 1
			}
			totalCount += dupCount
			toRemove[dup] = true
		}
		keeper.OccurrenceCount = totalCount
		// LastOccurredAt is already correct on the newest record (or nil for old data).
		// If nil, set it to CreatedAt.
		if keeper.LastOccurredAt == nil {
			t := keeper.CreatedAt
			keeper.LastOccurredAt = &t
		}
	}

	if !needsSave {
		return nil
	}

	// Remove duplicates from s.records, preserving order
	var cleaned []*NotificationRecord
	for _, r := range s.records {
		if !toRemove[r] {
			cleaned = append(cleaned, r)
		}
	}
	s.records = cleaned

	return s.saveToDisk()
}

// DemoteExpiredUrgency downgrades URGENT-priority records whose urgency has aged past ttl
// (measured from LastOccurredAt, falling back to CreatedAt, same effective-time rule as
// enforceRetention) from URGENT to HIGH. It never deletes, archives, or marks a record
// read — urgency is time-bound ("a 1-hour-old notification is no longer urgent") but
// importance isn't, so a demoted record stays exactly as visible in the unread/in-app
// list, just no longer eligible for push (server/push/subscriber.go's shouldNotify gates
// push on priority == URGENT). Returns the number of records demoted and persists to disk
// if any changed.
func (s *NotificationHistoryStore) DemoteExpiredUrgency(now time.Time, ttl time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	demoted := 0
	for _, r := range s.records {
		if r.Priority != priorityUrgent {
			continue
		}
		effective := r.CreatedAt
		if r.LastOccurredAt != nil {
			effective = *r.LastOccurredAt
		}
		if now.Sub(effective) >= ttl {
			r.Priority = priorityHigh
			demoted++
		}
	}

	if demoted > 0 {
		if err := s.saveToDisk(); err != nil {
			log.Warn("NotificationHistoryStore: failed to persist urgency demotion", "err", err)
		}
	}
	return demoted
}

// enforceRetention trims records to MaxNotifications and prunes expired entries.
// Must be called with the write lock held.
func (s *NotificationHistoryStore) enforceRetention() {
	now := time.Now()
	cutoff := now.Add(-MaxNotificationAge)

	// Prune expired entries. The cutoff is keyed on LastOccurredAt (falling back to
	// CreatedAt for pre-migration records with no LastOccurredAt) so a still-recurring
	// record isn't pruned just because its first occurrence aged out.
	var kept []*NotificationRecord
	for _, r := range s.records {
		effective := r.CreatedAt
		if r.LastOccurredAt != nil {
			effective = *r.LastOccurredAt
		}
		if effective.After(cutoff) || effective.Equal(cutoff) {
			kept = append(kept, r)
		}
	}
	s.records = kept

	// Trim to max count
	if len(s.records) > MaxNotifications {
		s.records = s.records[:MaxNotifications]
	}

	// Orphan pruning: batch-fetch existing session IDs at most once per
	// orphanPruneInterval (never per-record, never on every single Append()) and
	// remove session-scoped records whose session no longer exists.
	if s.existenceChecker != nil && time.Since(s.lastOrphanPruneAt) >= orphanPruneInterval {
		s.lastOrphanPruneAt = now
		if removed := s.pruneOrphanedRecords(s.existenceChecker); removed > 0 {
			log.Info("NotificationHistoryStore: pruned orphaned records", "count", removed)
		}
	}
}

// loadFromDisk loads the JSON file into memory. On parse error, logs a warning
// and returns an error (caller should start with empty state).
func (s *NotificationHistoryStore) loadFromDisk() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, start empty
		}
		return fmt.Errorf("read notifications file: %w", err)
	}

	if len(data) == 0 {
		return nil // Empty file, start empty
	}

	var file notificationsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse notifications file: %w", err)
	}

	s.records = file.Notifications
	if s.records == nil {
		s.records = make([]*NotificationRecord, 0)
	}

	return nil
}

// saveToDisk writes the current records to disk using atomic write (temp file + rename).
// Must be called with the write lock held.
func (s *NotificationHistoryStore) saveToDisk() error {
	file := notificationsFile{
		Version:       1,
		UpdatedAt:     time.Now(),
		Notifications: s.records,
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal notifications: %w", err)
	}

	// Atomic write: write to temp file, sync, rename
	tmpPath := s.filePath + ".tmp"
	// #nosec G304 -- s.filePath is configDir/notifications.json (see server.go), built from
	// the internal config dir and a literal filename; never network/RPC input.
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath) // best-effort cleanup; we're already returning the real write error
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath) // best-effort cleanup; we're already returning the real sync error
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup; we're already returning the real close error
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.filePath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup; we're already returning the real rename error
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
