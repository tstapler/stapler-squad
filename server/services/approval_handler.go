package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"

	"github.com/google/uuid"
)

// hookDecisionResponse is the JSON response Claude Code expects from an HTTP hook.
type hookDecisionResponse struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName string       `json:"hookEventName"`
	Decision      hookDecision `json:"decision"`
}

type hookDecision struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}

// ReviewQueueChecker is an interface for triggering immediate review queue checks.
// This avoids importing the session package's concrete ReviewQueuePoller type directly.
type ReviewQueueChecker interface {
	FindInstance(sessionID string) *session.Instance
	CheckSession(inst *session.Instance)
}

// approvalNotificationStamper is a narrow interface for stamping approval outcomes
// on notification records after the approval is resolved (or times out).
type approvalNotificationStamper interface {
	SetMetadata(id, key, value string) error
}

// ApprovalHandler handles Claude Code HTTP hooks for PermissionRequest events.
// It blocks the HTTP connection open while waiting for the user's decision,
// then returns the decision in the hookSpecificOutput JSON format.
type ApprovalHandler struct {
	store               *ApprovalStore
	storage             *session.Storage
	eventBus            *events.EventBus
	queueChecker        ReviewQueueChecker          // optional: triggers immediate review queue check on new approval
	classifier          *RuleBasedClassifier        // optional: auto-classify before escalating to manual review
	analyticsStore      *AnalyticsStore             // optional: record classification decisions
	domainChecker       *DomainAgeChecker           // optional: escalate requests to newly-registered domains
	notificationStamper approvalNotificationStamper // optional: stamps approval outcomes on notification records
	timeout             time.Duration               // default 4m; overridable in tests
}

// NewApprovalHandler creates a new ApprovalHandler.
func NewApprovalHandler(store *ApprovalStore, storage *session.Storage, eventBus *events.EventBus) *ApprovalHandler {
	return &ApprovalHandler{store: store, storage: storage, eventBus: eventBus, timeout: 4 * time.Minute}
}

// approvalTimeout returns the configured timeout, falling back to 4 minutes.
func (h *ApprovalHandler) approvalTimeout() time.Duration {
	if h.timeout > 0 {
		return h.timeout
	}
	return 4 * time.Minute
}

// SetQueueChecker injects a ReviewQueueChecker for triggering immediate review queue updates
// when a new approval is created. This provides <100ms feedback instead of waiting for the
// next 2-second poll cycle.
func (h *ApprovalHandler) SetQueueChecker(checker ReviewQueueChecker) {
	h.queueChecker = checker
}

// SetClassifier injects a RuleBasedClassifier for auto-approving/denying tool use requests
// before they reach the manual review queue.
func (h *ApprovalHandler) SetClassifier(c *RuleBasedClassifier) {
	h.classifier = c
}

// SetAnalyticsStore injects an AnalyticsStore for recording classification decisions.
func (h *ApprovalHandler) SetAnalyticsStore(a *AnalyticsStore) {
	h.analyticsStore = a
}

// SetDomainChecker injects a DomainAgeChecker for escalating requests to newly-registered domains.
func (h *ApprovalHandler) SetDomainChecker(d *DomainAgeChecker) {
	h.domainChecker = d
}

// SetNotificationStamper injects a stamper for persisting approval outcomes on notification records.
// When set, resolved and timed-out approvals are stamped with approval_decision in their metadata
// so the notification panel can show a persistent badge after page refresh.
func (h *ApprovalHandler) SetNotificationStamper(s approvalNotificationStamper) {
	h.notificationStamper = s
}

