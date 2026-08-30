package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/notifications"
)

// newTestNotificationHistoryStore constructs a *notifications.NotificationHistoryStore
// backed by a temp file, mirroring server/notifications/store_test.go's newTestStore
// convention (that helper is unexported and lives in a different package).
func newTestNotificationHistoryStore(t *testing.T) *notifications.NotificationHistoryStore {
	t.Helper()
	fp := filepath.Join(t.TempDir(), "notifications.json")
	store, err := notifications.NewNotificationHistoryStore(fp)
	if err != nil {
		t.Fatalf("NewNotificationHistoryStore: %v", err)
	}
	return store
}

// TestNotificationDecisionListerAdapter_should_MapApprovalDecisionFromMetadata_When_RecordsExist
// covers notificationDecisionListerAdapter.ListDecisionRecords' core mapping logic:
// each notifications.NotificationRecord for the session becomes a
// session.DecisionRecord carrying its NotificationType and the
// Metadata["approval_decision"] value (empty string when the key is absent).
func TestNotificationDecisionListerAdapter_should_MapApprovalDecisionFromMetadata_When_RecordsExist(t *testing.T) {
	store := newTestNotificationHistoryStore(t)
	sessionID := "sess-decisions"

	if err := store.Append(&notifications.NotificationRecord{
		ID:               "rec-1",
		SessionID:        sessionID,
		NotificationType: 13, // NOTIFICATION_TYPE_AUTO_APPROVED
		Metadata:         map[string]string{"approval_decision": "auto_approved"},
		CreatedAt:        time.Now(),
	}); err != nil {
		t.Fatalf("Append rec-1: %v", err)
	}
	if err := store.Append(&notifications.NotificationRecord{
		ID:               "rec-2",
		SessionID:        sessionID,
		NotificationType: 10, // a distinct type so this doesn't dedup-collapse into rec-1
		Metadata:         nil,
		CreatedAt:        time.Now(),
	}); err != nil {
		t.Fatalf("Append rec-2: %v", err)
	}
	// A record for a different session must not be included.
	if err := store.Append(&notifications.NotificationRecord{
		ID:               "rec-other-session",
		SessionID:        "sess-different",
		NotificationType: 13,
		Metadata:         map[string]string{"approval_decision": "manually_approved"},
		CreatedAt:        time.Now(),
	}); err != nil {
		t.Fatalf("Append rec-other-session: %v", err)
	}

	adapter := &notificationDecisionListerAdapter{store: store}
	records, err := adapter.ListDecisionRecords(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDecisionRecords: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 decision records for %s, got %d: %+v", sessionID, len(records), records)
	}

	byType := map[int32]string{}
	for _, r := range records {
		byType[r.NotificationType] = r.ApprovalDecision
	}
	if got := byType[13]; got != "auto_approved" {
		t.Errorf("expected NotificationType=13 to map ApprovalDecision=%q, got %q", "auto_approved", got)
	}
	if got, ok := byType[10]; !ok || got != "" {
		t.Errorf("expected NotificationType=10 (no approval_decision metadata) to map to empty ApprovalDecision, got %q (present=%v)", got, ok)
	}
}

// TestNotificationDecisionListerAdapter_should_ReturnEmptySlice_When_NoRecordsForSession
// covers the no-records case: a session with no notification history must return an
// empty (not nil-panicking, not erroring) slice.
func TestNotificationDecisionListerAdapter_should_ReturnEmptySlice_When_NoRecordsForSession(t *testing.T) {
	store := newTestNotificationHistoryStore(t)
	adapter := &notificationDecisionListerAdapter{store: store}

	records, err := adapter.ListDecisionRecords(context.Background(), "sess-with-no-history")
	if err != nil {
		t.Fatalf("ListDecisionRecords: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 decision records, got %d: %+v", len(records), records)
	}
}
