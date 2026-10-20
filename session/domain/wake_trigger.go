package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// WakeTriggerType is the closed set of conditions that can wake a session.
// See ADR-006 (project_plans/notification-actions-and-agent-messaging).
type WakeTriggerType string

const (
	WakeTriggerAt               WakeTriggerType = "at"
	WakeTriggerCron             WakeTriggerType = "cron"
	WakeTriggerBacklogEvent     WakeTriggerType = "backlog_event"
	WakeTriggerPREvent          WakeTriggerType = "pr_event"
	WakeTriggerCIEvent          WakeTriggerType = "ci_event"
	WakeTriggerSessionState     WakeTriggerType = "session_state"
	WakeTriggerGuidanceAnswered WakeTriggerType = "guidance_answered"
)

var allWakeTriggerTypes = []WakeTriggerType{
	WakeTriggerAt, WakeTriggerCron, WakeTriggerBacklogEvent, WakeTriggerPREvent,
	WakeTriggerCIEvent, WakeTriggerSessionState, WakeTriggerGuidanceAnswered,
}

// AllWakeTriggerTypes returns every valid trigger type.
func AllWakeTriggerTypes() []WakeTriggerType {
	return append([]WakeTriggerType(nil), allWakeTriggerTypes...)
}

// IsValid reports whether t is a known trigger type.
func (t WakeTriggerType) IsValid() bool {
	for _, v := range allWakeTriggerTypes {
		if t == v {
			return true
		}
	}
	return false
}

// IsTimeBased reports whether the trigger fires from the clock (next_fire_at
// is set) rather than from an event.
func (t WakeTriggerType) IsTimeBased() bool {
	return t == WakeTriggerAt || t == WakeTriggerCron
}

// IsRecurring reports whether a row of this type cycles pending -> claimed ->
// pending between fires.
func (t WakeTriggerType) IsRecurring() bool { return t == WakeTriggerCron }

// WakeStatus is the lifecycle state of a WakeTrigger row.
type WakeStatus string

const (
	WakeStatusPending     WakeStatus = "pending"
	WakeStatusClaimed     WakeStatus = "claimed"
	WakeStatusDelivered   WakeStatus = "delivered"
	WakeStatusNeedsRevive WakeStatus = "needs_revive"
	WakeStatusMissed      WakeStatus = "missed"
	WakeStatusCancelled   WakeStatus = "cancelled"
	WakeStatusSkipped     WakeStatus = "skipped"
)

var allWakeStatuses = []WakeStatus{
	WakeStatusPending, WakeStatusClaimed, WakeStatusDelivered, WakeStatusNeedsRevive,
	WakeStatusMissed, WakeStatusCancelled, WakeStatusSkipped,
}

// AllWakeStatuses returns every valid status.
func AllWakeStatuses() []WakeStatus { return append([]WakeStatus(nil), allWakeStatuses...) }

