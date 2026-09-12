package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/guidancerequest"
)

// DefaultGuidanceRequestPendingCap is the fallback per-(scope, scope_key)
// pending cap used when a caller doesn't supply one (CreateGuidanceRequestInput.Cap
// == 0). Phase 4 of the durable-guidance-request plan wires a real,
// operator-configurable value (config.GetGuidanceRequestPendingCap) through as
// the Cap field on every call site; until then, this constant is what backs
// it. Cap is a caller-supplied parameter rather than something this method
// reads from config directly, so that wiring lands as a call-site change, not
// a change to CreateGuidanceRequest's own logic.
const DefaultGuidanceRequestPendingCap = 4

// ErrPendingCapExceeded is returned by CreateGuidanceRequest when creating a
// new GuidanceRequest would push a scope's open (pending) row count past its
// cap. Never returned for a call that resolves to a pre-existing open row via
// dedup — see CreateGuidanceRequest's doc comment.
var ErrPendingCapExceeded = errors.New("guidance request pending cap exceeded")

// GuidanceRequestStatus is the derived (never stored) lifecycle state of a
// GuidanceRequest row — see GuidanceRequestData.Status.
type GuidanceRequestStatus string

const (
	GuidanceRequestStatusPending   GuidanceRequestStatus = "pending"
	GuidanceRequestStatusAnswered  GuidanceRequestStatus = "answered"
	GuidanceRequestStatusCancelled GuidanceRequestStatus = "cancelled"
)

// GuidanceRequestData is the plain, ent-independent view of a GuidanceRequest
// row returned by this file's repository methods — mirrors OpenStuckStateData's
// role of exposing entity data outside the ent package.
type GuidanceRequestData struct {
	ID           string
	Scope        domain.RequestScope
	ItemID       *uuid.UUID
	SessionUUID  string
	QuestionText string
	QuestionType domain.QuestionType
	Options      string
	Answer       string
	CreatedAt    time.Time
	NotifiedAt   *time.Time
	AnsweredAt   *time.Time
	CancelledAt  *time.Time
}

// Status derives the row's lifecycle state from its timestamps. Cancelled is
// terminal and dominates even if AnsweredAt is also set (matching the Domain
// Glossary's "cancelled, terminal" rule) — an answer recorded moments before a
// self-heal sweep cancels the row (e.g. its owning item was hard-deleted in
// between) should not read back as "answered".
func (d GuidanceRequestData) Status() GuidanceRequestStatus {
	switch {
	case d.CancelledAt != nil:
		return GuidanceRequestStatusCancelled
	case d.AnsweredAt != nil:
		return GuidanceRequestStatusAnswered
	default:
		return GuidanceRequestStatusPending
	}
}

// CreateGuidanceRequestInput carries the fields needed by CreateGuidanceRequest.
type CreateGuidanceRequestInput struct {
	Scope        domain.RequestScope
	ItemID       *uuid.UUID // required for Scope == RequestScopeBacklogItem
	SessionUUID  string     // required for Scope == RequestScopeSession
	QuestionText string
	QuestionType domain.QuestionType
	Options      string // JSON-encoded []string; only meaningful for QuestionTypeMultipleChoice
	// Cap is the caller-supplied per-(scope, scope_key) pending-request limit.
	// Zero means "use DefaultGuidanceRequestPendingCap" — see that constant's
	// doc comment for why this is a parameter rather than a config read here.
	Cap int
}

// scopeKey derives the always-non-null dedup/cap key described on the
// GuidanceRequest ent schema's scope_key field: item_id's string form for
// backlog-item scope, the session UUID for session scope, "" for standalone.
func (in CreateGuidanceRequestInput) scopeKey() string {
	switch in.Scope {
	case domain.RequestScopeBacklogItem:
		if in.ItemID != nil {
			return in.ItemID.String()
		}
		return ""
	case domain.RequestScopeSession:
		return in.SessionUUID
	default:
		return ""
	}
}

// guidanceRequestToData converts a generated ent row into the plain
// GuidanceRequestData view.
func guidanceRequestToData(row *ent.GuidanceRequest) *GuidanceRequestData {
	return &GuidanceRequestData{
		ID:           row.ID.String(),
		Scope:        domain.RequestScope(row.Scope),
		ItemID:       row.ItemID,
		SessionUUID:  row.SessionUUID,
		QuestionText: row.QuestionText,
		QuestionType: domain.QuestionType(row.QuestionType),
		Options:      row.Options,
		Answer:       row.Answer,
		CreatedAt:    row.CreatedAt,
		NotifiedAt:   row.NotifiedAt,
		AnsweredAt:   row.AnsweredAt,
		CancelledAt:  row.CancelledAt,
	}
}

