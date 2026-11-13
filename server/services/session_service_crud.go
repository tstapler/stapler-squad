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
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/hostclaim"
)

// hiddenVisibleInList applies include_hidden / hidden_only to one session. hidden_only
// wins over include_hidden: the Background view must never receive visible sessions.
func hiddenVisibleInList(hidden bool, req *sessionv1.ListSessionsRequest) bool {
	if req.HiddenOnly {
		return hidden
	}
	return !hidden || req.IncludeHidden
}

// itemSessionBatchLookup is the one storage call background_outcome needs.
type itemSessionBatchLookup interface {
	ListItemSessionsBySessionUUIDs(ctx context.Context, uuids []string) (map[string]backlogdto.ItemSessionSummary, error)
}

type hiddenOutcomeEntry struct {
	proto  *sessionv1.Session
	uuid   string
	status domain.Status
}

// populateBackgroundOutcomes sets Session.background_outcome for hidden_only
// responses using one batched ItemSession query. On lookup failure it degrades
// to status-only outcomes rather than failing the list.
func (s *SessionService) populateBackgroundOutcomes(ctx context.Context, entries []hiddenOutcomeEntry) {
	if len(entries) == 0 {
		return
	}
	lookup := s.outcomeLookup
	if lookup == nil && s.concStorage != nil {
		lookup = s.concStorage
	}
	var byUUID map[string]backlogdto.ItemSessionSummary
	if lookup != nil {
		uuids := make([]string, 0, len(entries))
		for _, e := range entries {
			uuids = append(uuids, e.uuid)
		}
		var err error
		if byUUID, err = lookup.ListItemSessionsBySessionUUIDs(ctx, uuids); err != nil {
			log.Warn("ListSessions: background outcome lookup failed", "err", err)
		}
	}
	for _, e := range entries {
		var is *backlogdto.ItemSessionSummary
		if row, ok := byUUID[e.uuid]; ok {
			is = &row
		}
		e.proto.BackgroundOutcome = string(session.ComputeBackgroundOutcome(e.status, is))
	}
}

// ListSessions returns all sessions with optional filtering.
// This includes both managed sessions and external mux-enabled sessions.
// +api: session:list
func (s *SessionService) ListSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSessionsRequest],
) (*connect.Response[sessionv1.ListSessionsResponse], error) {
	resp, err := s.listLocalSessions(ctx, req)
	if err != nil || s.federation == nil || req.Msg.HiddenOnly {
		return resp, err
	}
	resp.Msg.Sessions = append(resp.Msg.Sessions, s.federation.filtered(req.Msg)...)
	return resp, nil
}