// HandlePermissionRequest handles POST /api/hooks/permission-request.
// This endpoint is configured as an HTTP hook in Claude Code's settings.
// It blocks until the user approves/denies or the context is canceled.
func (h *ApprovalHandler) HandlePermissionRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the hook payload from request body
	var payload PermissionRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.WarningLog.Printf("[ApprovalHandler] Failed to parse hook payload: %v", err)
		// Don't block Claude on parse errors - let the terminal handle it
		h.writeDecision(w, "allow", "")
		return
	}

	// Map to a stapler-squad session using the X-CS-Session-ID header first,
	// then fall back to cwd prefix matching against session paths.
	sessionID := r.Header.Get("X-CS-Session-ID")
	if sessionID == "" {
		sessionID = h.mapSessionByCwd(payload.Cwd)
	}
	if sessionID == "" {
		sessionID = "unknown"
	}

	// Secret scan: auto-deny any command that appears to contain a plaintext secret.
	// Runs on the full command text (before any truncation) so it catches long secrets.
	if cmd, ok := payload.ToolInput["command"].(string); ok && cmd != "" {
		if hit := ScanForSecrets(cmd); hit.Found {
			msg := FormatSecretDenyMessage(hit.PatternName)
			log.InfoLog.Printf("[ApprovalHandler] Auto-denied %s/%s — plaintext secret detected (%s)", sessionID, payload.ToolName, hit.PatternName)
			if h.analyticsStore != nil {
				h.analyticsStore.RecordFromResult(payload, ClassificationResult{
					Decision:  AutoDeny,
					RiskLevel: RiskCritical,
					RuleID:    "secret-scan",
					RuleName:  "Plaintext Secret Detection",
					Reason:    msg,
				}, sessionID, "", 0)
			}
			h.writeDecision(w, "deny", msg)
			return
		}
	}

	// Domain age check: if a Bash command is contacting a newly-registered domain,
	// escalate immediately regardless of other rules.
	if h.domainChecker != nil {
		if cmd, ok := payload.ToolInput["command"].(string); ok && cmd != "" {
			domains := ExtractDomainsFromCommand(cmd)
			for _, domain := range domains {
				isNew, err := h.domainChecker.IsNewlyRegistered(r.Context(), domain)
				if err != nil {
					log.WarningLog.Printf("[ApprovalHandler] Domain age check error for %s: %v", domain, err)
					continue
				}
				if isNew {
					threshDays := int(h.domainChecker.NewDomainThreshold().Hours() / 24)
					reason := fmt.Sprintf("Domain %q was registered within the last %d days — possible phishing or supply-chain risk.", domain, threshDays)
					log.InfoLog.Printf("[ApprovalHandler] Escalating %s/%s — newly-registered domain %s", sessionID, payload.ToolName, domain)
					if h.analyticsStore != nil {
						h.analyticsStore.RecordFromResult(payload, ClassificationResult{
							Decision:  Escalate,
							RiskLevel: RiskHigh,
							RuleID:    "new-domain-check",
							RuleName:  "New Domain Check",
							Reason:    reason,
						}, sessionID, "", 0)
					}
					// Fall through to manual review queue (do NOT return here).
					// The domain reason will appear in the pending approval context.
					_ = reason // will be surfaced when the approval is shown in review queue
					goto createApproval
				}
			}
		}
	}

	// Classify the request: auto-allow/deny if a rule matches; escalate to manual review otherwise.
	if h.classifier != nil {
		start := time.Now()
		classCtx := h.classifier.BuildContext(payload.Cwd)
		result := h.classifier.Classify(payload, classCtx)
		durationMs := time.Since(start).Milliseconds()

		if h.analyticsStore != nil {
			h.analyticsStore.RecordFromResult(payload, result, sessionID, "", durationMs)
		}

		switch result.Decision {
		case AutoAllow:
			log.InfoLog.Printf("[ApprovalHandler] Auto-allowed %s/%s (rule=%s)", sessionID, payload.ToolName, result.RuleID)
			h.writeDecision(w, "allow", "")
			return
		case AutoDeny:
			msg := result.Reason
			if result.Alternative != "" {
				msg = fmt.Sprintf("%s %s", msg, result.Alternative)
			}
			log.InfoLog.Printf("[ApprovalHandler] Auto-denied %s/%s (rule=%s): %s", sessionID, payload.ToolName, result.RuleID, msg)
			h.writeDecision(w, "deny", msg)
			return
			// Escalate: fall through to manual review queue
		}
	}

