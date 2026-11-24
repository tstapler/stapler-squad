package featureflags

import (
	"github.com/tstapler/stapler-squad/config"
)

// sddDefaultPipelineFlagName is shared between the flag registry (feature_flag_specs.go) below and
// CreateBacklogItem's default-resolution branch (backlog_service_lifecycle.go)
// so the flag name can't drift between where it's declared and where it's
// read.
const SddDefaultPipelineFlagName = "backlog:sdd-default-pipeline"

// workspacePeersNudgeFlagName is shared between the flag registry (feature_flag_specs.go) below and
// workspacePeersBlockFor below so the flag name can't drift between where it's declared and
// where it's read.
const WorkspacePeersNudgeFlagName = "session:workspace-peers-nudge"

// CrossHostClaimDedupFlagName gates every cross-host claim check. Off by
// default: with it off, creation and dequeue behave exactly as they did before
// the claim substrate existed. Claim recording (Storage.CreateBacklogItem) is
// always on.
const CrossHostClaimDedupFlagName = "cross_host_claim_dedup"

// backlogFlagName gates every BacklogService RPC (server.go's feature-flag interceptor).
const BacklogFlagName = "backlog"

// terminalResyncCorrelationIDFlagName is shared between the flag registry (feature_flag_specs.go) below and
// handleCurrentPaneRequest's resync_id echo (connectrpc_websocket.go) so the flag name
// can't drift between where it's declared and where it's read.
const TerminalResyncCorrelationIDFlagName = "terminal:resync-correlation-id"

// terminalResyncSkipStaleDimensionSlowpathFlagName is shared between the flag registry (feature_flag_specs.go)
// below and handleCurrentPaneRequest's stale-dimension skip branch (connectrpc_websocket.go)
// so the flag name can't drift between where it's declared and where it's read.
const TerminalResyncSkipStaleDimensionSlowpathFlagName = "terminal:resync-skip-stale-dimension-slowpath"

// terminalResyncExecGateFastLaneFlagName is shared between the flag registry (feature_flag_specs.go) below and
// currentResyncOptions' UseFastLane field (wsconn/connectrpc_websocket.go) so the flag name can't
// drift between where it's declared and where it's read. session/instance_tmux.go duplicates
// this literal (as terminalResyncExecGateFastLaneFlagName there too) rather than importing it,
// since session cannot import server/services without creating an import cycle — keep both
// in sync if this flag is ever renamed.
const TerminalResyncExecGateFastLaneFlagName = "terminal:resync-exec-gate-fast-lane"

// terminalResyncCompressionFlagName is shared between the flag registry (feature_flag_specs.go) below and
// writeCurrentPaneResponse's envelope-compression branch (connectrpc_websocket.go) so the
// flag name can't drift between where it's declared and where it's read.
const TerminalResyncCompressionFlagName = "terminal:resync-compression"

// terminalResyncVisibilityScopeFlagName, terminalResyncStaggerFlagName, and
// terminalResyncBatchingFlagName are pure client-side concerns (see
// connectrpc_websocket_test.go's allTerminalResyncFlagNames doc comment) with no Go
// production call site to share a constant with — they're named here purely for
// consistency with the other four terminal:resync-* flags above, and so
// the flag registry (feature_flag_specs.go)/allTerminalResyncFlagNames reference one declaration instead of a
// raw string literal.
const TerminalResyncVisibilityScopeFlagName = "terminal:resync-visibility-scope"

const TerminalResyncStaggerFlagName = "terminal:resync-stagger"

const TerminalResyncBatchingFlagName = "terminal:resync-batching"

// terminalAppScrollForwardingClaudeFlagName mirrors
// config.FeatureAppScrollForwardingClaude so the flag registry (feature_flag_specs.go) below doesn't
// duplicate the literal -- see config/config.go for the flag's full
// documentation and
// project_plans/app-scrollback-forwarding/implementation/plan.md, Epic 1.5.
const TerminalAppScrollForwardingClaudeFlagName = config.FeatureAppScrollForwardingClaude
