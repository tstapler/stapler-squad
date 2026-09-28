package session

// storage_nudge_cap.go — ADR-003's durable NudgeCapRecord row
// (project_plans/backlog-diagnose-and-nudge/decisions/ADR-003-nudge-cap-cooldown-shape.md).
// The ent-backed query logic lives here, in the session package, rather than
// in server/services (where NudgeCapStore's mutex-guarded orchestration
// lives) because server/services' no_ent_in_services depguard rule
// (.golangci.yml) forbids new files there from importing session/ent
// directly — the same reason TestBackdateCreationProgress (session/testing.go)
// exists instead of server/services touching session/ent for that fixture.
// Crosses the package boundary only as the plain NudgeCapRecordData DTO.

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/nudgecaprecord"
)

// NudgeCapRecordData is the ent-free view of a NudgeCapRecord row.
type NudgeCapRecordData struct {
	ItemID        string
	NudgeCount    int
	WindowStartAt *time.Time
	LastNudgeAt   *time.Time
}

// GetNudgeCapRecord returns itemID's nudge cap/cooldown state, or a zero-value
// record (NudgeCount 0, no LastNudgeAt) if itemID has never been nudged --
// "never nudged" is not an error condition (Story 3.3.1 AC).
func (r *EntRepository) GetNudgeCapRecord(ctx context.Context, itemID string) (NudgeCapRecordData, error) {
	row, err := r.client.NudgeCapRecord.Query().
		Where(nudgecaprecord.ItemID(itemID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return NudgeCapRecordData{ItemID: itemID}, nil
		}
		return NudgeCapRecordData{}, fmt.Errorf("get nudge cap record for item %s: %w", itemID, err)
	}
	return dataFromEntNudgeCapRecord(row), nil
}

// ReserveNudgeCapRecord unconditionally persists one more nudge reservation
// for itemID: creates a fresh row with NudgeCount at 1 and both timestamps
// set to now if none exists yet, or increments NudgeCount and stamps
// LastNudgeAt on an existing one. The cap/cooldown decision belongs to the
// caller -- this method has no notion of a limit. The sole intended caller is
// server/services' diagnoseNudgeGuardMu-guarded NudgeCapStore.CheckAndReserve
// (ADR-003); calling this directly without holding that mutex reopens the
// JulesDispatchService check-then-reserve race ADR-003 closed.
func (r *EntRepository) ReserveNudgeCapRecord(ctx context.Context, itemID string) (NudgeCapRecordData, error) {
	now := time.Now().UTC()

	existing, err := r.client.NudgeCapRecord.Query().
		Where(nudgecaprecord.ItemID(itemID)).
		Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return NudgeCapRecordData{}, fmt.Errorf("get nudge cap record for item %s: %w", itemID, err)
		}
		row, createErr := r.client.NudgeCapRecord.Create().
			SetItemID(itemID).
			SetNudgeCount(1).
			SetWindowStartAt(now).
			SetLastNudgeAt(now).
			Save(ctx)
		if createErr != nil {
			return NudgeCapRecordData{}, fmt.Errorf("create nudge cap record for item %s: %w", itemID, createErr)
		}
		return dataFromEntNudgeCapRecord(row), nil
	}

	row, err := existing.Update().
		SetNudgeCount(existing.NudgeCount + 1).
		SetLastNudgeAt(now).
		Save(ctx)
	if err != nil {
		return NudgeCapRecordData{}, fmt.Errorf("increment nudge cap record for item %s: %w", itemID, err)
	}
	return dataFromEntNudgeCapRecord(row), nil
}

func dataFromEntNudgeCapRecord(row *ent.NudgeCapRecord) NudgeCapRecordData {
	return NudgeCapRecordData{
		ItemID:        row.ItemID,
		NudgeCount:    row.NudgeCount,
		WindowStartAt: row.WindowStartAt,
		LastNudgeAt:   row.LastNudgeAt,
	}
}

// GetNudgeCapRecord returns itemID's nudge cap/cooldown state. See
// EntRepository.GetNudgeCapRecord.
func (s *Storage) GetNudgeCapRecord(ctx context.Context, itemID string) (NudgeCapRecordData, error) {
	return s.repo.GetNudgeCapRecord(ctx, itemID)
}

// ReserveNudgeCapRecord persists one more nudge reservation for itemID. See
// EntRepository.ReserveNudgeCapRecord.
func (s *Storage) ReserveNudgeCapRecord(ctx context.Context, itemID string) (NudgeCapRecordData, error) {
	return s.repo.ReserveNudgeCapRecord(ctx, itemID)
}
