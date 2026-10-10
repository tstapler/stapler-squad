//go:build sinkguard

package services

// The pinned tables of check (c) (plan Story 5.1d, T-RO-42, T-RO-45, T-RO-46).
// Derived with go/types over ./session ./server ./daemon ./cmd and main.go at the
// PR 5u merge, interface calls resolved by method name and signature. Each row
// needs a kind and a one-line reason; a new caller fails until a reviewer adds
// its row, and a removed caller fails until the stale row is deleted.
//
// Kinds: ui (check (d) applies), acquirer (takes the lease once; the closed
// acquirer list), receiver (takes a *HeldLease, never acquires), chain (an
// entry point whose own callees are pinned below it), lifecycle,
// receiver-via-wrapper (the ratelimit manager's call). No Pause, Delete,
// Hibernate or Stop unit is on the acquirer list.

// pinnedCallers: every non-test unit that calls the primitive set, a chain
// entry point or a lease constructor.
var pinnedCallers = map[string]callerRow{
	"daemon:RunDaemon.func1":                                         {kind: kindLifecycle, reason: "legacy daemon AutoYes TapEnter, gated on AutoYes; a separate process from the web server, so an in-process lease cannot serialize it"},
	"server/mcp:acquireMCPWriteLease":                                {kind: kindAcquirer, reason: "the MCP write tools take the lease here, once per call"},
	"server/mcp:diagnoseHandlers.writeNudge":                         {kind: kindReceiver, reason: "diagnose_nudge_session write under the lease acquireMCPWriteLease took; dispatched agents only"},
	"server/mcp:terminalHandlers.runCommand":                         {kind: kindReceiver, reason: "MCP run_command write under the lease acquireMCPWriteLease took"},
	"server/mcp:terminalHandlers.sendControl.func1":                  {kind: kindReceiver, reason: "MCP send_control writing goroutine; it captures the lease and releases it"},
	"server/mcp:terminalHandlers.steerSession":                       {kind: kindReceiver, reason: "MCP steer_session write under the lease acquireMCPWriteLease took"},
	"server/mcp:terminalHandlers.writeToSession":                     {kind: kindReceiver, reason: "MCP write_to_session under the lease acquireMCPWriteLease took"},
	"server/services:BacklogService.steerActiveSessionForPRFix":      {kind: kindChain, reason: "PR-fix steer of the item's active work session; refuses a hidden target (Task 5.1d-d, T-RO-39)"},
	"server/services:BacklogService.steerWorkSessionWithVerdict":     {kind: kindChain, reason: "verdict steer of the item's live work session; refuses a hidden target (Task 5.1d-d, T-RO-39)"},
	"server/services:BacklogSteerWriter.Steer":                       {kind: kindChain, reason: "the O7 typed writer; only obtainable with a BacklogReviewLink"},
	"server/services:GitHubUserService.deliver":                      {kind: kindChain, reason: "NudgeSessionForPR delivery through SteerInstanceGuarded; refuses a hidden target in resolveSession"},
	"server/services:NewCapacityMonitor.func1":                       {kind: kindAcquirer, reason: "default compactor: takes the lease once for the /compact write"},
	"server/services:SessionService.SteerActiveSession":              {kind: kindChain, reason: "internal steer of a live session by UUID; callers are the pinned backlog steers"},
	"server/services:SessionService.SteerInstanceGuarded":            {kind: kindChain, reason: "guarded steer behind the nudge guard; writes through steerInternal"},
	"server/services:SessionService.SteerSessionGuarded":             {kind: kindChain, reason: "UUID-only wrapper of SteerInstanceGuarded for the PR-fix auto-steer"},
	"server/services:SessionService.StreamTerminal.func4":            {kind: kindUI, reason: "StreamTerminal input of a visible session; dropped for a hidden one (Story 5.1)"},
	"server/services:SessionService.runBypassSteer":                  {kind: kindChain, reason: "guards-off steer of a non-qualifying hidden target: guard_bypass line first, then steerUnderLease"},
	"server/services:SessionService.runSteer":                        {kind: kindChain, reason: "dispatches UpdateSession's steer to the visible acquirer, the typed O7 writer or the audited bypass"},
	"server/services:SessionService.steerAuthorized":                 {kind: kindReceiver, reason: "the verified body of the steer chain; never acquires, releases the lease it was given"},
	"server/services:SessionService.steerAuthorized.func1":           {kind: kindReceiver, reason: "autonomous steer writing goroutine; releases the lease"},
	"server/services:SessionService.steerBacklogLinked":              {kind: kindChain, reason: "builds the backlog_link token for steerAuthorized (the one file that constructs tokens)"},
	"server/services:SessionService.steerHiddenReviewViaBacklogLink": {kind: kindAcquirer, reason: "the O7 chain's acquirer: audit line first, then the lease, then steerAuthorized"},
	"server/services:SessionService.steerInternal":                   {kind: kindAcquirer, reason: "acquirer for in-process steers; builds the internal token"},
	"server/services:SessionService.steerUnderLease":                 {kind: kindAcquirer, reason: "acquirer for UpdateSession's visible steer"},
	"server/services:TerminalService.WriteToSession":                 {kind: kindUI, acquires: true, reason: "UI raw input; refused for a hidden target (Story 5.2); also its own acquirer"},
	"session/detection/ratelimit:Manager.sendRecoveryInput":          {kind: kindReceiverViaWrapper, reason: "the lease is taken by the session-package wrapper that implements SessionAccessor, so this call is lease-held by construction"},
	"session:AssertLeaseFree":                                        {kind: kindAcquirer, reason: "test helper in a non-test file: takes and releases the lease to assert none leaked"},
	"session:AutonomousDriver.run":                                   {kind: kindAcquirer, reason: "automated writer: takes the lease once per turn"},
	"session:Instance.Restart":                                       {kind: kindLifecycle, reason: "restart marker typed into the pane Restart just started; reached only from guarded or internal entries (T-RO-46)"},
	"session:Instance.changeDirectory":                               {kind: kindLifecycle, reason: "cd typed for a directory switch; reached only from SwitchWorkspace, guarded by Story 5.2"},
	"session:attemptBacklogNudge":                                    {kind: kindAcquirer, reason: "session driver idle nudge: takes the lease once"},
	"session:leasedSessionAccessor.WriteToPTY":                       {kind: kindAcquirer, reason: "the wrapper that implements ratelimit.SessionAccessor: takes the lease, writes, releases"},
	"session:sendAnswerKeyUnderLease":                                {kind: kindAcquirer, reason: "session driver answer key: takes the lease once"},
	"session:sendInitialPromptTick":                                  {kind: kindAcquirer, reason: "session driver initial prompt: takes the lease once, before the attempt counter"},
	"session:submitInitialPrompt.func1":                              {kind: kindReceiver, reason: "initial-prompt submit under the lease sendInitialPromptTick took (a variable only so a test can force a lease error)"},
	"session:switchWorkspaceLocked":                                  {kind: kindLifecycle, reason: "directory switch under the workspace lock; reached only from SwitchWorkspace, guarded by Story 5.2"},
}