// LocalSessions lists only this instance's sessions with default filters. It is
// what peers are served: the merged view would echo their own sessions back.
func (s *SessionService) LocalSessions(ctx context.Context) ([]*sessionv1.Session, error) {
	resp, err := s.listLocalSessions(ctx, connect.NewRequest(&sessionv1.ListSessionsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Sessions, nil
}

// LocalPaneSnapshot captures the pane of a live local session, for serving a
// peer's read-only terminal stream. It never starts or revives a session.
func (s *SessionService) LocalPaneSnapshot(sessionID string) (string, error) {
	if s.ReviewQueuePoller == nil {
		return "", fmt.Errorf("session not found")
	}
	inst := s.ReviewQueuePoller.FindInstance(sessionID)
	if inst == nil {
		return "", fmt.Errorf("session not found")
	}
	return inst.CapturePaneContent()
}

// SetFederation enables merging peer sessions into ListSessions.
func (s *SessionService) SetFederation(f *FederatedSessions) { s.federation = f }

// instanceMatchesListFilters applies ListSessions' status, category, hidden,
// archived and workflow filters to a managed instance.
func instanceMatchesListFilters(inst *session.Instance, msg *sessionv1.ListSessionsRequest) bool {
	// Read the status directly off the instance instead of building the full proto
	// just to inspect one field: the proto build runs the expensive
	// GetEffectiveStatus/DetectStateFromContent chain, so doing it twice per
	// instance roughly doubles ListSessions' allocation cost under a status filter.
	if msg.Status != nil && *msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED {
		effProto, err := adapters.StatusToProto(inst.GetEffectiveStatus())
		if err != nil {
			log.Error("ListSessions: unrecognized session.Status", "status", int(inst.GetEffectiveStatus()), "err", err)
			return false
		}
		if effProto != *msg.Status {
			return false
		}
	}
	if msg.Category != nil && *msg.Category != "" && inst.Category != *msg.Category {
		return false
	}
	// hidden_only keeps only hidden sessions and implies include_hidden.
	if !hiddenVisibleInList(inst.Hidden, msg) {
		return false
	}
	if inst.ArchivedAt != nil && !msg.IncludeArchived {
		return false
	}
	return msg.WorkflowId == nil || *msg.WorkflowId == "" || inst.WorkflowID == *msg.WorkflowId
}

// externalMatchesListFilters applies the status, category and hidden filters to
// a mux-discovered external session (always running, hence ACTIVE).
func externalMatchesListFilters(ext *session.Instance, msg *sessionv1.ListSessionsRequest) bool {
	if msg.Status != nil && *msg.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED &&
		*msg.Status != sessionv1.SessionStatus_SESSION_STATUS_ACTIVE {
		return false
	}
	if msg.Category != nil && *msg.Category != "" && ext.Category != *msg.Category {
		return false
	}
	return hiddenVisibleInList(ext.Hidden, msg)
}

func (s *SessionService) listLocalSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSessionsRequest],
) (*connect.Response[sessionv1.ListSessionsResponse], error) {
	// Use the poller's live in-memory instances to avoid the side effect of
	// LoadInstances() → FromInstanceData() → Start() which restarts every session.
	var instances []*session.Instance
	if s.ReviewQueuePoller != nil {
		instances = s.ReviewQueuePoller.GetInstances()
	} else {
		var err error
		instances, err = s.loadInstancesWithWiring()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
	}

	prFilter, hasPRFilter, err := listPRFilter(req.Msg.GetPr())
	if err != nil {
		return nil, err
	}

	// Convert instances to proto messages
	sessions := make([]*sessionv1.Session, 0, len(instances))
	var hiddenManaged []hiddenOutcomeEntry
	for _, inst := range instances {
		if !instanceMatchesListFilters(inst, req.Msg) {
			continue
		}

		if hasPRFilter && !instanceHasPR(inst, prFilter) {
			continue
		}

		protoSess := adapters.InstanceToProto(inst, s.workflowNames())
		if s.memoryCacheReader != nil && inst.IsActive() {
			rss := s.memoryCacheReader.GetCachedRSSMB(inst.UUID)
			protoSess.MemoryRssMb = rss
			protoSess.EstimatedSavingsMb = rss
		}
		sessions = append(sessions, protoSess)
		if req.Msg.HiddenOnly {
			hiddenManaged = append(hiddenManaged, hiddenOutcomeEntry{proto: protoSess, uuid: inst.Snapshot().UUID, status: inst.GetEffectiveStatus()})
		}
	}
	s.populateBackgroundOutcomes(ctx, hiddenManaged)

	// Include external sessions from mux discovery if available
	if s.externalDiscovery != nil {
		for _, extInst := range s.externalDiscovery.GetSessions() {
			if !externalMatchesListFilters(extInst, req.Msg) {
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

	if !hostclaim.ParseFederatedSessionID(req.Msg.Id).IsLocal() {
		if s.federation != nil {
			if remote, ok := s.federation.Session(req.Msg.Id); ok {
				return connect.NewResponse(&sessionv1.GetSessionResponse{Session: remote}), nil
			}
		}
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Use the poller's live in-memory instances to avoid the side effect of
	// LoadInstances() → FromInstanceData() → Start() which restarts every session.
	if s.ReviewQueuePoller != nil {
		wfNames := s.workflowNames()
		if inst := s.ReviewQueuePoller.FindInstance(req.Msg.Id); inst != nil {
			return connect.NewResponse(&sessionv1.GetSessionResponse{
				Session: withSessionPRHolds(adapters.InstanceToProto(inst, wfNames), inst),
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
			Session: withSessionPRHolds(adapters.InstanceToProto(inst, wfNames), inst),
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
	return WorkspacePeersBlockFor(ctx, s.concStorage, repoPath)
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
