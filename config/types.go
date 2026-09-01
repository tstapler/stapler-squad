package config

import (
	"slices"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// NotificationPrefs holds the user's notification delivery preferences.
type NotificationPrefs struct {
	// PushEnabled controls whether web push notifications are sent.
	// Default is false (opt-in).
	PushEnabled bool `json:"push_enabled"`
}

// CallbackConfig holds the global singleton outbound-callback URLs fired by
// server/services.CallbackDispatcher on the three lifecycle events FR7
// covers. Never echoed back in plaintext by any RPC (see
// sessionv1.CallbackConfigProto, which reports booleans only) — same
// masked-boolean-not-value shape as the (unimplemented)
// project_plans/slack-review-notifications design, applied fresh here since
// that project has no shipped code to reuse.
type CallbackConfig struct {
	// OnSessionCompleteURL receives a POST when a backlog item transitions to
	// BacklogStatusDone. Empty string means disabled.
	OnSessionCompleteURL string `json:"on_session_complete_url,omitempty"`
	// OnSessionStaleURL receives a POST the first time a work session is
	// detected stale (StuckReasonStaleWork). Empty string means disabled.
	OnSessionStaleURL string `json:"on_session_stale_url,omitempty"`
	// OnQueueItemCreatedURL receives a POST when an item is added to the
	// review queue. Empty string means disabled.
	OnQueueItemCreatedURL string `json:"on_queue_item_created_url,omitempty"`
}

// HibernationConfig holds configuration for the session hibernation feature.
type HibernationConfig struct {
	// Enabled controls whether hibernation is active. Default: true.
	Enabled bool `json:"enabled"`
	// IdleTimeoutMinutes is the number of minutes a session must be idle before
	// the sweeper automatically hibernates it. Default: 20.
	IdleTimeoutMinutes int `json:"idle_timeout_minutes"`
	// ResourcePressureThreshold is the memory usage percentage at which the
	// sweeper begins hibernating idle sessions. Default: 85.
	ResourcePressureThreshold int `json:"resource_pressure_threshold_pct"`
	// CheckpointDir is the directory where hibernation checkpoint data is stored.
	// Default: "~/.stapler-squad/checkpoints". Tilde is expanded at runtime.
	CheckpointDir string `json:"checkpoint_dir"`
	// RetentionDays is the number of days to retain stale checkpoint data.
	// Default: 30.
	RetentionDays int `json:"retention_days"`
}

// SlackConfig holds configuration for the Slack review-queue notification
// feature (Phase 1: notify-only; Phase 2: interactive approval buttons).
//
// WebhookURLEncrypted and SigningSecretEncrypted store ciphertext only, per
// ADR-001 (project_plans/slack-review-notifications/decisions/ADR-001-slack-secret-storage-encryption.md):
// both values are encrypted at rest with Config.GetOrCreateEncryptionKey() +
// session.EncryptToken/DecryptToken, the same primitive already used for
// backlog ItemSource tokens. The config package cannot decrypt them itself
// (it would need to import session, which already imports config); decryption
// happens in server/services, which imports both.
type SlackConfig struct {
	// WebhookURLEncrypted is the AES-256-GCM-encrypted Slack Incoming Webhook
	// URL, or empty if not configured. Never store or log the plaintext value.
	WebhookURLEncrypted string `json:"webhook_url_encrypted,omitempty"`
	// SigningSecretEncrypted is the AES-256-GCM-encrypted Slack app signing
	// secret (Phase 2, used to verify interactive-button callbacks), or empty
	// if not configured. Never store or log the plaintext value.
	SigningSecretEncrypted string `json:"signing_secret_encrypted,omitempty"`
	// NotifyOnQueueItem controls whether a Slack message is sent when an item
	// enters the review queue. Default: false (opt-in).
	NotifyOnQueueItem bool `json:"notify_on_queue_item,omitempty"`
	// QueueDepthThreshold is the review-queue depth at which a digest
	// notification is sent (edge-triggered: one digest per burst). 0 disables
	// depth-based notifications.
	QueueDepthThreshold int `json:"queue_depth_threshold,omitempty"`
	// ApprovalEnabled controls whether outbound Slack messages include
	// interactive allow/deny buttons (Phase 2) and whether the interactive
	// callback route is registered. Default: false.
	ApprovalEnabled bool `json:"approval_enabled,omitempty"`
	// DashboardBaseURL is the base URL used to build "view in dashboard" links
	// in Slack messages. Empty string means links are omitted.
	DashboardBaseURL string `json:"dashboard_base_url,omitempty"`
}

// JulesConfig holds configuration for the Google Jules dispatch-and-poll
// integration (config.Config.Jules). The API key is deliberately absent from
// this struct — it lives in the OS keychain (jules.KeyringTokenSource,
// ADR-003), never in config.json.
type JulesConfig struct {
	// Enabled gates the whole feature: even a resolvable API key and
	// acknowledged repos are refused while this is false (Risk Control's
	// three-part AND gate). Default: false (opt-in).
	Enabled bool `json:"enabled,omitempty"`
	// EgressAcknowledgedRepos is the list of local backlog-item RepoPath
	// values the user has explicitly confirmed may leave the machine for
	// Google's cloud VM. Entries are full local paths, not owner/repo —
	// consent is granted per local checkout, since that is what is actually
	// read from disk and sent. The only writer is ConfirmEgressConsent
	// (Story 2.4.2); DispatchToJules's checkEgressConsent only ever reads
	// this slice (pre-mortem P1 #3).
	EgressAcknowledgedRepos []string `json:"egress_acknowledged_repos,omitempty"`
	// MaxConcurrentJulesSessions caps how many jules_work ItemSessions may be
	// open (not yet ended) at the same time. 0 = use the default (2). Values
	// above maxConcurrentJulesSessionsHardCeiling are clamped to the ceiling.
	MaxConcurrentJulesSessions int `json:"max_concurrent_jules_sessions,omitempty"`
	// MaxJulesSessionsPerDay caps how many jules_work ItemSessions may be
	// *created* in a trailing 24h window, bounding creation rate rather than
	// concurrency (a retry-loop bug that always ends its sessions quickly
	// would sail past a concurrency-only cap). 0 = use the default (15).
	// Values above maxJulesSessionsPerDayHardCeiling are clamped to the
	// ceiling.
	MaxJulesSessionsPerDay int `json:"max_jules_sessions_per_day,omitempty"`
}

// defaultSessionRetentionDays is used by RetentionDaysOrDefault whenever
// RetentionDays is unset (zero), including for configs saved before this field
// existed.
const defaultSessionRetentionDays = 14

// defaultStaleSessionThresholdMinutes is used by ThresholdMinutesOrDefault
// whenever ThresholdMinutes is unset (zero or negative), including for
// configs saved before this field existed.
const defaultStaleSessionThresholdMinutes = 30

// SessionRetentionConfig holds configuration for the automatic session-retention
// cleanup sweep, which deletes archived sessions past a retention window once they
// pass safety checks (clean worktree, no open PR).
type SessionRetentionConfig struct {
	// Enabled controls whether the retention sweep runs. A pointer so a config
	// saved before this field existed (nil) can be distinguished from an explicit
	// `false` — nil defaults to enabled, matching AutoSpawnReadyItems's pattern.
	Enabled *bool `json:"enabled,omitempty"`
	// RetentionDays is how many days after a session is archived before the sweep
	// is eligible to delete it (still subject to safety checks). Default: 14.
	RetentionDays int `json:"retention_days,omitempty"`
}

// EnabledOrDefault returns whether the sweep is enabled, defaulting to true when unset.
func (c SessionRetentionConfig) EnabledOrDefault() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// RetentionDaysOrDefault returns RetentionDays, falling back to
// defaultSessionRetentionDays when unset (<=0).
func (c SessionRetentionConfig) RetentionDaysOrDefault() int {
	if c.RetentionDays <= 0 {
		return defaultSessionRetentionDays
	}
	return c.RetentionDays
}

// StaleSessionConfig holds configuration for stale-session detection: how long a
// session may go without activity before it's flagged stale, and whether that
// triggers a notification.
type StaleSessionConfig struct {
	// ThresholdMinutes is how many minutes of inactivity before a session is
	// considered stale. Default: 30.
	ThresholdMinutes int `json:"threshold_minutes,omitempty"`
	// NotifyEnabled controls whether a notification is sent when a session goes
	// stale. A pointer so a config saved before this field existed (nil) can be
	// distinguished from an explicit `false` — nil defaults to enabled, matching
	// SessionRetentionConfig.Enabled's pattern.
	NotifyEnabled *bool `json:"notify_enabled,omitempty"`
}

// ThresholdMinutesOrDefault returns ThresholdMinutes, falling back to
// defaultStaleSessionThresholdMinutes when unset (<=0).
func (c StaleSessionConfig) ThresholdMinutesOrDefault() int {
	if c.ThresholdMinutes <= 0 {
		return defaultStaleSessionThresholdMinutes
	}
	return c.ThresholdMinutes
}

// NotifyEnabledOrDefault returns whether stale-session notifications are
// enabled, defaulting to true when unset.
func (c StaleSessionConfig) NotifyEnabledOrDefault() bool {
	if c.NotifyEnabled == nil {
		return true
	}
	return *c.NotifyEnabled
}

// defaultRetryMaxAttempts is used by MaxAttemptsOrDefault whenever
// MaxAttempts is unset (<=0), including for configs saved before this field
// existed. 1 preserves today's exact single-retry behavior (AC7).
const defaultRetryMaxAttempts = 1

// defaultRetryMaxDelaySeconds is used by MaxDelaySecondsOrDefault whenever
// MaxDelaySeconds is unset (<=0).
const defaultRetryMaxDelaySeconds = 300

// validRetryOnReasons is the complete vocabulary RetryOnOrDefault accepts;
// used both as the all-three fallback and to filter out typos.
var validRetryOnReasons = []string{"crashed", "stalled", "tmux_exited"} //nolint:gochecknoglobals // read-only vocabulary, never mutated; callers only get defensive copies (see append([]string(nil), ...) below)

// RetryPolicyConfig holds configuration for the automated crash/stall retry
// policy: how many attempts, what backoff, and which failure reasons are
// eligible. Global default in config.json; may be overridden per-session via
// session.Instance.RetryPolicyOverride.
type RetryPolicyConfig struct {
	// Enabled controls whether automated retry runs at all. A pointer so a
	// config saved before this field existed (nil) can be distinguished from
	// an explicit false — nil defaults to enabled, matching
	// SessionRetentionConfig's pattern.
	Enabled *bool `json:"enabled,omitempty"`
	// MaxAttempts is the number of automated retries before the session
	// transitions to PermanentlyFailed. Default: 1 (preserves today's exact
	// single-retry behavior — AC7).
	MaxAttempts int `json:"max_attempts,omitempty"`
	// Backoff selects the backoff strategy. Only "exponential" is implemented;
	// see BackoffOrWarn.
	Backoff string `json:"backoff,omitempty"`
	// InitialDelaySeconds is the backoff formula's base delay. Default: 0
	// (preserves today's immediate-restart behavior).
	InitialDelaySeconds int `json:"initial_delay_seconds,omitempty"`
	// MaxDelaySeconds caps the backoff formula's computed delay. Default: 300.
	MaxDelaySeconds int `json:"max_delay_seconds,omitempty"`
	// RetryOn is the subset of ["crashed","stalled","tmux_exited"] eligible
	// for automated retry. Empty/nil defaults to all three.
	RetryOn []string `json:"retry_on,omitempty"`
	// StaleTriggersRetry is an opt-in flag that, once a stale-session
	// notification's config (config.StaleSessionConfig) crosses its threshold,
	// treats that as an additional "stalled"-classified trigger. Currently
	// unconsumed — StaleSessionConfig (see StaleSessionConfig above) has no
	// wiring into the retry driver yet; this field exists so a config can
	// opt in ahead of that integration landing without a later schema change.
	StaleTriggersRetry *bool `json:"stale_triggers_retry,omitempty"`
}

// EnabledOrDefault returns whether automated retry is enabled, defaulting to
// true when unset.
func (c RetryPolicyConfig) EnabledOrDefault() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// MaxAttemptsOrDefault returns MaxAttempts, falling back to
// defaultRetryMaxAttempts when unset (<=0) — a fat-fingered 0/negative value
// must not silently disable retry.
func (c RetryPolicyConfig) MaxAttemptsOrDefault() int {
	if c.MaxAttempts <= 0 {
		return defaultRetryMaxAttempts
	}
	return c.MaxAttempts
}

// MaxDelaySecondsOrDefault returns MaxDelaySeconds, falling back to
// defaultRetryMaxDelaySeconds when unset (<=0).
func (c RetryPolicyConfig) MaxDelaySecondsOrDefault() int {
	if c.MaxDelaySeconds <= 0 {
		return defaultRetryMaxDelaySeconds
	}
	return c.MaxDelaySeconds
}

// FilteredRetryOn returns RetryOn with any entry that isn't one of the three
// known reasons dropped (with a logged warning, not a hard error) — a config
// typo like "crashd" would otherwise silently produce a policy that never
// matches that reason, with no error/warning/UI signal anywhere that the
// entry is being ignored. Unlike RetryOnOrDefault, this does NOT widen an
// empty result back to all three reasons: session.resolveRetryPolicy's
// per-session override path needs the filtered result on its own, so an
// override where every entry is a typo falls back to the already-resolved
// *global* value instead of silently widening back to all three reasons.
func (c RetryPolicyConfig) FilteredRetryOn() []string {
	if len(c.RetryOn) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(c.RetryOn))
	for _, reason := range c.RetryOn {
		if slices.Contains(validRetryOnReasons, reason) {
			filtered = append(filtered, reason)
		} else {
			log.Warn("RetryPolicyConfig.RetryOn: ignoring unknown retry reason", "reason", reason)
		}
	}
	return filtered
}

