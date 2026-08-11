package adapters

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	sessiongit "github.com/tstapler/stapler-squad/session/git"
)

// ---------------------------------------------------------------------------
// priorityToProto
// ---------------------------------------------------------------------------

func TestPriorityToProto(t *testing.T) {
	tests := []struct {
		name  string
		input session.Priority
		want  sessionv1.Priority
	}{
		{"urgent", session.PriorityUrgent, sessionv1.Priority_PRIORITY_URGENT},
		{"high", session.PriorityHigh, sessionv1.Priority_PRIORITY_HIGH},
		{"medium", session.PriorityMedium, sessionv1.Priority_PRIORITY_MEDIUM},
		{"low", session.PriorityLow, sessionv1.Priority_PRIORITY_LOW},
		{"unknown defaults to unspecified", session.Priority(99), sessionv1.Priority_PRIORITY_UNSPECIFIED},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, priorityToProto(tc.input))
		})
	}
}

// ---------------------------------------------------------------------------
// attentionReasonToProto
// ---------------------------------------------------------------------------

func TestAttentionReasonToProto(t *testing.T) {
	tests := []struct {
		name  string
		input session.AttentionReason
		want  sessionv1.AttentionReason
	}{
		{"approval_pending", session.ReasonApprovalPending, sessionv1.AttentionReason_ATTENTION_REASON_APPROVAL_PENDING},
		{"input_required", session.ReasonInputRequired, sessionv1.AttentionReason_ATTENTION_REASON_INPUT_REQUIRED},
		{"error_state", session.ReasonErrorState, sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE},
		{"idle_timeout", session.ReasonIdleTimeout, sessionv1.AttentionReason_ATTENTION_REASON_IDLE_TIMEOUT},
		{"task_complete", session.ReasonTaskComplete, sessionv1.AttentionReason_ATTENTION_REASON_TASK_COMPLETE},
		{"uncommitted_changes", session.ReasonUncommittedChanges, sessionv1.AttentionReason_ATTENTION_REASON_UNCOMMITTED_CHANGES},
		{"idle", session.ReasonIdle, sessionv1.AttentionReason_ATTENTION_REASON_IDLE},
		{"stale", session.ReasonStale, sessionv1.AttentionReason_ATTENTION_REASON_STALE},
		{"waiting_for_user", session.ReasonWaitingForUser, sessionv1.AttentionReason_ATTENTION_REASON_WAITING_FOR_USER},
		{"unknown defaults to unspecified", session.AttentionReason("not_a_reason"), sessionv1.AttentionReason_ATTENTION_REASON_UNSPECIFIED},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, attentionReasonToProto(tc.input))
		})
	}
}

// ---------------------------------------------------------------------------
// ProtoToPriority
// ---------------------------------------------------------------------------

func TestProtoToPriority(t *testing.T) {
	tests := []struct {
		name  string
		input sessionv1.Priority
		want  session.Priority
	}{
		{"urgent", sessionv1.Priority_PRIORITY_URGENT, session.PriorityUrgent},
		{"high", sessionv1.Priority_PRIORITY_HIGH, session.PriorityHigh},
		{"medium", sessionv1.Priority_PRIORITY_MEDIUM, session.PriorityMedium},
		{"low", sessionv1.Priority_PRIORITY_LOW, session.PriorityLow},
		// PRIORITY_UNSPECIFIED defaults to medium
		{"unspecified defaults to medium", sessionv1.Priority_PRIORITY_UNSPECIFIED, session.PriorityMedium},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ProtoToPriority(tc.input))
		})
	}
}

// ---------------------------------------------------------------------------
// ProtoToAttentionReason
// ---------------------------------------------------------------------------

