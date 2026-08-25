package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// NewConfigWithExecutor creates a Config with an explicit command executor.
// Pass nil to use the default timeout executor.
func NewConfigWithExecutor(exec CommandExecutor) *Config {
	if exec == nil {
		if IsTestMode() {
			exec = &lookPathOnlyExecutor{}
		} else {
			exec = newTimeoutCommandExecutor(5 * time.Second)
		}
	}
	return &Config{executor: exec}
}

// NewConfig creates a Config with the default timeout executor.
func NewConfig() *Config {
	return NewConfigWithExecutor(nil)
}

const (
	ConfigFileName = "config.json"
	defaultProgram = "proxy-claude"
)

// isWithinStateDir reports whether workDir is baseDir itself or a descendant of it.
// A cwd inside stapler-squad's own state directory (e.g. a session worktree under
// ~/.stapler-squad/workspaces/.../worktrees/...) hashes to a workspace distinct from
// the one normally used from the project/home directory.
func isWithinStateDir(workDir, baseDir string) bool {
	if workDir == "" || baseDir == "" {
		return false
	}
	workDir = filepath.Clean(workDir)
	baseDir = filepath.Clean(baseDir)
	return workDir == baseDir || strings.HasPrefix(workDir, baseDir+string(filepath.Separator))
}

// IsTestMode detects if the application is running in test/benchmark mode
func IsTestMode() bool {

	// Check command line arguments for test/benchmark indicators
	for _, arg := range os.Args {
		if strings.Contains(arg, ".test") ||
			strings.Contains(arg, "-test.") ||
			strings.HasSuffix(arg, ".test.exe") ||
			strings.Contains(arg, "-bench") {
			return true
		}
	}
	return false
}

// IsNamedInstance reports whether this process is running as an explicitly
// named, non-default instance (STAPLER_SQUAD_INSTANCE set to anything other
// than "" or "shared" — see GetConfigDirForDir's priority hierarchy above).
// A named instance gets its own isolated DB/config directory but does NOT get
// its own tmux socket — it shares the default tmux server with every other
// instance on the machine, including the real production one. IsTestMode()
// alone doesn't catch this: this repo's own E2E harness (tests/e2e, per
// CLAUDE.md: "STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad
// --tmux-keep-server") runs the real production binary, not a `go test`
// binary, so IsTestMode() returns false for it even though it has exactly the
// same "small, isolated instance list vs. the shared tmux socket" hazard a
// `go test` binary does. Confirmed live: an e2e-local run's orphan sweep
// killed 5 unrelated production tmux sessions it had never heard of,
// including the interactive session this very fix was written in.
func IsNamedInstance() bool {
	instanceID := os.Getenv("STAPLER_SQUAD_INSTANCE")
	return instanceID != "" && instanceID != "shared"
}

// IsIsolatedInstance reports whether this process's config/DB state is
// isolated from the shared default (~/.stapler-squad) directory by ANY known
// mechanism: a `go test` binary (IsTestMode), an explicit named instance
// (IsNamedInstance), or a STAPLER_SQUAD_TEST_DIR override (GetConfigDirForDir
// priority 1 — used by --test-mode harnesses like tests/demo/helpers.go's
// StartDemoServer). Isolated DB state does NOT imply an isolated tmux socket
// under any of these mechanisms — see IsNamedInstance's doc comment for the
// confirmed incident that motivated this check. Call sites that could
// otherwise touch shared, non-isolated resources (like the default tmux
// socket in ReconcileOrphanedTmuxSessions) must skip when this is true.
// STAPLER_SQUAD_TEST_DIR was the still-missing case: a demo/test-mode harness
// process gets a fully isolated DB via GetConfigDirForDir but, before this
// check existed, its startup orphan sweep still targeted the shared default
// tmux socket — killing every real production session it didn't recognize.
func IsIsolatedInstance() bool {
	return IsTestMode() || IsNamedInstance() || os.Getenv("STAPLER_SQUAD_TEST_DIR") != ""
}

// GetConfigDir returns the path to the application's configuration directory
// with hierarchical isolation for safe multi-instance and test execution.
//
// Priority hierarchy:
//  1. Test directory override via STAPLER_SQUAD_TEST_DIR (for --test-mode flag)
//  2. Explicit instance ID via STAPLER_SQUAD_INSTANCE environment variable
//  3. Test mode auto-detection (automatic isolation for tests/benchmarks)
//  4. Preferred workspace from preference file (explicit switch via SwitchDatabase RPC)
//  5. Per-directory workspace isolation, opt-in via STAPLER_SQUAD_WORKSPACE_MODE=true
//  6. Global shared state (default)
func GetConfigDir() (string, error) {
	return GetConfigDirForDir("")
}

// GetConfigDirForDir returns the path to the application's configuration directory
// using the provided directory for workspace-based isolation.
func GetConfigDirForDir(dir string) (string, error) {
	// Priority 1: Test directory override (from --test-mode flag)
	if testDir := os.Getenv("STAPLER_SQUAD_TEST_DIR"); testDir != "" {
		// Create the test directory if it doesn't exist
		if err := os.MkdirAll(testDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create test directory: %w", err)
		}
		return testDir, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".stapler-squad")

	// One-time migration: if ~/.stapler-squad doesn't exist but ~/.claude-squad does, migrate automatically
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		legacyDir := filepath.Join(homeDir, ".claude-squad")
		if _, legacyErr := os.Stat(legacyDir); legacyErr == nil {
			if migrateErr := os.Rename(legacyDir, baseDir); migrateErr == nil {
				log.Info("migrated data directory", "from", legacyDir, "to", baseDir)
			}
		}
	}

	// Priority 2: Explicit instance ID (tests, named instances, backward compat)
	if instanceID := os.Getenv("STAPLER_SQUAD_INSTANCE"); instanceID != "" {
		// Special value "shared" maintains backward compatibility
		if instanceID == "shared" {
			return baseDir, nil
		}
		return filepath.Join(baseDir, "instances", instanceID), nil
	}

	// Priority 3: Test mode auto-detection (automatic isolation)
	// Must be checked before the preferred workspace file so that a workspace
	// preference set by a production instance cannot leak into test runs.
	if IsTestMode() {
		// Each test/benchmark process gets its own isolated state
		pid := os.Getpid()
		return filepath.Join(baseDir, "test", fmt.Sprintf("test-%d", pid)), nil
	}

	return resolveDefaultConfigDir(dir, baseDir)
}

