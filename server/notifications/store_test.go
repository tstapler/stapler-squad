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

// notifTypeDedup is a notification type that participates in (sessionID, notificationType)
// deduplication — any type except APPROVAL_NEEDED (1) which is intentionally excluded.
const notifTypeDedup = int32(10) // INFO

// notifTypeApprovalNeededTest is the value of NOTIFICATION_TYPE_APPROVAL_NEEDED for use in tests.
const notifTypeApprovalNeededTest = int32(1)

// TestAppendDedup_SameSessionAndType verifies that two appends with the same
// (sessionID, notificationType) produce one record with OccurrenceCount=2.
func TestAppendDedup_SameSessionAndType(t *testing.T) {
	store := newTestStore(t)

	r1 := makeRecord("id-1", "session-A", notifTypeDedup)
	r2 := makeRecord("id-2", "session-A", notifTypeDedup)

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
// TestAppendDedup_ReadThenRecur_CollapsesAndUnreads verifies that a recurrence of an
// already-read (sessionID, notificationType) collapses into the existing record in
// place -- bumping OccurrenceCount and resetting it back to unread -- rather than
// forking a second row.
func TestAppendDedup_ReadThenRecur_CollapsesAndUnreads(t *testing.T) {
	store := newTestStore(t)

	r1 := makeRecord("id-1", "session-A", notifTypeDedup)
	if err := store.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}

	// Mark the first record as read
	if _, err := store.MarkRead([]string{"id-1"}); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	// Append a second record with the same key
	r2 := makeRecord("id-2", "session-A", notifTypeDedup)
	if err := store.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	records, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected exactly 1 record (collapsed in place), got %d", total)
	}

	rec := records[0]
	if rec.ID != "id-1" {
		t.Errorf("expected collapsed record to keep original ID 'id-1', got %q", rec.ID)
	}
	if rec.OccurrenceCount != 2 {
		t.Errorf("expected OccurrenceCount=2, got %d", rec.OccurrenceCount)
	}
	if rec.IsRead {
		t.Error("expected collapsed record to be reset to unread")
	}
	if rec.ReadAt != nil {
		t.Error("expected ReadAt to be cleared on collapse")
	}
}

