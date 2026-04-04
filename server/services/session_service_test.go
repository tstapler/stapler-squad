package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	connect "connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// createTestStorage creates a test storage backed by a temporary SQLite database.
func createTestStorage(t *testing.T) *session.Storage {
	t.Helper()

	testDir := filepath.Join(os.TempDir(), "stapler-squad-test-delete-session")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(testDir) })

	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(testDir, "sessions.db")))
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	return storage
}

// TestDeleteSession_RemovesFromReviewQueue verifies that when a session is deleted
// via DeleteSession RPC, it's also removed from the review queue.
// This is a regression test for the bug where deleted sessions persisted in the review queue.
func TestDeleteSession_RemovesFromReviewQueue(t *testing.T) {
	// Create in-memory test storage
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)

	// Create session service
	svc := NewSessionService(storage, eventBus)

	// Create and add a test instance to storage
	testInstance := &session.Instance{
		Title:   "test-session",
		Path:    "/tmp/test",
		Status:  session.Running,
		Program: "claude",
	}

	if err := storage.AddInstance(testInstance); err != nil {
		t.Fatalf("Failed to add test instance: %v", err)
	}

	// Add session to review queue
	reviewQueue := svc.GetReviewQueueInstance()
	reviewItem := &session.ReviewItem{
		SessionID:   "test-session",
		SessionName: "test-session",
		Reason:      session.ReasonIdle,
		Priority:    session.PriorityLow,
	}
	reviewQueue.Add(reviewItem)

	// Verify session is in queue before deletion
	if _, exists := reviewQueue.Get("test-session"); !exists {
		t.Fatal("Session should be in review queue before deletion")
	}

	// Call DeleteSession
	req := connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "test-session",
	})

	resp, err := svc.DeleteSession(context.Background(), req)
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}
	if !resp.Msg.Success {
		t.Errorf("DeleteSession returned success=false")
	}

	// Verify session is removed from review queue
	if _, exists := reviewQueue.Get("test-session"); exists {
		t.Error("Session should be removed from review queue after deletion")
	}

	// Verify session is removed from storage
	instances, err := storage.LoadInstances()
	if err != nil {
		t.Fatalf("Failed to load instances: %v", err)
	}
	for _, inst := range instances {
		if inst.Title == "test-session" {
			t.Error("Session should be removed from storage after deletion")
		}
	}
}

// TestDeleteSession_NonExistentSession verifies that deleting a non-existent session
// returns a proper error.
func TestDeleteSession_NonExistentSession(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)

	svc := NewSessionService(storage, eventBus)

	req := connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "non-existent-session",
	})

	_, err := svc.DeleteSession(context.Background(), req)
	if err == nil {
		t.Error("Expected error when deleting non-existent session")
	}

	// Verify it's a NotFound error
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Expected CodeNotFound, got %v", connectErr.Code())
	}
}

// TestDeleteSession_EmptyId verifies that deleting with empty ID returns an error.
func TestDeleteSession_EmptyId(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)

	svc := NewSessionService(storage, eventBus)

	req := connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "",
	})

	_, err := svc.DeleteSession(context.Background(), req)
	if err == nil {
		t.Error("Expected error when deleting with empty ID")
	}

	// Verify it's an InvalidArgument error
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected CodeInvalidArgument, got %v", connectErr.Code())
	}
}
