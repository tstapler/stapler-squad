package session

// storage_diagnose_dispatch.go — Story 5.2.1's DiagnoseDispatch persistence
// (project_plans/backlog-diagnose-and-nudge/implementation/plan.md). Mirrors
// storage_nudge_cap.go's split: the ent-backed query/mutation logic lives
// here in the session package (which may import session/ent), while
// server/services/diagnose_dispatch_store.go -- forbidden by the
// no_ent_in_services depguard rule (.golangci.yml) from importing
// session/ent directly -- consumes only the plain DiagnoseDispatchData /
// DiagnoseDispatchOutcomeData DTOs below through a narrow interface.
//
// Status/OutcomeKind/SafetyGateReason travel as plain strings here, not
// session/diagnose's typed enums: this package has no reason to import
// session/diagnose, and server/services already does, so the string<->enum
// conversion belongs there.

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/diagnosedispatch"
)

// DiagnoseDispatchCreateInput groups the fields needed to create a new
// DiagnoseDispatch row -- a value object in place of a same-typed
// (itemID, targetSessionUUID, diagnosticSessionUUID string) parameter pile
// (primitive-obsession-checklist).
type DiagnoseDispatchCreateInput struct {
	ItemID                string
	TargetSessionUUID     string
	DiagnosticSessionUUID string
}

// DiagnoseDispatchData is the ent-free view of a DiagnoseDispatch row.
type DiagnoseDispatchData struct {
	ID                    string
	ItemID                string
	TargetSessionUUID     string
	DiagnosticSessionUUID string
	Status                string
	OutcomeKind           *string
	SafetyGateReason      *string
	BugItemID             *string
	NoteText              *string
	WriteAttempted        *bool
	FailureReason         *string
	CreatedAt             time.Time
	CompletedAt           *time.Time
}

// DiagnoseDispatchOutcomeData carries the outcome fields
// CompleteDiagnoseDispatch applies when marking a row Completed.
type DiagnoseDispatchOutcomeData struct {
	OutcomeKind      string
	SafetyGateReason *string
	BugItemID        *string
	NoteText         *string
	WriteAttempted   *bool
	FailureReason    *string
}

// CreateDiagnoseDispatch persists a new DiagnoseDispatch row with status
// "pending" and no outcome, returning its generated ID. in.DiagnosticSessionUUID
// may be empty -- Task 5.1.1d records this row before the diagnostic session
// exists, so a mid-dispatch page refresh still has something to read.
func (r *EntRepository) CreateDiagnoseDispatch(ctx context.Context, in DiagnoseDispatchCreateInput) (string, error) {
	id := uuid.New().String()
	row, err := r.client.DiagnoseDispatch.Create().
		SetID(id).
		SetItemID(in.ItemID).
		SetTargetSessionUUID(in.TargetSessionUUID).
		SetDiagnosticSessionUUID(in.DiagnosticSessionUUID).
		Save(ctx)
	if err != nil {
		return "", fmt.Errorf("create diagnose dispatch for item %s: %w", in.ItemID, err)
	}
	return row.ID, nil
}

// CompleteDiagnoseDispatch updates dispatchID's existing row to status
// "completed" with outcome's fields and CompletedAt set -- never a second
// row (Story 5.2.1 AC).
func (r *EntRepository) CompleteDiagnoseDispatch(ctx context.Context, dispatchID string, outcome DiagnoseDispatchOutcomeData) error {
	_, err := r.client.DiagnoseDispatch.UpdateOneID(dispatchID).
		SetStatus("completed").
		SetOutcomeKind(outcome.OutcomeKind).
		SetNillableSafetyGateReason(outcome.SafetyGateReason).
		SetNillableBugItemID(outcome.BugItemID).
		SetNillableNoteText(outcome.NoteText).
		SetNillableWriteAttempted(outcome.WriteAttempted).
		SetNillableFailureReason(outcome.FailureReason).
		SetCompletedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("diagnose dispatch %s not found: %w", dispatchID, err)
		}
		return fmt.Errorf("complete diagnose dispatch %s: %w", dispatchID, err)
	}
	return nil
}

// ListDiagnoseDispatchesByItem returns all DiagnoseDispatch rows for itemID,
// ordered by created_at ascending (oldest first).
func (r *EntRepository) ListDiagnoseDispatchesByItem(ctx context.Context, itemID string) ([]DiagnoseDispatchData, error) {
	rows, err := r.client.DiagnoseDispatch.Query().
		Where(diagnosedispatch.ItemID(itemID)).
		Order(ent.Asc(diagnosedispatch.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list diagnose dispatches for item %s: %w", itemID, err)
	}
	result := make([]DiagnoseDispatchData, len(rows))
	for i, row := range rows {
		result[i] = dataFromEntDiagnoseDispatch(row)
	}
	return result, nil
}

func dataFromEntDiagnoseDispatch(row *ent.DiagnoseDispatch) DiagnoseDispatchData {
	return DiagnoseDispatchData{
		ID:                    row.ID,
		ItemID:                row.ItemID,
		TargetSessionUUID:     row.TargetSessionUUID,
		DiagnosticSessionUUID: row.DiagnosticSessionUUID,
		Status:                row.Status,
		OutcomeKind:           row.OutcomeKind,
		SafetyGateReason:      row.SafetyGateReason,
		BugItemID:             row.BugItemID,
		NoteText:              row.NoteText,
		WriteAttempted:        row.WriteAttempted,
		FailureReason:         row.FailureReason,
		CreatedAt:             row.CreatedAt,
		CompletedAt:           row.CompletedAt,
	}
}

// CreateDiagnoseDispatch persists a new DiagnoseDispatch row. See
// EntRepository.CreateDiagnoseDispatch.
func (s *Storage) CreateDiagnoseDispatch(ctx context.Context, in DiagnoseDispatchCreateInput) (string, error) {
	return s.repo.CreateDiagnoseDispatch(ctx, in)
}

// CompleteDiagnoseDispatch marks dispatchID's row Completed with outcome. See
// EntRepository.CompleteDiagnoseDispatch.
func (s *Storage) CompleteDiagnoseDispatch(ctx context.Context, dispatchID string, outcome DiagnoseDispatchOutcomeData) error {
	return s.repo.CompleteDiagnoseDispatch(ctx, dispatchID, outcome)
}

// ListDiagnoseDispatchesByItem returns all DiagnoseDispatch rows for itemID.
// See EntRepository.ListDiagnoseDispatchesByItem.
func (s *Storage) ListDiagnoseDispatchesByItem(ctx context.Context, itemID string) ([]DiagnoseDispatchData, error) {
	return s.repo.ListDiagnoseDispatchesByItem(ctx, itemID)
}