createApproval:

	// Create a pending approval record
	approvalID := uuid.New().String()
	approval := &PendingApproval{
		ID:              approvalID,
		SessionID:       sessionID,
		ClaudeSessionID: payload.SessionID,
		ToolName:        payload.ToolName,
		ToolInput:       payload.ToolInput,
		Cwd:             payload.Cwd,
		PermissionMode:  payload.PermissionMode,
		CreatedAt:       time.Now(),
		// Use the configured timeout (default 4 minutes), strictly less than the 5-minute hook timeout.
		ExpiresAt: time.Now().Add(h.approvalTimeout()),
	}

	if err := h.store.Create(approval); err != nil {
		log.ErrorLog.Printf("[ApprovalHandler] Failed to store approval: %v", err)
		h.writeDecision(w, "allow", "")
		return
	}

	// Notify all web UI clients about the pending approval
	h.broadcastApprovalNotification(sessionID, approval)

	// Trigger immediate review queue check for this session (Story 3, Task 3.1).
	// This provides <100ms feedback in the review queue instead of waiting for the
	// next 2-second poll cycle.
	if h.queueChecker != nil && sessionID != "unknown" {
		if inst := h.queueChecker.FindInstance(sessionID); inst != nil {
			h.queueChecker.CheckSession(inst)
			log.InfoLog.Printf("[ApprovalHandler] Triggered immediate queue check for session '%s'", sessionID)
		}
	}

	log.InfoLog.Printf("[ApprovalHandler] Waiting for decision on approval %s (session=%s, tool=%s)",
		approvalID, sessionID, payload.ToolName)

	// Block until user decides, server times out, or connection closes
	var decision ApprovalDecision
	select {
	case decision = <-approval.decisionCh:
		// User responded via ResolveApproval RPC
		log.InfoLog.Printf("[ApprovalHandler] Approval %s resolved: %s", approvalID, decision.Behavior)
	case <-time.After(h.approvalTimeout()):
		// Server-side timeout (before the hook's 5-minute timeout).
		// Return an empty HTTP response so the hook script gets no hookSpecificOutput
		// and Claude Code falls back to its native terminal permission dialog.
		// This lets the user still approve/deny in the terminal rather than being
		// silently allowed or denied.
		h.store.Remove(approvalID)
		// Stamp the notification so the panel shows a "timed out" badge instead of
		// live Approve/Deny buttons after page refresh.
		if h.notificationStamper != nil {
			if err := h.notificationStamper.SetMetadata(approvalID, "approval_decision", "timeout"); err != nil {
				log.WarningLog.Printf("[ApprovalHandler] Could not stamp timeout on notification %s: %v", approvalID, err)
			}
		}
		log.InfoLog.Printf("[ApprovalHandler] Approval %s timed out — returning empty response (native dialog fallback)", approvalID)
		w.WriteHeader(http.StatusOK)
		return
	case <-r.Context().Done():
		// Claude Code disconnected (e.g., stapler-squad restarted, network issue)
		h.store.Remove(approvalID)
		decision = ApprovalDecision{Behavior: "allow", Message: ""}
		log.InfoLog.Printf("[ApprovalHandler] Approval %s context canceled", approvalID)
		return // Don't write to disconnected client
	}

	h.writeDecision(w, decision.Behavior, decision.Message)
}

// broadcastApprovalNotification notifies all connected web UI clients about a pending approval.
// The approval ID is passed in the notification metadata so the UI can resolve it.
func (h *ApprovalHandler) broadcastApprovalNotification(sessionID string, approval *PendingApproval) {
	metadata := map[string]string{
		"approval_id": approval.ID,
		"tool_name":   approval.ToolName,
		"cwd":         approval.Cwd,
	}

	// Extract tool-specific display fields
	if cmd, ok := approval.ToolInput["command"].(string); ok && cmd != "" {
		metadata["tool_input_command"] = cmd
	}
	if filePath, ok := approval.ToolInput["file_path"].(string); ok && filePath != "" {
		metadata["tool_input_file"] = filePath
	}
	if desc, ok := approval.ToolInput["description"].(string); ok && desc != "" {
		metadata["tool_input_description"] = desc
	}

	title := fmt.Sprintf("Permission Required: %s", approval.ToolName)
	message := buildApprovalMessage(approval)

	event := events.NewNotificationEvent(
		sessionID,
		sessionID,
		approval.ID, // Use approval ID as notification ID for correlation
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT),
		title,
		message,
		metadata,
	)
	h.eventBus.Publish(event)
}

// buildApprovalMessage builds the human-readable message for an approval notification.
func buildApprovalMessage(approval *PendingApproval) string {
	if cmd, ok := approval.ToolInput["command"].(string); ok && cmd != "" {
		if len(cmd) > 120 {
			return cmd[:120] + "..."
		}
		return cmd
	}
	if filePath, ok := approval.ToolInput["file_path"].(string); ok && filePath != "" {
		return filePath
	}
	return fmt.Sprintf("Claude needs permission to use %s", approval.ToolName)
}

// mapSessionByCwd finds a stapler-squad session by matching cwd against session paths.
// Returns the session title of the best (longest-prefix) match, or "" if no match.
func (h *ApprovalHandler) mapSessionByCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	instances, err := h.storage.LoadInstances()
	if err != nil {
		return ""
	}
	bestTitle := ""
	bestLen := 0
	for _, inst := range instances {
		if p := inst.Path; p != "" && strings.HasPrefix(cwd, p) && len(p) > bestLen {
			bestTitle = inst.Title
			bestLen = len(p)
		}
		if wd := inst.WorkingDir; wd != "" && strings.HasPrefix(cwd, wd) && len(wd) > bestLen {
			bestTitle = inst.Title
			bestLen = len(wd)
		}
	}
	return bestTitle
}

