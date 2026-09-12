package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GuidanceRequestService backs the GuidanceRequestService RPCs — the
// ConnectRPC surface for durable-guidance-request (see
// project_plans/durable-guidance-request/implementation/plan.md Phase 2,
// Story 2.1.2). Delegates every mutation/read to the Phase 1 repository
// methods (session/ent_repository_guidance.go, forwarded through *Storage
// per session/storage_guidance.go), applying ownership/cap enforcement on
// top. A concrete type with a single implementation, per the
// interface-pollution-checklist convention this codebase's other
// single-implementation services (StreamHubRolloutService et al.) already
// follow.
type GuidanceRequestService struct {
	storage *session.Storage
}

// NewGuidanceRequestService creates a GuidanceRequestService backed by storage.
func NewGuidanceRequestService(storage *session.Storage) *GuidanceRequestService {
	return &GuidanceRequestService{storage: storage}
}

// connectErrorForItemLink translates a *session.ItemLinkError into the
// connect.Error shape CreateGuidanceRequest returns for a backlog-item-scope
// ownership failure — the identical (code, remediation) pair
// server/mcp's resolveItemLink already produces for every other mutating
// backlog MCP tool (AC6). See
// server/services/guidance_request_ownership_parity_test.go for the
// regression test guarding this against drifting from the MCP adapter.
func connectErrorForItemLink(linkErr *session.ItemLinkError) error {
	msg := linkErr.Message
	if linkErr.Remediation != "" {
		msg = fmt.Sprintf("%s: %s", linkErr.Message, linkErr.Remediation)
	}
	switch linkErr.Code {
	case session.ItemLinkNotFound:
		return connect.NewError(connect.CodeNotFound, errors.New(msg))
	case session.ItemLinkPermissionDenied:
		return connect.NewError(connect.CodePermissionDenied, errors.New(msg))
	default:
		return connect.NewError(connect.CodeInternal, errors.New(msg))
	}
}

// encodeGuidanceOptions JSON-encodes a repeated-string options field into the
// string column session.CreateGuidanceRequestInput.Options expects; "" for
// no options (matches GuidanceRequestData.Options' "empty/absent for the
// other two [question] types" convention).
func encodeGuidanceOptions(opts []string) string {
	if len(opts) == 0 {
		return ""
	}
	b, err := json.Marshal(opts)
	if err != nil {
		return ""
	}
	return string(b)
}

// decodeGuidanceOptions is encodeGuidanceOptions' inverse, tolerant of an
// empty or malformed column (never fails the read).
func decodeGuidanceOptions(raw string) []string {
	if raw == "" {
		return nil
	}
	var opts []string
	if err := json.Unmarshal([]byte(raw), &opts); err != nil {
		return nil
	}
	return opts
}

// guidanceRequestToProto converts the repository's plain data view into the
// wire message.
func guidanceRequestToProto(d *session.GuidanceRequestData) *sessionv1.GuidanceRequest {
	out := &sessionv1.GuidanceRequest{
		Id:           d.ID,
		Scope:        string(d.Scope),
		SessionUuid:  d.SessionUUID,
		QuestionText: d.QuestionText,
		QuestionType: string(d.QuestionType),
		Options:      decodeGuidanceOptions(d.Options),
		Answer:       d.Answer,
		CreatedAt:    timestamppb.New(d.CreatedAt),
		Status:       string(d.Status()),
	}
	if d.ItemID != nil {
		itemID := d.ItemID.String()
		out.ItemId = &itemID
	}
	if d.NotifiedAt != nil {
		out.NotifiedAt = timestamppb.New(*d.NotifiedAt)
	}
	if d.AnsweredAt != nil {
		out.AnsweredAt = timestamppb.New(*d.AnsweredAt)
	}
	if d.CancelledAt != nil {
		out.CancelledAt = timestamppb.New(*d.CancelledAt)
	}
	return out
}

