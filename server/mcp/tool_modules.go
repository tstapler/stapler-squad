package mcp

import (
	mcpserver "github.com/mark3labs/mcp-go/server"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/backlogflow"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog/reviewgate"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// ToolModule registers one family of MCP tools. A new family is a new file with
// a constructor returning a ToolModule, plus one entry in defaultToolModules (or
// a WithToolModules option at the composition root). A constructor whose
// dependencies are missing returns a literal nil interface, never a typed-nil
// pointer wrapped in one, and the assembler skips it.
type ToolModule interface {
	Register(s *mcpserver.MCPServer)
}

type toolModuleFunc func(s *mcpserver.MCPServer)

func (f toolModuleFunc) Register(s *mcpserver.MCPServer) { f(s) }

// WithToolModules appends modules to the built-in set; they register after it,
// in the order given.
func WithToolModules(mods ...ToolModule) CoreOption {
	return func(c *coreConfig) { c.extraModules = append(c.extraModules, mods...) }
}

// coreDeps carries NewCore's positional arguments to the module constructors,
// the one place server/mcp acts as a second composition root.
type coreDeps struct {
	store          session.InstanceStore
	svc            *services.SessionService
	sbMgr          *scrollback.ScrollbackManager
	storage        *session.Storage
	eventBus       *events.EventBus
	prCache        *githubpkg.UserPRCache
	backlogEnabled func() bool
	autoReopener   reviewgate.AutoReopenSpawner
	backlogSvc     *backlogsvc.BacklogService
	liveCheck      func(sessionUUID string) bool
	cfg            coreConfig
}

// defaultToolModules lists the built-in families in registration order; the
// order is pinned by tool_set_golden_test.go and TestDefaultToolModules_*.
func defaultToolModules(d coreDeps) []ToolModule {
	// A nil *services.SessionService or *session.Storage stored directly in an
	// interface-typed field would be a non-nil interface around a nil pointer,
	// so each gate below is an explicit pointer comparison.
	var diagnoseCheck diagnoseCallerCheck
	if d.storage != nil {
		diagnoseCheck = d.storage.IsDiagnoseCaller
	}
	var liveFinder liveInstanceFinder
	if d.svc != nil {
		liveFinder = d.svc
	}

	mods := []ToolModule{
		newDiscoveryModule(d.store),
		newLifecycleModule(d.store, d.svc, diagnoseCheck),
		newTerminalModule(d.store, liveFinder, d.sbMgr, diagnoseCheck),
		newVCSModule(d.store),
		newWorkflowModule(d.svc),
		newRulesModule(d.svc),
		newTaggingRulesModule(d.svc),
		newNotificationsModule(d.svc),
		newHistoryModule(d.svc),
	}
	mods = append(mods, newBacklogModules(d, liveFinder)...)
	mods = append(mods, newIdentityGatedModules(d.storage, d.eventBus)...)
	mods = append(mods, newGitHubModule(d.prCache, d.store, d.svc))
	return append(mods, d.cfg.extraModules...)
}

func registerModules(s *mcpserver.MCPServer, mods []ToolModule) {
	for _, m := range mods {
		if m != nil {
			m.Register(s)
		}
	}
}

func newDiscoveryModule(store session.InstanceStore) ToolModule {
	return toolModuleFunc(func(s *mcpserver.MCPServer) { registerDiscoveryTools(s, &discoveryHandlers{store: store}) })
}

func newLifecycleModule(store session.InstanceStore, svc *services.SessionService, diagnoseCheck diagnoseCallerCheck) ToolModule {
	return toolModuleFunc(func(s *mcpserver.MCPServer) {
		registerLifecycleTools(s, &lifecycleHandlers{store: store, svc: svc, diagnoseCheck: diagnoseCheck})
	})
}

func newTerminalModule(store session.InstanceStore, live liveInstanceFinder, sbMgr *scrollback.ScrollbackManager, diagnoseCheck diagnoseCallerCheck) ToolModule {
	return toolModuleFunc(func(s *mcpserver.MCPServer) {
		registerTerminalTools(s, &terminalHandlers{
			store:         store,
			live:          live,
			scrollback:    sbMgr,
			writeLim:      newTokenBucket(writeRateLimitPerSec, writeRateLimitPerSec),
			diagnoseCheck: diagnoseCheck,
		})
	})
}

func newVCSModule(store session.InstanceStore) ToolModule {
	return toolModuleFunc(func(s *mcpserver.MCPServer) { registerVCSTools(s, &vcsHandlers{store: store}) })
}

func newWorkflowModule(svc *services.SessionService) ToolModule {
	if svc == nil {
		return nil
	}
	return toolModuleFunc(func(s *mcpserver.MCPServer) { registerWorkflowTools(s, &workflowHandlers{svc: svc}) })
}

func newRulesModule(svc *services.SessionService) ToolModule {
	if svc == nil {
		return nil
	}
	return toolModuleFunc(func(s *mcpserver.MCPServer) { registerRulesTools(s, &rulesHandlers{svc: svc}) })
}

func newTaggingRulesModule(svc *services.SessionService) ToolModule {
	if svc == nil {
		return nil
	}
	return toolModuleFunc(func(s *mcpserver.MCPServer) { registerTaggingRulesTools(s, &taggingRulesHandlers{svc: svc}) })
}

func newNotificationsModule(svc *services.SessionService) ToolModule {
	if svc == nil {
		return nil
	}
	return toolModuleFunc(func(s *mcpserver.MCPServer) { registerNotificationTools(s, &notificationHandlers{svc: svc}) })
}

func newHistoryModule(svc *services.SessionService) ToolModule {
	if svc == nil {
		return nil
	}
	return toolModuleFunc(func(s *mcpserver.MCPServer) { registerHistoryTools(s, &historyHandlers{svc: svc}) })
}

// newBacklogModules builds the backlog-gated families. The one *backlogHandlers
// is shared by the backlog, stuck-report and guidance modules (guidance calls
// h.resolveItemLink). Gated on storage and the live backlog flag at startup;
// handlers re-check the flag per call.
func newBacklogModules(d coreDeps, liveFinder liveInstanceFinder) []ToolModule {
	if d.storage == nil || (d.backlogEnabled != nil && !d.backlogEnabled()) {
		return nil
	}
	h := &backlogHandlers{storage: d.storage, store: d.store, eventBus: d.eventBus, reviewStopper: d.svc, reviewTrigger: d.svc, enabledCheck: d.backlogEnabled, autoReopener: d.autoReopener, backlogSvc: d.backlogSvc, liveCheck: d.liveCheck, plugins: d.cfg.plugins}
	return []ToolModule{
		toolModuleFunc(func(s *mcpserver.MCPServer) { registerBacklogTools(s, h) }),
		toolModuleFunc(func(s *mcpserver.MCPServer) { registerStuckReportTools(s, h) }),
		newClaimModule(d.backlogSvc, d.backlogEnabled),
		toolModuleFunc(func(s *mcpserver.MCPServer) {
			registerGoalTools(s, &goalHandlers{storage: d.storage, store: d.store, eventBus: d.eventBus, enabledCheck: d.backlogEnabled})
		}),
		toolModuleFunc(func(s *mcpserver.MCPServer) { registerGuidanceTools(s, h) }),
		newFlowModule(d.cfg.flowSvc, d.backlogEnabled),
		toolModuleFunc(func(s *mcpserver.MCPServer) {
			registerDiagnoseTools(s, &diagnoseHandlers{storage: d.storage, live: liveFinder, eventBus: d.eventBus})
		}),
	}
}

func newClaimModule(backlogSvc *backlogsvc.BacklogService, enabled func() bool) ToolModule {
	if backlogSvc == nil {
		return nil
	}
	return toolModuleFunc(func(s *mcpserver.MCPServer) {
		registerClaimTools(s, &claimHandlers{svc: backlogSvc, enabledCheck: enabled})
	})
}

// newFlowModule always registers: a nil flowSvc makes the tools answer ErrUnavailable.
func newFlowModule(flowSvc *backlogflow.BacklogFlowService, enabled func() bool) ToolModule {
	return toolModuleFunc(func(s *mcpserver.MCPServer) {
		registerFlowTools(s, &flowHandlers{svc: flowSvc, enabledCheck: enabled})
	})
}

// newIdentityGatedModules returns the wake, peer-help and owner-action modules,
// which share one identity gate and (peer-help, owner-action) one handler set.
func newIdentityGatedModules(storage *session.Storage, bus *events.EventBus) []ToolModule {
	if storage == nil {
		return nil
	}
	gate := &callerIdentityGate{
		metrics: wakes.NewMetrics(),
		roleOf:  roleLookupFrom(storage.GetItemSessionBySessionUUID),
	}
	peerHelp := newPeerHelpHandlers(gate, storage, bus)
	return []ToolModule{
		toolModuleFunc(func(s *mcpserver.MCPServer) { registerWakeTools(s, newWakeHandlers(gate)) }),
		toolModuleFunc(func(s *mcpserver.MCPServer) { registerPeerHelpTools(s, peerHelp) }),
		toolModuleFunc(func(s *mcpserver.MCPServer) { registerOwnerActionTools(s, peerHelp) }),
	}
}

func newGitHubModule(prCache *githubpkg.UserPRCache, store session.InstanceStore, svc *services.SessionService) ToolModule {
	if prCache == nil {
		return nil
	}
	return toolModuleFunc(func(s *mcpserver.MCPServer) {
		registerGitHubTools(s, &githubHandlers{cache: prCache, store: store, svc: svc})
	})
}