// writeDecision writes the hookSpecificOutput JSON response to the HTTP response.
func (h *ApprovalHandler) writeDecision(w http.ResponseWriter, behavior, message string) {
	resp := hookDecisionResponse{
		HookSpecificOutput: hookSpecificOutput{
			HookEventName: "PermissionRequest",
			Decision:      hookDecision{Behavior: behavior, Message: message},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.WarningLog.Printf("[ApprovalHandler] Failed to write decision response: %v", err)
	}
}

// StartExpirationCleanup starts a background goroutine that periodically removes expired approvals.
// The goroutine stops when ctx is canceled.
func StartExpirationCleanup(ctx context.Context, store *ApprovalStore) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if expired := store.CleanupExpired(); len(expired) > 0 {
					log.InfoLog.Printf("[ApprovalStore] Cleaned up %d expired approvals: %v", len(expired), expired)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// hookEntry is the individual hook definition within a matcher group.
type hookEntry struct {
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	URL     string            `json:"url,omitempty"`
	Timeout int               `json:"timeout,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// hookMatcherGroup is a group of hooks optionally filtered by a matcher.
type hookMatcherGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

// claudeSettingsHooks is the top-level hooks map in settings.local.json.
type claudeSettingsHooks struct {
	PermissionRequest []hookMatcherGroup `json:"PermissionRequest,omitempty"`
}

// claudeSettings is the partial structure of .claude/settings.local.json.
// Only the "hooks" key is read/written; other fields are preserved via rawOther.
type claudeSettings struct {
	Hooks    claudeSettingsHooks        `json:"hooks"`
	rawOther map[string]json.RawMessage // preserves unknown fields
}

const (
	hookApprovalURL = "http://localhost:8543/api/hooks/permission-request"
	hookTimeout     = 300 // seconds — must be ≤ Claude Code's 5-minute hook timeout
)

// InjectHookConfig writes (or merges) the stapler-squad PermissionRequest HTTP hook
// into <rootDir>/.claude/settings.local.json.
//
// If the file already contains a hook pointing to hookApprovalURL, it is left unchanged.
// If the file exists but lacks our hook, the hook is prepended to PermissionRequest.
// If the file does not exist, it is created with just our hook config.
func InjectHookConfig(rootDir, sessionTitle string) error {
	claudeDir := filepath.Join(rootDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	// Desired hook entry for this session.
	// settings.local.json only supports "command" type hooks; use curl to POST to the approval URL.
	curlCmd := fmt.Sprintf(
		"curl -s --max-time %d -X POST '%s' -H 'Content-Type: application/json' -H 'X-CS-Session-ID: %s' -d @-",
		hookTimeout, hookApprovalURL, sessionTitle,
	)
	entry := hookEntry{
		Type:    "command",
		Command: curlCmd,
		Timeout: hookTimeout,
	}
	group := hookMatcherGroup{Hooks: []hookEntry{entry}}

	// Read existing settings (if any).
	raw := map[string]json.RawMessage{}
	data, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", settingsPath, err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			// Malformed JSON — attempt targeted repair before falling back to a fresh config.
			log.WarningLog.Printf("[InjectHookConfig] %s has invalid JSON (%v), attempting repair", settingsPath, err)
			repaired, repairErr := repairSettingsJSON(data)
			if repairErr == nil {
				log.InfoLog.Printf("[InjectHookConfig] Repaired %s successfully", settingsPath)
				_ = json.Unmarshal(repaired, &raw) // best-effort; raw may still be partial
			} else {
				log.WarningLog.Printf("[InjectHookConfig] Could not repair %s (%v), resetting to minimal config", settingsPath, repairErr)
				raw = map[string]json.RawMessage{}
			}
		}
	}

	// Check whether our command-type hook is already present.
	if hooksRaw, ok := raw["hooks"]; ok {
		var hooks map[string]json.RawMessage
		if err := json.Unmarshal(hooksRaw, &hooks); err == nil {
			if prRaw, ok := hooks["PermissionRequest"]; ok {
				var groups []hookMatcherGroup
				if err := json.Unmarshal(prRaw, &groups); err == nil {
					for _, g := range groups {
						for _, h := range g.Hooks {
							if h.Type == "command" && strings.Contains(h.Command, hookApprovalURL) {
								log.DebugLog.Printf("[InjectHookConfig] Hook already present in %s", settingsPath)
								return nil
							}
						}
					}
				}
			}
		}
	}

	// Merge: prepend our group to PermissionRequest hooks.
	// Also remove any old http-type entries pointing to our URL (migration from old format).
	var prGroups []hookMatcherGroup
	if hooksRaw, ok := raw["hooks"]; ok {
		var hooks map[string]json.RawMessage
		if err := json.Unmarshal(hooksRaw, &hooks); err == nil {
			if prRaw, ok := hooks["PermissionRequest"]; ok {
				var existingGroups []hookMatcherGroup
				if err := json.Unmarshal(prRaw, &existingGroups); err == nil {
					for _, g := range existingGroups {
						// Strip out any old http-type hooks pointing to our URL.
						filtered := g.Hooks[:0]
						for _, h := range g.Hooks {
							if h.URL != hookApprovalURL {
								filtered = append(filtered, h)
							}
						}
						if len(filtered) > 0 {
							g.Hooks = filtered
							prGroups = append(prGroups, g)
						}
					}
				}
			}
		}
	}
	prGroups = append([]hookMatcherGroup{group}, prGroups...)

	// Rebuild hooks object.
	hooksMap := map[string]json.RawMessage{}
	if hooksRaw, ok := raw["hooks"]; ok {
		_ = json.Unmarshal(hooksRaw, &hooksMap)
	}
	prJSON, err := json.Marshal(prGroups)
	if err != nil {
		return fmt.Errorf("marshal PermissionRequest hooks: %w", err)
	}
	hooksMap["PermissionRequest"] = json.RawMessage(prJSON)

	hooksJSON, err := json.Marshal(hooksMap)
	if err != nil {
		return fmt.Errorf("marshal hooks map: %w", err)
	}
	raw["hooks"] = json.RawMessage(hooksJSON)

	// Write back with indentation.
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}
	// Write atomically via temp file to avoid partial writes corrupting the file.
	tmpPath := settingsPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0o644); err != nil {
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, settingsPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename %s: %w", tmpPath, err)
	}
	log.InfoLog.Printf("[InjectHookConfig] Wrote hook config to %s (session=%s)", settingsPath, sessionTitle)
	return nil
}

// repairSettingsJSON attempts to fix common JSON syntax errors in Claude settings files.
//
// The most common corruption seen in settings.local.json is a missing comma between
// adjacent values (e.g. two string entries in the permissions.allow array written by
// separate code paths without coordinating on the trailing comma).
//
// Strategy: use json.SyntaxError.Offset to locate the exact byte where the parser
// choked, then insert a comma just after the last non-whitespace byte before that
// position.  Repeats up to maxRepairs times to handle multiple missing commas.
// Returns the repaired bytes, or an error if the JSON could not be made valid.
func repairSettingsJSON(data []byte) ([]byte, error) {
	const maxRepairs = 20
	current := make([]byte, len(data))
	copy(current, data)

	for i := 0; i < maxRepairs; i++ {
		var syntaxErr *json.SyntaxError
		if err := json.Unmarshal(current, new(interface{})); err == nil {
			return current, nil
		} else if !errors.As(err, &syntaxErr) {
			return nil, fmt.Errorf("non-syntax error, cannot repair: %w", err)
		} else {
			offset := int(syntaxErr.Offset)
			if offset <= 0 || offset > len(current) {
				return nil, fmt.Errorf("offset %d out of range (len=%d): %w", offset, len(current), err)
			}
			errMsg := err.Error()

			// Missing comma between array elements or object key:value pairs.
			// syntaxErr.Offset points to the byte AFTER the one that was just read
			// (i.e. the erroneous character is at index Offset-1).
			// Walk backwards past whitespace to find the end of the previous token,
			// then insert a comma there.
			if strings.Contains(errMsg, "after array element") ||
				strings.Contains(errMsg, "after object key:value pair") {
				insertAt := offset - 1 // index of the unexpected character
				for insertAt > 0 && isJSONWhitespace(current[insertAt-1]) {
					insertAt--
				}
				fixed := make([]byte, 0, len(current)+1)
				fixed = append(fixed, current[:insertAt]...)
				fixed = append(fixed, ',')
				fixed = append(fixed, current[insertAt:]...)
				current = fixed
				continue
			}

			return nil, fmt.Errorf("unsupported JSON syntax error at offset %d: %w", offset, err)
		}
	}
	return nil, fmt.Errorf("still invalid after %d repair attempts", maxRepairs)
}

// isJSONWhitespace reports whether b is a JSON whitespace character.
func isJSONWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
