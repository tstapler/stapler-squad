package sessioncreate

import (
	"context"
	"fmt"
	"sync"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// +api: session:batch-create
// BatchCreateSessions creates multiple sessions with bounded concurrency (max 3) and
// per-repo serialization to prevent git worktree races.
func (h *Handler) BatchCreateSessions(
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
	existing, err := h.d.Store.LoadInstances()
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

			resp, createErr := h.CreateSession(ctx, createReq)
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