// IsValid reports whether s is a known status.
func (s WakeStatus) IsValid() bool {
	for _, v := range allWakeStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// IsTerminal reports whether no transition leaves s.
func (s WakeStatus) IsTerminal() bool {
	switch s {
	case WakeStatusDelivered, WakeStatusMissed, WakeStatusCancelled, WakeStatusSkipped:
		return true
	default:
		return false
	}
}

// wakeTransitions is the complete transition table of ADR-006 rule 2. The
// repository rejects every pair not listed. claimed -> pending is the cron
// re-arm only; pending -> skipped is the one-shot rate-limit deferral only
// (both extra conditions are enforced by the repository, not by this table).
var wakeTransitions = map[WakeStatus][]WakeStatus{
	WakeStatusPending:     {WakeStatusClaimed, WakeStatusMissed, WakeStatusCancelled, WakeStatusSkipped},
	WakeStatusClaimed:     {WakeStatusDelivered, WakeStatusPending, WakeStatusNeedsRevive, WakeStatusMissed, WakeStatusSkipped, WakeStatusCancelled},
	WakeStatusNeedsRevive: {WakeStatusClaimed, WakeStatusMissed, WakeStatusSkipped, WakeStatusCancelled},
}

// CanTransitionTo reports whether the table allows s -> to.
func (s WakeStatus) CanTransitionTo(to WakeStatus) bool {
	for _, v := range wakeTransitions[s] {
		if v == to {
			return true
		}
	}
	return false
}

// WakeStatusesReaching returns every status with a table edge into to, in
// declaration order. The repository uses it as the WHERE status IN (...) guard.
func WakeStatusesReaching(to WakeStatus) []WakeStatus {
	var out []WakeStatus
	for _, from := range allWakeStatuses {
		if from.CanTransitionTo(to) {
			out = append(out, from)
		}
	}
	return out
}

// WakeReason explains why a row ended in missed or skipped (or was deferred).
type WakeReason string

const (
	WakeReasonRateLimited      WakeReason = "rate_limited"
	WakeReasonOwnerStopped     WakeReason = "owner_stopped"
	WakeReasonArchived         WakeReason = "archived"
	WakeReasonOneShot          WakeReason = "one_shot"
	WakeReasonItemTerminal     WakeReason = "item_terminal"
	WakeReasonOwnerMissing     WakeReason = "owner_missing"
	WakeReasonExpired          WakeReason = "expired"
	WakeReasonOwnerBusyTimeout WakeReason = "owner_busy_timeout"
	WakeReasonDSTGap           WakeReason = "dst_gap"
)

// WakeActionKind is what a fired trigger does.
type WakeActionKind string

const (
	// WakeActionSteerText writes a provenance-labelled prompt into the owner's pane.
	WakeActionSteerText WakeActionKind = "steer_text"
	// WakeActionNotifyOperator raises an operator notification; nothing is
	// written to a session, so the revival guards do not apply (ADR-006 rule 2).
	WakeActionNotifyOperator WakeActionKind = "notify_operator"
)

// IsValid reports whether k is a known action kind.
func (k WakeActionKind) IsValid() bool {
	return k == WakeActionSteerText || k == WakeActionNotifyOperator
}

// WakeAction is the validated action of a trigger.
type WakeAction struct {
	Kind    WakeActionKind
	Payload string
}

// NewWakeAction validates kind and payload (non-empty after trimming).
func NewWakeAction(kind WakeActionKind, payload string) (WakeAction, error) {
	if !kind.IsValid() {
		return WakeAction{}, fmt.Errorf("invalid wake action kind %q", kind)
	}
	if strings.TrimSpace(payload) == "" {
		return WakeAction{}, errors.New("wake action payload must not be empty")
	}
	return WakeAction{Kind: kind, Payload: payload}, nil
}

// WakeSpec is the per-type trigger condition, a sealed sum type: exactly the
// implementations below exist, and Canonical gives a deterministic form for
// dedup.
type WakeSpec interface {
	TriggerType() WakeTriggerType
	// Canonical is a deterministic rendering used for the dedup key.
	Canonical() string
	validate() error
	wakeSpec()
}

// AtSpec fires once at a wall-clock instant.
type AtSpec struct {
	At time.Time `json:"at"`
}

// CronSpec fires on a standard 5-field cron expression, optionally prefixed
// "CRON_TZ=<IANA zone> " (default zone UTC, applied by server/wakes).
type CronSpec struct {
	Expr string `json:"expr"`
}

// BacklogEventSpec fires when a backlog item reaches Kind (a BacklogChangeKind
// such as verdict_recorded).
type BacklogEventSpec struct {
	ItemID string `json:"item_id"`
	Kind   string `json:"kind"`
}

// PREventSpec fires on a pull-request event.
type PREventSpec struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Kind   string `json:"kind"`
}

// CIEventSpec fires on a CI event for a pull request.
type CIEventSpec struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Kind   string `json:"kind"`
}

// SessionStateSpec fires when another session reaches State.
type SessionStateSpec struct {
	SessionUUID string `json:"session_uuid"`
	State       string `json:"state"`
}

// GuidanceAnsweredSpec fires when a guidance request is answered.
type GuidanceAnsweredSpec struct {
	GuidanceRequestID string `json:"guidance_request_id"`
}

func (AtSpec) wakeSpec()               {}
func (CronSpec) wakeSpec()             {}
func (BacklogEventSpec) wakeSpec()     {}
func (PREventSpec) wakeSpec()          {}
func (CIEventSpec) wakeSpec()          {}
func (SessionStateSpec) wakeSpec()     {}
func (GuidanceAnsweredSpec) wakeSpec() {}

func (AtSpec) TriggerType() WakeTriggerType               { return WakeTriggerAt }
func (CronSpec) TriggerType() WakeTriggerType             { return WakeTriggerCron }
func (BacklogEventSpec) TriggerType() WakeTriggerType     { return WakeTriggerBacklogEvent }
func (PREventSpec) TriggerType() WakeTriggerType          { return WakeTriggerPREvent }
func (CIEventSpec) TriggerType() WakeTriggerType          { return WakeTriggerCIEvent }
func (SessionStateSpec) TriggerType() WakeTriggerType     { return WakeTriggerSessionState }
func (GuidanceAnsweredSpec) TriggerType() WakeTriggerType { return WakeTriggerGuidanceAnswered }

func (s AtSpec) Canonical() string   { return "at:" + s.At.UTC().Format(time.RFC3339Nano) }
func (s CronSpec) Canonical() string { return "cron:" + strings.Join(strings.Fields(s.Expr), " ") }
func (s BacklogEventSpec) Canonical() string {
	return "backlog_event:" + s.ItemID + ":" + s.Kind
}
func (s PREventSpec) Canonical() string {
	return fmt.Sprintf("pr_event:%s#%d:%s", s.Repo, s.Number, s.Kind)
}
func (s CIEventSpec) Canonical() string {
	return fmt.Sprintf("ci_event:%s#%d:%s", s.Repo, s.Number, s.Kind)
}
func (s SessionStateSpec) Canonical() string {
	return "session_state:" + s.SessionUUID + ":" + s.State
}
func (s GuidanceAnsweredSpec) Canonical() string {
	return "guidance_answered:" + s.GuidanceRequestID
}