// resolveDefaultConfigDir implements Priority 4-6 of GetConfigDirForDir: preferred
// workspace file, opt-in per-directory workspace isolation, then shared state.
// Split out from GetConfigDirForDir so it can be tested directly — Priority 3
// (test mode auto-detection) is always true inside a `go test` binary, which
// would otherwise make this logic unreachable in tests.
func resolveDefaultConfigDir(dir, baseDir string) (string, error) {
	// Priority 4: Preferred workspace from preference file
	// Written by SwitchDatabase RPC; cleared automatically on removal.
	// Skipped in test mode (above) so tests always get isolated state.
	if data, err := os.ReadFile(GetPreferredWorkspaceFile(baseDir)); err == nil {
		prefDir := strings.TrimSpace(string(data))
		if filepath.IsAbs(prefDir) &&
			(prefDir == baseDir || strings.HasPrefix(prefDir, baseDir+string(filepath.Separator))) {
			if _, statErr := os.Stat(prefDir); statErr == nil {
				return prefDir, nil
			}
		}
	}

	// Priority 5: Per-directory workspace isolation — opt-in only.
	// A single shared workspace is the default; per-cwd auto-isolation must be
	// explicitly enabled with STAPLER_SQUAD_WORKSPACE_MODE=true. Switching between
	// workspaces is meant to be an explicit user action (see SwitchDatabase RPC /
	// the workspace switcher UI), not an automatic side effect of the cwd a process
	// happens to be started from — the latter is what caused sessions to silently
	// "disappear" when the binary was started from inside a worktree.
	if os.Getenv("STAPLER_SQUAD_WORKSPACE_MODE") == "true" {
		workDir := dir
		var err error
		if workDir == "" {
			workDir, err = os.Getwd()
		}
		if err == nil && workDir != "" {
			if isWithinStateDir(workDir, baseDir) {
				// Running with a cwd inside stapler-squad's own state directory (e.g. a
				// session worktree) hashes to a workspace distinct from the one the user
				// normally works from, silently landing on an empty database that looks
				// like all sessions vanished. Almost always means the binary was started
				// manually from within a worktree instead of via the installed service.
				log.Warn("cwd is inside stapler-squad state directory; this process will use a different workspace than usual and may appear to have no sessions",
					"cwd", workDir, "state_dir", baseDir)
			}
			// Hash the workspace path for a stable, filesystem-safe identifier
			hash := sha256.Sum256([]byte(workDir))
			workspaceID := fmt.Sprintf("%x", hash[:8])
			return filepath.Join(baseDir, "workspaces", workspaceID), nil
		}
		if err != nil {
			// If we can't get working directory, fall through to shared state
			log.Warn("failed to get working directory for workspace isolation", "err", err)
		}
	}

	// Priority 6: Global shared state (default)
	return baseDir, nil
}

