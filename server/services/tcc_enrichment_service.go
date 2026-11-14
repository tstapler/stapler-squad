package services

import (
	"context"
	"errors"
	"sort"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tcc"
)

type tccEnrichmentState struct {
	gate     *tcc.ConsentGate
	enricher *instance.WorktreeEnricher
	enabled  bool
}

// SetTCCEnrichment wires the startup-owned gate before serving requests.
func (s *SessionService) SetTCCEnrichment(gate *tcc.ConsentGate, enricher *instance.WorktreeEnricher, enabled bool) {
	s.tccEnrichment = tccEnrichmentState{gate: gate, enricher: enricher, enabled: enabled}
}

// +api: tcc:enrichment-status
func (s *SessionService) GetTCCEnrichmentStatus(_ context.Context, _ *connect.Request[sessionv1.GetTCCEnrichmentStatusRequest]) (*connect.Response[sessionv1.GetTCCEnrichmentStatusResponse], error) {
	state := s.tccEnrichment
	resp := &sessionv1.GetTCCEnrichmentStatusResponse{Enabled: state.enabled}
	if !state.enabled || state.gate == nil {
		return connect.NewResponse(resp), nil
	}
	// Read live snapshots only: loading storage here can restart sessions and do I/O.
	titles := map[string][]string{}
	if s.ReviewQueuePoller != nil {
		for _, inst := range s.ReviewQueuePoller.GetInstances() {
			snap := inst.Snapshot()
			path := inst.ActiveDir()
			titles[path] = append(titles[path], snap.Title)
		}
	}
	for _, root := range state.gate.Roots() {
		row := &sessionv1.TCCEnrichmentRoot{Id: root.ID, Root: root.Root, Paths: root.Paths, AppConsented: root.Consented, Outcome: enrichmentRootOutcome(state.enricher, root.Paths)}
		if root.Denied {
			row.Outcome = tccOutcomeDenied
			row.SettingsUrl = root.Service.SettingsURL()
		}
		for _, path := range root.Paths {
			row.SessionTitles = append(row.SessionTitles, titles[path]...)
		}
		sort.Strings(row.SessionTitles)
		resp.Roots = append(resp.Roots, row)
	}
	return connect.NewResponse(resp), nil
}

const tccOutcomeDenied = "denied"

// enrichmentRootOutcome reports the enrichment outcome only; "denied" is the
// gate's sticky EPERM hint and is applied by the caller.
func enrichmentRootOutcome(enricher *instance.WorktreeEnricher, paths []string) string {
	outcome := "completed"
	for _, path := range paths {
		current := enricher.Outcome(path)
		switch {
		case current == "unresolved/slow":
			return current
		case current == "error":
			outcome = current
		case current == "pending" && outcome != "error":
			outcome = current
		}
	}
	return outcome
}

// +api: tcc:continue-enrichment
func (s *SessionService) ContinueTCCEnrichment(ctx context.Context, req *connect.Request[sessionv1.ContinueTCCEnrichmentRequest]) (*connect.Response[sessionv1.ContinueTCCEnrichmentResponse], error) {
	if _, err := EvaluateLocalWrite(ctx, req.Peer().Addr, LocalWriteRebinding); err != nil {
		return nil, err
	}
	state := s.tccEnrichment
	if !state.enabled || state.gate == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("enrichment consent gate is not enabled"))
	}
	paths, err := state.gate.Continue(req.Msg.RootId)
	if err != nil {
		code := connect.CodeInternal
		if errors.Is(err, tcc.ErrUnknownRoot) {
			code = connect.CodeInvalidArgument
		}
		return nil, connect.NewError(code, err)
	}
	for _, path := range paths {
		state.enricher.Unpark(path)
	}
	rechecked, err := state.gate.Recheck(req.Msg.RootId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, path := range rechecked {
		state.enricher.Recheck(path)
	}
	return connect.NewResponse(&sessionv1.ContinueTCCEnrichmentResponse{}), nil
}
