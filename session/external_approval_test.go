package session

import (
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// TestExternalApprovalMonitor_IngestRelayedApproval_CreatesSessionAndNotifies
// verifies the ingestion path session/sshremote.RemoteApprovalRelay uses
// (Task 5.1.1c): a relayed request becomes visible via GetPendingApprovals
// keyed by sessionKey, and triggers a callback exactly the way a
// locally-detected approval does.
func TestExternalApprovalMonitor_IngestRelayedApproval_CreatesSessionAndNotifies(t *testing.T) {
	m := NewExternalApprovalMonitor()
	m.Start()
	defer m.Stop()

	var mu sync.Mutex
	var events []*ExternalApprovalEvent
	done := make(chan struct{}, 1)
	m.OnApproval(func(e *ExternalApprovalEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
		select {
		case done <- struct{}{}:
		default:
		}
	})

	req := &detection.ApprovalRequest{
		ID:           "req-1",
		Type:         detection.ApprovalCommand,
		DetectedText: "rm -rf /",
		Confidence:   0.9,
	}
	m.IngestRelayedApproval("session-key-1", "Remote Session", req)

	pending := m.GetPendingApprovals("session-key-1")
	if len(pending) != 1 {
		t.Fatalf("GetPendingApprovals() returned %d requests, want 1", len(pending))
	}
	if pending[0].ID != "req-1" {
		t.Errorf("pending[0].ID = %q, want %q", pending[0].ID, "req-1")
	}
	if pending[0].Status != detection.ApprovalPending {
		t.Errorf("pending[0].Status = %q, want %q (default when unset)", pending[0].Status, detection.ApprovalPending)
	}
	if pending[0].Timestamp.IsZero() {
		t.Error("pending[0].Timestamp is zero, want it defaulted to now")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnApproval callback was not invoked for a relayed request")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Fatalf("callback invoked %d times, want 1", len(events))
	}
	if events[0].Source != SourceRemote {
		t.Errorf("event.Source = %q, want %q", events[0].Source, SourceRemote)
	}
	if events[0].SessionID != "session-key-1" {
		t.Errorf("event.SessionID = %q, want %q", events[0].SessionID, "session-key-1")
	}
}

// TestExternalApprovalMonitor_IngestRelayedApproval_DedupesByID verifies a
// redelivered request (same ID) does not appear twice in the pending list
// nor trigger a second callback -- the idempotency Story 5.1.2's chosen
// in-flight-request-survival behavior (agent-side retry against a freshly
// reopened relay channel, see session/sshremote/approval_relay.go's doc
// comment) depends on.
func TestExternalApprovalMonitor_IngestRelayedApproval_DedupesByID(t *testing.T) {
	m := NewExternalApprovalMonitor()
	m.Start()
	defer m.Stop()

	var callCount int
	var mu sync.Mutex
	m.OnApproval(func(*ExternalApprovalEvent) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})

	req := &detection.ApprovalRequest{ID: "req-retry", Type: detection.ApprovalCommand, DetectedText: "rm -rf /"}
	m.IngestRelayedApproval("session-key-2", "Remote Session", req)
	m.IngestRelayedApproval("session-key-2", "Remote Session", req)
	m.IngestRelayedApproval("session-key-2", "Remote Session", req)

	pending := m.GetPendingApprovals("session-key-2")
	if len(pending) != 1 {
		t.Fatalf("GetPendingApprovals() returned %d requests after 3 identical-ID ingests, want 1", len(pending))
	}

	// Give the async callback (notifyCallbacks fires it in its own
	// goroutine) a moment to land before asserting the count is stable.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := callCount
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if callCount != 1 {
		// A redelivered request (same ID) returns early before reaching
		// notifyCallbacks -- deduplication applies to BOTH the pending list
		// and callback fan-out, so an agent-side retry never produces a
		// duplicate UI notification for the same request.
		t.Errorf("callback invoked %d times for 3 identical-ID ingests, want 1", callCount)
	}
}

// TestExternalApprovalMonitor_IngestRelayedApproval_MultipleSessionsIsolated
// verifies two distinct sessionKeys never share or leak pending state.
func TestExternalApprovalMonitor_IngestRelayedApproval_MultipleSessionsIsolated(t *testing.T) {
	m := NewExternalApprovalMonitor()
	m.Start()
	defer m.Stop()

	m.IngestRelayedApproval("session-a", "A", &detection.ApprovalRequest{ID: "a1"})
	m.IngestRelayedApproval("session-b", "B", &detection.ApprovalRequest{ID: "b1"})

	pendingA := m.GetPendingApprovals("session-a")
	pendingB := m.GetPendingApprovals("session-b")
	if len(pendingA) != 1 || pendingA[0].ID != "a1" {
		t.Errorf("session-a pending = %+v, want exactly [a1]", pendingA)
	}
	if len(pendingB) != 1 || pendingB[0].ID != "b1" {
		t.Errorf("session-b pending = %+v, want exactly [b1]", pendingB)
	}
}

// TestExternalApprovalMonitor_IngestRelayedApproval_NilRequestIsNoop
// verifies a nil request (should never happen given approval_relay.go's own
// decode step, but defensively) does not panic or create an empty session
// entry.
func TestExternalApprovalMonitor_IngestRelayedApproval_NilRequestIsNoop(t *testing.T) {
	m := NewExternalApprovalMonitor()
	m.Start()
	defer m.Stop()

	m.IngestRelayedApproval("session-nil", "Nil", nil)

	if pending := m.GetPendingApprovals("session-nil"); len(pending) != 0 {
		t.Errorf("GetPendingApprovals() after nil ingest = %+v, want empty", pending)
	}
}

// TestExternalApprovalMonitor_IngestRelayedApproval_EmptyIDIsRejected proves
// a request with a blank ID never becomes a pending entry -- an empty ID
// can't be deduped against itself (every ingest would compare "" == "" and
// still append, since the dedup loop's own condition is what's being
// guarded against here) and can never be targeted by MarkApprovalHandled,
// so accepting it would let a malformed or malicious relayed payload pile
// up an unbounded number of un-resolvable pending entries.
func TestExternalApprovalMonitor_IngestRelayedApproval_EmptyIDIsRejected(t *testing.T) {
	m := NewExternalApprovalMonitor()
	m.Start()
	defer m.Stop()

	for i := 0; i < 3; i++ {
		m.IngestRelayedApproval("session-empty-id", "Empty ID", &detection.ApprovalRequest{
			ID:           "",
			Type:         detection.ApprovalCommand,
			DetectedText: "cmd",
		})
	}

	if pending := m.GetPendingApprovals("session-empty-id"); len(pending) != 0 {
		t.Errorf("GetPendingApprovals() after empty-ID ingest = %+v, want empty (no pending entries, not deduped-to-one)", pending)
	}
}