// Config represents the application configuration
type Config struct {
	// executor is the command executor used for shell command discovery.
	// Set via NewConfigWithExecutor; defaults to a 5-second timeout executor.
	executor CommandExecutor
	// lazyMu guards the generate-on-first-use fields below (MachineEncryptionKey,
	// ClaimantHostID) against concurrent first-callers racing to generate and
	// persist their own value — see GetOrCreateEncryptionKey/GetOrCreateClaimantHostID.
	lazyMu sync.Mutex
	// slackWebhookURLOverride holds the SLACK_WEBHOOK_URL env var value, if
	// set at load time. Never persisted to config.json — see ADR-001. Not
	// serialized since the field is unexported; read via SlackWebhookURLOverride().
	slackWebhookURLOverride string
	// slackSigningSecretOverride holds the SLACK_SIGNING_SECRET env var value,
	// if set at load time. Never persisted to config.json — see ADR-001. Not
	// serialized since the field is unexported; read via SlackSigningSecretOverride().
	slackSigningSecretOverride string
	// ListenAddress is the address the HTTP server listens on.
	// Default: "localhost:8543". Set to "0.0.0.0:8543" for remote access.
	ListenAddress string `json:"listen_address"`
	// PasskeyRPID is the WebAuthn Relying Party ID (effective domain, no scheme/port).
	// Example: "192.168.1.42" or "myhost.local". Must match the hostname clients use.
	// Required when remote access is enabled.
	PasskeyRPID string `json:"passkey_rp_id"`
	// PasskeyEnabled controls whether passkey authentication is enforced.
	// Automatically set to true when non-localhost listen address is used.
	PasskeyEnabled bool `json:"passkey_enabled"`
	// DefaultProgram is the default program to run in new instances
	DefaultProgram string `json:"default_program"`
	// AutoYes is a flag to automatically accept all prompts.
	AutoYes bool `json:"auto_yes"`
	// DaemonPollInterval is the interval (ms) at which the daemon polls sessions for autoyes mode.
	DaemonPollInterval int `json:"daemon_poll_interval"`
	// BranchPrefix is the prefix used for git branches created by the application.
	BranchPrefix string `json:"branch_prefix"`
	// DetectNewSessions is a flag to enable detection of new sessions from other windows
	DetectNewSessions bool `json:"detect_new_sessions"`
	// SessionDetectionInterval is the interval (ms) at which the daemon checks for new sessions
	SessionDetectionInterval int `json:"session_detection_interval"`
	// StateRefreshInterval is the interval (ms) at which the state is refreshed from disk
	StateRefreshInterval int `json:"state_refresh_interval"`
	// LogsEnabled is a flag to enable logging to files
	LogsEnabled bool `json:"logs_enabled"`
	// LogsDir is the directory where logs are stored (defaults to ~/.stapler-squad/logs)
	LogsDir string `json:"logs_dir"`
	// LogMaxSize is the maximum size of a log file in megabytes before it gets rotated
	LogMaxSize int `json:"log_max_size"`
	// LogMaxFiles is the maximum number of rotated log files to keep (not including the current log file)
	LogMaxFiles int `json:"log_max_files"`
	// LogMaxAge is the maximum number of days to keep rotated log files
	LogMaxAge int `json:"log_max_age"`
	// LogCompress is a flag to enable compression of rotated log files
	LogCompress bool `json:"log_compress"`
	// UseSessionLogs is a flag to enable per-session log files
	UseSessionLogs bool `json:"use_session_logs"`
	// TmuxSessionPrefix allows customizing the tmux session prefix for process isolation
	TmuxSessionPrefix string `json:"tmux_session_prefix"`
	// PerformBackgroundHealthChecks enables non-blocking health checks for session maintenance
	PerformBackgroundHealthChecks bool `json:"perform_background_health_checks"`
	// KeyCategories defines custom category mappings for key bindings in help system
	KeyCategories map[string]string `json:"key_categories"`
	// VCSPreference controls which version control system to prefer when both are available
	// Options: "auto" (prefer JJ if available), "jj" (always use JJ), "git" (always use Git)
	VCSPreference string `json:"vcs_preference"`
	// AvailablePrograms is a list of detected CLI programs
	AvailablePrograms []string `json:"available_programs"`
	// ConfigVersion tracks the schema version for future migrations (1 = session_defaults added)
	ConfigVersion int `json:"config_version,omitempty"`
	// SessionDefaults holds named profiles, directory rules, and global defaults for new sessions.
	SessionDefaults SessionDefaults `json:"session_defaults,omitempty"`
	// Notifications holds the user's notification delivery preferences.
	Notifications NotificationPrefs `json:"notifications,omitempty"`
	// Remotes is a named list of SSH-reachable remote hosts sessions can be
	// created against (ssh-remote-workspaces feature). Holds connection
	// coordinates only — no SSH key material; see RemoteConfig's doc
	// comment. Looked up by name via RemoteByName.
	Remotes []RemoteConfig `json:"remotes,omitempty"`
	// OneOffBaseDir is the base directory where one-off session directories are created.
	// Default: "~/oneoff". Tilde is expanded at runtime. Created automatically on first use.
	OneOffBaseDir string `json:"one_off_base_dir,omitempty"`
	// PyroscopeServerAddress is the Pyroscope server URL for continuous profiling.
	// Empty string (the default) disables continuous profiling.
	// Example: "http://localhost:4040"
	PyroscopeServerAddress string `json:"pyroscope_server_address,omitempty"`
	// NewProjectBaseDir is the base directory where new project directories are created.
	// Default: "~/Projects". Tilde is expanded at runtime. Created on first use.
	// Zero-value (empty string) is backwards-compatible — existing configs load without change.
	NewProjectBaseDir string `json:"new_project_base_dir,omitempty"`
	// MachineEncryptionKey is a base64-encoded 32-byte AES-256-GCM key for local data encryption.
	// Generated on first run and persisted here. Used to encrypt sensitive token data in ItemSource configs.
	MachineEncryptionKey string `json:"machine_encryption_key,omitempty"`
	// ClaimantHostID is a randomly generated identifier for THIS physical process/config
	// directory, generated on first use and persisted here. Recorded on backlog ItemSession
	// rows to show which host/process claimed or attached a session (see
	// GetOrCreateClaimantHostID). Stable across restarts of this same process/config dir;
	// distinct across different hosts and across different STAPLER_SQUAD_INSTANCE-namespaced
	// config dirs on the same machine, since each gets its own config.json. Unrelated to
	// STAPLER_SQUAD_INSTANCE (which only namespaces config/state directories on a single
	// machine) and unrelated to session/contexts.go's CloudContext.InstanceID (a cloud
	// provider's instance identifier, not populated for local/dev sessions).
	ClaimantHostID string `json:"claimant_host_id,omitempty"`
	// MaxAutoReworkIterations caps how many automated work sessions the backlog auto-reopen
	// loop will spawn for a single item before leaving it for manual review. 0 = use the
	// default (20). Individual items can also override this via
	// BacklogItemData.ReworkCapOverride (0 = unlimited for that item, >0 = that item's own
	// cap) — see effectiveReworkCap in server/services/backlog_service_triage.go.
	MaxAutoReworkIterations int `json:"max_auto_rework_iterations,omitempty"`
	// MaxConcurrentBacklogWorkItems caps how many distinct backlog items may be
	// "in_progress" at the same time. 0 = use the default (2). Values above
	// maxConcurrentBacklogWorkItemsHardCeiling are clamped to the ceiling.
	MaxConcurrentBacklogWorkItems int `json:"max_concurrent_backlog_work_items,omitempty"`
	// AutoSpawnReadyItems controls whether "ready" backlog items (post-triage, plan
	// approved or SkipPlanning) automatically claim a free WIP slot and spawn a work
	// session — in priority order (P1 first) — the moment one is free, without a
	// human clicking "Spawn Session". A *bool, not bool: the zero value of bool
	// can't represent "unset" the way 0 does for the int settings above, and this
	// setting's default is true (unlike SkipReviewGate/AutoCreatePR's per-item
	// false-by-default opt-ins), so nil must mean "use the default", not "disabled".
	// Pass explicit false to require manual spawning instead.
	AutoSpawnReadyItems *bool `json:"auto_spawn_ready_items,omitempty"`

	// AnalyticsMaxRows is the maximum number of analytics events to retain in the database.
	// When exceeded, the oldest rows are deleted. 0 means no row-count limit.
	// Default: 100_000.
	AnalyticsMaxRows int `json:"analytics_max_rows,omitempty"`
	// AnalyticsMaxAgeDays is the maximum age in days of analytics events to retain.
	// Events older than this are deleted. 0 means no age limit.
	// Default: 90.
	AnalyticsMaxAgeDays int `json:"analytics_max_age_days,omitempty"`
	// BrowserPassthrough configures the per-session Xvfb + x11vnc virtual display feature.
	BrowserPassthrough BrowserPassthroughConfig `json:"browser_passthrough,omitempty"`
	// FeatureFlags stores the enabled/disabled state of named runtime feature flags.
	// Keys are machine names (e.g. "backlog"); values are booleans.
	// Absent key == disabled (false is the safe default for all flags).
	FeatureFlags map[string]bool `json:"feature_flags,omitempty"`
	// Hibernation holds configuration for the session hibernation feature.
	Hibernation HibernationConfig `json:"hibernation,omitempty"`
	// Capacity holds configuration for the provider capacity monitoring and transition feature.
	Capacity CapacityConfig `json:"capacity,omitempty"`
	// HandoffSummary holds configuration for the restart-with-handoff-summary feature.
	HandoffSummary HandoffSummaryConfig `json:"handoff_summary,omitempty"`
	// Quota holds configuration for the account-wide session-quota gate that
	// pauses/resumes backlog automation based on inferred quota headroom.
	Quota QuotaConfig `json:"quota,omitempty"`
	// TmuxExecGate bounds concurrent tmux subprocess execution across all processes.
	TmuxExecGate TmuxExecGateConfig `json:"tmux_exec_gate,omitempty"`
	// SessionRetention holds configuration for the automatic session-retention cleanup sweep.
	SessionRetention SessionRetentionConfig `json:"session_retention,omitempty"`
	// StaleSession holds configuration for stale-session detection (inactivity threshold
	// and notify-on-stale toggle).
	StaleSession StaleSessionConfig `json:"stale_session,omitempty"`
	// Callbacks holds the global singleton outbound-callback URLs (webhook-triggers
	// Phase 5, FR7) fired by CallbackDispatcher on session-complete/session-stale/
	// queue-item-created lifecycle events.
	Callbacks CallbackConfig `json:"callbacks,omitempty"`
	// Slack holds configuration for the Slack review-queue notification
	// feature. Secret fields are ciphertext only — see ADR-001.
	Slack SlackConfig `json:"slack,omitempty"`

	// Escape analytics configuration

	// EscapeAnalyticsCaptureLevel controls the verbosity of escape sequence capture.
	// Valid values: "full" (store raw bytes + hash), "summary" (type/length only), "off" (disabled).
	// Default: "summary".
	EscapeAnalyticsCaptureLevel string `json:"escapeAnalyticsCaptureLevel,omitempty"`
	// EscapeAnalyticsSamplingRate is the fraction of sessions to capture, in [0.0, 1.0].
	// 1.0 captures all sessions; 0.0 captures none.
	// A nil pointer means "unset" and defaults to 1.0 at load time.
	// Using a pointer allows 0.0 (capture nothing) to be distinguished from the zero value.
	// Default: 1.0.
	EscapeAnalyticsSamplingRate *float64 `json:"escapeAnalyticsSamplingRate,omitempty"`
	// EscapeAnalyticsMaxRowsPerSession is the maximum number of escape event rows stored per session.
	// Default: 10000.
	EscapeAnalyticsMaxRowsPerSession int `json:"escapeAnalyticsMaxRowsPerSession,omitempty"`
	// EscapeAnalyticsDisableOSCRedaction disables OSC payload redaction when true.
	// By default (false), OSC payloads (clipboard, window title, CWD) are redacted for security.
	// Set to true only if you explicitly need to capture raw OSC payload content.
	EscapeAnalyticsDisableOSCRedaction bool `json:"escapeAnalyticsDisableOSCRedaction,omitempty"`
	// EscapeAnalyticsRetentionDays is the number of days to retain escape event rows.
	// Default: 7.
	EscapeAnalyticsRetentionDays int `json:"escapeAnalyticsRetentionDays,omitempty"`
	// AnthropicAPIKey is the API key for the Anthropic AI API.
	// Used by the AI rule generation feature (GenerateSuggestedRule RPC).
	// Set via config.json or the ANTHROPIC_API_KEY environment variable.
	// Do not log this value.
	AnthropicAPIKey string `json:"anthropicApiKey,omitempty"`
	// ProcessManagerBackend selects the process manager implementation.
	// Valid values: "tmux" (default), "native" (Phase 2).
	// Empty string is backwards-compatible and defaults to "tmux".
	ProcessManagerBackend string `json:"process_manager_backend,omitempty"`
	// GitHubEnterpriseHosts registers GitHub Enterprise Server instances (beyond
	// github.com) with their own OAuth App client IDs, enabling device-flow login,
	// PR polling, and link detection against those hosts. Empty means github.com only.
	GitHubEnterpriseHosts []GitHubEnterpriseHost `json:"github_enterprise_hosts,omitempty"`

	// StreamHubSessionOverrides forces the terminal-multi-connection-streaming
	// project's PathHubOwned resolution for specific named tmux sessions,
	// regardless of the global STAPLER_SQUAD_USE_STREAM_HUB default — the
	// per-session canary mechanism (Story 3.3.1). Keys are tmux session
	// names; an absent key means "no override, use the global default".
	// Consulted via streamhub.SetSessionOverrideLookup, wired at process
	// startup in server/services so package session/streamhub never imports
	// package config directly.
	StreamHubSessionOverrides map[string]bool `json:"stream_hub_session_overrides,omitempty"`
	// RollbackRehearsalCompletedAt records when Story 3.3.2's rollback
	// rehearsal (flip STAPLER_SQUAD_USE_STREAM_HUB's per-session override on
	// for a disposable session, use it briefly, remove the override, confirm
	// a clean reconnect under the legacy path) was last completed
	// successfully. nil means "never completed". ResolveGlobalStreamHubDefault
	// refuses to let the *global* default resolve to true until this is set
	// (pre-mortem P1 #4's mechanical gate) — the per-session override above
	// is unaffected by this gate. Set via RecordRollbackRehearsalCompleted.
	RollbackRehearsalCompletedAt *time.Time `json:"rollback_rehearsal_completed_at,omitempty"`
}

