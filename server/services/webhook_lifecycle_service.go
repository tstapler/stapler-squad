package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

// WebhookLifecycleRequest is the body of disable and delete. Compat must equal, exactly, the
// tuple persisted when the registration was created, and name a currently supported revision.
type WebhookLifecycleRequest struct {
	RequestID       string        `json:"request_id"`
	ExpectedVersion int64         `json:"expected_version"`
	Compat          WebhookCompat `json:"compat"`
}

// WebhookEmergencyRequest is the body of emergency cleanup: a disable or delete that still
// works after the registration's creation-time capability revision has been revoked. Compat
// is checked only for equality with the persisted tuple, never for current support.
type WebhookEmergencyRequest struct {
	RequestID       string        `json:"request_id"`
	ExpectedVersion int64         `json:"expected_version"`
	Compat          WebhookCompat `json:"compat"`
	Action          string        `json:"action"`
}

// Disable turns the caller's registration off. Disabling an already-disabled one is a no-op
// reported as "unchanged", so a retry after a lost response is safe.
func (s *WebhookRegistrationService) Disable(ctx context.Context, caller *WebhookCaller, instanceID string, req WebhookLifecycleRequest) (*WebhookReconcileResponse, error) {
	return s.applyLifecycle(ctx, lifecycleCall{caller: caller, instanceID: instanceID, req: req, action: session.WebhookActionDisable, mode: lifecycleOrdinary})
}

// Delete removes the caller's registration and the workflow it owns.
func (s *WebhookRegistrationService) Delete(ctx context.Context, caller *WebhookCaller, instanceID string, req WebhookLifecycleRequest) (*WebhookReconcileResponse, error) {
	return s.applyLifecycle(ctx, lifecycleCall{caller: caller, instanceID: instanceID, req: req, action: session.WebhookActionDelete, mode: lifecycleOrdinary})
}

// EmergencyCleanup disables or deletes a registration without requiring its creation-time
// capability revision to still be supported. Every failure to prove ownership or provenance
// is the same bounded NOT_FOUND: nothing about the registration is revealed.
func (s *WebhookRegistrationService) EmergencyCleanup(ctx context.Context, caller *WebhookCaller, instanceID string, req WebhookEmergencyRequest) (*WebhookReconcileResponse, error) {
	var action session.WebhookLifecycleAction
	switch req.Action {
	case string(session.WebhookActionDisable):
		action = session.WebhookActionDisable
	case string(session.WebhookActionDelete):
		action = session.WebhookActionDelete
	default:
		return nil, apiErr(http.StatusBadRequest, "INVALID_REQUEST", `action must be "disable" or "delete"`)
	}
	lifecycle := WebhookLifecycleRequest{RequestID: req.RequestID, ExpectedVersion: req.ExpectedVersion, Compat: req.Compat}
	return s.applyLifecycle(ctx, lifecycleCall{caller: caller, instanceID: instanceID, req: lifecycle, action: action, mode: lifecycleEmergency})
}

// lifecycleMode distinguishes the ordinary path, which requires a currently supported
// capability revision, from emergency cleanup, which does not.
type lifecycleMode int

const (
	lifecycleOrdinary lifecycleMode = iota
	lifecycleEmergency
)

// lifecycleCall bundles one disable/delete request with who is asking and how.
type lifecycleCall struct {
	caller     *WebhookCaller
	instanceID string
	req        WebhookLifecycleRequest
	action     session.WebhookLifecycleAction
	mode       lifecycleMode
}

func (c lifecycleCall) emergency() bool { return c.mode == lifecycleEmergency }