func TestProtoToAttentionReason(t *testing.T) {
	tests := []struct {
		name  string
		input sessionv1.AttentionReason
		want  session.AttentionReason
	}{
		{"approval_pending", sessionv1.AttentionReason_ATTENTION_REASON_APPROVAL_PENDING, session.ReasonApprovalPending},
		{"input_required", sessionv1.AttentionReason_ATTENTION_REASON_INPUT_REQUIRED, session.ReasonInputRequired},
		{"error_state", sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE, session.ReasonErrorState},
		{"idle_timeout", sessionv1.AttentionReason_ATTENTION_REASON_IDLE_TIMEOUT, session.ReasonIdleTimeout},
		{"task_complete", sessionv1.AttentionReason_ATTENTION_REASON_TASK_COMPLETE, session.ReasonTaskComplete},
		{"uncommitted_changes", sessionv1.AttentionReason_ATTENTION_REASON_UNCOMMITTED_CHANGES, session.ReasonUncommittedChanges},
		{"idle", sessionv1.AttentionReason_ATTENTION_REASON_IDLE, session.ReasonIdle},
		{"stale", sessionv1.AttentionReason_ATTENTION_REASON_STALE, session.ReasonStale},
		{"waiting_for_user", sessionv1.AttentionReason_ATTENTION_REASON_WAITING_FOR_USER, session.ReasonWaitingForUser},
		// ATTENTION_REASON_UNSPECIFIED defaults to input_required
		{"unspecified defaults to input_required", sessionv1.AttentionReason_ATTENTION_REASON_UNSPECIFIED, session.ReasonInputRequired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ProtoToAttentionReason(tc.input))
		})
	}
}

// ---------------------------------------------------------------------------
// ReviewItemToProto
// ---------------------------------------------------------------------------

func TestReviewItemToProto_Nil(t *testing.T) {
	result := ReviewItemToProto(nil, nil)
	assert.Nil(t, result, "nil input should return nil")
}

func TestReviewItemToProto_Basic(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	lastActivity := now.Add(-5 * time.Minute)

	item := &session.ReviewItem{
		SessionID:    "sess-001",
		SessionName:  "my-session",
		Reason:       session.ReasonInputRequired,
		Priority:     session.PriorityHigh,
		DetectedAt:   now,
		Context:      "waiting for user input",
		PatternName:  "input-detection",
		Metadata:     map[string]string{"key1": "val1"},
		Program:      "claude",
		Branch:       "feature/foo",
		Path:         "/home/user/project",
		WorkingDir:   "/home/user/project",
		Status:       "Running",
		Tags:         []string{"urgent", "backend"},
		Category:     "Work",
		LastActivity: lastActivity,
	}

	proto := ReviewItemToProto(item, nil)
	require.NotNil(t, proto)

	assert.Equal(t, "sess-001", proto.SessionId)
	assert.Equal(t, "my-session", proto.SessionName)
	assert.Equal(t, sessionv1.AttentionReason_ATTENTION_REASON_INPUT_REQUIRED, proto.Reason)
	assert.Equal(t, sessionv1.Priority_PRIORITY_HIGH, proto.Priority)
	assert.Equal(t, "waiting for user input", proto.Context)
	assert.Equal(t, "input-detection", proto.PatternName)
	assert.Equal(t, "claude", proto.Program)
	assert.Equal(t, "feature/foo", proto.Branch)
	assert.Equal(t, "/home/user/project", proto.Path)
	assert.Equal(t, "/home/user/project", proto.WorkingDir)
	assert.Equal(t, []string{"urgent", "backend"}, proto.Tags)
	assert.Equal(t, "Work", proto.Category)
	assert.Nil(t, proto.DiffStats)

	// Timestamps must be round-tripped faithfully
	require.NotNil(t, proto.DetectedAt)
	assert.Equal(t, now, proto.DetectedAt.AsTime().UTC().Truncate(time.Second))
	require.NotNil(t, proto.LastActivity)
	assert.Equal(t, lastActivity, proto.LastActivity.AsTime().UTC().Truncate(time.Second))

	// The base metadata key should be present
	assert.Equal(t, "val1", proto.Metadata["key1"])
}