// RetryOnOrDefault returns RetryOn, defaulting to all three known reasons
// when empty or when every entry is unknown. See FilteredRetryOn for the
// filtering-without-fallback variant used by the override-resolution path.
func (c RetryPolicyConfig) RetryOnOrDefault() []string {
	if filtered := c.FilteredRetryOn(); len(filtered) > 0 {
		return filtered
	}
	return append([]string(nil), validRetryOnReasons...)
}

// BackoffOrWarn validates Backoff at config-load time: if it's set and isn't
// "exponential" (the only implemented strategy), logs a warning and falls
// back to "exponential" rather than silently accepting a value that has no
// behavior behind it.
func (c RetryPolicyConfig) BackoffOrWarn() string {
	if c.Backoff == "" || c.Backoff == "exponential" {
		return "exponential"
	}
	log.Warn("RetryPolicyConfig.Backoff: unknown strategy, falling back to exponential", "backoff", c.Backoff)
	return "exponential"
}

// TmuxExecGateConfig bounds how many tmux subprocesses may run concurrently
// against one tmux server, across every process on the machine (the main
// daemon and every --mcp process) — tmux's server is single-threaded, so
// unbounded concurrent subprocess spawns degrade it for everyone.
type TmuxExecGateConfig struct {
	// Slots is the number of concurrent tmux subprocess execution slots.
	// Zero or unset means "use the default" — see SlotsOrDefault. Default: 8.
	Slots int `json:"slots"`

	// ResyncFastLaneSlots is the number of concurrent tmux subprocess execution
	// slots reserved for terminal-resync traffic when the
	// "terminal:resync-exec-gate-fast-lane" feature flag is on, so resync calls
	// don't contend with other tmux exec traffic for the shared Slots pool.
	// Zero or unset means "use the default" — see ResyncFastLaneSlotsOrDefault.
	// Default: 4.
	ResyncFastLaneSlots int `json:"resyncFastLaneSlots"`

	// InputFastLaneSlots is the number of concurrent tmux subprocess execution
	// slots reserved for keystroke input traffic (the legacy per-keystroke
	// send-keys path used when STAPLER_SQUAD_USE_CONTROL_MODE=false), so a
	// poller flooding the shared Slots pool with capture-pane calls never
	// makes user keystrokes queue behind it. Zero or unset means "use the
	// default" — see InputFastLaneSlotsOrDefault. Default: 4.
	InputFastLaneSlots int `json:"inputFastLaneSlots"`
}