func (s *WebhookRegistrationService) applyLifecycle(ctx context.Context, call lifecycleCall) (*WebhookReconcileResponse, error) {
	caller, instanceID, req, action, emergency := call.caller, call.instanceID, call.req, call.action, call.emergency()
	in, err := s.buildLifecycleInput(call)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.ApplyWebhookLifecycle(ctx, *in)
	if err != nil {
		mapped := mapWebhookRepoError(err)
		if emergency && mapped.Code == "COMPAT_TUPLE_MISMATCH" {
			mapped = apiErr(http.StatusNotFound, "NOT_FOUND", "registration not found")
		}
		log.Warn("[WebhookManagement] lifecycle refused", "principal", caller.PrincipalID, "instance", instanceID,
			"request_id", req.RequestID, "action", string(action), "emergency", emergency, "code", mapped.Code)
		return nil, mapped
	}
	s.publishLifecycle(result)
	log.Info("[WebhookManagement] lifecycle", "principal", caller.PrincipalID, "instance", instanceID,
		"request_id", req.RequestID, "action", string(action), "emergency", emergency,
		"outcome", result.Outcome, "replayed", result.Replayed)
	return &WebhookReconcileResponse{
		Outcome:      result.Outcome,
		Replayed:     result.Replayed,
		Registration: registrationBodyOf(&result.Registration),
	}, nil
}

func (s *WebhookRegistrationService) buildLifecycleInput(call lifecycleCall) (*session.WebhookLifecycleInput, error) {
	caller, instanceID, req, action, emergency := call.caller, call.instanceID, call.req, call.action, call.emergency()
	if !webhookInstanceIDRe.MatchString(instanceID) {
		return nil, apiErr(http.StatusBadRequest, "INVALID_REQUEST", "invalid instance id")
	}
	if !webhookRequestIDRe.MatchString(req.RequestID) {
		return nil, apiErr(http.StatusBadRequest, "INVALID_REQUEST", "request_id must be 8-128 characters of [A-Za-z0-9._-]")
	}
	if req.ExpectedVersion < 1 {
		return nil, apiErr(http.StatusBadRequest, "INVALID_REQUEST", "expected_version must be >= 1")
	}
	tuple, tupleSHA, err := s.canonicalCompat(req.Compat, !emergency)
	if err != nil {
		return nil, err
	}
	_, macKey, err := s.secretKeys()
	if err != nil {
		return nil, apiErr(http.StatusInternalServerError, "INTERNAL", "secret storage unavailable")
	}
	in := &session.WebhookLifecycleInput{
		Scope: caller.WebhookScope, RequestID: req.RequestID, InstanceID: instanceID, Action: action,
		Emergency: emergency, ExpectedVersion: req.ExpectedVersion, CompatTuple: tuple, CompatTupleSHA256: tupleSHA,
	}
	canonical, marshalErr := json.Marshal(struct {
		Operation         string `json:"operation"`
		InstanceID        string `json:"instance_id"`
		ExpectedVersion   int64  `json:"expected_version"`
		CompatTupleSHA256 string `json:"compat_tuple_sha256"`
	}{Operation: lifecycleOperation(action, emergency), InstanceID: instanceID, ExpectedVersion: req.ExpectedVersion, CompatTupleSHA256: tupleSHA})
	if marshalErr != nil {
		return nil, apiErr(http.StatusInternalServerError, "INTERNAL", "could not fingerprint request")
	}
	in.Fingerprint = keyedHex(macKey, canonical)
	return in, nil
}

func lifecycleOperation(action session.WebhookLifecycleAction, emergency bool) string {
	if emergency {
		return fmt.Sprintf("emergency_%s", action)
	}
	return string(action)
}

// publishLifecycle tells open UIs about the change. Replays and no-ops publish nothing.
func (s *WebhookRegistrationService) publishLifecycle(result *session.WebhookReconcileResult) {
	if s.eventBus == nil || result.Replayed {
		return
	}
	switch result.Outcome {
	case session.WebhookOutcomeDisabled:
		s.eventBus.Publish(events.NewWorkflowChangedEvent(&events.WorkflowEventPayload{
			Kind: events.WorkflowChangeUpdated, Workflow: result.ChangedWorkflow}))
	case session.WebhookOutcomeDeleted:
		s.eventBus.Publish(events.NewWorkflowChangedEvent(&events.WorkflowEventPayload{
			Kind: events.WorkflowChangeDeleted, WorkflowID: result.Registration.WorkflowID.String()}))
	}
}
