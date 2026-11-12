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
)
