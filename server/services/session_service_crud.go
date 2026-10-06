package services

import (
	"context"
	"fmt"
	"sync"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/session"
)

// ListSessions returns all sessions with optional filtering.
// This includes both managed sessions and external mux-enabled sessions.
// +api: session:list
func (s *SessionService) ListSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSessionsRequest],
) (*connect.Response[sessionv1.ListSessionsResponse], error) {
	// Use the poller's live in-memory instances to avoid the side effect of
	// LoadInstances() → FromInstanceData() → Start() which restarts every session.
	var instances []*session.Instance
	if s.reviewQueuePoller != nil {
		instances = s.reviewQueuePoller.GetInstances()
	} else {
		var err error
		instances, err = s.loadInstancesWithWiring()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
	}

	// Convert instances to proto messages
	sessions := make([]*sessionv1.Session, 0, len(instances))
	for _, inst := range instances {
		// Apply optional status filter. Read the status directly off the instance
		// instead of building the full proto just to inspect one field — building it
		// runs the full GetEffectiveStatus/GetStatusAndIdleInfo/DetectStateFromContent
		// chain, which is expensive enough that doing it twice per instance (once here,
		// once for the real output below) roughly doubles ListSessions' allocation cost
		// whenever a status filter is applied.
		if req.Msg.Status != nil && *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
			effProto, err := adapters.StatusToProto(inst.GetEffectiveStatus())
			if err != nil {
				log.Error("ListSessions: unrecognized session.Status", "status", int(inst.GetEffectiveStatus()), "err", err)
				continue
			}
			if effProto != *req.Msg.Status {
				continue
			}
		}

		// Apply optional category filter
		if req.Msg.Category != nil && *req.Msg.Category != "" && inst.Category != *req.Msg.Category {
			continue
		}

		// Exclude hidden (system/background) sessions unless explicitly requested
		if inst.Hidden && !req.Msg.IncludeHidden {
			continue
		}

		// Exclude archived sessions unless explicitly requested
		if inst.ArchivedAt != nil && !req.Msg.IncludeArchived {
			continue
		}

		// Filter by workflow_id when specified
		if req.Msg.WorkflowId != nil && *req.Msg.WorkflowId != "" && inst.WorkflowID != *req.Msg.WorkflowId {
			continue
		}

		protoSess := adapters.InstanceToProto(inst, s.workflowNames())
		if s.memoryCacheReader != nil && inst.IsActive() {
			rss := s.memoryCacheReader.GetCachedRSSMB(inst.UUID)
			protoSess.MemoryRssMb = rss
			protoSess.EstimatedSavingsMb = rss
		}
		sessions = append(sessions, protoSess)
	}

	// Include external sessions from mux discovery if available
	if s.externalDiscovery != nil {
		for _, extInst := range s.externalDiscovery.GetSessions() {
			// Apply optional status filter (external sessions are always "running")
			if req.Msg.Status != nil && *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
				// External sessions are running
				if *req.Msg.Status != sessionv1.SessionStatus_SESSION_STATUS_ACTIVE {
					continue
				}
			}

			// Apply optional category filter
			if req.Msg.Category != nil && *req.Msg.Category != "" && extInst.Category != *req.Msg.Category {
				continue
			}

			// Exclude hidden external sessions unless requested
			if extInst.Hidden && !req.Msg.IncludeHidden {
				continue
			}

			sessions = append(sessions, adapters.InstanceToProto(extInst, nil))
		}
	}

	var sysPct float32
	if s.memoryCacheReader != nil {
		pct, err := s.memoryCacheReader.SystemMemoryPct()
		if err != nil {
			log.Warn("ListSessions: cannot read system memory", "err", err)
		}
		sysPct = float32(pct)
	}

	return connect.NewResponse(&sessionv1.ListSessionsResponse{
		Sessions:        sessions,
		SystemMemoryPct: sysPct,
	}), nil
}

