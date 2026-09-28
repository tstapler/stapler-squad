package services

// diagnose_dispatch_store.go — Story 5.2.1: DiagnoseDispatch persistence
// (project_plans/backlog-diagnose-and-nudge/implementation/plan.md).
//
// The ent-backed query/mutation logic lives in the session package
// (session/storage_diagnose_dispatch.go), not here, because this package's
// no_ent_in_services depguard rule (.golangci.yml) forbids a new
// server/services file from importing session/ent directly -- the same split
// nudge_cap_store.go documents for NudgeCapStore. DiagnoseDispatchStore below
// consumes it only through the narrow diagnoseDispatchPersistence interface
// and session's plain DiagnoseDispatch* DTOs, converting to/from
// session/diagnose's typed enums at this boundary.

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// diagnoseDispatchPersistence is the narrow storage surface
// DiagnoseDispatchStore needs. Satisfied by *session.Storage.
type diagnoseDispatchPersistence interface {
	CreateDiagnoseDispatch(ctx context.Context, in session.DiagnoseDispatchCreateInput) (string, error)
	CompleteDiagnoseDispatch(ctx context.Context, dispatchID string, outcome session.DiagnoseDispatchOutcomeData) error
	ListDiagnoseDispatchesByItem(ctx context.Context, itemID string) ([]session.DiagnoseDispatchData, error)
}

// DiagnoseDispatchRequest describes a new dispatch to record.
type DiagnoseDispatchRequest struct {
	ItemID                string
	TargetSessionUUID     string
	DiagnosticSessionUUID string
}

// DiagnoseDispatchRecord is the domain-typed view of a DiagnoseDispatch row,
// handed back to callers such as ListDiagnoseDispatches (Story 7.1.1) and
// notifyDiagnoseEvent (Story 5.2.2).
type DiagnoseDispatchRecord struct {
	ID                    string
	ItemID                string
	TargetSessionUUID     string
	DiagnosticSessionUUID string
	Status                diagnose.DiagnoseDispatchStatus
	Outcome               *diagnose.DiagnoseOutcome
	CreatedAt             time.Time
	CompletedAt           *time.Time
}

// DiagnoseDispatchStore is a narrow repository over DiagnoseDispatch rows.
type DiagnoseDispatchStore interface {
	// Record inserts a new row with Status Pending and no OutcomeKind,
	// returning the generated dispatch ID.
	Record(ctx context.Context, req DiagnoseDispatchRequest) (dispatchID string, err error)
	// MarkCompleted updates the same dispatchID row to Status Completed with
	// outcome's fields and CompletedAt set -- never a second row.
	MarkCompleted(ctx context.Context, dispatchID string, outcome diagnose.DiagnoseOutcome) error
	// ListByItem returns all dispatches for itemID, chronological oldest-first.
	ListByItem(ctx context.Context, itemID string) ([]DiagnoseDispatchRecord, error)
}

// entDiagnoseDispatchStore is the ent-backed DiagnoseDispatchStore
// implementation (Task 5.2.1c), built on diagnoseDispatchPersistence rather
// than session/ent directly.
type entDiagnoseDispatchStore struct {
	persistence diagnoseDispatchPersistence
}

// NewDiagnoseDispatchStore constructs a DiagnoseDispatchStore backed by
// persistence (typically *session.Storage).
func NewDiagnoseDispatchStore(persistence diagnoseDispatchPersistence) DiagnoseDispatchStore {
	return &entDiagnoseDispatchStore{persistence: persistence}
}

// Record implements Story 5.2.1's AC: a fresh Pending row, no outcome yet.
func (s *entDiagnoseDispatchStore) Record(ctx context.Context, req DiagnoseDispatchRequest) (string, error) {
	dispatchID, err := s.persistence.CreateDiagnoseDispatch(ctx, session.DiagnoseDispatchCreateInput{
		ItemID:                req.ItemID,
		TargetSessionUUID:     req.TargetSessionUUID,
		DiagnosticSessionUUID: req.DiagnosticSessionUUID,
	})
	if err != nil {
		return "", fmt.Errorf("diagnose dispatch store: record for item %s: %w", req.ItemID, err)
	}
	return dispatchID, nil
}

// MarkCompleted implements Story 5.2.1's AC: updates the same row in place.
// outcome is validated before persisting -- an invalid Kind/field pairing
// must never reach storage as a malformed row.
func (s *entDiagnoseDispatchStore) MarkCompleted(ctx context.Context, dispatchID string, outcome diagnose.DiagnoseOutcome) error {
	if err := outcome.Validate(); err != nil {
		return fmt.Errorf("diagnose dispatch store: mark completed for dispatch %s: %w", dispatchID, err)
	}

	if err := s.persistence.CompleteDiagnoseDispatch(ctx, dispatchID, outcomeToData(outcome)); err != nil {
		return fmt.Errorf("diagnose dispatch store: mark completed for dispatch %s: %w", dispatchID, err)
	}
	return nil
}

// ListByItem implements Story 5.2.1's AC: chronological, oldest-first.
func (s *entDiagnoseDispatchStore) ListByItem(ctx context.Context, itemID string) ([]DiagnoseDispatchRecord, error) {
	rows, err := s.persistence.ListDiagnoseDispatchesByItem(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("diagnose dispatch store: list for item %s: %w", itemID, err)
	}
	records := make([]DiagnoseDispatchRecord, len(rows))
	for i, row := range rows {
		records[i] = dataToRecord(row)
	}
	return records, nil
}

// outcomeToData converts a validated diagnose.DiagnoseOutcome into the
// session package's ent-free write DTO.
func outcomeToData(outcome diagnose.DiagnoseOutcome) session.DiagnoseDispatchOutcomeData {
	var safetyGateReason *string
	if outcome.GateReason != nil {
		reason := string(*outcome.GateReason)
		safetyGateReason = &reason
	}
	return session.DiagnoseDispatchOutcomeData{
		OutcomeKind:      string(outcome.Kind),
		SafetyGateReason: safetyGateReason,
		BugItemID:        outcome.BugItemID,
		NoteText:         outcome.NoteText,
		WriteAttempted:   outcome.WriteAttempted,
		FailureReason:    outcome.FailureReason,
	}
}

// dataToRecord converts the session package's ent-free read DTO into the
// domain-typed DiagnoseDispatchRecord, reconstructing a *diagnose.DiagnoseOutcome
// only once the row has completed (OutcomeKind set); a Pending or Stalled row
// carries no outcome, matching diagnose.DiagnoseOutcomeKind's closed sum type.
func dataToRecord(row session.DiagnoseDispatchData) DiagnoseDispatchRecord {
	record := DiagnoseDispatchRecord{
		ID:                    row.ID,
		ItemID:                row.ItemID,
		TargetSessionUUID:     row.TargetSessionUUID,
		DiagnosticSessionUUID: row.DiagnosticSessionUUID,
		Status:                diagnose.DiagnoseDispatchStatus(row.Status),
		CreatedAt:             row.CreatedAt,
		CompletedAt:           row.CompletedAt,
	}
	if row.OutcomeKind != nil {
		outcome := diagnose.DiagnoseOutcome{
			Kind:           diagnose.DiagnoseOutcomeKind(*row.OutcomeKind),
			BugItemID:      row.BugItemID,
			NoteText:       row.NoteText,
			WriteAttempted: row.WriteAttempted,
			FailureReason:  row.FailureReason,
		}
		if row.SafetyGateReason != nil {
			reason := diagnose.SafetyGateReason(*row.SafetyGateReason)
			outcome.GateReason = &reason
		}
		record.Outcome = &outcome
	}
	return record
}
