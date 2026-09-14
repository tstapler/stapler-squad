package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
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
// or resolves to a pre-existing OPEN row asking the IDENTICAL question for the
// same (scope, scope_key) — dedup-first, cap-check second, in that strict
// order and inside one transaction, mirroring MarkStuck's atomic-upsert shape
// (session/ent_repository_backlog.go).
//
// Dedup is scoped to OPEN rows only (answered_at AND cancelled_at both NULL):
// once a question is answered or cancelled, re-asking the identical
// question_text must create a fresh pending row, not silently resolve to the
// stale resolved one — see upsertGuidanceRequest's doc comment.
//
// Ordering matters: a legitimate re-ask of an already-open, IDENTICAL
// question must resolve to the pre-existing row via dedup even when the scope
// is already at its cap — cap-rejecting it would be wrong, since dedup
// creates no new row and the scope's open-question count doesn't actually
// increase. Concretely:
//  1. Validate Scope/QuestionType via IsValid() before opening a transaction
//     (fails fast, no DB round trip for a malformed request).
//  2. Query for an existing OPEN row matching (scope, scope_key,
//     question_text) inside the transaction; if found, that row's ID is what
//     this call resolves to — no new row is created. If not found, Create()
//     a new row with a caller-chosen candidate ID. This is safe without an
//     OnConflict upsert because every transaction is serialized process-wide
//     (session/ent_repository.go's SetMaxOpenConns(1)) — no concurrent
//     transaction can race this read-then-create.
//  3. Comparing the resolved ID against the candidate ID is how the caller
//     distinguishes "created new" from "resolved to existing".
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

