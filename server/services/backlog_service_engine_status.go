package services

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// WorkflowEngineStatusSource is everything GetWorkflowEngineStatus reads. A nil
// optional field degrades to "unknown" for that part; a nil ModeFor or Ledger
// makes the whole RPC Unavailable so a caller can never read a default as ENFORCE.
type WorkflowEngineStatusSource struct {
	ModeFor          func(session.CallerClass) session.EngineMode
	Ledger           func() (session.ShadowLedger, error)
	ConfigDiff       func(ctx context.Context) (session.ConfigDiff, []session.StrandedItem, error)
	Acks             func() session.SoakAcks
	InvalidatorWired func() bool
	GateWired        func() bool
	FallbackActive   func() bool
	Now              func() time.Time
}

// SetWorkflowEngineStatusSource wires GetWorkflowEngineStatus; nil leaves it Unavailable.
func (s *BacklogService) SetWorkflowEngineStatusSource(src *WorkflowEngineStatusSource) {
	s.engineStatus = src
}

var engineStatusClasses = []session.CallerClass{session.ClassOperator, session.ClassAgent, session.ClassSystem}

// GetWorkflowEngineStatus reports per-class engine modes and flip-readiness evidence.
// +api: backlog:get-workflow-engine-status
func (s *BacklogService) GetWorkflowEngineStatus(
	ctx context.Context,
	_ *connect.Request[sessionv1.GetWorkflowEngineStatusRequest],
) (*connect.Response[sessionv1.GetWorkflowEngineStatusResponse], error) {
	src := s.engineStatus
	if src == nil || src.ModeFor == nil || src.Ledger == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow engine status is not wired"))
	}
	now := time.Now()
	if src.Now != nil {
		now = src.Now()
	}
	ledger, ledgerErr := src.Ledger()

	resp := &sessionv1.GetWorkflowEngineStatusResponse{
		ClassModes:       map[string]sessionv1.WorkflowEngineMode{},
		ClassReadiness:   map[string]*sessionv1.ClassReadiness{},
		InvalidatorWired: src.InvalidatorWired != nil && src.InvalidatorWired(),
		GateWired:        src.GateWired != nil && src.GateWired(),
		FallbackActive:   src.FallbackActive != nil && src.FallbackActive(),
		ConfigProvenance: sessionv1.WorkflowEngineConfigProvenance_WORKFLOW_ENGINE_CONFIG_PROVENANCE_UNSPECIFIED,
		ReviewState:      sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_UNAVAILABLE,
	}
	anyEnforced := false
	for _, c := range engineStatusClasses {
		m := src.ModeFor(c)
		anyEnforced = anyEnforced || m == session.ModeEnforce
		resp.ClassModes[string(c)] = engineModeToProto(m)
	}

	var stranded int64
	if src.ConfigDiff != nil {
		if diff, items, err := src.ConfigDiff(ctx); err == nil {
			stranded = int64(len(items))
			resp.ConfigProvenance = provenanceToProto(diff.Label)
		}
	}
	divergence := divergenceCounts(ledger, ledgerErr, now)
	resp.Divergence_24H = divergence[""]
	for _, c := range engineStatusClasses {
		v := evaluateClassSoak(src, c, ledger, ledgerErr, now, anyEnforced)
		resp.ClassReadiness[string(c)] = &sessionv1.ClassReadiness{
			Divergence_24H:    divergence[string(c)],
			SoakVerdict:       verdictToProto(v.Status),
			StrandedItemCount: stranded,
		}
	}
	fillReviewState(resp, ledger, ledgerErr, now, anyEnforced)
	return connect.NewResponse(resp), nil
}

func evaluateClassSoak(src *WorkflowEngineStatusSource, c session.CallerClass, l session.ShadowLedger, ledgerErr error, now time.Time, anyEnforced bool) session.Verdict {
	p := session.DefaultSoakParams(c, now)
	p.AnyClassEnforced = anyEnforced
	if src.Acks != nil {
		p.Acks = src.Acks()
	}
	return session.EvaluateSoakFromSource(c, func() (session.ShadowLedger, error) { return l, ledgerErr }, p)
}

// divergenceCounts counts the repro ring's last-24h records overall (key "")
// and per class. The ring holds the last 200, so a busy day reads as a floor.
func divergenceCounts(l session.ShadowLedger, err error, now time.Time) map[string]int64 {
	out := map[string]int64{}
	if err != nil {
		return out
	}
	cutoff := now.Add(-24 * time.Hour)
	for _, r := range l.Repro {
		if r.At.After(cutoff) {
			out[""]++
			out[r.Class]++
		}
	}
	return out
}

func fillReviewState(resp *sessionv1.GetWorkflowEngineStatusResponse, l session.ShadowLedger, err error, now time.Time, anyEnforced bool) {
	due, ok := session.SoakReviewDueAt(l)
	if err != nil || !ok {
		return
	}
	resp.ReviewDueAt = timestamppb.New(due)
	switch {
	case anyEnforced:
		resp.ReviewState = sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_ENFORCED
	case now.After(due):
		resp.ReviewState = sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_OVERDUE
		resp.ReviewOverdue = true
	default:
		resp.ReviewState = sessionv1.WorkflowEngineReviewState_WORKFLOW_ENGINE_REVIEW_STATE_NOT_DUE
	}
}

func engineModeToProto(m session.EngineMode) sessionv1.WorkflowEngineMode {
	switch m {
	case session.ModeOff:
		return sessionv1.WorkflowEngineMode_WORKFLOW_ENGINE_MODE_OFF
	case session.ModeShadow:
		return sessionv1.WorkflowEngineMode_WORKFLOW_ENGINE_MODE_SHADOW
	case session.ModeEnforce:
		return sessionv1.WorkflowEngineMode_WORKFLOW_ENGINE_MODE_ENFORCE
	}
	return sessionv1.WorkflowEngineMode_WORKFLOW_ENGINE_MODE_UNSPECIFIED
}

func provenanceToProto(label string) sessionv1.WorkflowEngineConfigProvenance {
	switch label {
	case session.ConfigSeeded:
		return sessionv1.WorkflowEngineConfigProvenance_WORKFLOW_ENGINE_CONFIG_PROVENANCE_SEEDED
	case session.ConfigEdited:
		return sessionv1.WorkflowEngineConfigProvenance_WORKFLOW_ENGINE_CONFIG_PROVENANCE_EDITED
	}
	return sessionv1.WorkflowEngineConfigProvenance_WORKFLOW_ENGINE_CONFIG_PROVENANCE_UNSPECIFIED
}

func verdictToProto(v session.VerdictStatus) sessionv1.WorkflowEngineSoakVerdict {
	switch v {
	case session.VerdictReady:
		return sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_READY
	case session.VerdictNotReady:
		return sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_NOT_READY
	case session.VerdictNeedsOwnerAck:
		return sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_NEEDS_OWNER_ACK
	}
	return sessionv1.WorkflowEngineSoakVerdict_WORKFLOW_ENGINE_SOAK_VERDICT_UNAVAILABLE
}
