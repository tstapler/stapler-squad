package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/log"
)

// HookName is a typed constant for the built-in hooks that can be injected.
type HookName string

const (
	HookPermissionApproval HookName = "permission_approval" // maps to PermissionRequest event
	HookStopNotification   HookName = "stop_notification"   // maps to Stop event
	HookPreToolLogging     HookName = "pre_tool_logging"    // maps to PreToolUse event
	HookPostToolLogging    HookName = "post_tool_logging"   // maps to PostToolUse event
	HookPromptSubmit       HookName = "prompt_submit"       // maps to UserPromptSubmit event

	// HookGitDriftCheck also maps to PostToolUse, but on its own dedicated endpoint
	// (distinct from HookPostToolLogging's) so it can be injected independently of
	// the generic hooks a manually-created session opts into. Scoped strictly to
	// autonomous backlog work sessions — see spawnSessionAfterGates in
	// server/services/backlog_service_triage.go, the only call site that injects
	// it, and hook_receiver_drift.go for the receiver. Fires the same branch-drift
	// check that gates review (BUG-044) right after every git commit/push, so an
	// autonomous session notices and can self-correct immediately instead of only
	// learning about drift from a review verdict hours or days later.
	HookGitDriftCheck HookName = "git_drift_check" // maps to PostToolUse event

	// HookExtensionHealth maps to the pi approval extension's health-ping endpoint
	// (/api/hooks/pi-extension-loaded, pi-support Epic 4.2). Unlike the HookNames
	// above, this one is never passed to InjectHooksConfig/RemoveHooksConfig — the
	// pi extension reads this URL from a value baked into ssqApprovalExtensionTemplate
	// at render time (cmd/ssq-hooks/main.go), not from a session's injected hooks.json.
	// It's added to this registry anyway for consistency: one place that knows every
	// hook-shaped URL this server serves, per hookEndpoints' own doc comment.
	HookExtensionHealth HookName = "pi_extension_health" // maps to the pi extension's load-confirmation ping
)

// hookEventName maps a HookName to the Claude Code hooks.* key.
var hookEventName = map[HookName]string{
	HookPermissionApproval: "PermissionRequest",
	HookStopNotification:   "Stop",
	HookPreToolLogging:     "PreToolUse",
	HookPostToolLogging:    "PostToolUse",
	HookPromptSubmit:       "UserPromptSubmit",
	HookGitDriftCheck:      "PostToolUse",
}

// hookBaseURLFn resolves the base URL (scheme + host + port) used to build hook callback
// endpoints. It defaults to the historical localhost:8543 address for backward compatibility
// with callers that never wire a real server (e.g. existing unit tests), and is overridden via
// SetHookBaseURLFn during real server wiring (server.go's wireDepsIntoServer) with a closure
// that reads the server's real listen address lazily — e.g. via srv.GetAddr() — so hook URLs
// are never snapshotted before the server has bound its real port (PORT=0 support).
//
// hookBaseURLFnMu guards concurrent read/write of the closure from a test goroutine (setter)
// and every other parallel test in this package that resolves hook URLs (reader) — required
// under -race since the t.Parallel() rollout made this package's tests run concurrently.
// Modeled on backlog_service_triage.go's testTriageCompleteHook.
var (
	hookBaseURLFnMu sync.Mutex
	hookBaseURLFn   = func() string { return "http://localhost:8543" }
)

// SetHookBaseURLFn overrides the base URL function used when building hook endpoint URLs via
// hookEndpoints. Call once during server wiring; passing nil is a no-op.
func SetHookBaseURLFn(fn func() string) {
	if fn == nil {
		return
	}
	hookBaseURLFnMu.Lock()
	defer hookBaseURLFnMu.Unlock()
	hookBaseURLFn = fn
}

// getHookBaseURLFn returns the currently-configured base URL closure.
func getHookBaseURLFn() func() string {
	hookBaseURLFnMu.Lock()
	defer hookBaseURLFnMu.Unlock()
	return hookBaseURLFn
}