// upsertGuidanceRequest performs the dedup-first create: it looks for an
// existing OPEN row (answered_at AND cancelled_at both NULL) matching
// (scope, scope_key, question_text) and returns its ID if found; otherwise it
// creates a new row identified by candidateID. Restricting the dedup lookup
// to open rows (rather than an unconditional DB-level unique constraint
// across all rows, which the schema no longer declares — see the
// GuidanceRequest schema's Indexes() comment) is what lets an identical
// question be asked again after a prior instance was answered or cancelled,
// instead of resolving forever to that stale resolved row. Comparing the
// returned ID against candidateID is how the caller distinguishes "created
// new" from "resolved to existing".
func upsertGuidanceRequest(ctx context.Context, tx *ent.Tx, in CreateGuidanceRequestInput, scopeKey string, candidateID uuid.UUID) (uuid.UUID, error) {
	existing, err := tx.GuidanceRequest.Query().
		Where(
			guidancerequest.Scope(string(in.Scope)),
			guidancerequest.ScopeKey(scopeKey),
			guidancerequest.QuestionText(in.QuestionText),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		First(ctx)
	switch {
	case err == nil:
		return existing.ID, nil
	case !ent.IsNotFound(err):
		return uuid.Nil, fmt.Errorf("query existing open guidance request: %w", err)
	}

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
	created, err := create.Save(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create guidance request row: %w", err)
	}
	return created.ID, nil
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

// guidanceRequestsToData converts a slice of generated ent rows into their
// plain GuidanceRequestData view, shared by every List* method below.
func guidanceRequestsToData(rows []*ent.GuidanceRequest) []*GuidanceRequestData {
	result := make([]*GuidanceRequestData, 0, len(rows))
	for _, row := range rows {
		result = append(result, guidanceRequestToData(row))
	}
	return result
}

// queryOpenGuidanceRequests returns every open (answered_at AND cancelled_at
// both NULL) row for (scope, scopeKey), newest first.
func (r *EntRepository) queryOpenGuidanceRequests(ctx context.Context, scope domain.RequestScope, scopeKey string) ([]*ent.GuidanceRequest, error) {
	return r.client.GuidanceRequest.Query().
		Where(
			guidancerequest.Scope(string(scope)),
			guidancerequest.ScopeKey(scopeKey),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		Order(guidancerequest.ByCreatedAt(sql.OrderDesc())).
		All(ctx)
}

// queryAnsweredGuidanceRequests returns up to limit answered (non-cancelled)
// rows for (scope, scopeKey), newest first.
func (r *EntRepository) queryAnsweredGuidanceRequests(ctx context.Context, scope domain.RequestScope, scopeKey string, limit int) ([]*ent.GuidanceRequest, error) {
	return r.client.GuidanceRequest.Query().
		Where(
			guidancerequest.Scope(string(scope)),
			guidancerequest.ScopeKey(scopeKey),
			guidancerequest.AnsweredAtNotNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		Order(guidancerequest.ByCreatedAt(sql.OrderDesc())).
		Limit(limit).
		All(ctx)
}

// ListPendingGuidanceRequests returns every open (answered_at AND
// cancelled_at both NULL) GuidanceRequest row for one (scope, scopeKey) pair,
// alongside the current pending count and the cap passed in — bundled
// together so an RPC handler can populate a ListGuidanceRequestsResponse's
// pending_count/cap fields directly from this one call.
func (r *EntRepository) ListPendingGuidanceRequests(ctx context.Context, scope domain.RequestScope, scopeKey string, cap int) ([]*GuidanceRequestData, int, int, error) {
	rows, err := r.queryOpenGuidanceRequests(ctx, scope, scopeKey)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("list pending guidance requests scope=%s scope_key=%s: %w", scope, scopeKey, err)
	}
	result := guidanceRequestsToData(rows)
	if cap <= 0 {
		cap = DefaultGuidanceRequestPendingCap
	}
	return result, len(result), cap, nil
}

// listGuidanceRequestsForScopeLimit bounds the total number of rows
// ListGuidanceRequestsForScope returns. It only ever trims ANSWERED rows —
// see the function's doc comment for why pending rows are always included in
// full regardless of this limit.
const listGuidanceRequestsForScopeLimit = 50

// ListGuidanceRequestsForScope returns every non-cancelled GuidanceRequest
// row (pending, and — unlike ListPendingGuidanceRequests — answered too) for
// one (scope, scopeKey) pair, alongside the current pending count and cap
// (cap is a parameter, matching ListPendingGuidanceRequests' shape, rather
// than hardcoding DefaultGuidanceRequestPendingCap — 0 falls back to that
// default). Backs UI views that must show a question's state after it's been
// answered (AC3), not just while pending.
//
// Pending rows are fetched in their own unbounded query and always placed
// first in the result — cheap, since they're already implicitly capped by
// the per-scope pending-question cap (at most a handful of rows). The
// remaining budget up to listGuidanceRequestsForScopeLimit is then filled
// with the newest answered rows. This two-query shape exists specifically so
// an old still-pending row can never be pushed out of the result by a flood
// of newer answered rows — a single `ORDER BY created_at DESC LIMIT N` query
// over all non-cancelled rows could silently drop a pending row from the page
// once enough newer rows were answered, directly undermining AC3/AC4's
// "pending questions must be visible."
func (r *EntRepository) ListGuidanceRequestsForScope(ctx context.Context, scope domain.RequestScope, scopeKey string, cap int) ([]*GuidanceRequestData, int, int, error) {
	if cap <= 0 {
		cap = DefaultGuidanceRequestPendingCap
	}

	pendingRows, err := r.queryOpenGuidanceRequests(ctx, scope, scopeKey)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("list guidance requests for scope=%s scope_key=%s: list pending: %w", scope, scopeKey, err)
	}
	result := guidanceRequestsToData(pendingRows)
	pendingCount := len(result)

	if remaining := listGuidanceRequestsForScopeLimit - len(pendingRows); remaining > 0 {
		answeredRows, err := r.queryAnsweredGuidanceRequests(ctx, scope, scopeKey, remaining)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("list guidance requests for scope=%s scope_key=%s: list answered: %w", scope, scopeKey, err)
		}
		result = append(result, guidanceRequestsToData(answeredRows)...)
	}

	return result, pendingCount, cap, nil
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
	return guidanceRequestsToData(rows), nil
}