// defaultTmuxExecGateSlots is used whenever Slots is unset (zero), including
// for configs saved before this field existed.
const defaultTmuxExecGateSlots = 8

// defaultResyncFastLaneSlots is used whenever ResyncFastLaneSlots is unset
// (zero), including for configs saved before this field existed.
const defaultResyncFastLaneSlots = 4

// defaultInputFastLaneSlots is used whenever InputFastLaneSlots is unset
// (zero), including for configs saved before this field existed.
const defaultInputFastLaneSlots = 4

// SlotsOrDefault returns Slots, falling back to defaultTmuxExecGateSlots when
// unset (covers both a fresh zero-value struct and a config.json saved before
// this field existed, which unmarshals the same way).
func (c TmuxExecGateConfig) SlotsOrDefault() int {
	if c.Slots <= 0 {
		return defaultTmuxExecGateSlots
	}
	return c.Slots
}

// ResyncFastLaneSlotsOrDefault returns ResyncFastLaneSlots, falling back to
// defaultResyncFastLaneSlots when unset (covers both a fresh zero-value
// struct and a config.json saved before this field existed, which unmarshals
// the same way).
func (c TmuxExecGateConfig) ResyncFastLaneSlotsOrDefault() int {
	if c.ResyncFastLaneSlots <= 0 {
		return defaultResyncFastLaneSlots
	}
	return c.ResyncFastLaneSlots
}

