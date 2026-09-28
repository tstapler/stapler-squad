package services

// nudge_cap_store.go — Epic 3.3: ADR-003's durable, mutex-guarded, actually-
// blocking nudge cap/cooldown
// (project_plans/backlog-diagnose-and-nudge/decisions/ADR-003-nudge-cap-cooldown-shape.md).
//
// The ent-backed persistence lives in the session package (session/storage_nudge_cap.go),
// not here, because this package's no_ent_in_services depguard rule
// (.golangci.yml) forbids a new server/services file from importing
// session/ent directly. NudgeCapStore below consumes it only through the
// narrow nudgeCapPersistence interface and the plain session.NudgeCapRecordData
// DTO -- mirroring supersededSessionStore's narrow-interface style
// (server/services/superseded_session_sweeper.go).
//
// Story 3.3.3's cap-check (NewNudgeCapGateCheck) deliberately does NOT define
// session/diagnose's NudgeGateCheck function type -- that lands with Story
// 3.4.1, a later worker. Defining it here would require session/diagnose to
// know about server/services to reference the type, or server/services to
// duplicate a type session/diagnose should own. Instead this returns a plain
// closure whose signature already matches what NudgeGateCheck is specified to
// be (func(ctx, itemID) (bool, diagnose.SafetyGateReason)); Story 3.4.1 wires
// it in by value with no import cycle, since a function value is assignable
// to a named func type without either side importing the other's package.

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// nudgeCapPersistence is the narrow storage surface NudgeCapStore needs.
// Satisfied by *session.Storage.
type nudgeCapPersistence interface {
	GetNudgeCapRecord(ctx context.Context, itemID string) (session.NudgeCapRecordData, error)
	ReserveNudgeCapRecord(ctx context.Context, itemID string) (session.NudgeCapRecordData, error)
}

// NudgeCapStore is a narrow repository over ADR-003's NudgeCapRecord rows.
type NudgeCapStore interface {
	// Get returns itemID's current cap/cooldown state, or a zero-value record
	// (NudgeCount 0, no LastNudgeAt) if itemID has never been nudged.
	Get(ctx context.Context, itemID string) (session.NudgeCapRecordData, error)
	// CheckAndReserve atomically checks itemID against cap/cooldown and, on
	// success, reserves one nudge. Returns (false, nil) -- never an error --
	// when the item is already at cap or still within its cooldown window;
	// callers must treat false as a hard stop (Story 3.3.3), never log-only.
	CheckAndReserve(ctx context.Context, itemID string, cap int, cooldown time.Duration) (bool, error)
}

// diagnoseNudgeGuardMu serializes CheckAndReserve's read-check-increment-write
// sequence across ALL items, not just the same one -- mirrors
// julesSpendGuardMu's exact precedent (jules_dispatch_service.go), guarding
// against the same race class: without a package-level lock, two concurrent
// CheckAndReserve calls (even for different items, and certainly for the
// same one) can each read a stale NudgeCount/LastNudgeAt and both decide to
// proceed, reopening the JulesDispatchService spend-cap race ADR-003 was
// written to close. Nudge dispatch is not a hot path, so a single
// process-wide lock is cheap.
var diagnoseNudgeGuardMu sync.Mutex

// entNudgeCapStore is the ent-backed NudgeCapStore implementation (Task
// 3.3.1c), built on nudgeCapPersistence rather than session/ent directly.
type entNudgeCapStore struct {
	persistence nudgeCapPersistence
}

// NewNudgeCapStore constructs a NudgeCapStore backed by persistence
// (typically *session.Storage).
func NewNudgeCapStore(persistence nudgeCapPersistence) NudgeCapStore {
	return &entNudgeCapStore{persistence: persistence}
}

// Get returns itemID's current cap/cooldown state. Unguarded by
// diagnoseNudgeGuardMu -- a plain read has nothing to race with, and the
// mutex exists to protect the check-then-increment window, not every read.
func (s *entNudgeCapStore) Get(ctx context.Context, itemID string) (session.NudgeCapRecordData, error) {
	return s.persistence.GetNudgeCapRecord(ctx, itemID)
}

// CheckAndReserve implements Story 3.3.1's AC and Story 3.3.2's concurrency
// guard: the entire read-check-increment-write sequence runs under
// diagnoseNudgeGuardMu, so two racing callers for the same (or a different)
// item can never both observe an under-cap/expired-cooldown state and both
// succeed.
func (s *entNudgeCapStore) CheckAndReserve(ctx context.Context, itemID string, cap int, cooldown time.Duration) (bool, error) {
	diagnoseNudgeGuardMu.Lock()
	defer diagnoseNudgeGuardMu.Unlock()

	rec, err := s.persistence.GetNudgeCapRecord(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("nudge cap store: get record for item %s: %w", itemID, err)
	}

	if rec.NudgeCount >= cap {
		return false, nil
	}
	if cooldown > 0 && rec.LastNudgeAt != nil && time.Since(*rec.LastNudgeAt) < cooldown {
		return false, nil
	}

	if _, err := s.persistence.ReserveNudgeCapRecord(ctx, itemID); err != nil {
		return false, fmt.Errorf("nudge cap store: reserve for item %s: %w", itemID, err)
	}
	return true, nil
}

// NewNudgeCapGateCheck returns the cap-check closure Story 3.3.3 calls for
// -- shaped to match the forthcoming session/diagnose.NudgeGateCheck function
// type (Story 3.4.1): func(ctx, itemID) (bool, diagnose.SafetyGateReason).
// cfg's DiagnoseNudge cap/cooldown are read fresh on every call (not captured
// at construction) so a live config change
// (feedback_rollout_flags_live_settable_no_env_vars) takes effect
// immediately, mirroring cfgFn's use elsewhere in this package
// (jules_dispatch_service.go).
//
// A CheckAndReserve failure -- whether "at cap/cooldown" (false, nil) or an
// unexpected store error -- always returns (false, SafetyGateReasonNudgeCapReached):
// this story's whole purpose is that a cap hit blocks the write, never
// logs-only (the crash-loop-restart-storm precedent, ADR-003's Context). An
// unexpected store error is logged and treated as fail-closed rather than
// propagated, since NudgeGateCheck's signature carries no error return --
// ambiguity about whether the reservation succeeded must resolve to "don't
// write", not "assume it's fine".
func NewNudgeCapGateCheck(store NudgeCapStore, cfgFn func() *config.Config) func(ctx context.Context, itemID string) (bool, diagnose.SafetyGateReason) {
	return func(ctx context.Context, itemID string) (bool, diagnose.SafetyGateReason) {
		var nudgeCfg config.DiagnoseNudgeConfig
		if cfg := cfgFn(); cfg != nil {
			nudgeCfg = cfg.DiagnoseNudge
		}
		maxNudges := nudgeCfg.MaxNudgesPerItemOrDefault()
		cooldown := time.Duration(nudgeCfg.CooldownSecondsOrDefault()) * time.Second

		ok, err := store.CheckAndReserve(ctx, itemID, maxNudges, cooldown)
		if err != nil {
			log.Error("nudge cap store check failed, failing closed", "item_id", itemID, "error", err)
			return false, diagnose.SafetyGateReasonNudgeCapReached
		}
		if !ok {
			return false, diagnose.SafetyGateReasonNudgeCapReached
		}
		return true, ""
	}
}
