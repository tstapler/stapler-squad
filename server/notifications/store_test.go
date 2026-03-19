package notifications

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore creates a NotificationHistoryStore backed by a temp file for testing.
// The caller does not need to clean up -- t.TempDir() handles that.
func newTestStore(t *testing.T) *NotificationHistoryStore {
	t.Helper()
	dir := t.TempDir()
	fp := filepath.Join(dir, "notifications.json")
	store, err := NewNotificationHistoryStore(fp)
	if err != nil {
		t.Fatalf("NewNotificationHistoryStore: %v", err)
	}
	return store
}

// makeRecord creates a minimal NotificationRecord for testing.
func makeRecord(id, sessionID string, notifType int32) *NotificationRecord {
	return &NotificationRecord{
		ID:               id,
		SessionID:        sessionID,
		NotificationType: notifType,
		Title:            "title-" + id,
		Message:          "message-" + id,
		Metadata:         map[string]string{"key": "value-" + id},
		CreatedAt:        time.Now(),
	}
}

// TestAppendDedup_SameSessionAndType verifies that two appends with the same
// (sessionID, notificationType) produce one record with OccurrenceCount=2.
func TestAppendDedup_SameSessionAndType(t *testing.T) {
	store := newTestStore(t)

	r1 := makeRecord("id-1", "session-A", 1)
	r2 := makeRecord("id-2", "session-A", 1)

	if err := store.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}
	if err := store.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	records, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 record, got %d", total)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record in slice, got %d", len(records))
	}
	if records[0].OccurrenceCount != 2 {
		t.Errorf("expected OccurrenceCount=2, got %d", records[0].OccurrenceCount)
	}
}

// TestAppendDedup_DifferentSessions verifies that two different sessions
// create two separate records.
func TestAppendDedup_DifferentSessions(t *testing.T) {
	store := newTestStore(t)

	r1 := makeRecord("id-1", "session-A", 1)
	r2 := makeRecord("id-2", "session-B", 1)

	if err := store.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}
	if err := store.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	_, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 records, got %d", total)
	}
}

// TestAppendDedup_DifferentTypes verifies that the same session with different
// notification types creates two separate records.
func TestAppendDedup_DifferentTypes(t *testing.T) {
	store := newTestStore(t)

	r1 := makeRecord("id-1", "session-A", 1)
	r2 := makeRecord("id-2", "session-A", 2)

	if err := store.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}
	if err := store.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	_, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 records, got %d", total)
	}
}

// TestAppendDedup_ReadThenNew verifies that an existing read record is NOT
// updated; a new unread record is created instead (per ADR-003).
func TestAppendDedup_ReadThenNew(t *testing.T) {
	store := newTestStore(t)

	r1 := makeRecord("id-1", "session-A", 1)
	if err := store.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}

	// Mark the first record as read
	if _, err := store.MarkRead([]string{"id-1"}); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	// Append a second record with the same key
	r2 := makeRecord("id-2", "session-A", 1)
	if err := store.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	records, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 records (read + new unread), got %d", total)
	}

	// The new record should be unread with OccurrenceCount=1
	if len(records) < 1 {
		t.Fatalf("expected at least 1 record, got %d", len(records))
	}
	newest := records[0]
	if newest.IsRead {
		t.Error("newest record should be unread")
	}
	if newest.OccurrenceCount != 1 {
		t.Errorf("newest record OccurrenceCount: expected 1, got %d", newest.OccurrenceCount)
	}

	// The old record should still be read
	oldest := records[1]
	if !oldest.IsRead {
		t.Error("oldest record should still be read")
	}
}

// TestAppendDedup_MetadataUpdated verifies that the latest metadata replaces
// old metadata after a dedup merge.
func TestAppendDedup_MetadataUpdated(t *testing.T) {
	store := newTestStore(t)

	r1 := makeRecord("id-1", "session-A", 1)
	r1.Metadata = map[string]string{"approval_id": "old-approval"}
	r1.Title = "Old Title"
	r1.Message = "Old Message"

	r2 := makeRecord("id-2", "session-A", 1)
	r2.Metadata = map[string]string{"approval_id": "new-approval"}
	r2.Title = "New Title"
	r2.Message = "New Message"

	if err := store.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}
	if err := store.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	records, _, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	rec := records[0]
	if rec.Metadata["approval_id"] != "new-approval" {
		t.Errorf("expected metadata approval_id='new-approval', got '%s'", rec.Metadata["approval_id"])
	}
	if rec.Title != "New Title" {
		t.Errorf("expected Title='New Title', got '%s'", rec.Title)
	}
	if rec.Message != "New Message" {
		t.Errorf("expected Message='New Message', got '%s'", rec.Message)
	}
}