// TestAppendDedup_MetadataUpdated verifies that the latest metadata replaces
// old metadata after a dedup merge.
func TestAppendDedup_MetadataUpdated(t *testing.T) {
	store := newTestStore(t)

	r1 := makeRecord("id-1", "session-A", notifTypeDedup)
	r1.Metadata = map[string]string{"key": "old-value"}
	r1.Title = "Old Title"
	r1.Message = "Old Message"

	r2 := makeRecord("id-2", "session-A", notifTypeDedup)
	r2.Metadata = map[string]string{"key": "new-value"}
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
	if rec.Metadata["key"] != "new-value" {
		t.Errorf("expected metadata key='new-value', got '%s'", rec.Metadata["key"])
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
		r := makeRecord("id-"+string(rune('0'+i)), "session-A", notifTypeDedup)
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
	rB := makeRecord("id-B", "session-B", notifTypeDedup)
	if err := store.Append(rB); err != nil {
		t.Fatalf("Append rB: %v", err)
	}

	// Insert record for session-A (will be at front)
	rA1 := makeRecord("id-A1", "session-A", notifTypeDedup)
	if err := store.Append(rA1); err != nil {
		t.Fatalf("Append rA1: %v", err)
	}

	// Now at this point: [session-A, session-B]
	// Insert another session-B record (at position 1, not 0). Dedup should
	// merge into the existing session-B record and move it to the front.
	rB2 := makeRecord("id-B2", "session-B", notifTypeDedup)
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
		NotificationType: notifTypeDedup,
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
	r2 := makeRecord("id-2", "session-A", notifTypeDedup)
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

	// 3 events for session A, same dedupable type
	for i := 0; i < 3; i++ {
		r := makeRecord("a-"+string(rune('0'+i)), "session-A", notifTypeDedup)
		if err := store.Append(r); err != nil {
			t.Fatalf("Append session-A #%d: %v", i, err)
		}
	}

	// 2 events for session B, same dedupable type
	for i := 0; i < 2; i++ {
		r := makeRecord("b-"+string(rune('0'+i)), "session-B", notifTypeDedup)
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

	// Create 5 duplicate unread records for the same (sessionID, type).
	// Use a dedupable type (not APPROVAL_NEEDED which is excluded from dedup).
	records := make([]*NotificationRecord, 5)
	baseTime := time.Now()
	for i := 0; i < 5; i++ {
		records[i] = &NotificationRecord{
			ID:               "dup-" + string(rune('0'+i)),
			SessionID:        "session-foo",
			NotificationType: notifTypeDedup,
			Title:            "Repeated notification",
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

// TestDeduplicateExisting_ReadRecordsConsolidate verifies that two read records
// sharing the same (sessionID, notificationType) key now consolidate during
// migration too -- read state no longer exempts a record from dedup grouping.
func TestDeduplicateExisting_ReadRecordsConsolidate(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "notifications.json")

	now := time.Now()
	readAt := now.Add(-time.Hour)
	records := []*NotificationRecord{
		{
			ID:               "read-1",
			SessionID:        "session-foo",
			NotificationType: notifTypeDedup,
			Title:            "Read notification",
			CreatedAt:        now.Add(-2 * time.Hour),
			IsRead:           true,
			ReadAt:           &readAt,
		},
		{
			ID:               "read-2",
			SessionID:        "session-foo",
			NotificationType: notifTypeDedup,
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
	if total != 1 {
		t.Fatalf("expected the two read duplicates to consolidate into 1 record, got %d", total)
	}
	if !result[0].IsRead {
		t.Error("consolidated record should still be read")
	}
	if result[0].OccurrenceCount != 2 {
		t.Errorf("expected OccurrenceCount=2, got %d", result[0].OccurrenceCount)
	}
}

// TestDeduplicateExisting_MixedReadUnread verifies that migration consolidates
// duplicates sharing the same (sessionID, notificationType) key regardless of
// read state -- unread and read records in the same group merge into one.
func TestDeduplicateExisting_MixedReadUnread(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "notifications.json")

	now := time.Now()
	readAt := now.Add(-time.Hour)
	records := []*NotificationRecord{
		// 2 unread duplicates + 1 read duplicate, all same key -- all merge into 1.
		{
			ID:               "unread-1",
			SessionID:        "session-foo",
			NotificationType: notifTypeDedup,
			Title:            "Unread 1",
			CreatedAt:        now.Add(-1 * time.Minute),
			IsRead:           false,
		},
		{
			ID:               "unread-2",
			SessionID:        "session-foo",
			NotificationType: notifTypeDedup,
			Title:            "Unread 2",
			CreatedAt:        now.Add(-2 * time.Minute),
			IsRead:           false,
		},
		{
			ID:               "read-1",
			SessionID:        "session-foo",
			NotificationType: notifTypeDedup,
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
	// All 3 share the same key, so they consolidate into 1 record.
	if total != 1 {
		t.Fatalf("expected 1 consolidated record, got %d", total)
	}
	if result[0].OccurrenceCount != 3 {
		t.Errorf("merged record OccurrenceCount: expected 3, got %d", result[0].OccurrenceCount)
	}
	// The keeper is the newest record (unread-1), so the merged record stays unread.
	if result[0].IsRead {
		t.Error("expected merged record (keyed on newest, unread-1) to remain unread")
	}
}

// TestDeduplicateExisting_ReadAndUnreadDuplicates_Consolidate verifies the Story
// 1.1.3 acceptance criterion directly: a read record and an unread record sharing
// the same (sessionID, notificationType) consolidate into one on load, keyed on
// whichever is newest by CreatedAt, with a summed OccurrenceCount.
func TestDeduplicateExisting_ReadAndUnreadDuplicates_Consolidate(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "notifications.json")

	now := time.Now()
	readAt := now.Add(-time.Hour)
	records := []*NotificationRecord{
		{
			ID:               "n6",
			SessionID:        "sess-a1b2c3",
			NotificationType: notifTypeDedup,
			Title:            "Newest (unread)",
			CreatedAt:        now,
			IsRead:           false,
		},
		{
			ID:               "n5",
			SessionID:        "sess-a1b2c3",
			NotificationType: notifTypeDedup,
			Title:            "Oldest (read)",
			CreatedAt:        now.Add(-time.Hour),
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
	if total != 1 {
		t.Fatalf("expected exactly 1 consolidated record, got %d", total)
	}
	if result[0].ID != "n6" {
		t.Errorf("expected keeper to be the newest record 'n6', got %q", result[0].ID)
	}
	if result[0].OccurrenceCount != 2 {
		t.Errorf("expected OccurrenceCount=2, got %d", result[0].OccurrenceCount)
	}
}

// TestAppendDedup_ApprovalNeededDeduped verifies that APPROVAL_NEEDED notifications
// (type 1) ARE deduplicated by (sessionID, notificationType) like other types, but
// the record ID is updated to the latest approval UUID so SetMetadata outcome-stamping
// continues to work. Three sequential approvals for the same unread session must
// collapse into a single record with OccurrenceCount=3 and ID=third approval UUID.
func TestAppendDedup_ApprovalNeededDeduped(t *testing.T) {
	store := newTestStore(t)

	ids := []string{"approval-1", "approval-2", "approval-3"}
	for _, id := range ids {
		r := makeRecord(id, "session-A", notifTypeApprovalNeededTest)
		if err := store.Append(r); err != nil {
			t.Fatalf("Append %s: %v", id, err)
		}
	}

	records, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 deduplicated APPROVAL_NEEDED record, got %d", total)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record in slice, got %d", len(records))
	}
	if records[0].OccurrenceCount != 3 {
		t.Errorf("expected OccurrenceCount=3, got %d", records[0].OccurrenceCount)
	}
	// Record ID must be updated to the latest approval UUID for SetMetadata correlation.
	if records[0].ID != "approval-3" {
		t.Errorf("expected record ID='approval-3' (latest approval), got '%s'", records[0].ID)
	}
}

// TestSetMetadata_ApprovalDecisionStamping verifies that SetMetadata correctly stamps
// the approval_decision on a notification record when the record ID equals the approval ID.
// This is the core mechanism for persisting resolution badges across page refreshes.
func TestSetMetadata_ApprovalDecisionStamping(t *testing.T) {
	store := newTestStore(t)

	approvalID := "approval-uuid-1"
	r := &NotificationRecord{
		ID:               approvalID, // notification ID == approval ID by convention
		SessionID:        "session-A",
		NotificationType: notifTypeApprovalNeededTest,
		Title:            "Permission Required: Bash",
		Message:          "git commit -m 'test'",
		Metadata:         map[string]string{"approval_id": approvalID, "tool_name": "Bash"},
		CreatedAt:        time.Now(),
	}
	if err := store.Append(r); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Simulate ResolveApproval stamping the decision
	if err := store.SetMetadata(approvalID, "approval_decision", "allow"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}

	records, _, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if got := records[0].Metadata["approval_decision"]; got != "allow" {
		t.Errorf("expected approval_decision='allow', got '%s'", got)
	}
}

// TestSetMetadata_Dedup_SecondApprovalStamped verifies that when two APPROVAL_NEEDED
// notifications are created for the same unread session, they collapse into a single
// record whose ID is updated to the second approval UUID. SetMetadata using that second
// UUID must stamp the merged record correctly.
func TestSetMetadata_Dedup_SecondApprovalStamped(t *testing.T) {
	store := newTestStore(t)

	approvalID1 := "approval-uuid-1"
	approvalID2 := "approval-uuid-2"

	r1 := &NotificationRecord{
		ID:               approvalID1,
		SessionID:        "session-A",
		NotificationType: notifTypeApprovalNeededTest,
		Metadata:         map[string]string{"approval_id": approvalID1},
		CreatedAt:        time.Now(),
	}
	r2 := &NotificationRecord{
		ID:               approvalID2,
		SessionID:        "session-A",
		NotificationType: notifTypeApprovalNeededTest,
		Metadata:         map[string]string{"approval_id": approvalID2},
		CreatedAt:        time.Now(),
	}

	if err := store.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}
	if err := store.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	// Both approvals for the same unread session should collapse into one record
	// whose ID is updated to the latest approval UUID.
	records, _, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 deduplicated record, got %d", len(records))
	}
	if records[0].ID != approvalID2 {
		t.Errorf("expected record ID=%s (latest approval UUID), got %s", approvalID2, records[0].ID)
	}
	if records[0].OccurrenceCount != 2 {
		t.Errorf("expected OccurrenceCount=2, got %d", records[0].OccurrenceCount)
	}

	// Stamp the latest approval's decision via its UUID — must find the merged record.
	if err := store.SetMetadata(approvalID2, "approval_decision", "deny"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}

	records, _, err = store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List after stamp: %v", err)
	}
	if got := records[0].Metadata["approval_decision"]; got != "deny" {
		t.Errorf("expected approval_decision='deny', got '%s'", got)
	}
}

// TestPruneOrphaned_RemovesEligibleRecord_KeepsItemLinkedAndNonSessionScoped verifies
// that PruneOrphaned removes only records that are SessionScoped==true AND have no
// Metadata["item_id"] AND whose SessionID is absent from the existence set — never a
// record with an item_id (SessionID overloaded to carry a backlog item ID) and never a
// record that isn't positively marked session-scoped. It also verifies the batch-fetch
// existingSessionIDs callback is invoked exactly once, not once per candidate record.
func TestPruneOrphaned_RemovesEligibleRecord_KeepsItemLinkedAndNonSessionScoped(t *testing.T) {
	store := newTestStore(t)

	const deadSessionID = "dead-session-id"

	orphan := makeRecord("id-orphan", deadSessionID, notifTypeDedup)
	orphan.SessionScoped = true
	orphan.Metadata = map[string]string{}

	itemLinked := makeRecord("id-item-linked", deadSessionID, int32(11))
	itemLinked.SessionScoped = true
	itemLinked.Metadata = map[string]string{"item_id": "item-123"}

	notScoped := makeRecord("id-not-scoped", deadSessionID, int32(12))
	notScoped.SessionScoped = false
	notScoped.Metadata = map[string]string{}

	for _, r := range []*NotificationRecord{orphan, itemLinked, notScoped} {
		if err := store.Append(r); err != nil {
			t.Fatalf("Append %s: %v", r.ID, err)
		}
	}

	callCount := 0
	existingSessionIDs := func() map[string]struct{} {
		callCount++
		return map[string]struct{}{"some-other-live-session": {}}
	}

	removed, err := store.PruneOrphaned(existingSessionIDs)
	if err != nil {
		t.Fatalf("PruneOrphaned: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 record removed, got %d", removed)
	}
	if callCount != 1 {
		t.Errorf("expected existingSessionIDs called exactly once, got %d", callCount)
	}

	records, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 records remaining, got %d", total)
	}
	seen := map[string]bool{}
	for _, r := range records {
		seen[r.ID] = true
	}
	if seen["id-orphan"] {
		t.Errorf("expected id-orphan to be removed, but it's still present")
	}
	if !seen["id-item-linked"] {
		t.Errorf("expected id-item-linked to be kept, but it's missing")
	}
	if !seen["id-not-scoped"] {
		t.Errorf("expected id-not-scoped to be kept, but it's missing")
	}
}

// TestPruneOrphaned_PrunesNothing_When_ExistingSessionIDsReturnsNil verifies that a nil
// map from existingSessionIDs is treated as "not ready to judge existence this pass" —
// distinct from a real, merely-empty map — so nothing is pruned rather than everything.
func TestPruneOrphaned_PrunesNothing_When_ExistingSessionIDsReturnsNil(t *testing.T) {
	store := newTestStore(t)

	orphan := makeRecord("id-orphan", "dead-session-id", notifTypeDedup)
	orphan.SessionScoped = true
	orphan.Metadata = map[string]string{}

	if err := store.Append(orphan); err != nil {
		t.Fatalf("Append: %v", err)
	}

	removed, err := store.PruneOrphaned(func() map[string]struct{} { return nil })
	if err != nil {
		t.Fatalf("PruneOrphaned: %v", err)
	}
	if removed != 0 {
		t.Errorf("expected 0 records removed when existingSessionIDs returns nil, got %d", removed)
	}

	_, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Errorf("expected the orphaned record to still be present, total=%d", total)
	}
}

// TestEnforceRetention_GatesOrphanSweep_ByOrphanPruneInterval verifies that the
// orphan-pruning sweep wired into enforceRetention (which runs on every Append()) only
// actually invokes the existence-check callback at most once per orphanPruneInterval,
// not once per Append call.
func TestEnforceRetention_GatesOrphanSweep_ByOrphanPruneInterval(t *testing.T) {
	store := newTestStore(t)

	callCount := 0
	store.SetSessionExistenceLookup(func() map[string]struct{} {
		callCount++
		return map[string]struct{}{}
	})

	if err := store.Append(makeRecord("id-1", "session-A", int32(20))); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if err := store.Append(makeRecord("id-2", "session-B", int32(21))); err != nil {
		t.Fatalf("Append 2: %v", err)
	}

	if callCount > 1 {
		t.Errorf("expected at most 1 existence-check call across 2 Appends within the gate interval, got %d", callCount)
	}
	firstCount := callCount

	// Advance lastOrphanPruneAt past the gate interval via the same-package test seam.
	store.mu.Lock()
	store.lastOrphanPruneAt = time.Now().Add(-orphanPruneInterval - time.Second)
	store.mu.Unlock()

	if err := store.Append(makeRecord("id-3", "session-C", int32(22))); err != nil {
		t.Fatalf("Append 3: %v", err)
	}

	if callCount <= firstCount {
		t.Errorf("expected existence-check to fire again after advancing past orphanPruneInterval, callCount stayed at %d", callCount)
	}
}

// TestAppendDedup_AutoApprovedStaysReadAcrossRecurrence verifies the AUTO_APPROVED
// carve-out: unlike every other notification type, a recurrence must NOT flip the
// collapsed record back to unread, since AUTO_APPROVED records are pre-read by design
// and never surface as an active alert.
func TestAppendDedup_AutoApprovedStaysReadAcrossRecurrence(t *testing.T) {
	store := newTestStore(t)

	if err := store.AppendAutoApproved("session-A", "Session A", "Bash", "/tmp/foo", "rule-1", "Allow Bash", "seed", "approve"); err != nil {
		t.Fatalf("AppendAutoApproved 1: %v", err)
	}
	if err := store.AppendAutoApproved("session-A", "Session A", "Bash", "/tmp/foo", "rule-1", "Allow Bash", "seed", "approve"); err != nil {
		t.Fatalf("AppendAutoApproved 2: %v", err)
	}

	records, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 collapsed AUTO_APPROVED record, got %d", total)
	}
	if records[0].OccurrenceCount != 2 {
		t.Errorf("expected OccurrenceCount=2, got %d", records[0].OccurrenceCount)
	}
	if !records[0].IsRead {
		t.Error("expected AUTO_APPROVED record to stay read across recurrence")
	}
}