// InputFastLaneSlotsOrDefault returns InputFastLaneSlots, falling back to
// defaultInputFastLaneSlots when unset (covers both a fresh zero-value
// struct and a config.json saved before this field existed, which unmarshals
// the same way).
func (c TmuxExecGateConfig) InputFastLaneSlotsOrDefault() int {
	if c.InputFastLaneSlots <= 0 {
		return defaultInputFastLaneSlots
	}
	return c.InputFastLaneSlots
}

// BrowserPassthroughCDPConfig holds tunable parameters for the Chrome DevTools
// Protocol screencast stream. All fields default to zero (use CDPConfigOrDefault
// to apply canonical defaults).
type BrowserPassthroughCDPConfig struct {
	// ScreencastQuality is the JPEG compression quality (1–100).
	// Default: 70.
	ScreencastQuality int `json:"screencast_quality,omitempty"`
	// ScreencastMaxWidth is the maximum frame width in pixels.
	// Default: 1280.
	ScreencastMaxWidth int `json:"screencast_max_width,omitempty"`
	// ScreencastMaxHeight is the maximum frame height in pixels.
	// Default: 800.
	ScreencastMaxHeight int `json:"screencast_max_height,omitempty"`
	// ScreencastMaxFPS is the target frame-rate cap (frames per second).
	// Default: 15 (one frame delivered every ~67 ms via everyNthFrame heuristic).
	ScreencastMaxFPS int `json:"screencast_max_fps,omitempty"`
}

