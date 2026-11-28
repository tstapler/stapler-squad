package services

// Delegators for session creation (server/services/sessioncreate): the create,
// batch, fan-out, cancel and retry RPCs and the internal spawners.

import (
	"context"
	"time"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services/autonomysvc"
	"github.com/tstapler/stapler-squad/server/services/sessioncreate"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/autonomous"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/registry"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/summaries"
	"github.com/tstapler/stapler-squad/session/tagpoller"
)

// creator is built per call.
func (s *SessionService) creator() *sessioncreate.Handler {
	return sessioncreate.New(s.createDeps())
}

// createDeps reads the service's fields when a dependency is used, so setters
// that run after construction, or while a pipeline is running, are seen. The
// bus, store, prompt store and test socket are fixed at construction.
func (s *SessionService) createDeps() sessioncreate.Deps {
	return sessioncreate.Deps{
		Bus:              s.eventBus,
		Store:            s.storage,
		PromptStore:      s.promptStore,
		TmuxServerSocket: s.testTmuxServerSocket,

		ConcreteStorage: s.GetStorage,
		ConcStorage:     func() *session.Storage { return s.concStorage },

		Registry:      func() *registry.Registry { return s.registry },
		Poller:        func() *session.ReviewQueuePoller { return s.ReviewQueuePoller },
		TagPoller:     func() *tagpoller.SessionTagClassificationPoller { return s.sessionTagPoller },
		StatusManager: func() *instance.InstanceStatusManager { return s.statusManager },
		UserPRCache:   func() *githubpkg.UserPRCache { return s.userPRCache },
		RemoteStores: func() (*sshremote.KeyStore, *sshremote.KnownHostsStore) {
			return s.remoteKeyStore, s.remoteKnownHosts
		},
		PermissionHandler: func() sshremote.PermissionRequestHandler { return s.permissionRequestHandler },
		BacklogListener:   func() *backlog.BacklogLifecycleListener { return s.backlogLifecycleListener },
		SummaryGenerator:  func() *summaries.SessionSummaryGenerator { return s.sessionSummaryGenerator },
		HeadlessPool:      func() *headless.Pool { return s.headlessPool },
		PhaseHook:         func() func(string) { return s.creationPhaseHook },

		DriverClient:      func() autonomous.HeadlessPoolClient { return s.autonomousDriverClient() },
		Autonomy:          func() *autonomysvc.AutonomousOrchestrationService { return s.AutonomousSvc },
		SSHClientPool:     s.sshClientPool,
		GitBackend:        s.gitBackendOrDefault,
		GitHubResolver:    func() sessioncreate.GitHubResolver { return s.githubResolver },
		ResolutionTimeout: func() time.Duration { return s.creationResolutionTimeout },
		MCPServerURL:      s.resolveMCPServerURL,
		WorkflowNames:     s.workflowNames,

		WorkspacePeersBlock: s.workspacePeersBlockFor,

		Track:                s.trackCleanup,
		WireCallbacks:        s.wireCallbacks,
		IndexForDelivery:     s.indexSessionForDelivery,
		FindLive:             s.FindLiveInstance,
		Instances:            s.GetInstances,
		RemoveFromAllPollers: s.removeFromAllPollers,
		ArchiveSession:       s.ArchiveSession,
	}
}

// CreateSession initializes a new AI agent session with tmux and git worktree.
func (s *SessionService) CreateSession(ctx context.Context, req *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	return s.creator().CreateSession(ctx, req)
}

// BatchCreateSessions creates several sessions with bounded concurrency.
func (s *SessionService) BatchCreateSessions(ctx context.Context, req *connect.Request[sessionv1.BatchCreateSessionsRequest]) (*connect.Response[sessionv1.BatchCreateSessionsResponse], error) {
	return s.creator().BatchCreateSessions(ctx, req)
}

// FanOutSession creates several variants of one session.
func (s *SessionService) FanOutSession(ctx context.Context, req *connect.Request[sessionv1.FanOutSessionRequest]) (*connect.Response[sessionv1.FanOutSessionResponse], error) {
	return s.creator().FanOutSession(ctx, req)
}

// PickFanoutWinner merges one variant and archives the others.
func (s *SessionService) PickFanoutWinner(ctx context.Context, req *connect.Request[sessionv1.PickFanoutWinnerRequest]) (*connect.Response[sessionv1.PickFanoutWinnerResponse], error) {
	return s.creator().PickFanoutWinner(ctx, req)
}

// PreviewDestinationPath reports where a new session's checkout would land.
func (s *SessionService) PreviewDestinationPath(ctx context.Context, req *connect.Request[sessionv1.PreviewDestinationPathRequest]) (*connect.Response[sessionv1.PreviewDestinationPathResponse], error) {
	return s.creator().PreviewDestinationPath(ctx, req)
}

// CancelSessionCreation cancels and removes a session that is still Creating.
func (s *SessionService) CancelSessionCreation(ctx context.Context, req *connect.Request[sessionv1.CancelSessionCreationRequest]) (*connect.Response[sessionv1.CancelSessionCreationResponse], error) {
	return s.creator().CancelSessionCreation(ctx, req)
}

// RetrySessionCreation reruns the resolution pipeline of a Failed session.
func (s *SessionService) RetrySessionCreation(ctx context.Context, req *connect.Request[sessionv1.RetrySessionCreationRequest]) (*connect.Response[sessionv1.RetrySessionCreationResponse], error) {
	return s.creator().RetrySessionCreation(ctx, req)
}

// AwaitCreationTerminal blocks until instanceID's creation reaches a terminal status.
func (s *SessionService) AwaitCreationTerminal(ctx context.Context, instanceID string, timeout time.Duration) (domain.CreationOutcome, error) {
	return s.creator().AwaitCreationTerminal(ctx, instanceID, timeout)
}

// SpawnReviewSession satisfies session.ReviewGateSpawner.
func (s *SessionService) SpawnReviewSession(ctx context.Context, item *backlogdto.BacklogItemData, itemSessionID string, prompt string) (*instance.Instance, error) {
	return s.creator().SpawnReviewSession(ctx, item, itemSessionID, prompt)
}

// SpawnDiagnosticSession satisfies diagnosesvc.DiagnosticSpawner.
func (s *SessionService) SpawnDiagnosticSession(ctx context.Context, item *backlogdto.BacklogItemData, prompt string) (*instance.Instance, error) {
	return s.creator().SpawnDiagnosticSession(ctx, item, prompt)
}

// CreateDirectorySession satisfies backlogsvc.SessionCreator.
func (s *SessionService) CreateDirectorySession(ctx context.Context, path string, opts domain.SessionSpawnOptions) (*instance.Instance, error) {
	return s.creator().CreateDirectorySession(ctx, path, opts)
}

// CreateWorktreeSession satisfies backlogsvc.SessionCreator.
func (s *SessionService) CreateWorktreeSession(ctx context.Context, repoPath, worktreePath string, opts domain.SessionSpawnOptions) (*instance.Instance, error) {
	return s.creator().CreateWorktreeSession(ctx, repoPath, worktreePath, opts)
}