// ErrRollbackRehearsalNotCompleted is returned by ResolveGlobalStreamHubDefault
// when the caller requests the global STAPLER_SQUAD_USE_STREAM_HUB default
// resolve to true but RollbackRehearsalCompletedAt is unset — Story 3.3.2's
// rollback rehearsal must be executed and recorded first (pre-mortem P1 #4).
var ErrRollbackRehearsalNotCompleted = errors.New("config: cannot enable the global stream-hub default: rollback rehearsal (RollbackRehearsalCompletedAt) has not been completed — see Story 3.3.2")

// ResolveGlobalStreamHubDefault applies Story 3.3.1/3.3.2's mechanical
// rollback-rehearsal gate to a raw requested value for the *global*
// STAPLER_SQUAD_USE_STREAM_HUB default (e.g. read from that environment
// variable). Requesting false is always permitted — the gate only blocks
// turning the risky path *on*. Requesting true is refused with
// ErrRollbackRehearsalNotCompleted, not a silent fallback to false, unless
// cfg.RollbackRehearsalCompletedAt is a recorded, non-zero timestamp. This
// gate does not apply to the per-session override path
// (StreamHubSessionOverrides / streamhub.SetSessionOverrideLookup), which
// callers resolve independently and which remains available even when this
// function returns an error.
func ResolveGlobalStreamHubDefault(cfg *Config, requested bool) (bool, error) {
	if !requested {
		return false, nil
	}
	if cfg == nil || cfg.RollbackRehearsalCompletedAt == nil || cfg.RollbackRehearsalCompletedAt.IsZero() {
		return false, ErrRollbackRehearsalNotCompleted
	}
	return true, nil
}

// RecordRollbackRehearsalCompleted persists the current time as
// RollbackRehearsalCompletedAt and saves the config — Story 3.3.2's Task
// 3.3.2c, intended to be called exactly once, after manually verifying a
// rollback rehearsal (flip on via the per-session override, use briefly,
// remove the override, confirm clean legacy reconnect) passed against a
// real disposable session. Unblocks ResolveGlobalStreamHubDefault from
// refusing to enable the global default.
func (c *Config) RecordRollbackRehearsalCompleted() error {
	now := time.Now()
	c.RollbackRehearsalCompletedAt = &now
	return SaveConfig(c)
}

// GetStreamHubSessionOverride reports whether sessionName has a per-session
// StreamHubSessionOverrides entry recorded, and if so, what it forces.
// Mirrors GetFeatureFlag's nil-safe shape: a nil Config or nil map reports
// (false, false) — no override.
func (c *Config) GetStreamHubSessionOverride(sessionName string) (forceHub bool, ok bool) {
	if c == nil || c.StreamHubSessionOverrides == nil {
		return false, false
	}
	forceHub, ok = c.StreamHubSessionOverrides[sessionName]
	return forceHub, ok
}

// SetStreamHubSessionOverride sets or clears sessionName's per-session
// PathHubOwned override and persists the config to disk — Story 3.3.1's
// canary mechanism. forceHub follows this file's existing *bool convention
// for a tri-state field (see AutoSpawnReadyItems): nil removes any override
// for sessionName (falling back to the global default), a non-nil false
// explicitly pins the session to the legacy path regardless of the global
// default, and a non-nil true forces PathHubOwned.
func (c *Config) SetStreamHubSessionOverride(sessionName string, forceHub *bool) error {
	if forceHub == nil {
		if c.StreamHubSessionOverrides != nil {
			delete(c.StreamHubSessionOverrides, sessionName)
		}
		return SaveConfig(c)
	}
	if c.StreamHubSessionOverrides == nil {
		c.StreamHubSessionOverrides = make(map[string]bool)
	}
	c.StreamHubSessionOverrides[sessionName] = *forceHub
	return SaveConfig(c)
}

// GetGitHubEnterpriseHosts returns the configured GHES hosts, or nil if c is nil.
func (c *Config) GetGitHubEnterpriseHosts() []GitHubEnterpriseHost {
	if c == nil {
		return nil
	}
	return c.GitHubEnterpriseHosts
}

// RemoteByName looks up a configured remote by its exact Name. Returns
// (nil, false) if c is nil or no remote with that name is registered.
// Consumed by session creation (Phase 4) and Settings UI validation (Phase 6).
func (c *Config) RemoteByName(name string) (*RemoteConfig, bool) {
	if c == nil {
		return nil, false
	}
	for i := range c.Remotes {
		if c.Remotes[i].Name == name {
			return &c.Remotes[i], true
		}
	}
	return nil, false
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return defaultConfigWithExecutor(nil)
}

// testModeSentinelProgram replaces the real default program whenever a Config is
// auto-constructed (no explicit executor) under `go test`. Without this,
// GetClaudeCommand's PATH fallback (lookPathOnlyExecutor.LookPath, which is a real
// exec.LookPath) finds and returns the genuine, locally-installed "claude" binary --
// so a test that forgets to isolate its config/cleanup doesn't just leak an idle
// process, it leaks a live, authenticated, tool-wielding Claude Code agent (with its
// own MCP server tree) running indefinitely. "true" exits immediately, so a leaked
// tmux pane closes itself instead of staying up forever.
const testModeSentinelProgram = "true"