// TestAppendDedup_CollapsePathTriggersRetention verifies that enforceRetention() is
// swept on the collapse branch of Append(), not only the new-record branch -- a
// stale, unrelated record past MaxNotificationAge is pruned by a collapse triggered
// on a completely different (sessionID, notificationType) pair.
func TestAppendDedup_CollapsePathTriggersRetention(t *testing.T) {
	store := newTestStore(t)

	// Seed a stale record directly (old CreatedAt/LastOccurredAt, past MaxNotificationAge)
	// for an unrelated session/type -- it should be pruned once a collapse happens
	// anywhere in the store, not just on a brand-new record.
	staleTime := time.Now().Add(-MaxNotificationAge - time.Hour)
	store.mu.Lock()
	store.records = append(store.records, &NotificationRecord{
		ID:               "stale-1",
		SessionID:        "session-stale",
		NotificationType: notifTypeDedup,
		Title:            "Stale",
		CreatedAt:        staleTime,
		LastOccurredAt:   &staleTime,
		OccurrenceCount:  1,
	})
	store.mu.Unlock()

	// First append for a different (sessionID, notificationType) pair to establish
	// a record to collapse into.
	if err := store.Append(makeRecord("id-1", "session-A", notifTypeDedup)); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	// Second append for the same pair triggers the collapse branch.
	if err := store.Append(makeRecord("id-2", "session-A", notifTypeDedup)); err != nil {
		t.Fatalf("Append 2: %v", err)
	}

	_, total, err := store.List(ListOptions{Limit: 100, SessionID: "session-stale"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 0 {
		t.Errorf("expected the stale record to be pruned by the collapse-path retention sweep, got %d matching records", total)
	}
}

// TestAppendDedup_ApprovalNeeded_IDReassignmentUnaffectedByCollapse pins the one
// documented side effect of collapsing an APPROVAL_NEEDED record: the ID still
// reassigns to the incoming approval UUID, and (per the widened dedup predicate)
// IsRead is reset to false even though this behavior was already exercised for the
// unread case elsewhere -- here the existing record starts out read.
func TestAppendDedup_ApprovalNeeded_IDReassignmentUnaffectedByCollapse(t *testing.T) {
	store := newTestStore(t)

	r1 := makeRecord("n3", "session-A", notifTypeApprovalNeededTest)
	if err := store.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}
	if _, err := store.MarkRead([]string{"n3"}); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	r2 := makeRecord("n4", "session-A", notifTypeApprovalNeededTest)
	if err := store.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	records, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected exactly 1 collapsed record, got %d", total)
	}
	if records[0].ID != "n4" {
		t.Errorf("expected collapsed record ID to reassign to incoming UUID 'n4', got %q", records[0].ID)
	}
	if records[0].IsRead {
		t.Error("expected collapsed record to be unread")
	}
}

