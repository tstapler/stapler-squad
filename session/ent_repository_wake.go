package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/waketrigger"
)

// Wake-trigger persistence (ADR-006). Every state change is one guarded
// UPDATE ... WHERE id = ? AND status IN (<sources of the target in
// domain.WakeStatus's transition table>), so concurrent callers race to a
// single winner and everyone else gets applied=false. Methods take the
// current time as a parameter instead of reading the clock, so the dispatcher
// (and tests) control time.

// DefaultWakePendingPerOwnerCap is the fallback for CreateWakeTriggerInput.PendingCap
// (the "Pending triggers per owner session" row of the plan's Wake limits).
const DefaultWakePendingPerOwnerCap = 5

var (
	// ErrWakeCapExceeded: creating the row would push the owner past its live-trigger cap.
	ErrWakeCapExceeded = errors.New("wake trigger cap exceeded")
	// ErrWakeTransitionRejected: the requested transition is structurally
	// forbidden for this row (for example skipping a pending cron row), as
	// opposed to merely losing a race, which reports applied=false.
	ErrWakeTransitionRejected = errors.New("wake trigger transition rejected")
)

// WakeTriggerData is the plain, ent-independent view of a WakeTrigger row.
type WakeTriggerData struct {
	ID                 string
	DedupKey           domain.WakeDedupKey
	OwnerSessionUUID   string
	Type               domain.WakeTriggerType
	Spec               domain.WakeSpec
	Action             domain.WakeAction
	Status             domain.WakeStatus
	EndReason          domain.WakeReason
	ClaimedAt          *time.Time
	DeliveredAt        *time.Time
	DeliveryAttempts   int
	RateLimitDeferrals int
	NextFireAt         *time.Time
	FireCount          int
	MaxFires           int
	Generation         int
	AllowRevive        bool
	ExpiresAt          *time.Time
	CreatedAt          time.Time
	EndedAt            *time.Time
}

// CreateWakeTriggerInput carries the fields needed by CreateWakeTrigger.
type CreateWakeTriggerInput struct {
	OwnerSessionUUID string
	Spec             domain.WakeSpec
	Action           domain.WakeAction
	// NextFireAt is required for time-based specs (at, cron).
	NextFireAt  *time.Time
	MaxFires    int // <= 0 means 1
	Generation  int
	AllowRevive bool
	ExpiresAt   *time.Time
	// PendingCap bounds the owner's live (pending, claimed, needs_revive)
	// rows; <= 0 means DefaultWakePendingPerOwnerCap.
	PendingCap int
}

// SetWakeChangeHook registers fn to run after every WakeTrigger row change.
// Call once at wiring, before the repository is shared.
func (r *EntRepository) SetWakeChangeHook(fn func()) { r.wakeChanged = fn }

func (r *EntRepository) notifyWakeChanged() {
	if r.wakeChanged != nil {
		r.wakeChanged()
	}
}

func wakeRowToData(row *ent.WakeTrigger) (*WakeTriggerData, error) {
	typ := domain.WakeTriggerType(row.TriggerType)
	spec, err := domain.ParseWakeSpec(typ, row.SpecJSON)
	if err != nil {
		return nil, fmt.Errorf("wake trigger %s: %w", row.ID, err)
	}
	return &WakeTriggerData{
		ID:                 row.ID.String(),
		DedupKey:           domain.WakeDedupKey(row.DedupKey),
		OwnerSessionUUID:   row.OwnerSessionUUID,
		Type:               typ,
		Spec:               spec,
		Action:             domain.WakeAction{Kind: domain.WakeActionKind(row.ActionKind), Payload: row.ActionPayload},
		Status:             domain.WakeStatus(row.Status),
		EndReason:          domain.WakeReason(row.EndReason),
		ClaimedAt:          row.ClaimedAt,
		DeliveredAt:        row.DeliveredAt,
		DeliveryAttempts:   row.DeliveryAttempts,
		RateLimitDeferrals: row.RateLimitDeferrals,
		NextFireAt:         row.NextFireAt,
		FireCount:          row.FireCount,
		MaxFires:           row.MaxFires,
		Generation:         row.Generation,
		AllowRevive:        row.AllowRevive,
		ExpiresAt:          row.ExpiresAt,
		CreatedAt:          row.CreatedAt,
		EndedAt:            row.EndedAt,
	}, nil
}