// defaultConfigWithExecutor creates the default Config using the provided executor.
// Pass nil to use the default timeout executor.
func defaultConfigWithExecutor(exec CommandExecutor) *Config {
	cfg := NewConfigWithExecutor(exec)

	var program string
	if exec == nil && IsTestMode() {
		// Auto-constructed (DefaultConfig()/LoadConfig()) and running under go test:
		// never resolve to a real launchable program. Tests that intentionally need
		// real program resolution pass an explicit executor (see TestDefaultConfig).
		program = testModeSentinelProgram
	} else {
		var err error
		program, err = cfg.GetClaudeCommand()
		if err != nil {
			log.Error("failed to get claude command", "err", err)
			program = defaultProgram
		}
	}

	availablePrograms := cfg.GetAvailablePrograms()

	cfg.ListenAddress = "localhost:8543"
	cfg.DefaultProgram = program
	cfg.AutoYes = false
	cfg.DaemonPollInterval = 1000
	cfg.BranchPrefix = func() string {
		user, err := user.Current()
		if err != nil || user == nil || user.Username == "" {
			log.Error("failed to get current user", "err", err)
			return "session/"
		}
		return fmt.Sprintf("%s/", strings.ToLower(user.Username))
	}()
	cfg.DetectNewSessions = true
	cfg.SessionDetectionInterval = 5000
	cfg.StateRefreshInterval = 3000
	cfg.LogsEnabled = true
	cfg.LogsDir = ""    // Empty string means use default location
	cfg.LogMaxSize = 10 // 10MB
	cfg.LogMaxFiles = 5 // Keep 5 rotated files
	cfg.LogMaxAge = 30  // 30 days
	cfg.LogCompress = true
	cfg.UseSessionLogs = true
	cfg.TmuxSessionPrefix = "staplersquad_"  // Default prefix for backward compatibility
	cfg.PerformBackgroundHealthChecks = true // Enabled by default for automated session maintenance
	cfg.KeyCategories = getDefaultKeyCategories()
	cfg.VCSPreference = "auto" // Default to auto-detection (prefer JJ if available)
	cfg.AvailablePrograms = availablePrograms
	cfg.TmuxExecGate = TmuxExecGateConfig{
		Slots: defaultTmuxExecGateSlots,
	}
	cfg.Hibernation = HibernationConfig{
		Enabled:                   true,
		IdleTimeoutMinutes:        20,
		ResourcePressureThreshold: 85,
		RetentionDays:             30,
	}
	cfg.Capacity = CapacityConfig{}.CapacityConfigOrDefault()
	cfg.HandoffSummary = HandoffSummaryConfig{}.HandoffSummaryConfigOrDefault()
	cfg.Quota = QuotaConfig{}.QuotaConfigOrDefault()
	// Initialize SessionDefaults maps so callers never encounter nil maps.
	// LoadConfigFromPath applies the same guards after JSON decode; DefaultConfig
	// must mirror them so the two code paths are equivalent.
	cfg.SessionDefaults.Profiles = make(map[string]ProfileDefaults)
	cfg.SessionDefaults.EnvVars = make(map[string]string)
	cfg.SessionDefaults.Tags = []string{}
	cfg.SessionDefaults.DirectoryRules = []DirectoryRule{}
	cfg.SessionDefaults.Aliases = []AliasConfig{}
	// Escape analytics defaults. LoadConfigFromPath applies the same defaults
	// after JSON decode (for fields absent from an existing config.json);
	// DefaultConfig must mirror them so the two code paths are equivalent.
	cfg.EscapeAnalyticsCaptureLevel = "summary"
	defaultEscapeSamplingRate := 1.0
	cfg.EscapeAnalyticsSamplingRate = &defaultEscapeSamplingRate
	cfg.EscapeAnalyticsMaxRowsPerSession = 10000
	cfg.EscapeAnalyticsRetentionDays = 7
	// Apply environment variable overrides (never log the value).
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.AnthropicAPIKey = v
	}
	if v := os.Getenv("SLACK_WEBHOOK_URL"); v != "" {
		cfg.slackWebhookURLOverride = v
	}
	if v := os.Getenv("SLACK_SIGNING_SECRET"); v != "" {
		cfg.slackSigningSecretOverride = v
	}
	return cfg
}

// OneOffBaseDirOrDefault returns the resolved one-off base directory.
// If OneOffBaseDir is empty, it returns "~/oneoff" with ~ expanded to the
// current user's home directory. The directory is NOT created here — call
// namegen.GenerateAndCreate to create it on first use.
func (c *Config) OneOffBaseDirOrDefault() (string, error) {
	dir := c.OneOffBaseDir
	if dir == "" {
		dir = "~/oneoff"
	}
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand home dir: %w", err)
		}
		dir = filepath.Join(home, dir[2:])
	} else if dir == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand home dir: %w", err)
		}
		dir = home
	}
	return dir, nil
}

// HibernationCheckpointDirOrDefault returns the resolved hibernation checkpoint directory.
// If CheckpointDir is empty, it returns "~/.stapler-squad/checkpoints" with ~ expanded.
// The directory is NOT created here — the checkpoint writer creates it on first use.
func (c *Config) HibernationCheckpointDirOrDefault() (string, error) {
	dir := c.Hibernation.CheckpointDir
	if dir == "" {
		dir = "~/.stapler-squad/checkpoints"
	}
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand home dir: %w", err)
		}
		dir = filepath.Join(home, dir[2:])
	} else if dir == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand home dir: %w", err)
		}
		dir = home
	}
	return dir, nil
}

// TriageArtifactDirOrDefault returns the resolved triage artifact directory.
// Triage workers write their planning files here instead of into the item's repo.
// Always defaults to "~/.stapler-squad/triage-artifacts".
func (c *Config) TriageArtifactDirOrDefault() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand home dir: %w", err)
	}
	return filepath.Join(home, ".stapler-squad", "triage-artifacts"), nil
}

// HeadlessFailureCaptureDirOrDefault returns the resolved directory for durable
// headless (triage/review claude -p) failure captures — see
// session.WriteHeadlessFailureCapture. Always defaults to
// "~/.stapler-squad/headless-failures".
func (c *Config) HeadlessFailureCaptureDirOrDefault() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand home dir: %w", err)
	}
	return filepath.Join(home, ".stapler-squad", "headless-failures"), nil
}

// BacklogAttachmentDirOrDefault returns the resolved backlog attachment directory.
// Uploaded images referenced from backlog item descriptions are stored here,
// durably (unlike the 24h temp paste dir) since they're linked from persisted
// markdown text. Always defaults to "~/.stapler-squad/backlog-attachments".
func (c *Config) BacklogAttachmentDirOrDefault() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand home dir: %w", err)
	}
	return filepath.Join(home, ".stapler-squad", "backlog-attachments"), nil
}

// PromptCacheDirOrDefault returns the resolved directory for temp-file-backed
// session launch prompts (see Instance.promptArg). Always defaults to
// "~/.stapler-squad/prompt-cache".
func (c *Config) PromptCacheDirOrDefault() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand home dir: %w", err)
	}
	return filepath.Join(home, ".stapler-squad", "prompt-cache"), nil
}

// NewProjectBaseDirOrDefault returns the resolved new-project base directory.
// If NewProjectBaseDir is empty, it defaults to "~/Projects" with ~ expanded.
func (c *Config) NewProjectBaseDirOrDefault() (string, error) {
	dir := c.NewProjectBaseDir
	if dir == "" {
		dir = "~/Projects"
	}
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand home dir: %w", err)
		}
		dir = filepath.Join(home, dir[2:])
	} else if dir == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand home dir: %w", err)
		}
		dir = home
	}
	return dir, nil
}

// AnalyticsMaxRowsOrDefault returns the configured max analytics rows, or 100_000
// if not set (zero value).
func (c *Config) AnalyticsMaxRowsOrDefault() int {
	if c.AnalyticsMaxRows <= 0 {
		return 100_000
	}
	return c.AnalyticsMaxRows
}