// TestEnforceRetention_KeyedByLastOccurredAt_SurvivesOldCreatedAt verifies that a
// frequently-recurring record with an old CreatedAt but a recent LastOccurredAt
// survives enforceRetention() -- retention keys off LastOccurredAt, not CreatedAt.
func TestEnforceRetention_KeyedByLastOccurredAt_SurvivesOldCreatedAt(t *testing.T) {
	store := newTestStore(t)

	createdAt := time.Now().Add(-10 * 24 * time.Hour)
	lastOccurredAt := time.Now().Add(-2 * time.Minute)
	store.mu.Lock()
	store.records = append(store.records, &NotificationRecord{
		ID:               "recurring-1",
		SessionID:        "session-A",
		NotificationType: notifTypeDedup,
		Title:            "Recurring",
		CreatedAt:        createdAt,
		LastOccurredAt:   &lastOccurredAt,
		OccurrenceCount:  47,
	})
	store.enforceRetention()
	store.mu.Unlock()

	_, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Errorf("expected the recurring record to survive retention, got %d records", total)
	}
}

// TestEnforceRetention_NoRecentOccurrence_StillPruned verifies that a record with no
// recent occurrences (both CreatedAt and LastOccurredAt past MaxNotificationAge) is
// still pruned by enforceRetention().
func TestEnforceRetention_NoRecentOccurrence_StillPruned(t *testing.T) {
	store := newTestStore(t)

	createdAt := time.Now().Add(-10 * 24 * time.Hour)
	lastOccurredAt := time.Now().Add(-9 * 24 * time.Hour)
	store.mu.Lock()
	store.records = append(store.records, &NotificationRecord{
		ID:               "cold-1",
		SessionID:        "session-A",
		NotificationType: notifTypeDedup,
		Title:            "Cold",
		CreatedAt:        createdAt,
		LastOccurredAt:   &lastOccurredAt,
		OccurrenceCount:  3,
	})
	store.enforceRetention()
	store.mu.Unlock()

	_, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 0 {
		t.Errorf("expected the cold record to be pruned, got %d records", total)
	}
}