// CDPConfigOrDefault returns a BrowserPassthroughCDPConfig with any zero-value
// fields replaced by the canonical defaults. This allows a partial JSON config
// (e.g. only ScreencastQuality set) to inherit the remaining defaults.
func (c *BrowserPassthroughCDPConfig) CDPConfigOrDefault() BrowserPassthroughCDPConfig {
	out := *c
	if out.ScreencastQuality <= 0 {
		out.ScreencastQuality = 70
	}
	if out.ScreencastMaxWidth <= 0 {
		out.ScreencastMaxWidth = 1280
	}
	if out.ScreencastMaxHeight <= 0 {
		out.ScreencastMaxHeight = 800
	}
	if out.ScreencastMaxFPS <= 0 {
		out.ScreencastMaxFPS = 15
	}
	return out
}

// BrowserPassthroughConfig controls the per-session virtual display (Xvfb + x11vnc) feature.
type BrowserPassthroughConfig struct {
	// Enabled controls whether VNC is started for new sessions.
	// When nil (absent from config), VNC is enabled when required binaries are present.
	// Set to false to unconditionally disable VNC for all sessions.
	Enabled *bool `json:"enabled,omitempty"`
	// DisplayBase is the first X11 display number to allocate (e.g. 100 for :100).
	// Default: 100.
	DisplayBase int `json:"display_base,omitempty"`
	// DisplayRangeMax is the number of display numbers to search above DisplayBase.
	// Default: 100 (searches :100–:199).
	DisplayRangeMax int `json:"display_range_max,omitempty"`
	// Resolution is the Xvfb screen resolution string (WxHxDepth).
	// Default: "1280x800x24".
	Resolution string `json:"resolution,omitempty"`
	// CDP holds tunable parameters for the CDP screencast stream.
	// Absent (zero) values are filled in by CDPConfigOrDefault().
	CDP BrowserPassthroughCDPConfig `json:"cdp,omitempty"`
}

// IsEnabled returns false unless the user has explicitly set enabled=true.
// When Enabled is nil (absent from config), browser passthrough is disabled.
func (c *BrowserPassthroughConfig) IsEnabled() bool {
	if c == nil || c.Enabled == nil {
		return false
	}
	return *c.Enabled
}

// SessionDefaults is the top-level container for all session default configuration.
type SessionDefaults struct {
	// Program is the default AI program (e.g., "claude", "aider").
	Program string `json:"program,omitempty"`
	// AutoYes auto-approves prompts in new sessions.
	AutoYes bool `json:"auto_yes,omitempty"`
	// Tags are pre-applied to every new session.
	Tags []string `json:"tags,omitempty"`
	// EnvVars are environment variables passed to new sessions.
	EnvVars map[string]string `json:"env_vars,omitempty"`
	// CLIFlags are additional CLI flags for the program.
	CLIFlags string `json:"cli_flags,omitempty"`
	// Profiles maps profile name → profile configuration.
	Profiles map[string]ProfileDefaults `json:"profiles,omitempty"`
	// DirectoryRules are path-based rules matched against the session's working directory.
	DirectoryRules []DirectoryRule `json:"directory_rules,omitempty"`
	// Aliases are named session presets invoked via @name in the omnibar.
	Aliases []AliasConfig `json:"aliases,omitempty"`
}

// ProfileDefaults holds the configurable fields for a named profile.
type ProfileDefaults struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Program     string            `json:"program,omitempty"`
	AutoYes     bool              `json:"auto_yes,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	EnvVars     map[string]string `json:"env_vars,omitempty"`
	CLIFlags    string            `json:"cli_flags,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// DirectoryRule associates a working-directory path prefix with profile defaults.