// hookCommandReferencesURL reports whether curlCmd is the hook command built for url (see the
// curl command template in InjectHooksConfig/InjectHookConfig, which always wraps the URL in
// single quotes: `-X POST '<url>' -H ...`). Matching on the quoted form, not a bare
// strings.Contains(command, url), is required because some hook URLs are string prefixes of
// others -- e.g. HookPostToolLogging's ".../api/hooks/post-tool-use" is a strict prefix of
// HookGitDriftCheck's ".../api/hooks/post-tool-use-drift-check". A bare substring check treats
// the shorter URL as "already present" whenever the longer one's command exists, which (via
// Go's randomized map iteration order over the `wanted` set in InjectHooksConfig) intermittently
// dropped one of the two PostToolUse hooks entirely and made RemoveHooksConfig delete the
// survivor's group too -- the root cause of the flaky
// TestRemoveHooksConfig_should_StripOnlyTheNamedHook_When_MultipleHooksPresent failure. The
// quote characters bound the match so a strict-prefix URL can never falsely match a longer one.
func hookCommandReferencesURL(curlCmd, url string) bool {
	return strings.Contains(curlCmd, "'"+url+"'")
}

// hookEndpoints builds the HookName -> URL map fresh from baseURLFn() on every call (never
// cached into a package-level map), so hook URLs written into a session's settings always
// reflect the base URL current at hook-injection time rather than one baked in at server- or
// package-construction time.
func hookEndpoints(baseURLFn func() string) map[HookName]string {
	base := baseURLFn()
	return map[HookName]string{
		HookPermissionApproval: base + "/api/hooks/permission-request",
		HookStopNotification:   base + "/api/hooks/stop",
		HookPreToolLogging:     base + "/api/hooks/pre-tool-use",
		HookPostToolLogging:    base + "/api/hooks/post-tool-use",
		HookPromptSubmit:       base + "/api/hooks/prompt-submit",
		HookGitDriftCheck:      base + "/api/hooks/post-tool-use-drift-check",
		HookExtensionHealth:    base + "/api/hooks/pi-extension-loaded",
	}
}

// RemoteHookTarget carries the remote-session-specific values needed to route a generated
// PermissionRequest hook command at a RemoteApprovalRelay's remote-side Unix socket
// (session/sshremote/approval_relay.go, ssh-remote-workspaces Phase 5 Epic 5.1) instead of
// hookBaseURLFn()'s http://localhost:8543 default -- Phase 5 Epic 5.2 / ADR-003
// ("Multiplex the Approval Callback Over the Existing SSH Connection").
//
// SocketPath and BearerToken are two independently meaningful strings a caller could
// otherwise transpose at a call site with no compiler error -- exactly what
// the `primitive-obsession-checklist` skill exists to catch -- so this is a named
// struct, not two positional string parameters threaded through InjectHooksConfig.
//
// PRODUCTION WIRING: the real session-creation call site (server/services/session_service.go's
// setupRemoteApprovalHooks, called from CreateSession for a remote session) does NOT build one
// of these and pass it to InjectHooksConfig via WithRemoteHookTarget below -- it calls
// InjectHookConfigRemote (approval_handler.go) directly, which builds a RemoteHookTarget from
// the session's just-started *sshremote.RemoteApprovalRelay and reuses remoteApprovalHookCommand
// (this file). RemoteHookTarget itself is shared by both entry points; WithRemoteHookTarget
// below remains available for InjectHooksConfig's other production callers
// (server/mcp/tools_lifecycle.go, server/mcp/tools_github.go,
// server/services/backlog_service_triage.go) if any of them ever need to route a
// PermissionRequest hook at a remote session's relay the same way:
//
//	token, _ := relay.BearerToken()
//	target := services.RemoteHookTarget{
//	    SocketPath:  sshremote.RemoteApprovalSocketPath(inst.GetEffectiveRootDir(), inst.GetStableID()),
//	    BearerToken: token,
//	}
//	services.InjectHooksConfig(inst.GetEffectiveRootDir(), inst.Title, hooks,
//	    services.WithRemoteHookTarget(target))
//
// None of those four callers pass this option today, so every hook command they generate --
// local or remote session alike -- is byte-identical to pre-Phase-5 behavior.
type RemoteHookTarget struct {
	// SocketPath is the remote-host Unix domain socket path RemoteApprovalRelay reads
	// from -- see sshremote.RemoteApprovalSocketPath(basePath, stableSessionID).
	SocketPath string
	// BearerToken is the relay's current bearer credential (sshremote.RemoteApprovalRelay.
	// BearerToken), embedded in the generated hook command's JSON payload so the relay's
	// verifyToken accepts it.
	BearerToken string
}