func wakeRowsToData(rows []*ent.WakeTrigger) ([]*WakeTriggerData, error) {
	out := make([]*WakeTriggerData, 0, len(rows))
	for _, row := range rows {
		d, err := wakeRowToData(row)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

var liveWakeStatuses = []string{
	string(domain.WakeStatusPending), string(domain.WakeStatusClaimed), string(domain.WakeStatusNeedsRevive),
}

func wakeStatusStrings(in []domain.WakeStatus) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = string(s)
	}
	return out
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// CreateWakeTrigger inserts a trigger, or returns the existing live row with
// the same dedup key (created=false). Dedup resolves before the owner cap, so
// re-creating an identical trigger at the cap is not rejected. The cap count
// runs in the same transaction as the insert (SQLite's single connection
// serializes it against concurrent creates).
func (r *EntRepository) CreateWakeTrigger(ctx context.Context, in CreateWakeTriggerInput) (data *WakeTriggerData, created bool, err error) {
	key, specJSON, err := validateCreateWakeTriggerInput(in)
	if err != nil {
		return nil, false, err
	}
	capLimit := in.PendingCap
	if capLimit <= 0 {
		capLimit = DefaultWakePendingPerOwnerCap
	}
	maxFires := in.MaxFires
	if maxFires <= 0 {
		maxFires = 1
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create wake trigger: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	existing, err := tx.WakeTrigger.Query().Where(waketrigger.DedupKey(string(key))).Only(ctx)
	switch {
	case err == nil:
		d, convErr := wakeRowToData(existing)
		return d, false, convErr
	case !ent.IsNotFound(err):
		return nil, false, fmt.Errorf("create wake trigger: query existing: %w", err)
	}

	live, err := tx.WakeTrigger.Query().
		Where(waketrigger.OwnerSessionUUID(in.OwnerSessionUUID), waketrigger.StatusIn(liveWakeStatuses...)).
		Count(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create wake trigger: cap count: %w", err)
	}
	if live >= capLimit {
		return nil, false, fmt.Errorf("%w: owner=%s live=%d cap=%d", ErrWakeCapExceeded, in.OwnerSessionUUID, live, capLimit)
	}

	create := tx.WakeTrigger.Create().
		SetDedupKey(string(key)).
		SetOwnerSessionUUID(in.OwnerSessionUUID).
		SetTriggerType(string(in.Spec.TriggerType())).
		SetSpecJSON(specJSON).
		SetActionKind(string(in.Action.Kind)).
		SetActionPayload(in.Action.Payload).
		SetMaxFires(maxFires).
		SetGeneration(in.Generation).
		SetAllowRevive(in.AllowRevive).
		SetNillableNextFireAt(utcPtr(in.NextFireAt)).
		SetNillableExpiresAt(utcPtr(in.ExpiresAt))
	row, err := create.Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create wake trigger: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("create wake trigger: commit: %w", err)
	}
	r.notifyWakeChanged()
	d, err := wakeRowToData(row)
	return d, true, err
}

func validateCreateWakeTriggerInput(in CreateWakeTriggerInput) (key domain.WakeDedupKey, specJSON string, err error) {
	if in.Spec == nil {
		return "", "", errors.New("create wake trigger: spec is required")
	}
	if !in.Action.Kind.IsValid() {
		return "", "", fmt.Errorf("create wake trigger: invalid action kind %q", in.Action.Kind)
	}
	if in.Spec.TriggerType().IsTimeBased() && in.NextFireAt == nil {
		return "", "", fmt.Errorf("create wake trigger: %s requires next_fire_at", in.Spec.TriggerType())
	}
	if key, err = domain.NewWakeDedupKey(in.OwnerSessionUUID, in.Spec, in.Action); err != nil {
		return "", "", fmt.Errorf("create wake trigger: %w", err)
	}
	if specJSON, err = domain.MarshalWakeSpec(in.Spec); err != nil {
		return "", "", fmt.Errorf("create wake trigger: %w", err)
	}
	return key, specJSON, nil
}

// GetWakeTrigger reads one row; ent.IsNotFound reports a missing id.
func (r *EntRepository) GetWakeTrigger(ctx context.Context, id uuid.UUID) (*WakeTriggerData, error) {
	row, err := r.client.WakeTrigger.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get wake trigger %s: %w", id, err)
	}
	return wakeRowToData(row)
}