// pinnedLifecycleCallers: every unit that calls Instance.Restart, SwitchProgram,
// SetAutoApprove or changeDirectory.
var pinnedLifecycleCallers = map[string]callerRow{
	"server/services:SessionService.RestartSession":       {kind: kindLifecycle, reason: "UI restart; refused for a hidden target"},
	"server/services:SessionService.UpdateSession":        {kind: kindLifecycle, reason: "program and auto_approve fields; refused for a hidden target (ADV-N28)"},
	"server/services:SessionService.UpdateSessionProgram": {kind: kindLifecycle, reason: "capacity guardrail's own CLI fallback; not UI-reachable"},
	"session:Instance.SetAutoApprove":                     {kind: kindLifecycle, reason: "restarts an Active pane; reached from UpdateSession only"},
	"session:Instance.SwitchProgram":                      {kind: kindLifecycle, reason: "restarts an Active pane; reached from UpdateSession and UpdateSessionProgram"},
	"session:restartForRetry":                             {kind: kindLifecycle, reason: "internal retry (Restart(false)): no marker"},
	"session:switchWorkspaceLocked":                       {kind: kindLifecycle, reason: "directory switch; reached only from SwitchWorkspace"},
}

// pinnedAccessCallers: the only units that may call AccessForUnary: the Story
// 5.2 unary handlers and their access-decision helpers, never a stream site.
var pinnedAccessCallers = map[string]callerRow{
	"server/services:SessionService.RestartSession":     {kind: kindUI, reason: "Story 5.2 unary handler"},
	"server/services:SessionService.RestartShell":       {kind: kindUI, reason: "Story 5.2 unary handler"},
	"server/services:SessionService.decideSteerAccess":  {kind: kindUI, reason: "steer access decision: the guards-off bypass of a non-qualifying hidden target"},
	"server/services:SessionService.decideUpdateAccess": {kind: kindUI, reason: "UpdateSession access-decision block"},
	"server/services:SessionService.runSteer":           {kind: kindUI, reason: "UpdateSession steer dispatch"},
	"server/services:TerminalService.WriteToSession":    {kind: kindUI, reason: "Story 5.2 unary handler"},
	"server/services:WorkspaceService.SwitchWorkspace":  {kind: kindUI, reason: "Story 5.2 unary handler"},
}