// TestEnforceRetention_NilLastOccurredAt_FallsBackToCreatedAt verifies that a
// pre-migration record with LastOccurredAt: nil falls back to CreatedAt for the
// cutoff check, matching pre-fix behavior exactly.
func TestEnforceRetention_NilLastOccurredAt_FallsBackToCreatedAt(t *testing.T) {
	store := newTestStore(t)

	// Old CreatedAt, no LastOccurredAt set (nil) -- should be pruned via CreatedAt fallback.
	oldCreatedAt := time.Now().Add(-MaxNotificationAge - time.Hour)
	store.mu.Lock()
	store.records = append(store.records, &NotificationRecord{
		ID:               "old-nil-lastoccurred",
		SessionID:        "session-A",
		NotificationType: notifTypeDedup,
		Title:            "Old, pre-migration",
		CreatedAt:        oldCreatedAt,
		LastOccurredAt:   nil,
		OccurrenceCount:  1,
	})
	// Recent CreatedAt, no LastOccurredAt set (nil) -- should survive via CreatedAt fallback.
	recentCreatedAt := time.Now().Add(-time.Minute)
	store.records = append(store.records, &NotificationRecord{
		ID:               "recent-nil-lastoccurred",
		SessionID:        "session-B",
		NotificationType: notifTypeDedup,
		Title:            "Recent, pre-migration",
		CreatedAt:        recentCreatedAt,
		LastOccurredAt:   nil,
		OccurrenceCount:  1,
	})
	store.enforceRetention()
	store.mu.Unlock()

	result, total, err := store.List(ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected exactly 1 surviving record, got %d", total)
	}
	if result[0].ID != "recent-nil-lastoccurred" {
		t.Errorf("expected surviving record to be 'recent-nil-lastoccurred', got %q", result[0].ID)
	}
}