// checkCreateGuidanceRequestOwnership applies the scope-appropriate
// ownership check for CreateGuidanceRequest:
//   - scope == "backlog-item": callerUUID must be linked to itemID, via the
//     SAME session.ResolveItemLink helper server/mcp's resolveItemLink wraps,
//     so the rejection is identical to every other mutating backlog MCP tool
//     (AC6).
//   - scope == "session": callerUUID must equal sessionUUID — a session may
//     only open a session-scoped question about itself.
//   - scope == "standalone": no check (human-only surface, matches
//     requirements.md's security classification).
func (s *GuidanceRequestService) checkCreateGuidanceRequestOwnership(ctx context.Context, scope domain.RequestScope, itemID, sessionUUID, callerUUID string) error {
	switch scope {
	case domain.RequestScopeBacklogItem:
		if itemID == "" {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("item_id is required for scope=backlog-item"))
		}
		if _, linkErr := session.ResolveItemLink(ctx, s.storage, callerUUID, itemID); linkErr != nil {
			return connectErrorForItemLink(linkErr)
		}
	case domain.RequestScopeSession:
		if sessionUUID == "" {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("session_uuid is required for scope=session"))
		}
		if callerUUID != sessionUUID {
			return connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("caller session %s may not create a guidance request scoped to a different session %s", callerUUID, sessionUUID))
		}
	case domain.RequestScopeStandalone:
		// No ownership check — human-only surface.
	}
	return nil
}

// parseCreateGuidanceRequestFields validates scope/question_type and parses
// item_id, split out of CreateGuidanceRequest to keep that handler under the
// house function-length gate.
func parseCreateGuidanceRequestFields(msg *sessionv1.CreateGuidanceRequestRequest) (domain.RequestScope, domain.QuestionType, *uuid.UUID, error) {
	scope := domain.RequestScope(msg.GetScope())
	if !scope.IsValid() {
		return "", "", nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid scope %q", msg.GetScope()))
	}
	questionType := domain.QuestionType(msg.GetQuestionType())
	if !questionType.IsValid() {
		return "", "", nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid question_type %q", msg.GetQuestionType()))
	}
	var itemID *uuid.UUID
	if id := msg.GetItemId(); id != "" {
		parsed, err := uuid.Parse(id)
		if err != nil {
			return "", "", nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid item_id: %w", err))
		}
		itemID = &parsed
	}
	return scope, questionType, itemID, nil
}

// CreateGuidanceRequest creates a new durable question (or resolves to a
// pre-existing identical open one — see EntRepository.CreateGuidanceRequest's
// doc comment), after checkCreateGuidanceRequestOwnership's scope-appropriate
// ownership check passes.
// +api: guidance-request:create
func (s *GuidanceRequestService) CreateGuidanceRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateGuidanceRequestRequest],
) (*connect.Response[sessionv1.CreateGuidanceRequestResponse], error) {
	msg := req.Msg
	scope, questionType, itemID, err := parseCreateGuidanceRequestFields(msg)
	if err != nil {
		return nil, err
	}
	if err := s.checkCreateGuidanceRequestOwnership(ctx, scope, msg.GetItemId(), msg.GetSessionUuid(), msg.GetCallerSessionUuid()); err != nil {
		return nil, err
	}

	// Cap is a caller-supplied parameter (session.CreateGuidanceRequestInput.Cap),
	// not something CreateGuidanceRequest reads from config itself — see that
	// field's doc comment. 0 falls back to session.DefaultGuidanceRequestPendingCap
	// until Phase 4 wires config.GetGuidanceRequestPendingCap() through here.
	data, err := s.storage.CreateGuidanceRequest(ctx, session.CreateGuidanceRequestInput{
		Scope:        scope,
		ItemID:       itemID,
		SessionUUID:  msg.GetSessionUuid(),
		QuestionText: msg.GetQuestionText(),
		QuestionType: questionType,
		Options:      encodeGuidanceOptions(msg.GetOptions()),
		Cap:          0,
	})
	if err != nil {
		if errors.Is(err, session.ErrPendingCapExceeded) {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&sessionv1.CreateGuidanceRequestResponse{Request: guidanceRequestToProto(data)}), nil
}

// AnswerGuidanceRequest records a human's answer. No ownership check — this
// is a human-only, single-operator-deployment surface (requirements.md's
// Security classification). If the request was already answered or
// cancelled, applied is false and the response reflects that CURRENT
// persisted state instead of a conflict error, per ux.md's "answer-to-
// already-answered returns the actual answer" requirement.
//
// TODO(phase 3): publish events.GuidanceRequestEventPayload via
// GuidanceRequestEventPublisher when applied==true, so
// GuidanceRequestDeliveryService can resume the asker without polling.
//
// +api: guidance-request:answer
func (s *GuidanceRequestService) AnswerGuidanceRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.AnswerGuidanceRequestRequest],
) (*connect.Response[sessionv1.AnswerGuidanceRequestResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid id: %w", err))
	}

	applied, err := s.storage.AnswerGuidanceRequest(ctx, id, req.Msg.GetAnswer())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	data, err := s.storage.GetGuidanceRequest(ctx, id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&sessionv1.AnswerGuidanceRequestResponse{
		Request: guidanceRequestToProto(data),
		Applied: applied,
	}), nil
}