type DirectoryRule struct {
	// Path is the absolute path prefix to match (longest match wins).
	Path string `json:"path"`
	// Profile is the optional named profile to apply when this rule matches.
	Profile string `json:"profile,omitempty"`
	// Overrides are field-level overrides applied after the profile (if any).
	Overrides ProfileDefaults `json:"overrides,omitempty"`
}

// SessionType is the session creation mode (directory, new_worktree, existing_worktree, etc.).
// Defined here so both the config layer and the session layer share the same type without
// a circular import — session already imports config.
type SessionType string

const (
	// SessionTypeDefault uses the default behavior (directory session).
	SessionTypeDefault SessionType = ""
	// SessionTypeDirectory creates a simple directory session without a worktree.
	SessionTypeDirectory SessionType = "directory"
	// SessionTypeNewWorktree creates a new git worktree for the session.
	SessionTypeNewWorktree SessionType = "new_worktree"
	// SessionTypeExistingWorktree reuses an existing git worktree.
	SessionTypeExistingWorktree SessionType = "existing_worktree"
	// SessionTypeNewProject creates a new directory with a git repo.
	SessionTypeNewProject SessionType = "new_project"
	// SessionTypeOneOff creates a temporary directory under OneOffBaseDir with a generated name.
	SessionTypeOneOff SessionType = "one_off"
)

// IsValid reports whether st is a recognized session type.
func (st SessionType) IsValid() bool {
	switch st {
	case SessionTypeDefault:
		return false
	case SessionTypeDirectory, SessionTypeNewWorktree, SessionTypeExistingWorktree,
		SessionTypeNewProject, SessionTypeOneOff:
		return true
	default:
		return false
	}
}

// AliasConfig defines a named session preset invoked via @name in the omnibar.
// Name must match ^[\w-]+$  (letters, digits, hyphens, underscores only).
type AliasConfig struct {
	// Name is the unique alias identifier (e.g. "myproj"). Must match ^[\w-]+$.
	Name string `json:"name"`
	// Group is an optional display group for palette organization.
	Group string `json:"group,omitempty"`
	// Path is the working directory for the session (supports ~/... expansion).
	Path string `json:"path,omitempty"`
	// Description is a human-readable summary shown in the palette.
	Description string `json:"description,omitempty"`
	// Profile is the named profile to apply when resolving defaults.
	Profile string `json:"profile,omitempty"`
	// Program overrides the default program (e.g. "aider").
	Program string `json:"program,omitempty"`
	// AutoYes auto-approves all prompts for this alias.
	AutoYes bool `json:"auto_yes,omitempty"`
	// Tags are pre-applied to sessions created from this alias.
	Tags []string `json:"tags,omitempty"`
	// EnvVars are environment variables set for sessions from this alias.
	EnvVars map[string]string `json:"env_vars,omitempty"`
	// CLIFlags are CLI flags appended to the program command for this alias.
	// At session creation, invocation-time extraFlags are appended after these.
	CLIFlags string `json:"cli_flags,omitempty"`
	// SessionType overrides the session creation mode for this alias.
	// SessionTypeDefault (empty) means use the default (directory session).
	SessionType SessionType `json:"session_type,omitempty"`
	// NamePrefix is prepended to the user-supplied session label when naming sessions.
	// For example, prefix "ssq-" + label "my-feature" → session name "ssq-my-feature".
	NamePrefix string `json:"name_prefix,omitempty"`
}

// GitHubEnterpriseHost registers a GitHub Enterprise Server instance's OAuth App
// client ID so device-flow login can target that host in addition to github.com.
type GitHubEnterpriseHost struct {
	// Host is the bare hostname (no scheme, no trailing slash), e.g. "github.example.com".
	Host string `json:"host"`
	// ClientID is the OAuth App client ID registered on that GHES instance.
	ClientID string `json:"client_id"`
}

// TransitionMode controls how the system responds when capacity thresholds are crossed.
type TransitionMode string

const (
	// TransitionModeManual displays a suggestion banner; the user must click to switch.
	TransitionModeManual TransitionMode = "manual"
	// TransitionModeAuto automatically transitions sessions without user interaction.
	TransitionModeAuto TransitionMode = "auto"
	// TransitionModeNotify shows a warning notification without offering transition UI.
	TransitionModeNotify TransitionMode = "notify"
)

// ProviderPriority defines a prioritized CLI and model target for transitions.
type ProviderPriority struct {
	CLI   string `json:"cli"`
	Model string `json:"model"`
}