// MaxAutoReworkIterationsOrDefault returns the configured rework-cap ceiling, or 20
// if not set (zero value) or c is nil (BacklogService's cfg is nil in some test setups).
// Raised from 3 to 20: 3 was tripping routinely on real, ultimately-fixable items
// (e.g. a multi-round diff/review-harness flake, or a straightforward merge conflict)
// well before the work was actually stuck, forcing manual "Reopen for Revision" clicks
// for otherwise-recoverable items. Genuinely stuck items still get caught — just
// later — and per-item overrides (BacklogItemData.ReworkCapOverride) exist for cases
// that need to go further still.
func (c *Config) MaxAutoReworkIterationsOrDefault() int {
	if c == nil || c.MaxAutoReworkIterations <= 0 {
		return 20
	}
	return c.MaxAutoReworkIterations
}

// maxConcurrentBacklogWorkItemsDefault is used when the config value is unset (0
// or negative — 0 concurrency would wedge the queue forever).
// maxConcurrentBacklogWorkItemsHardCeiling caps how high the setting can go even via a
// modified frontend request, to guard against reintroducing the 2026-07-12 OOM.
const (
	maxConcurrentBacklogWorkItemsDefault     = 2
	maxConcurrentBacklogWorkItemsHardCeiling = 10
)

// MaxConcurrentBacklogWorkItemsOrDefault returns the configured backlog work-item
// concurrency cap, clamped to [1, maxConcurrentBacklogWorkItemsHardCeiling]. Falls back
// to the default (2) if unset (<=0) or c is nil.
func (c *Config) MaxConcurrentBacklogWorkItemsOrDefault() int {
	if c == nil || c.MaxConcurrentBacklogWorkItems <= 0 {
		return maxConcurrentBacklogWorkItemsDefault
	}
	if c.MaxConcurrentBacklogWorkItems > maxConcurrentBacklogWorkItemsHardCeiling {
		return maxConcurrentBacklogWorkItemsHardCeiling
	}
	return c.MaxConcurrentBacklogWorkItems
}

// AutoSpawnReadyItemsOrDefault reports whether "ready" items should be automatically
// dequeued and spawned — in priority order, respecting the WIP cap — the moment a
// slot frees up, without a human manually clicking "Spawn Session". Defaults to true
// (nil or c == nil); pass explicit false to require manual spawning instead.
func (c *Config) AutoSpawnReadyItemsOrDefault() bool {
	if c == nil || c.AutoSpawnReadyItems == nil {
		return true
	}
	return *c.AutoSpawnReadyItems
}

// AnalyticsMaxAgeDaysOrDefault returns the configured max analytics age in days,
// or 90 if not set (zero value).
func (c *Config) AnalyticsMaxAgeDaysOrDefault() int {
	if c.AnalyticsMaxAgeDays <= 0 {
		return 90
	}
	return c.AnalyticsMaxAgeDays
}

// OSCPayloadsAreRedacted returns true when OSC payload redaction is enabled (the default).
// Redaction prevents PII (clipboard contents, window titles, CWD paths) from being stored
// in escape event records. Set EscapeAnalyticsDisableOSCRedaction=true in config to opt out.
func (c *Config) OSCPayloadsAreRedacted() bool {
	return !c.EscapeAnalyticsDisableOSCRedaction
}

// GetClaudeCommand attempts to find the "claude" command in the user's shell
// It checks in the following order:
// 1. Shell alias resolution (proxy-claude, then claude)
// 2. PATH lookup
//
// If both fail, it returns an error.
func (c *Config) GetClaudeCommand() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash" // Default to bash if SHELL is not set
	}

	// Try to resolve aliases for both proxy-claude and claude
	candidates := []string{"proxy-claude", "claude", "claude-code", "gemini", "agy"}

	for _, candidate := range candidates {
		// Attempt to get the alias definition from the shell
		var shellCmd string
		if strings.Contains(shell, "zsh") {
			// For zsh, use 'alias <name>' to get the full definition
			shellCmd = fmt.Sprintf("source ~/.zshrc &>/dev/null || true; alias %s 2>/dev/null || which %s 2>/dev/null", candidate, candidate)
		} else if strings.Contains(shell, "bash") {
			// For bash, use 'alias <name>' to get the full definition
			shellCmd = fmt.Sprintf("source ~/.bashrc &>/dev/null || true; alias %s 2>/dev/null || which %s 2>/dev/null", candidate, candidate)
		} else {
			shellCmd = fmt.Sprintf("which %s", candidate)
		}

		cmd := c.executor.Command(shell, "-c", shellCmd)
		output, err := c.executor.Output(cmd)
		if err == nil && len(output) > 0 {
			result := strings.TrimSpace(string(output))
			if result != "" {
				// Check if it's an alias definition
				// Formats:
				// 1. "claude: aliased to /path/to/command" (zsh alias output)
				// 2. "alias proxy-claude='command'" (bash/zsh alias definition)
				// 3. "proxy-claude='command'" (simplified alias format)
				// 4. "/path/to/command" (direct path from which)

				if strings.Contains(result, "aliased to ") {
					// Format: "name: aliased to /path/to/command"
					// Extract everything after "aliased to "
					parts := strings.SplitN(result, "aliased to ", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1]), nil
					}
				} else if strings.Contains(result, "alias ") {
					// Extract the command from alias definition
					// Pattern: alias name='command' or alias name="command"
					aliasRegex := regexp.MustCompile(`alias\s+\S+\s*=\s*['"](.+?)['"]`)
					matches := aliasRegex.FindStringSubmatch(result)
					if len(matches) > 1 {
						return matches[1], nil
					}
				} else if strings.Contains(result, "=") && (strings.Contains(result, "'") || strings.Contains(result, "\"")) {
					// Format: proxy-claude='command'
					aliasRegex := regexp.MustCompile(`\S+\s*=\s*['"](.+?)['"]`)
					matches := aliasRegex.FindStringSubmatch(result)
					if len(matches) > 1 {
						return matches[1], nil
					}
				} else {
					// It's just a path from 'which'
					return result, nil
				}
			}
		}
	}

	// Fallback: try to find in PATH directly
	for _, candidate := range candidates {
		path, err := c.executor.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("claude command not found in aliases or PATH")
}

// GetAvailablePrograms returns a list of all detected CLI programs.
func (c *Config) GetAvailablePrograms() []string {
	programs := []string{}
	seen := make(map[string]bool)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	candidates := []string{"proxy-claude", "claude", "claude-code", "gemini", "agy"}

	for _, candidate := range candidates {
		var shellCmd string
		if strings.Contains(shell, "zsh") {
			shellCmd = fmt.Sprintf("source ~/.zshrc &>/dev/null || true; which %s 2>/dev/null", candidate)
		} else if strings.Contains(shell, "bash") {
			shellCmd = fmt.Sprintf("source ~/.bashrc &>/dev/null || true; which %s 2>/dev/null", candidate)
		} else {
			shellCmd = fmt.Sprintf("which %s", candidate)
		}

		cmd := c.executor.Command(shell, "-c", shellCmd)
		if output, err := c.executor.Output(cmd); err == nil {
			path := strings.TrimSpace(string(output))
			if path != "" && !seen[path] {
				programs = append(programs, path)
				seen[path] = true
			}
		}
	}
	return programs
}