// injectHookOptions is InjectHooksConfig/InjectHookOption's internal option state.
// Unexported: callers configure it only through InjectHookOption functions returned by
// WithRemoteHookTarget (and any future With* option), never by constructing this directly --
// the functional-options pattern this package's public API already leans on elsewhere
// keeping the exported InjectHooksConfig signature additive rather than breaking.
type injectHookOptions struct {
	remote *RemoteHookTarget
}

// InjectHookOption configures a single InjectHooksConfig call.
type InjectHookOption func(*injectHookOptions)

// WithRemoteHookTarget routes the generated PermissionRequest hook command at target's
// remote Unix socket instead of hookBaseURLFn()'s HTTP endpoint (Phase 5 Epic 5.2). Every
// other hook type (Stop, PreToolUse, PostToolUse, UserPromptSubmit, git-drift-check)
// deliberately keeps using the HTTP command even when this option is supplied:
// RemoteApprovalRelay (session/sshremote/approval_relay.go) decodes the socket payload's
// "request" field as raw json.RawMessage and hands it, unmodified, to
// PermissionRequestHandler.HandlePermissionRequest -- the same handler a real PermissionRequest
// HTTP hook POSTs to -- which expects a classifier.PermissionRequestPayload-shaped body
// specifically. Routing an unrelated hook type's differently-shaped JSON through the same
// socket would reach HandlePermissionRequest as a malformed/misleading request, not fail
// cleanly. Widening remote routing to other hook types is out of this epic's stated scope
// (plan.md's Story 5.2.1 acceptance criteria only exercises PermissionRequest) and would need
// its own relay-side and handler-side support first.
//
// A zero-value target (SocketPath == "") is treated the same as omitting the option
// entirely, so a caller that hasn't resolved a real relay yet never emits a broken
// UNIX-CONNECT with an empty path.
func WithRemoteHookTarget(target RemoteHookTarget) InjectHookOption {
	return func(o *injectHookOptions) {
		if target.SocketPath == "" {
			return
		}
		t := target
		o.remote = &t
	}
}

// remoteApprovalWriteTool is the executable the remote-aware PermissionRequest hook shells out
// to, writing/reading the raw JSON payload directly on RemoteApprovalRelay's Unix socket with no
// protocol framing (curl's --unix-socket still speaks HTTP framing, which the relay's raw
// json.Decoder can't parse). Requires socat installed on the remote host.
//
// UNIX-LISTEN, not UNIX-CONNECT: RemoteApprovalRelay.dial (session/sshremote/approval_relay.go)
// makes the remote sshd connect() to this socket (direct-streamlocal@openssh.com), so the hook
// script must be the listening peer. mode=0600 restricts the socket to the session's own user --
// a plain UNIX-LISTEN would otherwise let any local user on the remote host connect first, read
// the bearer token, and forge an approval decision (review-found, verified empirically).
// unlink-early clears a stale socket file from an earlier invocation before binding.
const remoteApprovalWriteTool = "socat"

// remoteApprovalHookMaxAttempts bounds how many times the hook re-listens and resends the same
// captured request after a network blip (requirements.md AC4). Paired with
// RemoteApprovalRelay.handleConnection's redelivery buffering: a retry resending identical
// request bytes gets back an already-computed decision instead of re-prompting a human.
const remoteApprovalHookMaxAttempts = 3

// remoteApprovalHookAttemptTimeoutSeconds bounds a single UNIX-LISTEN attempt's wait for a
// response. Must cover a real human decision (ApprovalHandler.approvalTimeout(), 4 minutes by
// default), not just a connection probe -- socat's own default half-close wait is 0.5s, which
// review found (verified empirically) silently drops the connection before any human could
// possibly answer, handing the hook an empty, exit-0 response; the retry loop below then never
// fires either, since a non-empty check (not exit code) is what decides "done." Both socat's -t
// and the outer `timeout` wrapper use this bound.
const remoteApprovalHookAttemptTimeoutSeconds = 270 // 4.5 min: covers the 4-minute default approval wait + margin

