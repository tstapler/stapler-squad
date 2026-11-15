package session

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/guidancerequest"
)

// guidanceHooks are the guidance store's only calls out to the rest of the
// repository, so the store itself needs no other session symbol. Wired by
// EntRepository.initStores; every field is required.
type guidanceHooks struct {
	// answered observes a just-applied answer's latency (best effort).
	answered func(ctx context.Context, id uuid.UUID)
	// delivered counts a delivery transition that was applied.
	delivered func(ctx context.Context, route domain.DeliveryRoute, state domain.DeliveryState)
	// itemPendingChanged republishes the owning item after its
	// has_pending_guidance_requests flag may have flipped.
	itemPendingChanged func(ctx context.Context, item *ent.BacklogItem)
}

// guidanceInputSource is the pending-cap class and coalescing-key selector of this create.
func guidanceInputSource(in domain.CreateGuidanceRequestInput) domain.QuestionSource {
	if in.Mirror != nil {
		return in.Mirror.Source
	}
	return domain.QuestionSourceAgentMCP
}

// scopeKey derives the always-non-null dedup/cap key described on the
// GuidanceRequest ent schema's scope_key field: item_id's string form for
// backlog-item scope, the session UUID for session scope, "" for standalone.
func guidanceInputScopeKey(in domain.CreateGuidanceRequestInput) string {
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
func guidanceRequestToData(row *ent.GuidanceRequest) *domain.GuidanceRequestData {
	return &domain.GuidanceRequestData{
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

		Source:              domain.QuestionSource(row.Source),
		AskerSessionUUID:    row.AskerSessionUUID,
		AnswererKind:        domain.AnswererKind(row.AnswererKind),
		AnswererSessionUUID: row.AnswererSessionUUID,
		MultiSelect:         row.MultiSelect,
		QuestionsJSON:       row.QuestionsJSON,
		DeliveryRoute:       domain.DeliveryRoute(row.DeliveryRoute),
		DeliveryState:       domain.DeliveryState(row.DeliveryState),
		DeliveryAttempts:    row.DeliveryAttempts,
		HookCallID:          domain.HookCallID(row.HookCallID),
		ExpiresAt:           row.ExpiresAt,
		CancelReason:        domain.CancelReason(row.CancelReason),
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
//     OnConflict upsert because every transaction runs on the single writer connection
//     (session/ent_repository.go's SetMaxOpenConns(1)), so they serialize process-wide — no concurrent
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
func (r *EntRepository) CreateGuidanceRequest(ctx context.Context, in domain.CreateGuidanceRequestInput) (*domain.GuidanceRequestData, error) {
	return r.createGuidanceRequest(ctx, in)
}

// CreateMirroredGuidanceRequest creates (or, for a hook or peer source, dedups
// to) a row from a validated domain.GuidanceRequest. A hook or peer source
// coalesces on (asker_session_uuid, hook_call_id) and bypasses the legacy
// (scope, scope_key, question_text) coalesce; every source is capped per
// (scope, scope_key, source). capLimit <= 0 means the default cap.
func (r *EntRepository) CreateMirroredGuidanceRequest(ctx context.Context, req domain.GuidanceRequest, capLimit int) (*domain.GuidanceRequestData, error) {
	p := req.Params()
	in := domain.CreateGuidanceRequestInput{
		Scope: p.Scope, SessionUUID: p.SessionUUID, QuestionText: p.QuestionText,
		QuestionType: p.QuestionType, Options: p.Options, Cap: capLimit, Mirror: &p,
	}
	if p.ItemID != "" {
		itemID, err := uuid.Parse(p.ItemID)
		if err != nil {
			return nil, fmt.Errorf("create mirrored guidance request: item id %q: %w", p.ItemID, err)
		}
		in.ItemID = &itemID
	}
	return r.createGuidanceRequest(ctx, in)
}

func (r *EntRepository) createGuidanceRequest(ctx context.Context, in domain.CreateGuidanceRequestInput) (*domain.GuidanceRequestData, error) {
	capLimit, scopeKey, err := validateCreateGuidanceRequestInput(in)
	if err != nil {
		return nil, err
	}
	candidateID := uuid.New()

	tx, err := r.Client.Tx(ctx)
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
		if err := enforceGuidanceRequestPendingCap(ctx, tx, in.Scope, scopeKey, guidanceInputSource(in), capLimit); err != nil {
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
	r.pendingGuidance.invalidate() // the mutation hook ran before this commit
	r.publishGuidanceChange(ctx, row.Scope, row.ScopeKey)
	return guidanceRequestToData(row), nil
}

// validateCreateGuidanceRequestInput fails fast (before any DB round trip) on
// a malformed scope/question type, and resolves the cap limit and dedup
// scope_key CreateGuidanceRequest needs for the rest of its work.
func validateCreateGuidanceRequestInput(in domain.CreateGuidanceRequestInput) (capLimit int, scopeKey string, err error) {
	if !in.Scope.IsValid() {
		return 0, "", fmt.Errorf("invalid guidance request scope %q", in.Scope)
	}
	if !in.QuestionType.IsValid() {
		return 0, "", fmt.Errorf("invalid guidance request question type %q", in.QuestionType)
	}
	capLimit = in.Cap
	if capLimit <= 0 {
		capLimit = domain.DefaultGuidanceRequestPendingCap
	}
	return capLimit, guidanceInputScopeKey(in), nil
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
func upsertGuidanceRequest(ctx context.Context, tx *ent.Tx, in domain.CreateGuidanceRequestInput, scopeKey string, candidateID uuid.UUID) (uuid.UUID, error) {
	existing, err := tx.GuidanceRequest.Query().Where(coalescePredicates(in, scopeKey)...).First(ctx)
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
	if in.Mirror != nil {
		if create, err = setMirrorFields(create, *in.Mirror); err != nil {
			return uuid.Nil, err
		}
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
// the count exactly to the cap. The count is per source class, so hook and
// peer rows can neither exhaust agent_mcp slots nor each other (ADR-003 rule 1).
func enforceGuidanceRequestPendingCap(ctx context.Context, tx *ent.Tx, scope domain.RequestScope, scopeKey string, source domain.QuestionSource, capLimit int) error {
	count, err := tx.GuidanceRequest.Query().
		Where(
			guidancerequest.Scope(string(scope)),
			guidancerequest.ScopeKey(scopeKey),
			guidancerequest.Source(string(source)),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		Count(ctx)
	if err != nil {
		return fmt.Errorf("create guidance request: cap count: %w", err)
	}
	if count > capLimit {
		log.WarningLog().Printf(
			"guidance request pending cap exceeded: scope=%s scope_key=%s source=%s current_count=%d cap=%d",
			scope, scopeKey, source, count, capLimit,
		)
		return fmt.Errorf("%w: scope=%s source=%s current count=%d cap=%d", domain.ErrPendingCapExceeded, scope, source, count, capLimit)
	}
	return nil
}

// AnswerGuidanceRequest atomically, idempotently records an answer via a
// single conditional UPDATE ... WHERE answered_at IS NULL AND cancelled_at IS
// NULL, mirroring ResolveStuck exactly. Returns whether this call actually
// persisted the answer; a second call racing against an already-answered (or
// already-cancelled) row is a no-op, not an error or overwrite.
func (r *EntRepository) AnswerGuidanceRequest(ctx context.Context, id uuid.UUID, answer string) (applied bool, err error) {
	n, err := r.Client.GuidanceRequest.Update().
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
	if n > 0 {
		r.guidanceHooks.answered(ctx, id)
		r.publishGuidanceChangeByID(ctx, id)
	}
	return n > 0, nil
}

// MarkGuidanceRequestNotified sets notified_at=now on a not-yet-notified row
// via a single conditional UPDATE ... WHERE notified_at IS NULL, mirroring
// MarkStuckNotified exactly. A no-op (not an error) if already notified or
// nonexistent.
func (r *EntRepository) MarkGuidanceRequestNotified(ctx context.Context, id uuid.UUID) (bool, error) {
	n, err := r.Client.GuidanceRequest.Update().
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
// NULL, mirroring AnswerGuidanceRequest's shape. reason is stored in
// cancel_reason so a card can say why the question went away.
func (r *EntRepository) CancelGuidanceRequest(ctx context.Context, id uuid.UUID, reason string) (applied bool, err error) {
	n, err := r.Client.GuidanceRequest.Update().
		Where(
			guidancerequest.ID(id),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		SetCancelledAt(time.Now()).
		SetCancelReason(reason).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("cancel guidance request %s (%s): %w", id, reason, err)
	}
	if n > 0 {
		r.publishGuidanceChangeByID(ctx, id)
	}
	return n > 0, nil
}

// GetGuidanceRequest reads a single GuidanceRequest row by id.
func (r *EntRepository) GetGuidanceRequest(ctx context.Context, id uuid.UUID) (*domain.GuidanceRequestData, error) {
	row, err := r.Client.GuidanceRequest.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: guidance request %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("get guidance request %s: %w", id, err)
	}
	return guidanceRequestToData(row), nil
}

// guidanceRequestsToData converts a slice of generated ent rows into their
// plain GuidanceRequestData view, shared by every List* method below.
func guidanceRequestsToData(rows []*ent.GuidanceRequest) []*domain.GuidanceRequestData {
	result := make([]*domain.GuidanceRequestData, 0, len(rows))
	for _, row := range rows {
		result = append(result, guidanceRequestToData(row))
	}
	return result
}

// queryOpenGuidanceRequests returns every open (answered_at AND cancelled_at
// both NULL) row for (scope, scopeKey), newest first.
func (r *EntRepository) queryOpenGuidanceRequests(ctx context.Context, scope domain.RequestScope, scopeKey string) ([]*ent.GuidanceRequest, error) {
	return r.Client.GuidanceRequest.Query().
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
	return r.Client.GuidanceRequest.Query().
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
func (r *EntRepository) ListPendingGuidanceRequests(ctx context.Context, scope domain.RequestScope, scopeKey string, cap int) ([]*domain.GuidanceRequestData, int, int, error) {
	rows, err := r.queryOpenGuidanceRequests(ctx, scope, scopeKey)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("list pending guidance requests scope=%s scope_key=%s: %w", scope, scopeKey, err)
	}
	result := guidanceRequestsToData(rows)
	if cap <= 0 {
		cap = domain.DefaultGuidanceRequestPendingCap
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
func (r *EntRepository) ListGuidanceRequestsForScope(ctx context.Context, scope domain.RequestScope, scopeKey string, cap int) ([]*domain.GuidanceRequestData, int, int, error) {
	if cap <= 0 {
		cap = domain.DefaultGuidanceRequestPendingCap
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
func (r *EntRepository) ListAllPendingGuidanceRequests(ctx context.Context) ([]*domain.GuidanceRequestData, error) {
	if cached, ok := r.pendingGuidance.get(); ok {
		return cached, nil
	}
	readGen := r.pendingGuidance.gen.Load()
	rows, err := r.Client.GuidanceRequest.Query().
		Where(
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all pending guidance requests: %w", err)
	}
	data := guidanceRequestsToData(rows)
	r.pendingGuidance.put(readGen, data)
	return copyGuidanceRows(data), nil
}

// pendingGuidanceItemIDs returns the subset of itemIDs that have at least one
// open backlog-item-scoped GuidanceRequest, in a single query.
func (r *EntRepository) pendingGuidanceItemIDs(ctx context.Context, itemIDs []string) (map[string]bool, error) {
	if len(itemIDs) == 0 {
		return map[string]bool{}, nil
	}
	keys, err := r.Client.GuidanceRequest.Query().
		Where(
			guidancerequest.Scope(string(domain.RequestScopeBacklogItem)),
			guidancerequest.ScopeKeyIn(itemIDs...),
			guidancerequest.AnsweredAtIsNil(),
			guidancerequest.CancelledAtIsNil(),
		).
		Select(guidancerequest.FieldScopeKey).
		Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending guidance for items: %w", err)
	}
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set, nil
}

// attachPendingGuidance sets HasPendingGuidanceRequests on item. Best-effort:
// a lookup failure is logged and leaves the flag false.
func (r *EntRepository) attachPendingGuidance(ctx context.Context, item *backlogdto.BacklogItemData) {
	if item == nil || item.ID == "" {
		return
	}
	set, err := r.pendingGuidanceItemIDs(ctx, []string{item.ID})
	if err != nil {
		log.WarningLog().Printf("[EntRepository] attachPendingGuidance: item %s: %v", item.ID, err)
		return
	}
	item.HasPendingGuidanceRequests = set[item.ID]
}

// publishGuidanceChange republishes the owning item so WatchBacklogItems
// subscribers see HasPendingGuidanceRequests flip. No-op for non-item scopes.
func (r *EntRepository) publishGuidanceChange(ctx context.Context, scope string, scopeKey string) {
	if scope != string(domain.RequestScopeBacklogItem) || scopeKey == "" {
		return
	}
	// Best-effort republish after the write committed: must survive caller cancellation.
	ctx = context.WithoutCancel(ctx)
	itemID, err := uuid.Parse(scopeKey)
	if err != nil {
		return
	}
	row, err := r.Client.BacklogItem.Query().Where(backlogitem.ID(itemID)).WithSource().Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			log.WarningLog().Printf("[EntRepository] publishGuidanceChange: item %s: %v", scopeKey, err)
		}
		return
	}
	r.guidanceHooks.itemPendingChanged(ctx, row)
}

func (r *EntRepository) publishGuidanceChangeByID(ctx context.Context, id uuid.UUID) {
	row, err := r.Client.GuidanceRequest.Get(ctx, id)
	if err != nil {
		log.WarningLog().Printf("[EntRepository] publishGuidanceChangeByID: guidance request %s: %v", id, err)
		return
	}
	r.publishGuidanceChange(ctx, row.Scope, row.ScopeKey)
}

// applyPendingGuidance sets the flag for ids via set, using one batched query.
func (r *EntRepository) applyPendingGuidance(ctx context.Context, ids []string, set func(i int, pending bool)) {
	pending, err := r.pendingGuidanceItemIDs(ctx, ids)
	if err != nil {
		log.WarningLog().Printf("[EntRepository] applyPendingGuidance: %v", err)
		return
	}
	for i, id := range ids {
		set(i, pending[id])
	}
}

func (r *EntRepository) applyPendingGuidanceToData(ctx context.Context, items []backlogdto.BacklogItemData) {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	r.applyPendingGuidance(ctx, ids, func(i int, p bool) { items[i].HasPendingGuidanceRequests = p })
}

func (r *EntRepository) applyPendingGuidanceToSummaries(ctx context.Context, items []backlogdto.BacklogItemSummary) {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	r.applyPendingGuidance(ctx, ids, func(i int, p bool) { items[i].HasPendingGuidanceRequests = p })
}