// MarshalWakeSpec serializes spec for the spec_json column.
func MarshalWakeSpec(spec WakeSpec) (string, error) {
	if err := spec.validate(); err != nil {
		return "", fmt.Errorf("marshal wake spec: %w", err)
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshal wake spec: %w", err)
	}
	return string(b), nil
}

// ParseWakeSpec decodes the spec_json column for trigger type t and validates it.
func ParseWakeSpec(t WakeTriggerType, raw string) (WakeSpec, error) {
	var (
		spec WakeSpec
		err  error
	)
	switch t {
	case WakeTriggerAt:
		spec, err = decodeWakeSpec[AtSpec](raw)
	case WakeTriggerCron:
		spec, err = decodeWakeSpec[CronSpec](raw)
	case WakeTriggerBacklogEvent:
		spec, err = decodeWakeSpec[BacklogEventSpec](raw)
	case WakeTriggerPREvent:
		spec, err = decodeWakeSpec[PREventSpec](raw)
	case WakeTriggerCIEvent:
		spec, err = decodeWakeSpec[CIEventSpec](raw)
	case WakeTriggerSessionState:
		spec, err = decodeWakeSpec[SessionStateSpec](raw)
	case WakeTriggerGuidanceAnswered:
		spec, err = decodeWakeSpec[GuidanceAnsweredSpec](raw)
	default:
		return nil, fmt.Errorf("invalid wake trigger type %q", t)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s wake spec: %w", t, err)
	}
	return spec, nil
}

func decodeWakeSpec[T WakeSpec](raw string) (WakeSpec, error) {
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, err
	}
	if err := v.validate(); err != nil {
		return nil, err
	}
	return v, nil
}

type namedValue struct{ Name, Value string }

func requireNonEmpty(spec WakeSpec, fields ...namedValue) error {
	var errs []error
	for _, f := range fields {
		if strings.TrimSpace(f.Value) == "" {
			errs = append(errs, fmt.Errorf("%s wake spec: %s must not be empty", spec.TriggerType(), f.Name))
		}
	}
	return errors.Join(errs...)
}

func requirePositive(spec WakeSpec, v int) error {
	if v <= 0 {
		return fmt.Errorf("%s wake spec: number must be positive", spec.TriggerType())
	}
	return nil
}

func (s AtSpec) validate() error {
	if s.At.IsZero() {
		return errors.New("at wake spec: at must be set")
	}
	return nil
}
func (s CronSpec) validate() error { return requireNonEmpty(s, namedValue{"expr", s.Expr}) }
func (s BacklogEventSpec) validate() error {
	return requireNonEmpty(s, namedValue{"item_id", s.ItemID}, namedValue{wakeKindField, s.Kind})
}
func (s PREventSpec) validate() error {
	return errors.Join(requireNonEmpty(s, namedValue{"repo", s.Repo}, namedValue{wakeKindField, s.Kind}), requirePositive(s, s.Number))
}
func (s CIEventSpec) validate() error {
	return errors.Join(requireNonEmpty(s, namedValue{"repo", s.Repo}, namedValue{wakeKindField, s.Kind}), requirePositive(s, s.Number))
}
func (s SessionStateSpec) validate() error {
	return requireNonEmpty(s, namedValue{"session_uuid", s.SessionUUID}, namedValue{"state", s.State})
}
func (s GuidanceAnsweredSpec) validate() error {
	return requireNonEmpty(s, namedValue{"guidance_request_id", s.GuidanceRequestID})
}

const wakeKindField = "kind"

// WakeDedupKey is the non-null unique key of a live trigger. Holding one
// proves it was derived through NewWakeDedupKey (or tombstoned).
type WakeDedupKey string

// NewWakeDedupKey derives the key from owner, canonical spec and action. The
// action is included so two different reminders for the same instant do not
// collapse into one.
func NewWakeDedupKey(owner string, spec WakeSpec, action WakeAction) (WakeDedupKey, error) {
	if strings.TrimSpace(owner) == "" {
		return "", errors.New("wake dedup key: owner session uuid must not be empty")
	}
	if err := spec.validate(); err != nil {
		return "", fmt.Errorf("wake dedup key: %w", err)
	}
	h := sha256.New()
	for _, part := range []string{owner, string(spec.TriggerType()), spec.Canonical(), string(action.Kind), action.Payload} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return WakeDedupKey(hex.EncodeToString(h.Sum(nil))), nil
}

// Tombstone rewrites the key to "<key>#<id>" so a terminal row frees the live
// key while staying non-null and unique (ADR-006 rule 12).
func (k WakeDedupKey) Tombstone(id string) WakeDedupKey { return WakeDedupKey(string(k) + "#" + id) }

// IsTombstone reports whether the key has been tombstoned.
func (k WakeDedupKey) IsTombstone() bool { return strings.Contains(string(k), "#") }
