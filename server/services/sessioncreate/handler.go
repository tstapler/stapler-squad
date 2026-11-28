// Package sessioncreate creates sessions: CreateSession and its background
// resolution pipeline, batch and fan-out creation, cancel and retry of a
// creation, the one-shot, review and worktree spawners, and the sweeper that
// fails stale creations. It never imports server/services; the host passes what
// it reads and calls as Deps.
package sessioncreate

import (
	"context"
	"time"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/autonomysvc"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/autonomous"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/prompts"
	"github.com/tstapler/stapler-squad/session/registry"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/summaries"
	"github.com/tstapler/stapler-squad/session/tagpoller"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// GitHubResolver turns a GitHub URL into a local clone.
type GitHubResolver func(ctx context.Context, input string, enterpriseHosts []string) (localPath string, ref *instance.GitHubRef, err error)

// Deps are what session creation reads from and calls on its host.
//
// The funcs that return a dependency are called when it is used, so a setter that
// runs after the Handler is built (even while a pipeline is running) is seen.
// Every func and Bus must be non-nil; New does not check, and the host's wiring
// test does. A func documented as optional may return nil.
type Deps struct {
	Bus         *events.EventBus
	Store       session.InstanceStore
	PromptStore *prompts.PromptStore
	// TmuxServerSocket is applied to every Instance created; "" outside tests.
	TmuxServerSocket string

	// ConcreteStorage is Store as *session.Storage, or nil.
	ConcreteStorage func() *session.Storage
	// ConcStorage is the host's concrete backing store, or nil.
	ConcStorage func() *session.Storage

	// Optional values, installed after construction.
	Registry          func() *registry.Registry
	Poller            func() *session.ReviewQueuePoller
	TagPoller         func() *tagpoller.SessionTagClassificationPoller
	StatusManager     func() *instance.InstanceStatusManager
	UserPRCache       func() *githubpkg.UserPRCache
	RemoteStores      func() (*sshremote.KeyStore, *sshremote.KnownHostsStore)
	PermissionHandler func() sshremote.PermissionRequestHandler
	BacklogListener   func() *backlog.BacklogLifecycleListener
	SummaryGenerator  func() *summaries.SessionSummaryGenerator
	HeadlessPool      func() *headless.Pool
	// PhaseHook observes each Creation Phase message; test seam.
	PhaseHook func() func(msg string)

	DriverClient      func() autonomous.HeadlessPoolClient
	Autonomy          func() *autonomysvc.AutonomousOrchestrationService
	SSHClientPool     func() *tmux.SSHClientPool
	GitBackend        func() backend.Backend
	GitHubResolver    func() GitHubResolver
	ResolutionTimeout func() time.Duration
	MCPServerURL      func() string
	WorkflowNames     func() map[string]string

	// WorkspacePeersBlock is the optional prompt nudge about peers at a path.
	WorkspacePeersBlock func(ctx context.Context, repoPath string) string

	// Track runs fn in a goroutine the host's Shutdown waits for.
	Track                func(fn func())
	WireCallbacks        func(inst *instance.Instance)
	IndexForDelivery     func(inst *instance.Instance)
	FindLive             func(id string) *instance.Instance
	Instances            func() []*instance.Instance
	RemoveFromAllPollers func(id string)
	ArchiveSession       func(context.Context, *connect.Request[sessionv1.ArchiveSessionRequest]) (*connect.Response[sessionv1.ArchiveSessionResponse], error)
}

// Handler implements the session creation RPCs and spawners.
type Handler struct {
	d Deps
}

// New returns a Handler over d. It holds no state of its own, so a host may build
// one per call.
func New(d Deps) *Handler {
	return &Handler{d: d}
}