// remoteApprovalHookCommand builds the shell command a remote session's PermissionRequest hook
// runs to deliver its payload to target. Captures stdin once into $req via $(cat) (a later
// retry's `cat` would find stdin already exhausted), embeds it via printf's own %s substitution
// (not Go's, not shell interpolation -- $req is already complete JSON, safe to pass through
// literally), and shell-quotes every value that isn't provably safe (posixShellQuoteRemote) --
// SocketPath is caller-influenced (a session's remote working directory), so it is NOT safe to
// splice unquoted (review-found shell-injection RCE, verified empirically: a crafted path
// containing a single quote broke out of the command and ran arbitrary shell). Retries up to
// remoteApprovalHookMaxAttempts times, one second apart, on empty output (network blip or a
// timed-out wait, not merely a non-zero exit).
func remoteApprovalHookCommand(target RemoteHookTarget) string {
	socatAddr := posixShellQuoteRemote(fmt.Sprintf("UNIX-LISTEN:%s,unlink-early,mode=0600", target.SocketPath))
	return fmt.Sprintf(
		`req=$(cat); i=0; while [ $i -lt %d ]; do out=$(printf '{"token":"%s","request":%%s}' "$req" | timeout %d %s -t %d - %s); [ -n "$out" ] && { printf '%%s' "$out"; break; }; i=$((i+1)); sleep 1; done`,
		remoteApprovalHookMaxAttempts, target.BearerToken, remoteApprovalHookAttemptTimeoutSeconds+15, remoteApprovalWriteTool, remoteApprovalHookAttemptTimeoutSeconds, socatAddr,
	)
}

// hookCommandTargetsSocket reports whether curlCmd is the remote-aware hook command built by
// remoteApprovalHookCommand for socketPath. Matches on the exact quoted address
// remoteApprovalHookCommand itself produces (via the same posixShellQuoteRemote call), so a
// socket path that happens to be another's strict prefix can never falsely match (mirrors
// hookCommandReferencesURL's quote-bounded-match design).
func hookCommandTargetsSocket(curlCmd, socketPath string) bool {
	return strings.Contains(curlCmd, posixShellQuoteRemote(fmt.Sprintf("UNIX-LISTEN:%s,unlink-early,mode=0600", socketPath)))
}