func TestReviewItemToProto_WithDiffStats(t *testing.T) {
	now := time.Now()
	item := &session.ReviewItem{
		SessionID:    "sess-002",
		SessionName:  "diff-session",
		Reason:       session.ReasonUncommittedChanges,
		Priority:     session.PriorityLow,
		DetectedAt:   now,
		LastActivity: now,
		DiffStats: &sessiongit.DiffStats{
			Added:   42,
			Removed: 7,
			Content: "--- a/foo.go\n+++ b/foo.go\n",
		},
	}

	proto := ReviewItemToProto(item, nil)
	require.NotNil(t, proto)
	require.NotNil(t, proto.DiffStats, "DiffStats should be populated")

	assert.Equal(t, int32(42), proto.DiffStats.Added)
	assert.Equal(t, int32(7), proto.DiffStats.Removed)
	assert.Equal(t, "--- a/foo.go\n+++ b/foo.go\n", proto.DiffStats.Content)
}

func TestReviewItemToProto_ExtraMetadataIsMerged(t *testing.T) {
	now := time.Now()
	item := &session.ReviewItem{
		SessionID:    "sess-003",
		SessionName:  "approval-session",
		Reason:       session.ReasonApprovalPending,
		Priority:     session.PriorityUrgent,
		DetectedAt:   now,
		LastActivity: now,
		Metadata:     map[string]string{"base_key": "base_val"},
	}
	extra := map[string]string{"pending_approval_id": "approval-xyz"}

	proto := ReviewItemToProto(item, extra)
	require.NotNil(t, proto)

	assert.Equal(t, "base_val", proto.Metadata["base_key"], "original metadata should be present")
	assert.Equal(t, "approval-xyz", proto.Metadata["pending_approval_id"], "extra metadata should be injected")
}

func TestReviewItemToProto_ExtraMetadataOverwritesBase(t *testing.T) {
	now := time.Now()
	item := &session.ReviewItem{
		SessionID:    "sess-004",
		SessionName:  "overwrite-session",
		Reason:       session.ReasonErrorState,
		Priority:     session.PriorityUrgent,
		DetectedAt:   now,
		LastActivity: now,
		Metadata:     map[string]string{"conflict_key": "from_item"},
	}
	extra := map[string]string{"conflict_key": "from_extra"}

	proto := ReviewItemToProto(item, extra)
	require.NotNil(t, proto)

	// extra wins because it is applied second
	assert.Equal(t, "from_extra", proto.Metadata["conflict_key"])
}

func TestReviewItemToProto_MetadataIsIndependentCopy(t *testing.T) {
	now := time.Now()
	item := &session.ReviewItem{
		SessionID:    "sess-005",
		SessionName:  "copy-session",
		Reason:       session.ReasonIdle,
		Priority:     session.PriorityLow,
		DetectedAt:   now,
		LastActivity: now,
		Metadata:     map[string]string{"original": "value"},
	}
	extra := map[string]string{"extra_key": "extra_val"}

	proto := ReviewItemToProto(item, extra)
	require.NotNil(t, proto)

	// Mutating extra after the call must not affect the proto's metadata
	extra["injected_after"] = "should_not_appear"
	_, exists := proto.Metadata["injected_after"]
	assert.False(t, exists, "post-call mutation of extraMetadata must not affect the returned proto")

	// Mutating item.Metadata must also not affect the proto
	item.Metadata["original"] = "mutated"
	assert.Equal(t, "value", proto.Metadata["original"], "proto metadata must be an independent copy")
}

func TestReviewItemToProto_NilMetadataFields(t *testing.T) {
	now := time.Now()
	item := &session.ReviewItem{
		SessionID:    "sess-006",
		SessionName:  "no-meta",
		Reason:       session.ReasonStale,
		Priority:     session.PriorityMedium,
		DetectedAt:   now,
		LastActivity: now,
		// Metadata intentionally nil
	}

	proto := ReviewItemToProto(item, nil)
	require.NotNil(t, proto)
	// Should produce an empty map, not panic
	assert.NotNil(t, proto.Metadata)
	assert.Empty(t, proto.Metadata)
}

// ---------------------------------------------------------------------------
// ReviewQueueToProto
// ---------------------------------------------------------------------------

