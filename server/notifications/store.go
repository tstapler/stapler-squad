package notifications

import (
	"encoding/json"
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// MaxNotifications is the maximum number of notifications to retain.
	MaxNotifications = 500
	// MaxNotificationAge is the maximum age of a notification before it is pruned.
	MaxNotificationAge = 7 * 24 * time.Hour

	// notifTypeApprovalNeeded is the NOTIFICATION_TYPE_APPROVAL_NEEDED enum value from
	// session/v1/types.proto (value = 1). APPROVAL_NEEDED notifications are excluded from
	// (sessionID, notificationType) deduplication because each approval carries a unique
	// approval_id that must equal the notification record ID so that SetMetadata can stamp
	// the outcome after resolution. Merging records would overwrite approval_id in the
	// metadata, causing SetMetadata to silently no-op and badges to never persist after refresh.
	notifTypeApprovalNeeded = int32(1)
)

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
}

// NewNotificationHistoryStore creates a new store, loading existing data from disk.
// If the file does not exist or is corrupted, the store starts empty.
func NewNotificationHistoryStore(filePath string) (*NotificationHistoryStore, error) {
	// Ensure the parent directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create notifications directory: %w", err)
	}

	store := &NotificationHistoryStore{
		filePath: filePath,
		records:  make([]*NotificationRecord, 0),
	}

	if err := store.loadFromDisk(); err != nil {
		log.WarningLog.Printf("[NotificationStore] Failed to load from disk, starting empty: %v", err)
		store.records = make([]*NotificationRecord, 0)
	}

	// Enforce retention limits on load in case file was manually edited
	store.enforceRetention()

	// Deduplicate existing records that were persisted before dedup logic was added.
	// This consolidates unread duplicates into single records with accurate counts.
	if err := store.deduplicateExisting(); err != nil {
		log.WarningLog.Printf("[NotificationStore] Failed to deduplicate existing records: %v", err)
	}

	return store, nil
}

// Append adds a notification record, enforces retention limits, and persists to disk.
// If an unread record with the same (sessionID, notificationType) already exists,
// the existing record is updated in place (occurrence count incremented, metadata
// refreshed, moved to front) instead of inserting a new record. Per ADR-003, if the
// existing record is already read, a new unread record is created instead.
//
// APPROVAL_NEEDED notifications (type 1) are never deduplicated — see notifTypeApprovalNeeded.
func (s *NotificationHistoryStore) Append(record *NotificationRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for exact duplicates by ID (idempotency guard)
	for _, existing := range s.records {
		if existing.ID == record.ID {
			return nil // Already exists, skip
		}
	}

	// Check for unread duplicate by (sessionID, notificationType).
	// APPROVAL_NEEDED is excluded: each approval has a unique ID that must equal the
	// notification record ID; deduplication would break SetMetadata outcome-stamping.
	if record.NotificationType != notifTypeApprovalNeeded {
		if existing := s.findUnreadDuplicate(record.SessionID, record.NotificationType); existing != nil {
			// Update existing record with latest data
			existing.OccurrenceCount++
			existing.LastOccurredAt = &record.CreatedAt
			existing.Message = record.Message
			existing.Metadata = record.Metadata
			existing.Title = record.Title
			// Move updated record to front (newest-first ordering)
			s.moveToFront(existing)
			return s.saveToDisk()
		}
	}

	// No duplicate found -- insert as new record with count=1
	record.OccurrenceCount = 1
	now := record.CreatedAt
	record.LastOccurredAt = &now
	s.records = append([]*NotificationRecord{record}, s.records...)

	s.enforceRetention()

	return s.saveToDisk()
}

// findUnreadDuplicate scans s.records for an unread record matching the given
// (sessionID, notificationType) key. Returns nil if no match is found.
// Must be called with the write lock held.
func (s *NotificationHistoryStore) findUnreadDuplicate(sessionID string, notifType int32) *NotificationRecord {
	for _, r := range s.records {
		if r.SessionID == sessionID && r.NotificationType == notifType && !r.IsRead {
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
func (s *NotificationHistoryStore) Clear(before *time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	originalLen := len(s.records)

	if before == nil {
		s.records = make([]*NotificationRecord, 0)
	} else {
		var kept []*NotificationRecord
		for _, r := range s.records {
			if !r.CreatedAt.Before(*before) {
				kept = append(kept, r)
			}
		}
		s.records = kept
	}

	cleared := originalLen - len(s.records)
	if cleared > 0 {
		if err := s.saveToDisk(); err != nil {
			return cleared, err
		}
	}

	return cleared, nil
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

// deduplicateExisting consolidates unread records with the same (sessionID, notificationType)
// into the newest one. This runs once on startup to clean up pre-dedup persisted data.
// Read records are not touched. Idempotent -- safe to run multiple times.
func (s *NotificationHistoryStore) deduplicateExisting() error {
	// Group unread records by (sessionID, notificationType)
	type dedupKey struct {
		sessionID string
		notifType int32
	}
	groups := make(map[dedupKey][]*NotificationRecord)
	for _, r := range s.records {
		if r.IsRead {
			continue
		}
		key := dedupKey{sessionID: r.SessionID, notifType: r.NotificationType}
		groups[key] = append(groups[key], r)
	}

	// Find groups with duplicates
	needsSave := false
	toRemove := make(map[*NotificationRecord]bool)
	for _, group := range groups {
		// APPROVAL_NEEDED records are never deduped (see notifTypeApprovalNeeded).
		if group[0].NotificationType == notifTypeApprovalNeeded {
			continue
		}
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

// enforceRetention trims records to MaxNotifications and prunes expired entries.
// Must be called with the write lock held.
func (s *NotificationHistoryStore) enforceRetention() {
	now := time.Now()
	cutoff := now.Add(-MaxNotificationAge)

	// Prune expired entries
	var kept []*NotificationRecord
	for _, r := range s.records {
		if r.CreatedAt.After(cutoff) || r.CreatedAt.Equal(cutoff) {
			kept = append(kept, r)
		}
	}
	s.records = kept

	// Trim to max count
	if len(s.records) > MaxNotifications {
		s.records = s.records[:MaxNotifications]
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
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
