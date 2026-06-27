package config

import "time"

// NotificationPrefs holds the user's notification delivery preferences.
type NotificationPrefs struct {
	// PushEnabled controls whether web push notifications are sent.
	// Default is false (opt-in).
	PushEnabled bool `json:"push_enabled"`
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