// notifTypeTaskCompleteTest matches NOTIFICATION_TYPE_TASK_COMPLETE = 4 in
// types.proto -- not in IsActionableType's set, so it's clearable.
const notifTypeTaskCompleteTest = int32(4)

// TestClear_NeverDeletesUnreadActionableRecord_WithTimestampCutoff verifies Task
// 3.1.5c's guarantee: an unread APPROVAL_NEEDED record survives Clear(&future)
// even though its CreatedAt is before the cutoff, while an old, read, non-actionable
// record is removed as usual.
func TestClear_NeverDeletesUnreadActionableRecord_WithTimestampCutoff(t *testing.T) {
	store := newTestStore(t)

	old := time.Now().Add(-time.Hour)
	pending := &NotificationRecord{
		ID:               "pending-approval",
		SessionID:        "session-A",
		NotificationType: notifTypeApprovalNeededTest,
		CreatedAt:        old,
		IsRead:           false,
	}
	resolved := &NotificationRecord{
		ID:               "old-task-complete",
		SessionID:        "session-B",
		NotificationType: notifTypeTaskCompleteTest,
		CreatedAt:        old,
		IsRead:           true,
	}
	if err := store.Append(pending); err != nil {
		t.Fatalf("Append pending: %v", err)
	}
	if err := store.Append(resolved); err != nil {
		t.Fatalf("Append resolved: %v", err)
	}

	future := time.Now().Add(time.Hour)
	if _, err := store.Clear(&future); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	if _, ok := store.GetByID("pending-approval"); !ok {
		t.Error("expected unread actionable record to survive Clear(&future), but it was removed")
	}
	if _, ok := store.GetByID("old-task-complete"); ok {
		t.Error("expected old, read, non-actionable record to be removed by Clear(&future), but it survived")
	}
}

