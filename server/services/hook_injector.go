package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
var hookBaseURLFn = func() string { return "http://localhost:8543" }

// SetHookBaseURLFn overrides the base URL function used when building hook endpoint URLs via
// hookEndpoints. Call once during server wiring; passing nil is a no-op.
func SetHookBaseURLFn(fn func() string) {
	if fn != nil {
		hookBaseURLFn = fn
	}
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
// .claude/rules/primitive-obsession-checklist.md exists to catch -- so this is a named
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
//	    SocketPath:  sshremote.RemoteApprovalSocketPath(inst.GetEffectiveRootDir()),
//	    BearerToken: token,
//	}
//	services.InjectHooksConfig(inst.GetEffectiveRootDir(), inst.Title, hooks,
//	    services.WithRemoteHookTarget(target))
//
// None of those four callers pass this option today, so every hook command they generate --
// local or remote session alike -- is byte-identical to pre-Phase-5 behavior.
type RemoteHookTarget struct {
	// SocketPath is the remote-host Unix domain socket path RemoteApprovalRelay reads
	// from -- see sshremote.RemoteApprovalSocketPath(basePath).
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

// remoteApprovalWriteTool is the executable the remote-aware PermissionRequest hook command
// shells out to in order to write a raw JSON payload directly onto RemoteApprovalRelay's
// Unix domain socket. Deliberately NOT curl: curl's --unix-socket transport still speaks
// HTTP framing (a request line and headers) over the socket, but RemoteApprovalRelay.
// decodePayload (session/sshremote/approval_relay.go) calls json.NewDecoder(conn).Decode
// directly on the raw connection bytes with no HTTP parsing at all -- an HTTP-framed write
// would fail to decode as JSON (a request line like "POST / HTTP/1.1" is not valid JSON) and
// be silently dropped by handleConnection's decode-error branch. socat's `- UNIX-CONNECT:
// <path>` form writes stdin bytes verbatim to the socket with no protocol framing, matching
// the relay's actual wire format. This is a new remote-host dependency (socat must be
// installed there) that no previous hook command required -- flagged here rather than
// silently assumed; see this file's package doc / RemoteHookTarget's wiring-gap note for the
// broader context this lands in.
const remoteApprovalWriteTool = "socat"

// remoteApprovalHookCommand builds the shell command a remote session's PermissionRequest
// hook runs to deliver its payload to target instead of POSTing to hookBaseURLFn(). It wraps
// Claude Code's raw hook-event JSON (piped to the command's stdin, exactly like the local
// curl command's `-d @-` reads its POST body from stdin) as the "request" field of
// relayedApprovalPayload's JSON shape (session/sshremote/approval_relay.go) --
// {"token":"<token>","request":<stdin>} -- by string concatenation rather than a JSON
// library: the value being embedded (stdin) is already a complete JSON object emitted by
// Claude Code itself, so no escaping is needed for that composition, only for the token.
// token is bearerCredential's base64.RawURLEncoding output (session/sshremote/
// approval_relay.go's newBearerCredential) -- URL-safe base64 (letters, digits, '-', '_'
// only) -- safe to interpolate into a single-quoted shell string with no additional
// escaping.
func remoteApprovalHookCommand(target RemoteHookTarget) string {
	return fmt.Sprintf(
		`(printf '{"token":"%s","request":'; cat; printf '}') | %s - 'UNIX-CONNECT:%s'`,
		target.BearerToken, remoteApprovalWriteTool, target.SocketPath,
	)
}

// hookCommandTargetsSocket reports whether curlCmd is the remote-aware hook command built by
// remoteApprovalHookCommand for socketPath. Mirrors hookCommandReferencesURL's
// quote-bounded-match design (see its doc comment for the strict-prefix false-positive bug
// that pattern fixes): matching on the quoted `'UNIX-CONNECT:<path>'` form, not a bare
// strings.Contains(command, socketPath), so one socket path can never falsely match another
// that happens to be its strict prefix.
func hookCommandTargetsSocket(curlCmd, socketPath string) bool {
	return strings.Contains(curlCmd, "'UNIX-CONNECT:"+socketPath+"'")
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

	// Build the set of hooks to inject (permission_approval always included).
	wanted := map[HookName]struct{}{HookPermissionApproval: {}}
	for _, h := range hooks {
		if _, ok := hookEventName[h]; ok {
			wanted[h] = struct{}{}
		} else {
			log.Warn("[InjectHooksConfig] unknown hook name, skipping", "name", h)
		}
	}

	// Read existing settings.
	raw := map[string]json.RawMessage{}
	data, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", settingsPath, err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			log.Warn("[InjectHooksConfig] settings file has invalid JSON, attempting repair", "path", settingsPath, "err", err)
			repaired, repairErr := repairSettingsJSON(data)
			if repairErr == nil {
				_ = json.Unmarshal(repaired, &raw)
			} else {
				raw = map[string]json.RawMessage{}
			}
		}
	}

	// Parse existing hooks map.
	hooksMap := map[string]json.RawMessage{}
	if hooksRaw, ok := raw["hooks"]; ok {
		_ = json.Unmarshal(hooksRaw, &hooksMap)
	}

	// Resolved once per InjectHooksConfig call (i.e. per-session, at hook-injection time), not
	// cached at package-construction time, so it reflects the server's current base URL.
	endpoints := hookEndpoints(hookBaseURLFn)

	for hookName := range wanted {
		eventKey := hookEventName[hookName]
		url := endpoints[hookName]

		// Remote-aware branch (Phase 5 Epic 5.2 / ADR-003): only HookPermissionApproval
		// routes at the relay's socket -- see WithRemoteHookTarget's doc comment for why
		// every other hook type keeps the HTTP command even when cfg.remote is set.
		remoteTargeted := cfg.remote != nil && hookName == HookPermissionApproval
		var curlCmd string
		if remoteTargeted {
			curlCmd = remoteApprovalHookCommand(*cfg.remote)
		} else {
			curlCmd = fmt.Sprintf(
				"curl -s --max-time %d -X POST '%s' -H 'Content-Type: application/json' -H 'X-CS-Session-ID: %s' -d @-",
				hookTimeout, url, sessionTitle,
			)
		}

		// Check if this hook command is already present.
		if existing, ok := hooksMap[eventKey]; ok {
			var groups []hookMatcherGroup
			if err := json.Unmarshal(existing, &groups); err == nil {
				alreadyPresent := false
				for _, g := range groups {
					for _, h := range g.Hooks {
						matches := h.Type == "command"
						if matches && remoteTargeted {
							matches = hookCommandTargetsSocket(h.Command, cfg.remote.SocketPath)
						} else if matches {
							matches = hookCommandReferencesURL(h.Command, url)
						}
						if matches {
							alreadyPresent = true
							break
						}
					}
					if alreadyPresent {
						break
					}
				}
				if alreadyPresent {
					continue
				}
			}
		}

		// Prepend our entry.
		entry := hookEntry{Type: "command", Command: curlCmd, Timeout: hookTimeout}
		group := hookMatcherGroup{Hooks: []hookEntry{entry}}

		var existing []hookMatcherGroup
		if raw, ok := hooksMap[eventKey]; ok {
			_ = json.Unmarshal(raw, &existing)
		}
		merged := append([]hookMatcherGroup{group}, existing...)
		mergedJSON, err := json.Marshal(merged)
		if err != nil {
			return fmt.Errorf("marshal hooks for %s: %w", eventKey, err)
		}
		hooksMap[eventKey] = json.RawMessage(mergedJSON)
	}

	hooksJSON, err := json.Marshal(hooksMap)
	if err != nil {
		return fmt.Errorf("marshal hooks map: %w", err)
	}
	raw["hooks"] = json.RawMessage(hooksJSON)

	return writeSettingsAtomic(settingsPath, claudeDir, raw)
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

	endpoints := hookEndpoints(hookBaseURLFn)
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