// InjectHooksConfig writes (or merges) hook entries into
// <rootDir>/.claude/settings.local.json.
//
//   - HookPermissionApproval is always injected regardless of the hooks slice.
//   - Each hook entry is a curl command POSTing to the server endpoint with
//     X-CS-Session-ID set to sessionTitle.
//   - The write is atomic (temp file + rename).
//   - Idempotent: existing entries pointing to our URL are preserved.
//
// opts is variadic (InjectHookOption, e.g. WithRemoteHookTarget) so every pre-Phase-5 call
// site keeps compiling and keeps generating byte-identical local-session commands without
// modification -- see RemoteHookTarget's doc comment for what a remote-aware call looks like
// once a caller has a session's relay in hand, and its wiring-gap note for why no production
// call site passes one yet.
func InjectHooksConfig(rootDir, sessionTitle string, hooks []HookName, opts ...InjectHookOption) error {
	var cfg injectHookOptions
	for _, opt := range opts {
		opt(&cfg)
	}

	claudeDir := filepath.Join(rootDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	// Serializes the read-merge-write sequence below against InjectHookConfig and
	// RemoveHooksConfig, which independently read-modify-write the same settingsPath —
	// see settingsFileLocks' doc comment in mcp_injector.go for the lost-update hazard
	// this closes.
	defer lockSettingsPath(settingsPath)()

	wanted := wantedHookSet(hooks)

	raw, hooksMap, err := readExistingHooksSettings(settingsPath)
	if err != nil {
		return err
	}

	// Resolved once per InjectHooksConfig call (i.e. per-session, at hook-injection time), not
	// cached at package-construction time, so it reflects the server's current base URL.
	endpoints := hookEndpoints(getHookBaseURLFn())

	for hookName := range wanted {
		eventKey := hookEventName[hookName]
		url := endpoints[hookName]

		// Remote-aware branch (Phase 5 Epic 5.2 / ADR-003): only HookPermissionApproval
		// routes at the relay's socket -- see WithRemoteHookTarget's doc comment for why
		// every other hook type keeps the HTTP command even when cfg.remote is set.
		remoteTargeted := cfg.remote != nil && hookName == HookPermissionApproval
		curlCmd := buildHookCommand(remoteTargeted, cfg, url, sessionTitle)

		if hookAlreadyPresent(hooksMap[eventKey], remoteTargeted, cfg.remote, url) {
			continue
		}

		merged, err := prependHookEntry(hooksMap[eventKey], curlCmd)
		if err != nil {
			return fmt.Errorf("marshal hooks for %s: %w", eventKey, err)
		}
		hooksMap[eventKey] = merged
	}

	hooksJSON, err := json.Marshal(hooksMap)
	if err != nil {
		return fmt.Errorf("marshal hooks map: %w", err)
	}
	raw["hooks"] = json.RawMessage(hooksJSON)

	return writeSettingsAtomic(settingsPath, claudeDir, raw)
}

// wantedHookSet builds the set of hooks to inject from hooks, always including
// HookPermissionApproval, and logging (not erroring) any name InjectHooksConfig's caller
// passed that hookEventName doesn't recognize.
func wantedHookSet(hooks []HookName) map[HookName]struct{} {
	wanted := map[HookName]struct{}{HookPermissionApproval: {}}
	for _, h := range hooks {
		if _, ok := hookEventName[h]; ok {
			wanted[h] = struct{}{}
		} else {
			log.Warn("[InjectHooksConfig] unknown hook name, skipping", "name", h)
		}
	}
	return wanted
}

// readExistingHooksSettings reads settingsPath (a missing file is not an error -- InjectHooksConfig
// creates it), repairing common JSON corruption (see repairSettingsJSON) before giving up and
// starting fresh, and returns both the full top-level settings map and its parsed "hooks" sub-map.
func readExistingHooksSettings(settingsPath string) (raw map[string]json.RawMessage, hooksMap map[string]json.RawMessage, err error) {
	raw = map[string]json.RawMessage{}
	data, readErr := os.ReadFile(settingsPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, nil, fmt.Errorf("read %s: %w", settingsPath, readErr)
	}
	if len(data) > 0 {
		if unmarshalErr := json.Unmarshal(data, &raw); unmarshalErr != nil {
			log.Warn("[InjectHooksConfig] settings file has invalid JSON, attempting repair", "path", settingsPath, "err", unmarshalErr)
			if repaired, repairErr := repairSettingsJSON(data); repairErr == nil {
				_ = json.Unmarshal(repaired, &raw)
			} else {
				raw = map[string]json.RawMessage{}
			}
		}
	}

	hooksMap = map[string]json.RawMessage{}
	if hooksRaw, ok := raw["hooks"]; ok {
		_ = json.Unmarshal(hooksRaw, &hooksMap)
	}
	return raw, hooksMap, nil
}

// buildHookCommand returns the shell command InjectHooksConfig writes for one hook event --
// either remoteApprovalHookCommand's socat pipeline (remoteTargeted) or the local curl command,
// unchanged from pre-Phase-5 behavior for every non-remote-targeted call.
func buildHookCommand(remoteTargeted bool, cfg injectHookOptions, url, sessionTitle string) string {
	if remoteTargeted {
		return remoteApprovalHookCommand(*cfg.remote)
	}
	return fmt.Sprintf(
		"curl -s --max-time %d -X POST '%s' -H 'Content-Type: application/json' -H 'X-CS-Session-ID: %s' -d @-",
		hookTimeout, url, sessionTitle,
	)
}

// hookAlreadyPresent reports whether existingRaw (one event's current hookMatcherGroup list, or
// nil if the event has no entries yet) already contains a command hook targeting url (or, for a
// remote-targeted hook, remote.SocketPath).
func hookAlreadyPresent(existingRaw json.RawMessage, remoteTargeted bool, remote *RemoteHookTarget, url string) bool {
	if existingRaw == nil {
		return false
	}
	var groups []hookMatcherGroup
	if err := json.Unmarshal(existingRaw, &groups); err != nil {
		return false
	}
	for _, g := range groups {
		for _, h := range g.Hooks {
			if h.Type != "command" {
				continue
			}
			if remoteTargeted {
				if hookCommandTargetsSocket(h.Command, remote.SocketPath) {
					return true
				}
				continue
			}
			if hookCommandReferencesURL(h.Command, url) {
				return true
			}
		}
	}
	return false
}

// prependHookEntry adds a new command-hook entry (running curlCmd) as the first group ahead of
// existingRaw's groups (if any), returning the re-marshaled hookMatcherGroup list.
func prependHookEntry(existingRaw json.RawMessage, curlCmd string) (json.RawMessage, error) {
	entry := hookEntry{Type: "command", Command: curlCmd, Timeout: hookTimeout}
	group := hookMatcherGroup{Hooks: []hookEntry{entry}}

	var existing []hookMatcherGroup
	if existingRaw != nil {
		_ = json.Unmarshal(existingRaw, &existing)
	}
	merged := append([]hookMatcherGroup{group}, existing...)
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(mergedJSON), nil
}

// RemoveHooksConfig strips any previously-injected entries for the given hooks from
// <rootDir>/.claude/settings.local.json, leaving every other hook (ours or the
// user's own) untouched. It is the inverse of InjectHooksConfig, needed because a
// backlog work session's worktree/branch is reused across reopen cycles (see
// spawnSessionAfterGates in backlog_service_triage.go — same "backlog/<item>" branch
// every revision): without an explicit removal step, a hook injected while an item
// was spawned autonomously would otherwise persist in that worktree's settings file
// forever, even after a later manual ("Reopen for Revision") respawn on the same
// worktree — silently violating the "never inject into a human-driven session"
// scoping requirement HookGitDriftCheck depends on. Call this whenever spawning a
// session in a mode that must NOT have a given hook, symmetrically with the
// InjectHooksConfig call used for the mode that must.
//
// No-op (not an error) if the settings file doesn't exist or doesn't reference the
// hook — safe to call unconditionally on every spawn.
func RemoveHooksConfig(rootDir string, hooks []HookName) error {
	if len(hooks) == 0 {
		return nil
	}
	claudeDir := filepath.Join(rootDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	// Serializes the read-merge-write sequence below against InjectHookConfig and
	// InjectHooksConfig, which independently read-modify-write the same settingsPath —
	// see settingsFileLocks' doc comment in mcp_injector.go for the lost-update hazard
	// this closes.
	defer lockSettingsPath(settingsPath)()

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", settingsPath, err)
	}

	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Warn("[RemoveHooksConfig] settings file has invalid JSON, attempting repair", "path", settingsPath, "err", err)
		repaired, repairErr := repairSettingsJSON(data)
		if repairErr != nil {
			return fmt.Errorf("settings file is corrupt and could not be repaired: %w", repairErr)
		}
		if err := json.Unmarshal(repaired, &raw); err != nil {
			return fmt.Errorf("unmarshal repaired settings: %w", err)
		}
	}

	hooksRaw, ok := raw["hooks"]
	if !ok {
		return nil
	}
	hooksMap := map[string]json.RawMessage{}
	if err := json.Unmarshal(hooksRaw, &hooksMap); err != nil {
		return fmt.Errorf("unmarshal hooks map: %w", err)
	}

	endpoints := hookEndpoints(getHookBaseURLFn())
	changed := false

	for _, hookName := range hooks {
		eventKey, ok := hookEventName[hookName]
		if !ok {
			continue
		}
		url := endpoints[hookName]
		existingRaw, ok := hooksMap[eventKey]
		if !ok {
			continue
		}
		var groups []hookMatcherGroup
		if err := json.Unmarshal(existingRaw, &groups); err != nil {
			continue
		}

		filtered := make([]hookMatcherGroup, 0, len(groups))
		for _, g := range groups {
			keptHooks := make([]hookEntry, 0, len(g.Hooks))
			for _, h := range g.Hooks {
				if h.Type == "command" && hookCommandReferencesURL(h.Command, url) {
					changed = true
					continue
				}
				keptHooks = append(keptHooks, h)
			}
			if len(keptHooks) > 0 {
				g.Hooks = keptHooks
				filtered = append(filtered, g)
			} else if len(g.Hooks) > 0 {
				// The group had hooks but all were removed — drop the (now-empty) group.
				changed = true
			} else {
				filtered = append(filtered, g)
			}
		}

		if len(filtered) == 0 {
			delete(hooksMap, eventKey)
		} else {
			filteredJSON, err := json.Marshal(filtered)
			if err != nil {
				return fmt.Errorf("marshal hooks for %s: %w", eventKey, err)
			}
			hooksMap[eventKey] = json.RawMessage(filteredJSON)
		}
	}

	if !changed {
		return nil
	}

	hooksJSON, err := json.Marshal(hooksMap)
	if err != nil {
		return fmt.Errorf("marshal hooks map: %w", err)
	}
	raw["hooks"] = json.RawMessage(hooksJSON)

	return writeSettingsAtomic(settingsPath, claudeDir, raw)
}
