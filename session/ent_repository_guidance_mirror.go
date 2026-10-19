package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/guidancerequest"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
)

// ent_repository_guidance_mirror.go holds the hook and peer halves of the
// GuidanceRequest repository (ADR-003): coalescing keys, the extra columns on
// create, guarded delivery transitions and queries by asker.

// coalescePredicates selects the existing row a create resolves to instead of
// inserting. agent_mcp keeps the legacy open-row (scope, scope_key,
// question_text) match. A hook or peer source matches on (asker_session_uuid,
// hook_call_id) with any status, because a tool_use_id names exactly one call
// and a resolved row must not be recreated; without a call id it falls back to
// the same payload (or text) among open rows only.
func coalescePredicates(in CreateGuidanceRequestInput, scopeKey string) []predicate.GuidanceRequest {
	open := []predicate.GuidanceRequest{guidancerequest.AnsweredAtIsNil(), guidancerequest.CancelledAtIsNil()}
	if in.mirror == nil || in.mirror.Source == domain.QuestionSourceAgentMCP {
		return append([]predicate.GuidanceRequest{
			guidancerequest.Scope(string(in.Scope)),
			guidancerequest.ScopeKey(scopeKey),
			guidancerequest.QuestionText(in.QuestionText),
		}, open...)
	}
	m := in.mirror
	byAsker := []predicate.GuidanceRequest{
		guidancerequest.Source(string(m.Source)),
		guidancerequest.AskerSessionUUID(m.AskerSessionUUID),
	}
	if m.HookCallID != "" {
		return append(byAsker, guidancerequest.HookCallID(string(m.HookCallID)))
	}
	byAsker = append(byAsker, open...)
	if payload, err := m.Payload.JSON(); err == nil && payload != "" {
		return append(byAsker, guidancerequest.QuestionsJSON(payload))
	}
	return append(byAsker,
		guidancerequest.QuestionText(m.QuestionText),
		guidancerequest.AnswererSessionUUID(m.AnswererSessionUUID),
	)
}

// setMirrorFields adds the ADR-003 columns to a create. A peer row expires after domain.PeerHelpTTL.
func setMirrorFields(c *ent.GuidanceRequestCreate, m domain.GuidanceRequestParams) (*ent.GuidanceRequestCreate, error) {
	payload, err := m.Payload.JSON()
	if err != nil {
		return nil, fmt.Errorf("create guidance request row: %w", err)
	}
	c = c.SetSource(string(m.Source)).
		SetAskerSessionUUID(m.AskerSessionUUID).
		SetAnswererKind(string(m.AnswererKind)).
		SetAnswererSessionUUID(m.AnswererSessionUUID).
		SetMultiSelect(m.MultiSelect).
		SetQuestionsJSON(payload).
		SetDeliveryRoute(string(m.DeliveryRoute)).
		SetDeliveryState(string(m.DeliveryState)).
		SetHookCallID(string(m.HookCallID))
	if m.Source == domain.QuestionSourcePeer {
		c = c.SetExpiresAt(time.Now().Add(domain.PeerHelpTTL))
	}
	return c, nil
}

// SetDelivery moves a row's delivery to (route, state) and adds addAttempts to
// delivery_attempts, only when the row's current state may legally precede
// state (domain.PredecessorsOf): one conditional UPDATE, so a racing writer
// loses with applied=false instead of overwriting. A route/state pair that
// cannot coexist is an error and writes nothing.
func (r *EntRepository) SetDelivery(ctx context.Context, id uuid.UUID, route domain.DeliveryRoute, state domain.DeliveryState, addAttempts int) (applied bool, err error) {
	if !state.IsValid() || !route.IsValid() {
		return false, fmt.Errorf("set delivery %s: %w: route %q state %q", id, domain.ErrInvalidDelivery, route, state)
	}
	if err := domain.ValidateDelivery(route, state); err != nil {
		return false, fmt.Errorf("set delivery %s: %w", id, err)
	}
	if addAttempts < 0 {
		return false, fmt.Errorf("set delivery %s: %w", id, errors.New("addAttempts must not be negative"))
	}
	n, err := r.client.GuidanceRequest.Update().
		Where(guidancerequest.ID(id), anyDeliveryState(domain.PredecessorsOf(state))).
		SetDeliveryRoute(string(route)).
		SetDeliveryState(string(state)).
		AddDeliveryAttempts(addAttempts).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("set delivery %s to %s: %w", id, state, err)
	}
	return n > 0, nil
}

// anyDeliveryState matches a row whose state is one of states; the untracked
// state "" also matches the NULL a migrated or never-routed row carries.
func anyDeliveryState(states []domain.DeliveryState) predicate.GuidanceRequest {
	var preds []predicate.GuidanceRequest
	for _, s := range states {
		if s == "" {
			preds = append(preds, guidancerequest.DeliveryStateIsNil(), guidancerequest.DeliveryState(""))
			continue
		}
		preds = append(preds, guidancerequest.DeliveryState(string(s)))
	}
	return guidancerequest.Or(preds...)
}

// ListOpenGuidanceRequestsByAsker returns the open rows whose asker is
// askerSessionUUID, newest first, optionally limited to sources.
func (r *EntRepository) ListOpenGuidanceRequestsByAsker(ctx context.Context, askerSessionUUID string, sources ...domain.QuestionSource) ([]*GuidanceRequestData, error) {
	q := r.client.GuidanceRequest.Query().Where(
		guidancerequest.AskerSessionUUID(askerSessionUUID),
		guidancerequest.AnsweredAtIsNil(),
		guidancerequest.CancelledAtIsNil(),
	)
	if len(sources) > 0 {
		names := make([]string, len(sources))
		for i, s := range sources {
			names[i] = string(s)
		}
		q = q.Where(guidancerequest.SourceIn(names...))
	}
	rows, err := q.Order(guidancerequest.ByCreatedAt(sql.OrderDesc())).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list open guidance requests by asker %s: %w", askerSessionUUID, err)
	}
	return guidanceRequestsToData(rows), nil
}