// GetSession retrieves a specific session by ID (Title).
func (s *SessionService) GetSession(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSessionRequest],
) (*connect.Response[sessionv1.GetSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	// Use the poller's live in-memory instances to avoid the side effect of
	// LoadInstances() → FromInstanceData() → Start() which restarts every session.
	if s.reviewQueuePoller != nil {
		wfNames := s.workflowNames()
		if inst := s.reviewQueuePoller.FindInstance(req.Msg.Id); inst != nil {
			return connect.NewResponse(&sessionv1.GetSessionResponse{
				Session: adapters.InstanceToProto(inst, wfNames),
			}), nil
		}
		// Not in poller — also check external sessions
		if s.externalDiscovery != nil {
			if inst := s.externalDiscovery.GetSession(req.Msg.Id); inst != nil {
				return connect.NewResponse(&sessionv1.GetSessionResponse{
					Session: adapters.InstanceToProto(inst, nil),
				}), nil
			}
		}
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Fallback: poller not available — load from storage (has Start() side effect)
	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	// Find instance by ID (UUID or legacy Title).
	wfNames := s.workflowNames()
	if inst := findInstanceByID(instances, req.Msg.Id); inst != nil {
		return connect.NewResponse(&sessionv1.GetSessionResponse{
			Session: adapters.InstanceToProto(inst, wfNames),
		}), nil
	}

	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
}

// workspacePeersBlockFor returns a one-time "other active sessions in this workspace"
// nudge for a new session being created at repoPath, or "" when the workspacePeersNudgeFlagName
// feature flag is off (default), on any detection/lookup failure, when there's no concrete
// storage backing this service, or when there are no peers. Best-effort: this is a
// convenience nudge, not required session context. Delegates to
// session.WorkspacePeersBlockForPath, shared with BacklogService's initialPromptFor so the
// two callers can't drift on how the nudge is built.
func (s *SessionService) workspacePeersBlockFor(ctx context.Context, repoPath string) string {
	return workspacePeersBlockFor(ctx, s.concStorage, repoPath)
}

// ─── Batch Session Creation ───────────────────────────────────────────────────

// +api: session:batch-create
// BatchCreateSessions creates multiple sessions with bounded concurrency (max 3) and
// per-repo serialization to prevent git worktree races.
func (s *SessionService) BatchCreateSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.BatchCreateSessionsRequest],
) (*connect.Response[sessionv1.BatchCreateSessionsResponse], error) {
	if len(req.Msg.Sessions) == 0 {
		return connect.NewResponse(&sessionv1.BatchCreateSessionsResponse{}), nil
	}

	// Server-side cap: never more than 3 concurrent worktree creations.
	maxConc := int(req.Msg.MaxConcurrency)
	if maxConc <= 0 || maxConc > 3 {
		maxConc = 3
	}

	// Pre-check: load existing sessions to detect title conflicts before spawning goroutines.
	existing, err := s.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}
	existingTitles := make(map[string]struct{}, len(existing))
	for _, inst := range existing {
		existingTitles[inst.Title] = struct{}{}
	}

	results := make([]*sessionv1.BatchCreateResult, len(req.Msg.Sessions))
	seenTitles := make(map[string]struct{}, len(req.Msg.Sessions))
	var pendingIdx []int

	// Validate and dedup all requests upfront; fail-fast invalid ones without goroutines.
	for i, sess := range req.Msg.Sessions {
		var earlyErr string
		switch {
		case sess.Title == "":
			earlyErr = "title is required"
		case sess.Path == "":
			earlyErr = "path is required"
		default:
			if _, exists := existingTitles[sess.Title]; exists {
				earlyErr = fmt.Sprintf("session '%s' already exists", sess.Title)
			} else if _, dup := seenTitles[sess.Title]; dup {
				earlyErr = fmt.Sprintf("duplicate title '%s' in batch request", sess.Title)
			}
		}
		if earlyErr != "" {
			results[i] = &sessionv1.BatchCreateResult{Success: false, Title: sess.Title, Error: earlyErr}
			continue
		}
		seenTitles[sess.Title] = struct{}{}
		pendingIdx = append(pendingIdx, i)
	}

	// Semaphore bounds concurrent goroutines.
	sem := make(chan struct{}, maxConc)

	// Per-repo mutex serializes git worktree creation within the same repo directory.
	var repoMutexes sync.Map // map[string]*sync.Mutex

	var wg sync.WaitGroup
	for _, idx := range pendingIdx {
		sess := req.Msg.Sessions[idx]
		wg.Add(1)
		go func(i int, batchReq *sessionv1.BatchSessionRequest) {
			defer wg.Done()

			sem <- struct{}{} // acquire slot
			defer func() { <-sem }()

			// Serialize worktree creation per repo to avoid git conflicts.
			muIface, _ := repoMutexes.LoadOrStore(batchReq.Path, &sync.Mutex{})
			repoLock := muIface.(*sync.Mutex)
			repoLock.Lock()
			defer repoLock.Unlock()

			createReq := connect.NewRequest(&sessionv1.CreateSessionRequest{
				Title:       batchReq.Title,
				Path:        batchReq.Path,
				WorkingDir:  batchReq.WorkingDir,
				Branch:      batchReq.Branch,
				Program:     batchReq.Program,
				Category:    batchReq.Category,
				AutoYes:     batchReq.AutoYes,
				SessionType: batchReq.SessionType,
				ProjectId:   batchReq.ProjectId,
			})

			resp, createErr := s.CreateSession(ctx, createReq)
			// Each goroutine writes to a distinct index, so no mutex needed.
			if createErr != nil {
				results[i] = &sessionv1.BatchCreateResult{
					Success: false,
					Title:   batchReq.Title,
					Error:   createErr.Error(),
				}
			} else {
				results[i] = &sessionv1.BatchCreateResult{
					Success:   true,
					Title:     batchReq.Title,
					SessionId: resp.Msg.Session.Id,
				}
			}
		}(idx, sess)
	}
	wg.Wait()

	// Tally final counts from results.
	var succeeded, failed int32
	for _, r := range results {
		if r == nil {
			continue
		}
		if r.Success {
			succeeded++
		} else {
			failed++
		}
	}

	return connect.NewResponse(&sessionv1.BatchCreateSessionsResponse{
		Results:   results,
		Succeeded: succeeded,
		Failed:    failed,
	}), nil
}