// CapacityConfig holds configuration for the provider capacity monitoring and transition feature.
type CapacityConfig struct {
	// TransitionMode controls auto vs manual transition. Default: "manual".
	TransitionMode TransitionMode `json:"transition_mode,omitempty"`
	// ContextWindowWarnPct is the context usage percentage to trigger a warning. Default: 0.75.
	ContextWindowWarnPct float64 `json:"context_window_warn_pct,omitempty"`
	// ContextWindowAutoPct is the context usage percentage to trigger auto-transition (in auto mode). Default: 0.90.
	ContextWindowAutoPct float64 `json:"context_window_auto_pct,omitempty"`
	// RateLimitWarnRemaining triggers a warning when remaining requests fall below this. Default: 10.
	RateLimitWarnRemaining int `json:"rate_limit_warn_remaining,omitempty"`
	// CostBudgetUSD is the accumulated USD cost limit. 0 means no limit. Default: 0.
	CostBudgetUSD float64 `json:"cost_budget_usd,omitempty"`
	// PollIntervalSeconds controls limit API querying frequency. Default: 60.
	PollIntervalSeconds int `json:"poll_interval_seconds,omitempty"`
	// ProviderPriority lists fallback providers in order of preference.
	ProviderPriority []ProviderPriority `json:"provider_priority,omitempty"`
}

// QuotaConfig holds configuration for the account-wide Claude Code session-quota
// gate that pauses/resumes backlog automation (see BacklogController) based on
// an inferred quota-headroom signal, plus a foreground-session dispatch throttle.
type QuotaConfig struct {
	// Enabled gates the entire feature. When false, the gate is a no-op and
	// BacklogController's toggle behaves exactly as it does today. Default: false.
	Enabled bool `json:"enabled,omitempty"`
	// PauseBelowHeadroomPct is the soft/proactive threshold: backlog is paused
	// once estimated headroom drops below this percentage. Default: 20.0.
	PauseBelowHeadroomPct float64 `json:"pause_below_headroom_pct,omitempty"`
	// ResumeMarginPct is added to PauseBelowHeadroomPct to form the resume
	// threshold, avoiding flapping right at the pause line. Default: 15.0.
	ResumeMarginPct float64 `json:"resume_margin_pct,omitempty"`
	// ConsecutiveTicksToPause is how many consecutive below-threshold reconcile
	// ticks are required before the soft signal pauses backlog. Default: 2.
	ConsecutiveTicksToPause int `json:"consecutive_ticks_to_pause,omitempty"`
	// ConsecutiveTicksToResume is how many consecutive above-threshold reconcile
	// ticks are required before the soft signal resumes backlog. Default: 3.
	ConsecutiveTicksToResume int `json:"consecutive_ticks_to_resume,omitempty"`
	// AssumedWindowTokenBudget is the operator-supplied assumed token budget for
	// the trailing 5h window. Anthropic publishes no real budget, so this must be
	// calibrated manually; 0 (the default) disables the soft/percentage signal
	// entirely, leaving only the hard/reactive rate-limit override active.
	AssumedWindowTokenBudget int64 `json:"assumed_window_token_budget,omitempty"`
	// RateLimitWindowMinutes is how long a detected rate-limit event keeps the
	// hard/reactive override active. Default: 30.
	RateLimitWindowMinutes int `json:"rate_limit_window_minutes,omitempty"`
	// ManualOverrideGraceMinutes is how long after a detected manual override the
	// notification cooldown is bypassed for the next auto-transition. Default: 10.
	ManualOverrideGraceMinutes int `json:"manual_override_grace_minutes,omitempty"`
	// ForegroundThrottleDelaySeconds is how long the foreground-session dispatch
	// throttle stays active after the most recently observed foreground activity.
	// Default: 300.
	ForegroundThrottleDelaySeconds int `json:"foreground_throttle_delay_seconds,omitempty"`
}

// QuotaConfigOrDefault returns a QuotaConfig with standard defaults applied to zero fields.
func (c QuotaConfig) QuotaConfigOrDefault() QuotaConfig {
	out := c
	// Enabled intentionally stays false by default — this feature ships opt-in.
	if out.PauseBelowHeadroomPct <= 0 {
		out.PauseBelowHeadroomPct = 20.0
	}
	if out.ResumeMarginPct <= 0 {
		out.ResumeMarginPct = 15.0
	}
	if out.ConsecutiveTicksToPause <= 0 {
		out.ConsecutiveTicksToPause = 2
	}
	if out.ConsecutiveTicksToResume <= 0 {
		out.ConsecutiveTicksToResume = 3
	}
	// AssumedWindowTokenBudget intentionally stays 0 by default — no safe guess exists.
	if out.RateLimitWindowMinutes <= 0 {
		out.RateLimitWindowMinutes = 30
	}
	if out.ManualOverrideGraceMinutes <= 0 {
		out.ManualOverrideGraceMinutes = 10
	}
	if out.ForegroundThrottleDelaySeconds <= 0 {
		out.ForegroundThrottleDelaySeconds = 300
	}
	return out
}