// GetGuidanceRequest reads a single GuidanceRequest by id. No ownership
// check on read — matches this codebase's low-security-classification
// posture for guidance-request reads (see server/mcp's get_guidance_request).
// +api: guidance-request:get
func (s *GuidanceRequestService) GetGuidanceRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.GetGuidanceRequestRequest],
) (*connect.Response[sessionv1.GetGuidanceRequestResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid id: %w", err))
	}
	data, err := s.storage.GetGuidanceRequest(ctx, id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&sessionv1.GetGuidanceRequestResponse{Request: guidanceRequestToProto(data)}), nil
}

// ListGuidanceRequests lists open GuidanceRequests for one (scope, scope_key)
// pair, alongside the pending count and cap (ux.md's "N/cap pending" ask).
// +api: guidance-request:list
func (s *GuidanceRequestService) ListGuidanceRequests(
	ctx context.Context,
	req *connect.Request[sessionv1.ListGuidanceRequestsRequest],
) (*connect.Response[sessionv1.ListGuidanceRequestsResponse], error) {
	scope := domain.RequestScope(req.Msg.GetScope())
	rows, pendingCount, cap, err := s.storage.ListPendingGuidanceRequests(ctx, scope, req.Msg.GetScopeKey(), 0)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	protoRows := make([]*sessionv1.GuidanceRequest, 0, len(rows))
	for _, r := range rows {
		protoRows = append(protoRows, guidanceRequestToProto(r))
	}
	return connect.NewResponse(&sessionv1.ListGuidanceRequestsResponse{
		Requests:     protoRows,
		PendingCount: int32(pendingCount), //nolint:gosec // bounded by pending-cap, never near int32 overflow
		Cap:          int32(cap),          //nolint:gosec // same
	}), nil
}

// ListAllPendingGuidanceRequests lists every open GuidanceRequest across
// every scope — backs the global nav badge (Epic 7).
// +api: guidance-request:list-all-pending
func (s *GuidanceRequestService) ListAllPendingGuidanceRequests(
	ctx context.Context,
	req *connect.Request[sessionv1.ListAllPendingGuidanceRequestsRequest],
) (*connect.Response[sessionv1.ListAllPendingGuidanceRequestsResponse], error) {
	rows, err := s.storage.ListAllPendingGuidanceRequests(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	protoRows := make([]*sessionv1.GuidanceRequest, 0, len(rows))
	for _, r := range rows {
		protoRows = append(protoRows, guidanceRequestToProto(r))
	}
	return connect.NewResponse(&sessionv1.ListAllPendingGuidanceRequestsResponse{Requests: protoRows}), nil
}