// TestAppendDedup_OccurrenceCountIncrements verifies the count goes 1->2->3
// across 3 appends.
func TestAppendDedup_OccurrenceCountIncrements(t *testing.T) {
	store := newTestStore(t)

	for i := 1; i <= 3; i++ {
		r := makeRecord("id-"+string(rune('0'+i)), "session-A", 1)
		if err := store.Append(r); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	records, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 record, got %d", total)
	}
	if records[0].OccurrenceCount != 3 {
		t.Errorf("expected OccurrenceCount=3, got %d", records[0].OccurrenceCount)
	}
}

// TestAppendDedup_MoveToFront verifies that after a dedup merge, the updated
// record is at index 0 (front of the list).
func TestAppendDedup_MoveToFront(t *testing.T) {
	store := newTestStore(t)

	// Insert record for session-B first (will be at front initially)
	rB := makeRecord("id-B", "session-B", 1)
	if err := store.Append(rB); err != nil {
		t.Fatalf("Append rB: %v", err)
	}

	// Insert record for session-A (will be at front)
	rA1 := makeRecord("id-A1", "session-A", 1)
	if err := store.Append(rA1); err != nil {
		t.Fatalf("Append rA1: %v", err)
	}

	// Now at this point: [session-A, session-B]
	// Insert another session-B record (at position 1, not 0). Dedup should
	// merge into the existing session-B record and move it to the front.
	rB2 := makeRecord("id-B2", "session-B", 1)
	if err := store.Append(rB2); err != nil {
		t.Fatalf("Append rB2: %v", err)
	}

	records, _, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// The session-B record should now be at index 0 (moved to front)
	if records[0].SessionID != "session-B" {
		t.Errorf("expected session-B at index 0, got %s", records[0].SessionID)
	}
	if records[0].OccurrenceCount != 2 {
		t.Errorf("expected OccurrenceCount=2, got %d", records[0].OccurrenceCount)
	}
	if records[1].SessionID != "session-A" {
		t.Errorf("expected session-A at index 1, got %s", records[1].SessionID)
	}
}

// TestAppendDedup_BackwardCompatibility verifies that records loaded from JSON
// with OccurrenceCount=0 are treated correctly (0 means 1 occurrence for
// backward compatibility with old data).
func TestAppendDedup_BackwardCompatibility(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "notifications.json")

	// Write a JSON file with a record that has no occurrence_count field
	// (simulating pre-dedup data)
	oldRecord := &NotificationRecord{
		ID:               "old-id",
		SessionID:        "session-A",
		NotificationType: 1,
		Title:            "Old notification",
		Message:          "Old message",
		CreatedAt:        time.Now(),
	}
	file := notificationsFile{
		Version:       1,
		UpdatedAt:     time.Now(),
		Notifications: []*NotificationRecord{oldRecord},
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(fp, data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Load the store -- should succeed with OccurrenceCount=0
	store, err := NewNotificationHistoryStore(fp)
	if err != nil {
		t.Fatalf("NewNotificationHistoryStore: %v", err)
	}

	records, _, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	// OccurrenceCount=0 is valid -- frontend should interpret 0 as 1
	// The store does NOT retroactively set it to 1 on existing single records.
	// However, if a new record arrives with the same key, the dedup logic
	// will increment from the existing value.
	rec := records[0]
	if rec.OccurrenceCount != 0 {
		t.Logf("Note: old record has OccurrenceCount=%d (0 is expected for pre-dedup data)", rec.OccurrenceCount)
	}

	// Now append a duplicate -- should merge and increment correctly
	r2 := makeRecord("id-2", "session-A", 1)
	if err := store.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	records, _, err = store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record after dedup, got %d", len(records))
	}
	// 0 (old) + 1 increment = 1. This is the raw stored value.
	// The frontend should display max(occurrence_count, 1) for old data.
	if records[0].OccurrenceCount != 1 {
		t.Errorf("expected OccurrenceCount=1 after increment from 0, got %d", records[0].OccurrenceCount)
	}
}

// TestGetUnreadCount_WithDedup verifies that with 3 approval events for
// session A and 2 for session B (all unread), GetUnreadCount returns 2
// (distinct groups, not raw event count).
func TestGetUnreadCount_WithDedup(t *testing.T) {
	store := newTestStore(t)

	// 3 events for session A, type 1
	for i := 0; i < 3; i++ {
		r := makeRecord("a-"+string(rune('0'+i)), "session-A", 1)
		if err := store.Append(r); err != nil {
			t.Fatalf("Append session-A #%d: %v", i, err)
		}
	}

	// 2 events for session B, type 1
	for i := 0; i < 2; i++ {
		r := makeRecord("b-"+string(rune('0'+i)), "session-B", 1)
		if err := store.Append(r); err != nil {
			t.Fatalf("Append session-B #%d: %v", i, err)
		}
	}

	count := store.GetUnreadCount()
	if count != 2 {
		t.Errorf("expected GetUnreadCount=2, got %d", count)
	}

	// Verify the individual counts
	records, _, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}