func TestReviewQueueToProto_NilQueue(t *testing.T) {
	proto := ReviewQueueToProto(nil, nil)
	require.NotNil(t, proto, "nil queue must return an empty proto, not nil")

	assert.Equal(t, int32(0), proto.TotalItems)
	assert.Empty(t, proto.Items)
	assert.NotNil(t, proto.ByPriority)
	assert.NotNil(t, proto.ByReason)
}

func TestReviewQueueToProto_WithItems(t *testing.T) {
	now := time.Now()

	queue := session.NewReviewQueue()
	queue.Add(&session.ReviewItem{
		SessionID:    "s1",
		SessionName:  "Session One",
		Reason:       session.ReasonErrorState,
		Priority:     session.PriorityUrgent,
		DetectedAt:   now,
		LastActivity: now,
	})
	queue.Add(&session.ReviewItem{
		SessionID:    "s2",
		SessionName:  "Session Two",
		Reason:       session.ReasonTaskComplete,
		Priority:     session.PriorityLow,
		DetectedAt:   now,
		LastActivity: now,
	})

	proto := ReviewQueueToProto(queue, nil)
	require.NotNil(t, proto)

	assert.Equal(t, int32(2), proto.TotalItems)
	assert.Len(t, proto.Items, 2)

	// Priority breakdown: 1 urgent, 1 low
	urgentKey := int32(sessionv1.Priority_PRIORITY_URGENT)
	lowKey := int32(sessionv1.Priority_PRIORITY_LOW)
	assert.Equal(t, int32(1), proto.ByPriority[urgentKey])
	assert.Equal(t, int32(1), proto.ByPriority[lowKey])

	// Reason breakdown
	errorKey := int32(sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE)
	completeKey := int32(sessionv1.AttentionReason_ATTENTION_REASON_TASK_COMPLETE)
	assert.Equal(t, int32(1), proto.ByReason[errorKey])
	assert.Equal(t, int32(1), proto.ByReason[completeKey])
}

func TestReviewQueueToProto_ApprovalIDEnrichment(t *testing.T) {
	now := time.Now()

	queue := session.NewReviewQueue()
	queue.Add(&session.ReviewItem{
		SessionID:    "approval-sess",
		SessionName:  "Approval Session",
		Reason:       session.ReasonApprovalPending,
		Priority:     session.PriorityHigh,
		DetectedAt:   now,
		LastActivity: now,
	})

	approvalIDs := map[string]string{
		"approval-sess": "approval-abc-123",
	}

	proto := ReviewQueueToProto(queue, approvalIDs)
	require.NotNil(t, proto)
	require.Len(t, proto.Items, 1)

	item := proto.Items[0]
	assert.Equal(t, "approval-abc-123", item.Metadata["pending_approval_id"],
		"approval ID should be injected into item metadata")
}

func TestReviewQueueToProto_NoApprovalIDForOtherSessions(t *testing.T) {
	now := time.Now()

	queue := session.NewReviewQueue()
	queue.Add(&session.ReviewItem{
		SessionID:    "other-sess",
		SessionName:  "Other Session",
		Reason:       session.ReasonInputRequired,
		Priority:     session.PriorityMedium,
		DetectedAt:   now,
		LastActivity: now,
	})

	approvalIDs := map[string]string{
		"different-sess": "approval-xyz",
	}

	proto := ReviewQueueToProto(queue, approvalIDs)
	require.NotNil(t, proto)
	require.Len(t, proto.Items, 1)

	_, hasApproval := proto.Items[0].Metadata["pending_approval_id"]
	assert.False(t, hasApproval, "approval ID must not be injected for sessions not in approvalIDs map")
}

func TestReviewQueueToProto_EmptyQueue(t *testing.T) {
	queue := session.NewReviewQueue()

	proto := ReviewQueueToProto(queue, nil)
	require.NotNil(t, proto)

	assert.Equal(t, int32(0), proto.TotalItems)
	assert.Empty(t, proto.Items)
}