// CreateGuidanceRequest atomically creates a new, durable GuidanceRequest row,
// or resolves to a pre-existing open row asking the IDENTICAL question for the
// same (scope, scope_key) — dedup-first, cap-check second, in that strict
// order and inside one transaction, mirroring MarkStuck's atomic-upsert shape
// (session/ent_repository_backlog.go).
//
// Ordering matters: a legitimate re-ask of an already-open, IDENTICAL
// question must resolve to the pre-existing row via dedup even when the scope
// is already at its cap — cap-rejecting it would be wrong, since dedup
// creates no new row and the scope's open-question count doesn't actually
// increase. Concretely:
//  1. Validate Scope/QuestionType via IsValid() before opening a transaction
//     (fails fast, no DB round trip for a malformed request).
//  2. Attempt an OnConflictColumns(scope, scope_key, question_text).Ignore()
//     upsert with a caller-chosen candidate ID. Ignore() means "leave an
//     existing conflicting row completely untouched" (each column set to
//     itself) — this is a real dedup, not a refresh.
//  3. Read back the row's ID via the upsert's own RETURNING (GuidanceRequestUpsertOne.ID):
//     if it equals the candidate ID, this call's INSERT won and a genuinely
//     NEW row was created; any other ID means the call resolved to a
//     pre-existing row.
//  4. Only when a NEW row was created, count open (answered_at AND
//     cancelled_at both NULL) rows for the same (scope, scope_key) in the
//     SAME transaction. The newly-created row is already included in this
//     count, so the boundary check is count > cap (not >=), which correctly
//     allows the row that brings the count exactly to the cap. If over cap,
//     the entire transaction is rolled back (undoing the just-created row)
//     and ErrPendingCapExceeded is returned.
//  5. When the call resolved to a pre-existing row instead, the cap-check is
//     skipped entirely and that existing row is returned — this is what makes
//     re-asking an identical question at cap succeed instead of being
//     cap-rejected.
func (r *EntRepository) CreateGuidanceRequest(ctx context.Context, in CreateGuidanceRequestInput) (*GuidanceRequestData, error) {
	capLimit, scopeKey, err := validateCreateGuidanceRequestInput(in)
	if err != nil {
		return nil, err
	}
	candidateID := uuid.New()

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("create guidance request: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	resolvedID, err := upsertGuidanceRequest(ctx, tx, in, scopeKey, candidateID)
	if err != nil {
		return nil, fmt.Errorf("create guidance request: upsert: %w", err)
	}

	if resolvedID == candidateID {
		// A genuinely new row was just created (not a dedup hit) — the cap
		// check only ever applies to this case. See the doc comment above for
		// why dedup must be resolved BEFORE this check runs.
		if err := enforceGuidanceRequestPendingCap(ctx, tx, in.Scope, scopeKey, capLimit); err != nil {
			return nil, fmt.Errorf("create guidance request: %w", err)
		}
	}

	row, err := tx.GuidanceRequest.Get(ctx, resolvedID)
	if err != nil {
		return nil, fmt.Errorf("create guidance request: read back: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create guidance request: commit: %w", err)
	}
	return guidanceRequestToData(row), nil
}

// validateCreateGuidanceRequestInput fails fast (before any DB round trip) on
// a malformed scope/question type, and resolves the cap limit and dedup
// scope_key CreateGuidanceRequest needs for the rest of its work.
func validateCreateGuidanceRequestInput(in CreateGuidanceRequestInput) (capLimit int, scopeKey string, err error) {
	if !in.Scope.IsValid() {
		return 0, "", fmt.Errorf("invalid guidance request scope %q", in.Scope)
	}
	if !in.QuestionType.IsValid() {
		return 0, "", fmt.Errorf("invalid guidance request question type %q", in.QuestionType)
	}
	capLimit = in.Cap
	if capLimit <= 0 {
		capLimit = DefaultGuidanceRequestPendingCap
	}
	return capLimit, in.scopeKey(), nil
}

// upsertGuidanceRequest performs the dedup-first atomic upsert: it always
// attempts to insert a row identified by candidateID, but on a
// (scope, scope_key, question_text) conflict leaves the existing row
// completely untouched (Ignore()) instead of creating a duplicate. It returns
// the ID of whichever row now exists at that dedup key — comparing it against
// candidateID is how the caller distinguishes "created new" from "resolved to
// existing".
func upsertGuidanceRequest(ctx context.Context, tx *ent.Tx, in CreateGuidanceRequestInput, scopeKey string, candidateID uuid.UUID) (uuid.UUID, error) {
	create := tx.GuidanceRequest.Create().
		SetID(candidateID).
		SetScope(string(in.Scope)).
		SetSessionUUID(in.SessionUUID).
		SetScopeKey(scopeKey).
		SetQuestionText(in.QuestionText).
		SetQuestionType(string(in.QuestionType)).
		SetOptions(in.Options)
	if in.ItemID != nil {
		create = create.SetItemID(*in.ItemID)
	}
	return create.
		OnConflictColumns(
			guidancerequest.FieldScope,
			guidancerequest.FieldScopeKey,
			guidancerequest.FieldQuestionText,
		).
		Ignore().
		ID(ctx)
}

// enforceGuidanceRequestPendingCap counts open rows for (scope, scopeKey)
// within the given transaction and rejects with ErrPendingCapExceeded if a
// row just created by the caller pushed that count past capLimit. The
// newly-created row is already included in the count, so the boundary check
// is count > capLimit (not >=) — that correctly allows the row that brings
// the count exactly to the cap.
func enforceGuidanceRequestPendingCap(ctx context.Context, tx *ent.Tx, scope domain.RequestScope, scopeKey string, capLimit int) error {
	count, err := tx.GuidanceRequest.Query().
		Where(
			guidancerequest.Scope(string(scope)),
			guidancerequest.ScopeKey(scopeKey),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		Count(ctx)
	if err != nil {
		return fmt.Errorf("create guidance request: cap count: %w", err)
	}
	if count > capLimit {
		log.WarningLog().Printf(
			"guidance request pending cap exceeded: scope=%s scope_key=%s current_count=%d cap=%d",
			scope, scopeKey, count, capLimit,
		)
		return fmt.Errorf("%w: scope=%s current count=%d cap=%d", ErrPendingCapExceeded, scope, count, capLimit)
	}
	return nil
}

// AnswerGuidanceRequest atomically, idempotently records an answer via a
// single conditional UPDATE ... WHERE answered_at IS NULL AND cancelled_at IS
// NULL, mirroring ResolveStuck exactly. Returns whether this call actually
// persisted the answer; a second call racing against an already-answered (or
// already-cancelled) row is a no-op, not an error or overwrite.
func (r *EntRepository) AnswerGuidanceRequest(ctx context.Context, id uuid.UUID, answer string) (applied bool, err error) {
	n, err := r.client.GuidanceRequest.Update().
		Where(
			guidancerequest.ID(id),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		SetAnswer(answer).
		SetAnsweredAt(time.Now()).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("answer guidance request %s: %w", id, err)
	}
	return n > 0, nil
}

// MarkGuidanceRequestNotified sets notified_at=now on a not-yet-notified row
// via a single conditional UPDATE ... WHERE notified_at IS NULL, mirroring
// MarkStuckNotified exactly. A no-op (not an error) if already notified or
// nonexistent.
func (r *EntRepository) MarkGuidanceRequestNotified(ctx context.Context, id uuid.UUID) (bool, error) {
	n, err := r.client.GuidanceRequest.Update().
		Where(
			guidancerequest.ID(id),
			guidancerequest.NotifiedAtIsNil(),
		).
		SetNotifiedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("mark guidance request notified %s: %w", id, err)
	}
	return n > 0, nil
}

// CancelGuidanceRequest atomically, idempotently sets cancelled_at via a
// single conditional UPDATE ... WHERE answered_at IS NULL AND cancelled_at IS
// NULL, mirroring AnswerGuidanceRequest's shape. reason is accepted for
// call-site documentation/logging purposes; it is not persisted (no reason
// column on this schema).
func (r *EntRepository) CancelGuidanceRequest(ctx context.Context, id uuid.UUID, reason string) (applied bool, err error) {
	n, err := r.client.GuidanceRequest.Update().
		Where(
			guidancerequest.ID(id),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		SetCancelledAt(time.Now()).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("cancel guidance request %s (%s): %w", id, reason, err)
	}
	return n > 0, nil
}

// GetGuidanceRequest reads a single GuidanceRequest row by id.
func (r *EntRepository) GetGuidanceRequest(ctx context.Context, id uuid.UUID) (*GuidanceRequestData, error) {
	row, err := r.client.GuidanceRequest.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: guidance request %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get guidance request %s: %w", id, err)
	}
	return guidanceRequestToData(row), nil
}