// TestDeduplicateExisting_Migration verifies that loading a store with 5
// duplicate unread records consolidates them into 1 record with
// OccurrenceCount=5.
func TestDeduplicateExisting_Migration(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "notifications.json")

	// Create 5 duplicate unread records for the same (sessionID, type)
	records := make([]*NotificationRecord, 5)
	baseTime := time.Now()
	for i := 0; i < 5; i++ {
		records[i] = &NotificationRecord{
			ID:               "dup-" + string(rune('0'+i)),
			SessionID:        "session-foo",
			NotificationType: 1,
			Title:            "Approval needed",
			Message:          "msg-" + string(rune('0'+i)),
			Metadata:         map[string]string{"key": "val-" + string(rune('0'+i))},
			CreatedAt:        baseTime.Add(time.Duration(i) * time.Second),
			IsRead:           false,
		}
	}

	file := notificationsFile{
		Version:       1,
		UpdatedAt:     time.Now(),
		Notifications: records,
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(fp, data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Load the store -- deduplicateExisting() should run on startup
	store, err := NewNotificationHistoryStore(fp)
	if err != nil {
		t.Fatalf("NewNotificationHistoryStore: %v", err)
	}

	result, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 record after migration, got %d", total)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 record in slice, got %d", len(result))
	}
	if result[0].OccurrenceCount != 5 {
		t.Errorf("expected OccurrenceCount=5, got %d", result[0].OccurrenceCount)
	}
}

// TestDeduplicateExisting_ReadRecordsUntouched verifies that read records
// are not consolidated during migration, even if they share the same
// (sessionID, notificationType) key.
func TestDeduplicateExisting_ReadRecordsUntouched(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "notifications.json")

	now := time.Now()
	readAt := now.Add(-time.Hour)
	records := []*NotificationRecord{
		{
			ID:               "read-1",
			SessionID:        "session-foo",
			NotificationType: 1,
			Title:            "Read notification",
			CreatedAt:        now.Add(-2 * time.Hour),
			IsRead:           true,
			ReadAt:           &readAt,
		},
		{
			ID:               "read-2",
			SessionID:        "session-foo",
			NotificationType: 1,
			Title:            "Another read notification",
			CreatedAt:        now.Add(-3 * time.Hour),
			IsRead:           true,
			ReadAt:           &readAt,
		},
	}

	file := notificationsFile{
		Version:       1,
		UpdatedAt:     now,
		Notifications: records,
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(fp, data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store, err := NewNotificationHistoryStore(fp)
	if err != nil {
		t.Fatalf("NewNotificationHistoryStore: %v", err)
	}

	result, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Both read records should still be present (not merged)
	if total != 2 {
		t.Errorf("expected 2 read records unchanged, got %d", total)
	}
	for _, r := range result {
		if !r.IsRead {
			t.Errorf("record %s should still be read", r.ID)
		}
	}
}

// TestDeduplicateExisting_MixedReadUnread verifies that migration only
// consolidates unread duplicates, leaving read records intact.
func TestDeduplicateExisting_MixedReadUnread(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "notifications.json")

	now := time.Now()
	readAt := now.Add(-time.Hour)
	records := []*NotificationRecord{
		// 2 unread duplicates (should merge into 1)
		{
			ID:               "unread-1",
			SessionID:        "session-foo",
			NotificationType: 1,
			Title:            "Unread 1",
			CreatedAt:        now.Add(-1 * time.Minute),
			IsRead:           false,
		},
		{
			ID:               "unread-2",
			SessionID:        "session-foo",
			NotificationType: 1,
			Title:            "Unread 2",
			CreatedAt:        now.Add(-2 * time.Minute),
			IsRead:           false,
		},
		// 1 read record (should stay)
		{
			ID:               "read-1",
			SessionID:        "session-foo",
			NotificationType: 1,
			Title:            "Read",
			CreatedAt:        now.Add(-1 * time.Hour),
			IsRead:           true,
			ReadAt:           &readAt,
		},
	}

	file := notificationsFile{
		Version:       1,
		UpdatedAt:     now,
		Notifications: records,
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(fp, data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store, err := NewNotificationHistoryStore(fp)
	if err != nil {
		t.Fatalf("NewNotificationHistoryStore: %v", err)
	}

	result, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// 1 merged unread + 1 read = 2 records
	if total != 2 {
		t.Errorf("expected 2 records (1 merged unread + 1 read), got %d", total)
	}

	unreadCount := 0
	for _, r := range result {
		if !r.IsRead {
			unreadCount++
			if r.OccurrenceCount != 2 {
				t.Errorf("merged unread record OccurrenceCount: expected 2, got %d", r.OccurrenceCount)
			}
		}
	}
	if unreadCount != 1 {
		t.Errorf("expected 1 unread record, got %d", unreadCount)
	}
}