// RemoteConfig registers one SSH-reachable remote host that sessions can be
// created against (ssh-remote-workspaces feature). It holds only connection
// coordinates and a pointer to credential material — never the credential
// material itself. See sshremote.KeyStore (Phase 3.2) for where the actual
// SSH private key/passphrase bytes live (OS keychain), and
// Config.RemoteByName for the lookup helper consumed by session creation
// (Phase 4) and Settings UI validation (Phase 6).
type RemoteConfig struct {
	// Name is the unique, user-chosen identifier for this remote (e.g.
	// "prod-box"), referenced by session-creation flows. Not a hostname and
	// not required to resolve as one.
	Name string `json:"name"`
	// Host is the SSH-reachable hostname or address (e.g. "prod.example.com"
	// or "10.0.0.5"). Not a full "user@host" string — see User for the
	// login name — and not a URL (no scheme, no path, no port suffix; use a
	// standard SSH config Host block for non-default ports).
	Host string `json:"host"`
	// User is the SSH login username on the remote host. Not a local
	// username and not validated against the remote's actual user list at
	// save time.
	User string `json:"user"`
	// BasePath is the absolute filesystem path on the remote host under
	// which session worktrees/directories are created (e.g.
	// "/srv/workspaces"). Not a local path and not created automatically by
	// saving this config — it must already exist (or be creatable) on the
	// remote.
	BasePath string `json:"base_path"`
	// IdentityRef is an opaque, non-secret pointer to the SSH identity
	// (private key + optional passphrase) that authenticates to this
	// remote, resolved at connection time via sshremote.KeyStore against the
	// OS keychain. It is NOT a filesystem path to a key file, NOT the key's
	// raw bytes, and NOT the passphrase itself — no key material is ever
	// stored in config.json. An empty value means no identity has been
	// registered yet for this remote.
	IdentityRef string `json:"identity_ref"`
}

// CapacityConfigOrDefault returns a CapacityConfig with standard defaults applied to zero fields.
func (c CapacityConfig) CapacityConfigOrDefault() CapacityConfig {
	out := c
	if out.TransitionMode == "" {
		out.TransitionMode = TransitionModeManual
	}
	if out.ContextWindowWarnPct <= 0 {
		out.ContextWindowWarnPct = 0.75
	}
	if out.ContextWindowAutoPct <= 0 {
		out.ContextWindowAutoPct = 0.90
	}
	if out.RateLimitWarnRemaining <= 0 {
		out.RateLimitWarnRemaining = 10
	}
	if out.PollIntervalSeconds <= 0 {
		out.PollIntervalSeconds = 60
	}
	if len(out.ProviderPriority) == 0 {
		out.ProviderPriority = []ProviderPriority{
			{CLI: "agy", Model: "gemini-2.0-flash"},
			{CLI: "claude", Model: "claude-3-5-sonnet-20241022"},
		}
	}
	return out
}

// HandoffSummaryConfig holds configuration for the restart-with-handoff-summary feature.
type HandoffSummaryConfig struct {
	// Enabled toggles the feature. nil defaults to enabled, matching
	// AutoSpawnReadyItems's pattern — see EnabledOrDefault. A plain `bool` here
	// would make "explicitly false" indistinguishable from "key absent" (e.g. a
	// config.json written before this feature existed), silently defaulting
	// every pre-existing installation to disabled instead of the stated
	// default of enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// MaxMiddleExcerptTokens caps the proportional middle-transcript excerpt fed
	// to the summarizer. Default: 12000.
	MaxMiddleExcerptTokens int `json:"max_middle_excerpt_tokens,omitempty"`
}

// EnabledOrDefault reports whether the restart-with-handoff-summary feature is
// enabled. Defaults to true (nil, or unset config.json key); pass explicit
// false to disable.
func (c HandoffSummaryConfig) EnabledOrDefault() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// HandoffSummaryConfigOrDefault returns a HandoffSummaryConfig with standard
// defaults applied to zero fields.
func (c HandoffSummaryConfig) HandoffSummaryConfigOrDefault() HandoffSummaryConfig {
	out := c
	if out.MaxMiddleExcerptTokens <= 0 {
		out.MaxMiddleExcerptTokens = 12000
	}
	return out
}