// ListPendingGuidanceRequests returns every open (answered_at AND
// cancelled_at both NULL) GuidanceRequest row for one (scope, scopeKey) pair,
// alongside the current pending count and the cap passed in — bundled
// together so an RPC handler can populate a ListGuidanceRequestsResponse's
// pending_count/cap fields directly from this one call.
func (r *EntRepository) ListPendingGuidanceRequests(ctx context.Context, scope domain.RequestScope, scopeKey string, cap int) ([]*GuidanceRequestData, int, int, error) {
	rows, err := r.client.GuidanceRequest.Query().
		Where(
			guidancerequest.Scope(string(scope)),
			guidancerequest.ScopeKey(scopeKey),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("list pending guidance requests scope=%s scope_key=%s: %w", scope, scopeKey, err)
	}
	result := make([]*GuidanceRequestData, 0, len(rows))
	for _, row := range rows {
		result = append(result, guidanceRequestToData(row))
	}
	if cap <= 0 {
		cap = DefaultGuidanceRequestPendingCap
	}
	return result, len(result), cap, nil
}

// ListAllPendingGuidanceRequests returns every open GuidanceRequest row across
// every scope — backs the nav badge (Epic 7).
func (r *EntRepository) ListAllPendingGuidanceRequests(ctx context.Context) ([]*GuidanceRequestData, error) {
	rows, err := r.client.GuidanceRequest.Query().
		Where(
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all pending guidance requests: %w", err)
	}
	result := make([]*GuidanceRequestData, 0, len(rows))
	for _, row := range rows {
		result = append(result, guidanceRequestToData(row))
	}
	return result, nil
}
