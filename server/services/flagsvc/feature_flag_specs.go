package flagsvc

import (
	"slices"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session/paneactivity"
)

// coreFlagSpecs are the flags whose name constants live in package services. A flag owned by a leaf package
// declares its own featureflags.Spec there and the composition root (server/dependencies.go) appends it.
// Spec.Default is what GetFeatureFlags reports and what the call site resolves to when never persisted.
var coreFlagSpecTable = []featureflags.Spec{
	{
		Name:        ProgramCLIFlagProbeFlagName,
		Description: "Check that a program exists on the server and read its --help flags (Program Config and session creation). Turn off to disable the ProbeProgram RPC immediately. Default: on.",
		Default:     true,
	},
	{
		Name:        featureflags.GithubIssueProgressCommentsFlagName,
		Description: "Comment on the linked GitHub issue when a backlog item starts work (in_progress, naming the host) and when it opens a PR (review). The done-transition close comment is not gated by this flag. Default: off.",
	},
	{
		Name:        featureflags.ChangelogFlagName,
		Description: "Built-in changelog: the /changelog page, GET /api/changelog, and the update badge. Turn off to hide all three. Default: on.",
		Default:     true,
	},
	{
		Name:        featureflags.UpdateCheckFlagName,
		Description: "Update check: poll GitHub Releases (ETag-conditional, at most every 6 hours) and show a badge when a newer release exists. Contacts api.github.com, so it is opt-in. Never runs on source builds. Requires the changelog flag. Default: off.",
	},
	{
		Name:        "backlog",
		Description: "Backlog management with external sync sources and AI-driven triage",
	},
	{
		Name:        "browser-passthrough",
		Description: "Browser passthrough: stream Chrome/Chromium via CDP in the Browser tab",
	},
	{
		Name:        "backlog:conversation-view",
		Description: "Show JSONL conversation messages in the session monitor (default: terminal scrollback view)",
	},
	{
		Name:        "unfinished:mmap-index",
		Description: "Use the mmap-backed pack-index loader for the /unfinished scanner's git storage (session/unfinished/gogitstore) instead of the copy-based loader. See session/unfinished/design/mmap-activation-runbook.md before enabling.",
	},
	{
		Name:        featureflags.SddDefaultPipelineFlagName,
		Description: "New backlog items with no explicitly chosen pipeline mode default to the 'sdd' pipeline mode (research, plan, validate, implement, and an adversarial verify pass before review) instead of the flat default pipeline. Never affects existing items or an item with any explicit pipeline_mode value, including an explicit empty one.",
	},
	{
		Name:        "backlog-triage-results-panel",
		Description: "Show a formatted 'Triage Results' panel (priority, tasks with estimates, suggestions, raw-JSON toggle) on the backlog item detail view. Default: off.",
	},
	{
		Name:        "backlog-review-results-panel",
		Description: "Show a formatted 'Review Results' panel (overall outcome, per-criterion rows with evidence, raw-JSON toggle) on the backlog item detail view. Default: off.",
	},
	{
		Name:        BlockApprovalOnCIFailureFlagName,
		Description: "Block manual Approve when the session's branch has failing GitHub CI. Shows a visible inline explanation instead of a silent no-op; a reviewer can still bypass it per-approval via 'Approve anyway' (audited). Sessions with no associated PR are unaffected. Default: off.",
	},
	{
		Name:        featureflags.WorkspacePeersNudgeFlagName,
		Description: "Auto-inject an 'Other Active Sessions In This Workspace' nudge into every new session's initial prompt. Off by default — use the list_workspace_peers MCP tool on demand instead. Default: off.",
	},
	{
		Name:        featureflags.CompletionReviewerFlagName,
		Description: "Background completion reviewer: when a backlog item reaches done, run a tool-less one-shot model call that distills learnings for future triage/work/review sessions and appends them to a per-repo memory file under the config dir. Failures are logged, never surfaced. Default: off.",
	},
	{
		Name:        featureflags.CrossHostClaimDedupFlagName,
		Description: "Cross-host duplicate-work prevention: before importing a GitHub issue or creating an item with an external URL, and before dequeuing a queued item, check whether another stapler-squad host already claimed that URL. A blocked import offers 'Import anyway' with an audited reason; a blocked dequeue shows up as a stuck item with an override. Claims are always recorded regardless. Default: off.",
	},
	{
		Name:        handoffSummaryFlagName,
		Description: "Restart-with-summary: generate an AI handoff summary and restart into a fresh session. Read-only here -- the real toggle is config.json's handoff_summary.enabled key. Default: on.",
	},
	{
		Name:        featureflags.TerminalResyncVisibilityScopeFlagName,
		Description: "Scope terminal resync-on-visibility-change to only the terminal instance actually in the foreground, instead of every mounted terminal. Applies to newly-focused terminals only — already-open tabs need a reload to pick up the change. Default: off.",
	},
	{
		Name:        featureflags.TerminalResyncCorrelationIDFlagName,
		Description: "Tag each terminal resync request/reply pair with a correlation ID so a stale reply from an earlier resync can't be misapplied to a later one. Not live-updated on already-open tabs. Default: off.",
	},
	{
		Name:        featureflags.TerminalResyncSkipStaleDimensionSlowpathFlagName,
		Description: "Skip the stale-dimension slow path for backgrounded terminals during resync, avoiding unnecessary pane-size recalculation for terminals not currently visible. Not live-updated on already-open tabs. Default: off.",
	},
	{
		Name:        featureflags.TerminalResyncExecGateFastLaneFlagName,
		Description: "Route resync's tmux subprocess calls through a dedicated fast-lane slot pool (see TmuxExecGateConfig.ResyncFastLaneSlots) instead of contending with other tmux exec traffic for the shared gate. Default: on (2026-08-25) — without it, a resync-triggered resize's sequential subprocess calls can exceed the client's 4s stall watchdog under exec-gate contention, forcing an unnecessary disconnect+reconnect. Persist an explicit false to opt back out.",
		Default:     true,
	},
	{
		Name:        featureflags.TerminalResyncStaggerFlagName,
		Description: "Stagger resync bursts across multiple terminals instead of firing them all simultaneously, reducing thundering-herd load on the tmux server. Default: off.",
	},
	{
		Name:        featureflags.TerminalResyncCompressionFlagName,
		Description: "Compress terminal resync payloads on the wire to reduce bandwidth for large scrollback resyncs. Default: off.",
	},
	{
		Name:        featureflags.TerminalResyncBatchingFlagName,
		Description: "Batch multiple terminals' resync requests into a single round trip instead of issuing one request per terminal. Default: off.",
	},
	{
		Name:        GithubPriorityAdmissionFlagName,
		Description: "Priority-aware admission control for outbound GitHub API calls: background pollers/sync back off once a resource's (core/search/graphql) remaining quota drops below a reserved headroom, so interactive GitHub actions (merge, comment, refresh) keep succeeding. Protects this stapler-squad instance's own interactive GitHub calls only — does not coordinate with other machines sharing the same GitHub token. Default: off.",
	},
	{
		Name:        GithubGraphQLMigrationFlagName,
		Description: "Route GetPRInfoCtx's PR-metadata fetch through a single native GraphQL request instead of the `gh pr view` CLI shell-out, cutting the REST-equivalent call count for PR status refreshes. Default: off.",
	},
	{
		Name:        config.TCCEnrichmentConsentFlag,
		Description: "Explain protected-root metadata access and require app consent before background worktree enrichment on macOS. Does not gate other filesystem access or represent OS grants. Default: on on Darwin, no-op elsewhere. Restart required.",
		Default:     config.DefaultTCCEnrichmentConsent,
	},
	{
		Name:        WorktreeChangeDetectionFlagName,
		Description: "Watch each session's .git dir via fsnotify and run a staggered 15s periodic cheap dirty/HEAD check to invalidate the diff-stats and VCS-status caches, letting both widen from a 15s to a 5-minute TTL. Applies to newly-created worktrees only; already-open sessions keep today's 15s pure-TTL behavior until restarted. Default: off.",
	},
	{
		Name:        featureflags.TerminalAppScrollForwardingClaudeFlagName,
		Description: "Forward Claude Code's own PageUp scroll keybinding into its fullscreen conversation view instead of tmux-native scrollback capture, for eligible sessions (AppScrollGate: adapter coverage, alt-screen active, idle status, exactly one connected viewer). Flag-off always falls through to the unchanged tmux-native scrollback path, regardless of AppScrollGate's verdict. Default: off.",
	},
	{
		Name:        piSupportFlagName,
		Description: "pi coding agent support: program picker entry, resume across restarts, and approval-rule enforcement parity with Claude Code. Default: off. Disabling does not remove an already-installed global pi approval extension — see the settings UI warning.",
	},
	{
		Name:        paneactivity.FeatureFlagDriverPaneActivityGate,
		Description: "Session driver: skip the per-tick capture-pane when the pane has produced no output since the last capture (one shared list-panes snapshot for all sessions instead of one capture-pane per session per 2 s tick). Kill switch: turn off to capture every tick as before. Default: on.",
		Default:     config.DriverPaneActivityGateDefault,
	},
	{
		Name:        "request_coalescing",
		Description: "Web client request coalescing: merge duplicate in-flight unary RPCs and batch polling reads into fewer requests. Kill switch: turn off to restore one request per call. Read by the web client; applies on next page load. Default: on.",
		Default:     true,
	},
	{
		Name:        "unary_over_ws",
		Description: "Send unary RPCs over the already-open WebSocket instead of separate HTTP requests, avoiding the browser's per-origin connection cap. Kill switch: turn off to fall back to HTTP fetch. Read by the web client; applies on next page load. Default: off.",
	},
	{
		Name:        "unary_over_ws_remote",
		Description: "also use WebSocket unary on remote-access (https) origins; requires unary_over_ws",
	},
	{
		Name:        "terminal_ws_pool",
		Description: "Carry terminal streams over a small shared pool of multiplexed WebSockets (/api/ws/terminal-mux) instead of one WebSocket per terminal, so a page with many terminals opens a few sockets instead of one each. Kill switch: turn off to restore one WebSocket per terminal. Pool size: server STAPLER_SQUAD_TERMINAL_WS_POOL_SIZE (default 2) or per-browser localStorage ssq:terminal-pool-size. Read by the web client; applies on next page load. Default: off.",
	},
	{
		Name:        "batch_unary",
		Description: "Send independent cheap read RPCs issued in the same moment as one batched HTTP request (POST /api/batch/unary), freeing browser connection slots; each call still runs the full auth and guard chain, and any batch failure falls back to individual calls. Not used for calls that already travel over the unary WebSocket. Kill switch: turn off to send one request per call. Read by the web client; applies on next page load. Default: off.",
	},
	{
		Name:        "history_search_streaming",
		Description: "History page full-text search uses the server-streaming StreamSearchClaudeHistory RPC and renders result batches as they arrive, instead of one unary SearchClaudeHistory call. Kill switch: turn off to fall back to the unary call. Read by the web client; applies on next page load. Default: off.",
	},
	{
		Name:        "diagnose-session-panel",
		Description: "Show the rich Diagnose & Nudge view on the backlog item detail page: a structured result panel for each diagnose session and dismissible workflow-suggestion cards. Read by the web client. Default: off.",
	},
	{
		Name:        "ghostty_renderer",
		Description: "Render terminals with ghostty-web (Ghostty's VT parser compiled to WASM, drawn on a canvas) instead of xterm.js, for better VT conformance (synchronized output, REP, DECALN, Kitty keyboard). Kill switch: turn off to fall back to xterm.js. Read by the web client; applies on next page load, no rebuild. Default: off.",
	},
	{
		Name: "mobile_input_layer",
		// Task 5.1.4c (project_plans/mobile-input-layer) can flip this to false before merge if the phone gate fails;
		// web-app MOBILE_INPUT_LAYER_DEFAULT must change with it (a jest test asserts they match).
		Default:     true,
		Description: "Phone keyboard input layer: a hidden textarea feeds the terminal so Android soft-keyboard typing, autocorrect and suggestions work. Affects only Android touch-primary devices; desktop and iOS are unchanged whatever this says. Kill switch: turn off to fall back to the plain terminal (ghostty is then not used on Android phones). Read by the web client; the off value is saved on one page load and applies on the next, or open the app with ?mobile_input=0 to apply at once. Default: on.",
	},
	{
		Name:        "mobile_input_queue",
		Description: "Mobile input layer sub-feature: queue phone keystrokes while the connection is down and send them on reconnect. Needs mobile_input_layer. Read by the web client; applies on next page load. Default: off.",
	},
	{
		Name:        "mobile_input_diagnostics",
		Description: "Mobile input layer sub-feature: the full input diagnostics sheet. Needs mobile_input_layer. Read by the web client; applies on next page load. Default: off.",
	},
	{
		Name:        "mobile_input_compose",
		Description: "Mobile input layer sub-feature: the compose box for longer phone input. Needs mobile_input_layer. Read by the web client; applies on next page load. Default: off.",
	},
	{
		Name:        "mobile_input_edit_context",
		Description: "Mobile input layer sub-feature: use the EditContext API instead of the hidden textarea where the browser supports it. Needs mobile_input_layer. Read by the web client; applies on next page load. Default: off.",
	},
	{
		Name:        "window_keep_alive",
		Description: "Keep the web app's window state alive across route changes instead of remounting it. Kill switch: turn off to restore remount-on-navigate. Read by the web client; applies on next page load. Default: off.",
	},
	{
		Name:        config.TriageGuidanceHaltFeatureFlag,
		Default:     config.TriageGuidanceHaltDefault,
		Description: "Automated triage halts and asks a durable guidance request instead of guessing when a backlog item is genuinely ambiguous. Default: off — baseline guess-and-proceed triage behavior is unchanged until enabled.",
	},
	{
		Name:        config.DiagnoseNudgeFeatureFlag,
		Default:     config.DiagnoseNudgeDefault,
		Description: "Diagnose & Nudge: allow a dispatched diagnostic agent to autonomously send a redirect message (diagnose_nudge_session) to a linked stuck session, and the autonomous stall path to type its in-process nudge (also needs autonomous_stall_nudge_enabled). Read fresh at the write instant, so flipping this off blocks an already-dispatched agent too. Default: off — until enabled, a dispatch can still investigate and file a bug or post a note, but never nudge.",
	},
	{
		Name:        config.AutonomousStallNudgeFeatureFlag,
		Default:     config.AutonomousStallNudgeDefault,
		Description: "Autonomous stall nudge: when an autonomous work session stops while its pane sits idle at a prompt, type one fixed reminder into the pane and resume supervision instead of respawning it. Also requires Diagnose & Nudge to be on (that flag is the kill switch for every autonomous pane write). Read fresh at each stop. Default: off — every stall respawns as before.",
	},
	{
		Name:        config.EscapeEventsDBMigrateFeatureFlag,
		Default:     config.EscapeEventsDBMigrateDefault,
		Description: "Copy existing escape_events rows from analytics.db into the dedicated db/escape_events.db in small background batches after startup (resumable; the old table is left in place). Turn off to skip the copy: the new database then starts empty. Read once at startup. Default: on.",
	},
	{
		Name:        config.HiddenSessionGateFeatureFlag,
		Default:     config.HiddenSessionGateDefault,
		Description: "Hidden-session delivery gate: a hidden session (review, diagnose) notifies only for failures and needs-human events; routine completions are dropped on every channel. Turning it off (globally or per kind) is the rollback: hidden sessions then deliver everything and the gate only counts what it would have suppressed (see the status line). Can be enabled only while the stats writer runs, so the evidence is recorded. Can be overridden per hidden-session kind (review, diagnose, other). Default: on.",
		Scopes:      gateFlagScopes(),
	},
	{
		Name:        featureflags.TerminalWriteLeaseFlagName,
		Description: "Per-session terminal write lease: the driver's prompt and answer keys, steers, nudges, MCP writes and rate-limit recovery take turns writing to a session's terminal instead of interleaving. Turn off only to restore the pre-lease behavior if the lease misbehaves (every write then proceeds unserialized). Applies at once; global only. Default: on.",
		Default:     true,
	},
	{
		Name:        featureflags.HiddenSessionReplyFlagName,
		Description: "Hidden-session Reply: answer a background session's single-select question from the toast, tray or read-only view by writing one option digit to its terminal. The kill switch for the Reply path only; it does not affect the write guards. Every reply is audited before it is written. Applies at once; global only. Default: on.",
		Default:     true,
	},
	{
		Name:        BackgroundActivityFeedFlagName,
		Description: "Background tab activity feed: show what each background session actually did (finished, stopped, waiting) instead of the history-only selector. Turn off to restore the legacy selector with no data impact. Turning it back on after an explicit off is audit-fsynced and refused while the audit sink is down. Applies at once; global only. Default: on.",
		Default:     backgroundActivityFeedDefault,
	},
	{
		Name:        NotificationInlineActionsFlagName,
		Description: "Inline notification actions: approve, deny, answer and decide from the notification row. Server-checked on every invocation; off reduces actions to Open session and keeps the legacy approve/deny path. Scopes: channel:remote, channel:remote:tierB, action:<kind>. Applies at once. Default: off.",
		Scopes:      notificationInlineActionScopes(),
	},
	{
		Name:        AskUserQuestionStructuredFlagName,
		Description: "Structured AskUserQuestion: render, answer and hook-reply stages for durable question rows. A stage's new writes are refused when it resolves off; existing rows stay visible. Scopes: stage:render, stage:answer, stage:hook_reply (each also needs the stage before it). Applies at once. Default: off.",
		Scopes:      askUserQuestionStructuredScopes(),
	},
	{
		Name:        AgentWakeTriggersFlagName,
		Description: "Agent wake triggers: the dispatcher and the schedule_wake, list_wakes, cancel_wake and request_help tools. Off stops the dispatcher and the tools return flag_off; rows are kept. Scopes: trigger:<type>, tool:<name>; a scoped true runs the dispatcher for that type only. Applies at once. Default: off.",
		Scopes:      agentWakeTriggerScopes(),
	},
	{
		Name:        config.SessionMultiPRFlag,
		Description: "Session multi-PR tracking: per-PR state, the active PR and the item PR set. Scopes reader:poller, reader:backlog, reader:actions, reader:ui switch the readers on; enable them in that order (a later reader stays off until the earlier ones are on, applied at read time). The scope refresher_off is the kill switch of the always-on state refresher (default: refresher runs). Applies at once. Default: off.",
		Scopes:      config.SessionMultiPRScopes(),
	},
	{
		Name:        config.GoalModeFlag,
		Description: "Goal mode pilot: run opted-in chore/docs backlog items through Claude Code /goal as a direct subprocess (not visible in the terminal UI) with a cost ceiling and heartbeat, instead of the autonomous driver. Off leaves the driver untouched. Global only. Default: off.",
	},
	{
		Name:        config.ArtifactScanV2Flag,
		Description: "Artifact scan v2: the rescan that rebuilds every session's artifact blob with typed records. Off until the owner flips it after a dry run on a database copy; turning it off stops further rescans. Applies at once; global only. Default: off.",
	},
	{
		Name:        featureflags.HiddenSessionReadonlyGuardsFlagName,
		Description: "Hidden-session write guards: the UI's write RPCs (terminal input, steer, restart, workspace switch, program and auto-approve changes) refuse a hidden (background) session, except the backlog Steer of a live review session, which stays audited. Turn off only to restore the previous behavior if a guard blocks a legitimate action; every write allowed while off is recorded in the audit log. Applies at once; global only. Default: on.",
		Default:     true,
	},
	{
		Name:        config.WorkflowEngineShadowFeatureFlag,
		Description: "Workflow engine shadow: compare the configured stage graph and gates against today's answer on every status transition and count the differences (see /actuator/workflow-engine). Changes nothing about what is allowed. Applies within about a second. Default: on.",
		Default:     config.WorkflowEngineShadowDefault,
	},
	{
		Name:        config.WorkflowEngineEnforceFeatureFlag,
		Description: "Workflow engine enforce (operator class): the configured stage graph and operator gates decide status transitions instead of the built-in rules. Turn on only after make workflow-engine-soak-status reads READY. Applies within about a second; global only. Default: off.",
		Default:     config.WorkflowEngineEnforceDefault,
	},
	{
		Name:        config.WorkflowEngineEnforceAgentFeatureFlag,
		Description: "Workflow engine enforce (agent class): the configured stage graph and operator gates decide status transitions requested by agent tools (request_review, report_blocked, resume_work, report_duplicate, report_pr_created, and the generic transition tool) instead of today's checks. A denial is reported as a transition_denied stuck item. Turn on only after the agent class reads READY in make workflow-engine-soak-status. Applies within about a second; global only. Default: off.",
		Default:     config.WorkflowEngineEnforceAgentDefault,
	},
	{
		Name:        config.WorkflowEngineEnforceSystemFeatureFlag,
		Description: "Workflow engine enforce (system class): the configured stage graph and operator gates decide status transitions made by the server itself (queue claim, triage completion, lifecycle sweeps, PR merge and archive, dispatch) instead of today's unchecked writes. A denial is reported once as a transition_denied stuck item, the held automation waits quietly, and the sweep finishes the transition once the gate is satisfied. Turn on only after the system class reads READY in make workflow-engine-soak-status. Applies within about a second; global only. Default: off.",
		Default:     config.WorkflowEngineEnforceSystemDefault,
	},
	{
		Name:        FilesEditingFlagName,
		Description: "Files editing: allow saving edits from the Files tab (WriteFile). Enforced server-side; hidden-session and archived-session guards still apply. Applies at once; global only. Default: off.",
	},
	{
		Name:        FilesViewerV2FlagName,
		Description: "Files viewer v2: the revamped Files tab layout and content search. Turn off to fall back to the legacy layout at the next mount. Applies at once; global only. Default: on.",
		Default:     true,
	},
	{
		Name:        FilesMergePhoneFlagName,
		Description: "Files phone merge: the conflict-merge UI on phone-sized screens. Turn off to fall back to keep-mine/keep-theirs only. Applies at once; global only. Default: on.",
		Default:     true,
	},
	{
		Name:        FilesRangeReadFlagName,
		Description: "Files ranged reads: serve large files in byte ranges (ADR-036). Applies at once; global only. Default: off.",
	},
	{
		Name:        FilesDeltaWriteFlagName,
		Description: "Files delta writes: save large-file edits as patches (ADR-036). Applies at once; global only. Default: off.",
	},
	{
		Name:        FilesEditLargeFlagName,
		Description: "Files large-file editing: allow editing files over the 1 MiB cap through ranged reads and delta writes (ADR-036). Applies at once; global only. Default: off.",
	},
	{
		Name:        AttentionProseQuestionDetectionFlagName,
		Description: "Attention prose-question detection: treat a turn that ends in a plain-prose question as needing you (an Asked a question item). Applies at once; global only. Default: off.",
	},
	{
		Name:        SessionEventLogFlagName,
		Description: "Session event log: record per-session attention events (turn ends, state changes) for the timeline. Applies at once; global only. Default: off.",
	},
	{
		Name:        AttentionAckBypassFixFlagName,
		Description: "Attention ack-bypass fix: an acknowledged item no longer reappears or hides a newer question because of unrelated activity. Applies at once; global only. Default: off.",
	},
	{
		Name:        AttentionIdleClockOutputFlagName,
		Description: "Attention idle clock from output: measure idleness and waiting-for-agent time from the last meaningful output instead of any touch. Applies at once; global only. Default: off.",
	},
	{
		Name:        AttentionProseQuestionLLMTiebreakFlagName,
		Description: "Attention prose-question LLM tiebreak: ask a model to settle borderline prose-question detections. Needs prose-question detection on. Applies at once; global only. Default: off.",
	},
	{
		Name:        AttentionInboxConsolidationFlagName,
		Description: "Attention inbox consolidation: show the unified Attention list in place of the separate review queue views. Applies at once; global only. Default: off.",
	},
}

// coreFlagSpecs returns a copy so callers cannot reorder or edit the table.
func CoreFlagSpecs() []featureflags.Spec { return slices.Clone(coreFlagSpecTable) }