// GetClaudeCommand is a package-level convenience wrapper using the default executor.
// Callers that need a custom executor should use NewConfigWithExecutor(exec).GetClaudeCommand().
func GetClaudeCommand() (string, error) {
	return NewConfig().GetClaudeCommand()
}

// GetAvailablePrograms is a package-level convenience wrapper using the default executor.
// Callers that need a custom executor should use NewConfigWithExecutor(exec).GetAvailablePrograms().
func GetAvailablePrograms() []string {
	return NewConfig().GetAvailablePrograms()
}

func LoadConfig() *Config {
	configDir, err := GetConfigDir()
	if err != nil {
		log.Error("failed to get config directory", "err", err)
		return DefaultConfig()
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	cfg, err := LoadConfigFromPath(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return loadConfigWithDefaultFallback(configPath)
		}
		log.Warn("failed to load config file", "err", err)
		return DefaultConfig()
	}

	return cfg
}

// loadConfigWithDefaultFallback handles a not-yet-created configPath by writing
// DefaultConfig() as the initial file. It takes the same per-path lock saveConfig
// uses (see saveConfigMu's doc comment) and re-checks existence under that lock
// before writing: without the re-check, a caller that observed os.IsNotExist an
// instant before a concurrent, legitimate config.SaveConfig() call (e.g. a test
// seeding a profile) could still land its default-config write *after* that
// real save, silently clobbering it back to defaults. Re-checking under the
// same lock the real save also uses makes the two calls mutually exclusive and
// ordered, so whichever wins the race, the file reflects that decision instead
// of two writers reordering across the check-then-write gap.
func loadConfigWithDefaultFallback(configPath string) *Config {
	pathLock := saveConfigLockFor(configPath)
	pathLock.Lock()
	defer pathLock.Unlock()

	if cfg, err := LoadConfigFromPath(configPath); err == nil {
		return cfg
	}

	defaultCfg := DefaultConfig()
	if saveErr := saveConfigLocked(defaultCfg, configPath); saveErr != nil {
		log.Warn("failed to save default config", "err", saveErr)
	}
	return defaultCfg
}

// saveConfigMu serializes the write-tmp-then-rename sequence in saveConfig,
// keyed per configPath. Without it, two concurrent callers targeting the same
// tmpPath (e.g. two goroutines independently calling
// GetOrCreateEncryptionKey/SaveConfig) can interleave: both os.WriteFile the
// shared tmpPath, then both os.Rename it — the first rename succeeds and
// consumes tmpPath, so the second fails with "no such file or directory" and,
// in tighter interleavings, a torn write leaves config.json holding malformed
// JSON that the next LoadConfig call silently falls back to DefaultConfig()
// over (losing whatever was there). Keyed per path (rather than one global
// mutex) so concurrent saves to different configPaths — e.g. distinct
// per-instance state dirs under state-isolation, see .claude/docs/state-isolation.md
// — aren't needlessly serialized against each other.
var saveConfigMu sync.Map //nolint:gochecknoglobals // per-configPath *sync.Mutex, serializes concurrent saveConfig callers sharing the same tmpPath

// saveConfigLockFor returns the *sync.Mutex guarding writes to configPath,
// creating it on first use. LoadOrStore's atomicity is what makes this safe
// under concurrent callers racing to lock the same never-before-seen path.
func saveConfigLockFor(configPath string) *sync.Mutex {
	lock, _ := saveConfigMu.LoadOrStore(configPath, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// saveConfig saves the configuration to disk atomically via a temp-file rename.
// Accepts an optional explicit path; when omitted the path is derived from GetConfigDir().
func saveConfig(config *Config, paths ...string) error {
	var configPath string
	if len(paths) > 0 && paths[0] != "" {
		configPath = paths[0]
	} else {
		configDir, err := GetConfigDir()
		if err != nil {
			return fmt.Errorf("failed to get config directory: %w", err)
		}
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
		configPath = filepath.Join(configDir, ConfigFileName)
	}

	pathLock := saveConfigLockFor(configPath)
	pathLock.Lock()
	defer pathLock.Unlock()

	return saveConfigLocked(config, configPath)
}

// saveConfigLocked performs the marshal + write-tmp-then-rename sequence.
// Callers must hold saveConfigLockFor(configPath) — factored out so
// loadConfigWithDefaultFallback can share the same critical section as
// saveConfig without re-entering (and deadlocking on) the same mutex.
func saveConfigLocked(config *Config, configPath string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to a uniquely-named temp file in the same directory, then rename
	// for atomicity. The name must be unique per call (os.CreateTemp's random
	// suffix) rather than a fixed "config.json.tmp": two concurrent saveConfig
	// calls targeting the same directory previously raced on that shared name,
	// each truncating the other's in-progress write via O_TRUNC, corrupting the
	// JSON, and racing os.Rename against a tmpPath the other had already moved.
	// The pathLock above still serializes callers so the final rename order
	// matches call order instead of being left to goroutine scheduling.
	tmpFile, err := os.CreateTemp(filepath.Dir(configPath), filepath.Base(configPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}
	tmpPath := tmpFile.Name()
	// os.CreateTemp creates the file 0600; match the previous os.WriteFile mode
	// so the renamed config.json keeps its historical permissions.
	chmodErr := tmpFile.Chmod(0644)
	_, writeErr := tmpFile.Write(data)
	closeErr := tmpFile.Close()
	if chmodErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to chmod temp config: %w", chmodErr)
	}
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp config: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp config: %w", closeErr)
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup
		return fmt.Errorf("failed to rename config: %w", err)
	}
	return nil
}

// SaveConfig exports the saveConfig function for use by other packages.
func SaveConfig(config *Config) error {
	return saveConfig(config)
}

// LoadConfigFromPath loads and parses a config file from an explicit path.
// Returns the config and any error encountered.
func LoadConfigFromPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply zero-value defaults for newly-added fields.
	if cfg.KeyCategories == nil {
		cfg.KeyCategories = getDefaultKeyCategories()
	}
	if cfg.SessionDefaults.Profiles == nil {
		cfg.SessionDefaults.Profiles = make(map[string]ProfileDefaults)
	}
	if cfg.SessionDefaults.EnvVars == nil {
		cfg.SessionDefaults.EnvVars = make(map[string]string)
	}
	if cfg.SessionDefaults.Tags == nil {
		cfg.SessionDefaults.Tags = []string{}
	}
	if cfg.SessionDefaults.DirectoryRules == nil {
		cfg.SessionDefaults.DirectoryRules = []DirectoryRule{}
	}
	if cfg.SessionDefaults.Aliases == nil {
		cfg.SessionDefaults.Aliases = []AliasConfig{}
	}
	if cfg.ConfigVersion == 0 {
		cfg.ConfigVersion = 1
	}

	// Apply defaults for escape analytics fields.
	if cfg.EscapeAnalyticsCaptureLevel == "" {
		cfg.EscapeAnalyticsCaptureLevel = "summary"
	}
	if cfg.EscapeAnalyticsSamplingRate == nil {
		defaultRate := 1.0
		cfg.EscapeAnalyticsSamplingRate = &defaultRate
	}
	if cfg.EscapeAnalyticsMaxRowsPerSession == 0 {
		cfg.EscapeAnalyticsMaxRowsPerSession = 10000
	}
	if cfg.EscapeAnalyticsRetentionDays == 0 {
		cfg.EscapeAnalyticsRetentionDays = 7
	}

	// Validate escape analytics fields.
	switch cfg.EscapeAnalyticsCaptureLevel {
	case "full", "summary", "off":
		// valid
	default:
		cfg.EscapeAnalyticsCaptureLevel = "summary"
	}
	if *cfg.EscapeAnalyticsSamplingRate < 0 {
		zero := 0.0
		cfg.EscapeAnalyticsSamplingRate = &zero
	}
	if *cfg.EscapeAnalyticsSamplingRate > 1.0 {
		one := 1.0
		cfg.EscapeAnalyticsSamplingRate = &one
	}

	// Unmarshaling produces a zero Config with no executor; initialize it now
	// so GetClaudeCommand / GetAvailablePrograms don't panic on nil executor.
	cfg.executor = newTimeoutCommandExecutor(5 * time.Second)

	cfg.Capacity = cfg.Capacity.CapacityConfigOrDefault()
	cfg.HandoffSummary = cfg.HandoffSummary.HandoffSummaryConfigOrDefault()
	cfg.Quota = cfg.Quota.QuotaConfigOrDefault()

	// Apply environment variable overrides (never log the value).
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.AnthropicAPIKey = v
	}
	if v := os.Getenv("SLACK_WEBHOOK_URL"); v != "" {
		cfg.slackWebhookURLOverride = v
	}
	if v := os.Getenv("SLACK_SIGNING_SECRET"); v != "" {
		cfg.slackSigningSecretOverride = v
	}

	return &cfg, nil
}

