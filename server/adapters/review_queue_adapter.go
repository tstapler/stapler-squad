package adapters

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReviewItemToProto converts session.ReviewItem to proto ReviewItem.
func ReviewItemToProto(item *session.ReviewItem) *sessionv1.ReviewItem {
	if item == nil {
		return nil
	}

	protoItem := &sessionv1.ReviewItem{
		SessionId:    item.SessionID,
		SessionName:  item.SessionName,
		Reason:       attentionReasonToProto(item.Reason),
		Priority:     priorityToProto(item.Priority),
		DetectedAt:   timestamppb.New(item.DetectedAt),
		Context:      item.Context,
		PatternName:  item.PatternName,
		Metadata:     item.Metadata,
		// Session details for rich display
		Program:      item.Program,
		Branch:       item.Branch,
		Path:         item.Path,
		WorkingDir:   item.WorkingDir,
		Status:       StatusStringToProto(item.Status),
		Tags:         item.Tags,
		Category:     item.Category,
		LastActivity: timestamppb.New(item.LastActivity),
	}

	// Add diff stats if available
	if item.DiffStats != nil {
		protoItem.DiffStats = &sessionv1.DiffStats{
			Added:   int32(item.DiffStats.Added),
			Removed: int32(item.DiffStats.Removed),
			Content: item.DiffStats.Content,
		}
	}

	return protoItem
}

// ReviewQueueToProto converts session.ReviewQueue to proto ReviewQueue with statistics.
func ReviewQueueToProto(queue *session.ReviewQueue) *sessionv1.ReviewQueue {
	if queue == nil {
		return &sessionv1.ReviewQueue{
			TotalItems: 0,
			Items:      []*sessionv1.ReviewItem{},
			ByPriority: make(map[int32]int32),
			ByReason:   make(map[int32]int32),
		}
	}

	// Get all items sorted by priority
	items := queue.List()
	protoItems := make([]*sessionv1.ReviewItem, 0, len(items))
	for _, item := range items {
		protoItems = append(protoItems, ReviewItemToProto(item))
	}

	// Get statistics
	stats := queue.GetStatistics()

	// Convert priority stats to map
	byPriority := make(map[int32]int32)
	for priority, count := range stats.ByPriority {
		protoP := priorityToProto(priority)
		byPriority[int32(protoP)] = int32(count)
	}

	// Convert reason stats to map
	byReason := make(map[int32]int32)
	for reason, count := range stats.ByReason {
		protoR := attentionReasonToProto(reason)
		byReason[int32(protoR)] = int32(count)
	}

	protoQueue := &sessionv1.ReviewQueue{
		TotalItems:         int32(stats.TotalItems),
		Items:              protoItems,
		ByPriority:         byPriority,
		ByReason:           byReason,
		AverageAgeSeconds:  int64(stats.AverageAge.Seconds()),
		OldestItemId:       stats.OldestItem,
		OldestAgeSeconds:   int64(stats.OldestAge.Seconds()),
	}

	return protoQueue
}

// priorityToProto converts session.Priority to proto Priority enum.
func priorityToProto(priority session.Priority) sessionv1.Priority {
	switch priority {
	case session.PriorityUrgent:
		return sessionv1.Priority_PRIORITY_URGENT
	case session.PriorityHigh:
		return sessionv1.Priority_PRIORITY_HIGH
	case session.PriorityMedium:
		return sessionv1.Priority_PRIORITY_MEDIUM
	case session.PriorityLow:
		return sessionv1.Priority_PRIORITY_LOW
	default:
		return sessionv1.Priority_PRIORITY_UNSPECIFIED
	}
}

// attentionReasonToProto converts session.AttentionReason to proto AttentionReason enum.
func attentionReasonToProto(reason session.AttentionReason) sessionv1.AttentionReason {
	switch reason {
	case session.ReasonApprovalPending:
		return sessionv1.AttentionReason_ATTENTION_REASON_APPROVAL_PENDING
	case session.ReasonInputRequired:
		return sessionv1.AttentionReason_ATTENTION_REASON_INPUT_REQUIRED
	case session.ReasonErrorState:
		return sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE
	case session.ReasonIdleTimeout:
		return sessionv1.AttentionReason_ATTENTION_REASON_IDLE_TIMEOUT
	case session.ReasonTaskComplete:
		return sessionv1.AttentionReason_ATTENTION_REASON_TASK_COMPLETE
	case session.ReasonUncommittedChanges:
		return sessionv1.AttentionReason_ATTENTION_REASON_UNCOMMITTED_CHANGES
	case session.ReasonIdle:
		return sessionv1.AttentionReason_ATTENTION_REASON_IDLE
	case session.ReasonStale:
		return sessionv1.AttentionReason_ATTENTION_REASON_STALE
	case session.ReasonWaitingForUser:
		return sessionv1.AttentionReason_ATTENTION_REASON_WAITING_FOR_USER
	case session.ReasonTestsFailing:
		return sessionv1.AttentionReason_ATTENTION_REASON_TESTS_FAILING
	default:
		return sessionv1.AttentionReason_ATTENTION_REASON_UNSPECIFIED
	}
}

// ProtoToPriority converts proto Priority enum to session.Priority.
func ProtoToPriority(priority sessionv1.Priority) session.Priority {
	switch priority {
	case sessionv1.Priority_PRIORITY_URGENT:
		return session.PriorityUrgent
	case sessionv1.Priority_PRIORITY_HIGH:
		return session.PriorityHigh
	case sessionv1.Priority_PRIORITY_MEDIUM:
		return session.PriorityMedium
	case sessionv1.Priority_PRIORITY_LOW:
		return session.PriorityLow
	default:
		return session.PriorityMedium // Default to medium
	}
}

// ProtoToAttentionReason converts proto AttentionReason enum to session.AttentionReason.
func ProtoToAttentionReason(reason sessionv1.AttentionReason) session.AttentionReason {
	switch reason {
	case sessionv1.AttentionReason_ATTENTION_REASON_APPROVAL_PENDING:
		return session.ReasonApprovalPending
	case sessionv1.AttentionReason_ATTENTION_REASON_INPUT_REQUIRED:
		return session.ReasonInputRequired
	case sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE:
		return session.ReasonErrorState
	case sessionv1.AttentionReason_ATTENTION_REASON_IDLE_TIMEOUT:
		return session.ReasonIdleTimeout
	case sessionv1.AttentionReason_ATTENTION_REASON_TASK_COMPLETE:
		return session.ReasonTaskComplete
	case sessionv1.AttentionReason_ATTENTION_REASON_UNCOMMITTED_CHANGES:
		return session.ReasonUncommittedChanges
	case sessionv1.AttentionReason_ATTENTION_REASON_IDLE:
		return session.ReasonIdle
	case sessionv1.AttentionReason_ATTENTION_REASON_STALE:
		return session.ReasonStale
	case sessionv1.AttentionReason_ATTENTION_REASON_WAITING_FOR_USER:
		return session.ReasonWaitingForUser
	case sessionv1.AttentionReason_ATTENTION_REASON_TESTS_FAILING:
		return session.ReasonTestsFailing
	default:
		return session.ReasonInputRequired // Default to input required
	}
}
