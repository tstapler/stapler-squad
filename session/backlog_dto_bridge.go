package session

// Temporary bridge for the backlogdto move; BL-E deletes it (alias_sunset_test.go ratchets it).

import "github.com/tstapler/stapler-squad/session/domain/backlogdto"

type (
	ActivityNoteData            = backlogdto.ActivityNoteData
	BacklogItemDependencyEdge   = backlogdto.BacklogItemDependencyEdge
	BacklogItemDependencyRef    = backlogdto.BacklogItemDependencyRef
	BacklogStatusEventData      = backlogdto.BacklogStatusEventData
	DeletedItemSessionCostEntry = backlogdto.DeletedItemSessionCostEntry
	DiagnoseResultData          = backlogdto.DiagnoseResultData
	ItemSourceData              = backlogdto.ItemSourceData
	ItemSourceUpdate            = backlogdto.ItemSourceUpdate
	OpenStuckStateData          = backlogdto.OpenStuckStateData
	ProgressNoteData            = backlogdto.ProgressNoteData
	ReviewVerdictData           = backlogdto.ReviewVerdictData
	ReviewVerdictSummary        = backlogdto.ReviewVerdictSummary
	SourceSyncEventData         = backlogdto.SourceSyncEventData
	StuckStateHistoryData       = backlogdto.StuckStateHistoryData
	BacklogItemData             = backlogdto.BacklogItemData
	BacklogItemSummary          = backlogdto.BacklogItemSummary
	BacklogItemUpdate           = backlogdto.BacklogItemUpdate
	BacklogItemFilter           = backlogdto.BacklogItemFilter
	BacklogItemPrecondition     = backlogdto.BacklogItemPrecondition
	ItemSessionSummary          = backlogdto.ItemSessionSummary
	ItemSessionData             = backlogdto.ItemSessionData
	ItemSessionBacklogEntry     = backlogdto.ItemSessionBacklogEntry
	PRReassignmentGuard         = backlogdto.PRReassignmentGuard
	BacklogItemID               = backlogdto.BacklogItemID
)

const BacklogItemIDPrefix = backlogdto.BacklogItemIDPrefix

// Same variable identity keeps errors.Is working across both names.
var (
	ErrDependencyCycle          = backlogdto.ErrDependencyCycle
	ErrPRReassignmentNotAllowed = backlogdto.ErrPRReassignmentNotAllowed
	ErrPRDetached               = backlogdto.ErrPRDetached
	ErrAcCriterionWaived        = backlogdto.ErrAcCriterionWaived
	ErrAcCriterionOutOfRange    = backlogdto.ErrAcCriterionOutOfRange
	ErrWaiverReasonRequired     = backlogdto.ErrWaiverReasonRequired
	ErrAcCriterionAlreadyDone   = backlogdto.ErrAcCriterionAlreadyDone
	ErrAcCriterionAlreadyWaived = backlogdto.ErrAcCriterionAlreadyWaived
	ErrAcWaiverViaUpdate        = backlogdto.ErrAcWaiverViaUpdate
)

var (
	NewBacklogItemID     = backlogdto.NewBacklogItemID
	ParseBacklogItemID   = backlogdto.ParseBacklogItemID
	IsBacklogItemIDShape = backlogdto.IsBacklogItemIDShape
)