// getDefaultKeyCategories returns the default key category mappings
func getDefaultKeyCategories() map[string]string {
	return map[string]string{
		// Session Management
		"n":     "Session Management",
		"D":     "Session Management",
		"enter": "Session Management",
		"c":     "Session Management",
		"r":     "Session Management",

		// Git Integration
		"g": "Git Integration",
		"P": "Git Integration",

		// Navigation
		"up":    "Navigation",
		"down":  "Navigation",
		"left":  "Navigation",
		"right": "Navigation",
		"j":     "Navigation",
		"k":     "Navigation",
		"h":     "Navigation",
		"l":     "Navigation",
		"/":     "Navigation",
		"s":     "Navigation",

		// Organization
		"f":     "Organization",
		"C":     "Organization",
		"space": "Organization",

		// System
		"tab": "System",
		"?":   "System",
		"q":   "System",
		"esc": "System",
	}
}

// GetKeyCategoryForKey returns the category for a specific key, or empty string if not found
func (c *Config) GetKeyCategoryForKey(key string) string {
	if c.KeyCategories == nil {
		return ""
	}
	return c.KeyCategories[key]
}

// SetKeyCategory updates the category for a specific key
func (c *Config) SetKeyCategory(key, category string) {
	if c.KeyCategories == nil {
		c.KeyCategories = make(map[string]string)
	}
	c.KeyCategories[key] = category
}

// RemoveKeyCategory removes the category mapping for a specific key
func (c *Config) RemoveKeyCategory(key string) {
	if c.KeyCategories != nil {
		delete(c.KeyCategories, key)
	}
}

// GetOrCreateEncryptionKey returns the 32-byte AES-256-GCM key for local data encryption.
// Generates and persists a new key on first call. Non-fatal errors during save are logged.
func (c *Config) GetOrCreateEncryptionKey() ([]byte, error) {
	c.lazyMu.Lock()
	defer c.lazyMu.Unlock()

	if c.MachineEncryptionKey != "" {
		data, err := base64.StdEncoding.DecodeString(c.MachineEncryptionKey)
		if err == nil && len(data) == 32 {
			return data, nil
		}
		// If existing key is invalid, regenerate
		log.WarningLog().Printf("[Config] existing encryption key is invalid, regenerating")
	}

	// Generate new 32-byte key
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate encryption key: %w", err)
	}

	c.MachineEncryptionKey = base64.StdEncoding.EncodeToString(key)

	// Persist to disk; non-fatal if it fails
	if err := SaveConfig(c); err != nil {
		log.WarningLog().Printf("[Config] failed to persist encryption key: %v", err)
	}

	return key, nil
}

// GetOrCreateClaimantHostID returns this process/config directory's stable ClaimantHostID,
// generating and persisting a new random UUID on first call. See the ClaimantHostID field
// doc comment for what this identifier is (and is not) used for.
func (c *Config) GetOrCreateClaimantHostID() (string, error) {
	c.lazyMu.Lock()
	defer c.lazyMu.Unlock()

	if c.ClaimantHostID != "" {
		return c.ClaimantHostID, nil
	}

	c.ClaimantHostID = uuid.New().String()

	// Persist to disk; non-fatal if it fails
	if err := SaveConfig(c); err != nil {
		log.WarningLog().Printf("[Config] failed to persist claimant host id: %v", err)
	}

	return c.ClaimantHostID, nil
}

// SlackWebhookURLOverride returns the SLACK_WEBHOOK_URL environment variable
// value captured at load time, or "" if it was unset. Exported because
// server/services (which resolves the effective Slack webhook URL per
// ADR-001: env override first, else decrypt the stored ciphertext) cannot
// read the unexported slackWebhookURLOverride field directly.
func (c *Config) SlackWebhookURLOverride() string {
	return c.slackWebhookURLOverride
}

// SlackSigningSecretOverride returns the SLACK_SIGNING_SECRET environment
// variable value captured at load time, or "" if it was unset. See
// SlackWebhookURLOverride for why this getter exists.
func (c *Config) SlackSigningSecretOverride() string {
	return c.slackSigningSecretOverride
}

// GetFeatureFlag returns the persisted enabled state of the named feature flag.
// Absent key returns false — all feature flags default to disabled.
// Currently recognized flags:
//
//	"backlog" — enables the Backlog tab and backlog lifecycle controller.
//	"webhook_triggers" — registers POST /webhooks/github and POST /webhooks/generic/{slug}.
//	"pr_event_webhooks" — reacts to check_run/workflow_run/pull_request_review/issue_comment
//	  GitHub deliveries on /webhooks/github by immediately reconciling a matching pr_pending
//	  item, instead of waiting for PRStatusPoller's next tick. Independently toggleable from
//	  "webhook_triggers", but has no effect unless "webhook_triggers" is also enabled (that
//	  flag gates whether the route is registered at all).
func (c *Config) GetFeatureFlag(name string) bool {
	if c == nil || c.FeatureFlags == nil {
		return false
	}
	return c.FeatureFlags[name]
}

// SetFeatureFlag sets the named feature flag and persists the config to disk.
func (c *Config) SetFeatureFlag(name string, value bool) error {
	if c.FeatureFlags == nil {
		c.FeatureFlags = make(map[string]bool)
	}
	c.FeatureFlags[name] = value
	return SaveConfig(c)
}

// ImportSessionEnabled reports whether the import-external-session feature
// (Phase 1: ssq-mux single-session import) is enabled. Unlike GetFeatureFlag,
// this is a plain environment variable rather than a persisted config flag —
// the feature involves signaling live, unmanaged processes (SIGSTOP/SIGCONT)
// outside Stapler Squad's own supervision, so it defaults to a deliberate,
// explicit opt-in per deployment/session rather than a UI-toggleable
// persisted setting. Re-read on every call (not cached) so it can be flipped
// without a server restart, matching the re-read behavior of GetFeatureFlag.
func ImportSessionEnabled() bool {
	return os.Getenv("STAPLER_SQUAD_ENABLE_SESSION_IMPORT") == "true"
}