// TestClear_NeverDeletesUnreadActionableRecord_ClearEverything verifies the same
// guarantee holds for the before==nil ("clear everything") path.
func TestClear_NeverDeletesUnreadActionableRecord_ClearEverything(t *testing.T) {
	store := newTestStore(t)

	pending := &NotificationRecord{
		ID:               "pending-approval",
		SessionID:        "session-A",
		NotificationType: notifTypeApprovalNeededTest,
		CreatedAt:        time.Now(),
		IsRead:           false,
	}
	resolved := &NotificationRecord{
		ID:               "read-task-complete",
		SessionID:        "session-B",
		NotificationType: notifTypeTaskCompleteTest,
		CreatedAt:        time.Now(),
		IsRead:           true,
	}
	if err := store.Append(pending); err != nil {
		t.Fatalf("Append pending: %v", err)
	}
	if err := store.Append(resolved); err != nil {
		t.Fatalf("Append resolved: %v", err)
	}

	if _, err := store.Clear(nil); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	if _, ok := store.GetByID("pending-approval"); !ok {
		t.Error("expected unread actionable record to survive Clear(nil), but it was removed")
	}
	if _, ok := store.GetByID("read-task-complete"); ok {
		t.Error("expected read, non-actionable record to be removed by Clear(nil), but it survived")
	}
}

// TestIsActionableType verifies the actionable/non-actionable classification
// mirrors notificationMapping.ts's ACTIONABLE_TYPES set.
func TestIsActionableType(t *testing.T) {
	actionable := []int32{
		notifTypeApprovalNeededTest, // APPROVAL_NEEDED
		notifTypeConfirmationNeed,   // CONFIRMATION_NEEDED
		notifTypeInputRequired,      // INPUT_REQUIRED (question)
		notifTypeError,              // ERROR
		notifTypeFailure,            // FAILURE (error)
		notifTypeWarning,            // WARNING
	}
	for _, ty := range actionable {
		if !IsActionableType(ty) {
			t.Errorf("expected NotificationType %d to be actionable", ty)
		}
	}

	nonActionable := []int32{notifTypeTaskCompleteTest, notifTypeAutoApproved, notifTypeDedup}
	for _, ty := range nonActionable {
		if IsActionableType(ty) {
			t.Errorf("expected NotificationType %d to be non-actionable", ty)
		}
	}
}
