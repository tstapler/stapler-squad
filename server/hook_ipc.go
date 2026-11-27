package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/server/services/approvals"
)

type hookIPCState struct {
	classifier *approvals.HookClassifier

	mu     sync.Mutex
	server *hookipc.Server
}

func newHookIPCState(deps *ServerDependencies) *hookIPCState {
	if deps == nil || deps.SessionService == nil {
		return nil
	}
	c := approvals.NewHookClassifier(deps.SessionService.GetClassifier(), deps.SessionService.ClassificationRecorder())
	c.SetHookReplier(deps.SessionService.HookReplier())
	return &hookIPCState{classifier: c}
}

func (s *hookIPCState) Start() error {
	if s == nil || s.classifier == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return nil
	}
	endpoint, err := hookipc.ResolveEndpoint("")
	if err != nil {
		return fmt.Errorf("start hook ipc: %w", err)
	}
	server, err := hookipc.NewServer(endpoint, s.classifier.Classify)
	if err != nil {
		return fmt.Errorf("start hook ipc: %w", err)
	}
	if err := server.Start(); err != nil {
		return fmt.Errorf("start hook ipc: %w", err)
	}
	s.server = server
	return nil
}

func (s *hookIPCState) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.mu.Unlock()
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = server.Close(ctx)
}
