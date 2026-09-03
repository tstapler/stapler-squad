package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/tmux"

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
	MarkRead(ids []string) (int, error)
}

// autoApprovalLogger is a narrow interface for writing silent auto-approval records
// directly to notification history without triggering toasts or push notifications.
type autoApprovalLogger interface {
	AppendAutoApproved(sessionID, sessionName, toolName, filePath, ruleID, ruleName, ruleSource, decision string) error
}

// headlessPoolApprover is the narrow interface ApprovalHandler needs from the headless pool.
type headlessPoolApprover interface {
	CallBlocking(ctx context.Context, key headless.FeatureKey, systemPrompt string, userPrompt string, opts headless.CallOptions, sink headless.CostSink) (string, error)
}

// ApprovalHandler handles Claude Code HTTP hooks for PermissionRequest events.
// It blocks the HTTP connection open while waiting for the user's decision,
// then returns the decision in the hookSpecificOutput JSON format.
type ApprovalHandler struct {
	store               *ApprovalStore
	storage             *session.Storage
	eventBus            *events.EventBus
	queueChecker        ReviewQueueChecker          // optional: triggers immediate review queue check on new approval
	classifier          classifier.Classifier       // optional: auto-classify before escalating to manual review
	analyticsStore      *AnalyticsStore             // optional: record classification decisions
	domainChecker       *DomainAgeChecker           // optional: escalate requests to newly-registered domains
	notificationStamper approvalNotificationStamper // optional: stamps approval outcomes on notification records
	autoApprovalLog     autoApprovalLogger          // optional: writes silent records for auto-approved/denied ops
	timeout             time.Duration               // default 4m; overridable in tests
	headlessPool        headlessPoolApprover        // optional: LLM approval for autonomous sessions
	autonomousChecker   func(string) bool           // optional: returns true if sessionID is an autonomous session
	pollInterval        time.Duration               // PRStatusPoller's configured interval; used to bound CI-status staleness. Zero value (bypassing NewApprovalHandler) makes every CI status read as stale — always construct via NewApprovalHandler.
	liveFinder          LiveInstanceFinder          // optional: resolves live in-memory Instance for CI status (not persisted — see PRStatusPoller)
	slackNotifier       *SlackNotifier              // optional: notifies a configured Slack webhook about new pending approvals; concrete type (no interface) since both live in this package
	dashboardBaseURLFn  func() string               // optional: lazily-read fallback for the Slack dashboard-link base URL, used only when cfg.Slack.DashboardBaseURL is unset. Mirrors ReactiveQueueManager.dashboardBaseURLFn exactly (server.go wires the same hookBaseURLFn into both).
	piHealthTracker     *PiExtensionHealthTracker   // optional: records pi approval-extension health pings (pi-support Epic 4.2)
}

// NewApprovalHandler creates a new ApprovalHandler.
func NewApprovalHandler(store *ApprovalStore, storage *session.Storage, eventBus *events.EventBus) *ApprovalHandler {
	return &ApprovalHandler{store: store, storage: storage, eventBus: eventBus, timeout: 4 * time.Minute, pollInterval: session.DefaultPRStatusPollerConfig().PollInterval}
}

// SetPollInterval overrides the interval used to bound CI-status staleness (Task 1.1.2b).
// Callers should pass the live PRStatusPoller's configured interval so the guard can't
// silently desync from the poller if it's ever tuned.
func (h *ApprovalHandler) SetPollInterval(d time.Duration) {
	h.pollInterval = d
}

// SetLiveInstanceFinder wires the live in-memory instance lookup used to populate
// ClassificationContext.CIStatus. GitHubCheckConclusion/LastPRStatusCheck are not
// persisted in the ent schema (see Storage.UpdateInstancePRStatus) — they only live on
// the in-memory Instance the PRStatusPoller keeps fresh — so a *session.Storage lookup
// cannot see them; this must be the live registry (typically *SessionService).
func (h *ApprovalHandler) SetLiveInstanceFinder(f LiveInstanceFinder) {
	h.liveFinder = f
}

// approvalTimeout returns the configured timeout, falling back to 4 minutes.
func (h *ApprovalHandler) approvalTimeout() time.Duration {
	if h.timeout > 0 {
		return h.timeout
	}
	return 4 * time.Minute
}

// stampResolved stamps an approval decision on the notification record,
// marks it read, and broadcasts a resolution event to connected clients.
// Called for the timeout and cancel arms where no HTTP response decision is sent.
func (h *ApprovalHandler) stampResolved(approvalID, sessionID, reason string) {
	if h.notificationStamper != nil {
		if err := h.notificationStamper.SetMetadata(approvalID, "approval_decision", reason); err != nil {
			log.Warn("[ApprovalHandler] could not stamp "+reason+" on notification", "approval_id", approvalID, "err", err)
		}
		if _, err := h.notificationStamper.MarkRead([]string{approvalID}); err != nil {
			log.Warn("[ApprovalHandler] could not mark "+reason+" approval read", "approval_id", approvalID, "err", err)
		}
	}
	if h.eventBus != nil && sessionID != "" && sessionID != "unknown" {
		h.eventBus.Publish(events.NewApprovalResponseEvent(sessionID, false, approvalID))
	}
}

// SetQueueChecker injects a ReviewQueueChecker for triggering immediate review queue updates
// when a new approval is created. This provides <100ms feedback instead of waiting for the
// next 2-second poll cycle.
func (h *ApprovalHandler) SetQueueChecker(checker ReviewQueueChecker) {
	h.queueChecker = checker
}

// SetClassifier injects a Classifier for auto-approving/denying tool use requests
// before they reach the manual review queue.
func (h *ApprovalHandler) SetClassifier(c classifier.Classifier) {
	h.classifier = c
}

// SetAnalyticsStore injects an AnalyticsStore for recording classification decisions.
func (h *ApprovalHandler) SetAnalyticsStore(a *AnalyticsStore) {
	h.analyticsStore = a
}

// SetPiExtensionHealthTracker injects a PiExtensionHealthTracker so
// HandlePiExtensionLoaded has somewhere to record pings (pi-support Epic 4.2).
func (h *ApprovalHandler) SetPiExtensionHealthTracker(t *PiExtensionHealthTracker) {
	h.piHealthTracker = t
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

// SetAutoApprovalLogger injects a logger for writing silent auto-approval records to notification
// history. When set, AutoAllow and AutoDeny decisions are recorded without triggering toasts or
// push notifications, giving users a reviewable log of what the classifier handled automatically.
func (h *ApprovalHandler) SetAutoApprovalLogger(l autoApprovalLogger) {
	h.autoApprovalLog = l
}

// SetHeadlessPool injects a headless LLM pool for autonomous session approval.
// When set and autonomousChecker returns true for a session, risky tool calls are sent to the
// LLM for approval instead of the human review queue.
func (h *ApprovalHandler) SetHeadlessPool(pool headlessPoolApprover) {
	h.headlessPool = pool
}

// SetSlackNotifier injects the Slack notifier used to notify a configured
// webhook about new pending approvals (see broadcastApprovalNotification).
// nil-safe: when never called, h.slackNotifier stays nil and Slack
// notification is silently skipped — matching every other optional Set*
// dependency in this file.
func (h *ApprovalHandler) SetSlackNotifier(n *SlackNotifier) {
	h.slackNotifier = n
}

// SetDashboardBaseURLFn wires the lazily-read dashboard-base-URL fallback
// (see dashboardBaseURLFn's doc comment) used when building Slack "view in
// dashboard" links for approval-pending notifications. nil-safe: when never
// called, broadcastApprovalNotification falls back to whatever
// cfg.Slack.DashboardBaseURL is (possibly empty, omitting the link).
func (h *ApprovalHandler) SetDashboardBaseURLFn(fn func() string) {
	h.dashboardBaseURLFn = fn
}

// SlackNotifierForTest returns the wired SlackNotifier instance. Exported
// only so cross-package wiring regression tests (server package) can assert
// pointer identity against the other consumers (ReactiveQueueManager,
// SessionService) without restructuring production code — not intended for
// any non-test caller.
func (h *ApprovalHandler) SlackNotifierForTest() *SlackNotifier {
	return h.slackNotifier
}

// SetAutonomousChecker injects a function that returns true when the given session ID is an
// autonomous session. Injected from server.go to avoid a construction-time circular dependency.
func (h *ApprovalHandler) SetAutonomousChecker(fn func(string) bool) {
	h.autonomousChecker = fn
}

// buildApprovalQuery constructs the LLM prompt for an autonomous approval decision.
// Tool arguments are JSON-encoded to prevent values containing "APPROVE:" or "DENY:"
// from influencing the LLM's decision (prompt injection via tool input).
func buildApprovalQuery(toolName string, toolInput map[string]interface{}, sessionTail string) string {
	argsJSON, err := json.Marshal(toolInput)
	argsStr := string(argsJSON)
	if err != nil {
		argsStr = "(encoding error)"
	}
	return fmt.Sprintf(
		"Requested tool: %s\nArguments (JSON): %s\nRecent session output:\n---\n%s\n---\nReply APPROVE: <reason> or DENY: <reason>",
		toolName, argsStr, sessionTail,
	)
}

// HandlePermissionRequest handles POST /api/hooks/permission-request.
// This endpoint is configured as an HTTP hook in Claude Code's settings.
// It blocks until the user approves/denies or the context is canceled.
func (h *ApprovalHandler) HandlePermissionRequest(w http.ResponseWriter, r *http.Request) {
	log.Info("[ApprovalHandler] received request", "remote_addr", r.RemoteAddr, "method", r.Method, "path", r.URL.Path)
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the hook payload from request body
	var payload classifier.PermissionRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("[ApprovalHandler] failed to parse hook payload", "err", err)
		// Don't block Claude on parse errors - let the terminal handle it
		h.writeDecision(w, "allow", "")
		return
	}

	// Map to a stapler-squad session using the X-CS-Session-ID header first,
	// then fall back to cwd prefix matching against session paths.
	// Always resolve to the stable ID (UUID) so the client can match the notification.
	sessionID := h.resolveSessionID(r.Header.Get("X-CS-Session-ID"), payload.Cwd)
	if sessionID == "" {
		sessionID = "unknown"
	}

	// source identifies which agent's hook produced this request, for audit/analytics
	// distinguishability (pi-support Epic 4.3). Defaulted to "claude" here, at the
	// recording boundary only — payload itself is never mutated, since Claude's
	// existing curl hook omits the field entirely and that omission is the expected,
	// backward-compatible shape of its wire payload.
	source := payload.Source
	if source == "" {
		source = "claude"
	}

	// Secret scan: auto-deny any command that appears to contain a plaintext secret.
	// Runs on the full command text (before any truncation) so it catches long secrets.
	if cmd, ok := payload.ToolInput["command"].(string); ok && cmd != "" {
		if hit := ScanForSecrets(cmd); hit.Found {
			msg := FormatSecretDenyMessage(hit.PatternName)
			log.ForSession(sessionID).Info("[ApprovalHandler] auto-denied — plaintext secret detected", "tool", payload.ToolName, "pattern", hit.PatternName)
			if h.analyticsStore != nil {
				// Before recording to analytics, replace the command so the secret is not persisted.
				// Shallow-copy the payload and replace ToolInput to avoid mutating the original
				// (the original payload pointer may be used after this branch for the response).
				sanitizedInput := make(map[string]interface{}, len(payload.ToolInput))
				for k, v := range payload.ToolInput {
					sanitizedInput[k] = v
				}
				sanitizedInput["command"] = redactedSecret
				sanitizedPayload := payload
				sanitizedPayload.ToolInput = sanitizedInput
				h.analyticsStore.RecordFromResult(sanitizedPayload, classifier.ClassificationResult{
					Decision:  classifier.AutoDeny,
					RiskLevel: classifier.RiskCritical,
					RuleID:    classifier.RuleIDSecretScan,
					RuleName:  "Plaintext Secret Detection",
					Reason:    msg,
				}, sessionID, "", 0, source)
			}
			h.writeDecision(w, "deny", msg)
			return
		}
	}

	// escalation captures the classification result (or its domain-age synthetic equivalent)
	// that led to this request being queued for manual review. Zero-valued (no-match) unless
	// set below.
	var escalation classifier.ClassificationResult

	// classified is true only when escalation was genuinely assigned below (domain-age or a
	// real classifier result). RiskLevel must never be read from escalation unless classified
	// is true — escalation's zero value (classifier.RiskLow) is indistinguishable from a real
	// Low risk, and reading it unconditionally would silently mislabel an unclassified/degraded
	// request (e.g. h.classifier == nil) as safe. See pre-mortem.md Failure #1.
	var classified bool

	// Domain age check: if a Bash command is contacting a newly-registered domain,
	// escalate immediately regardless of other rules.
	if h.domainChecker != nil {
		if cmd, ok := payload.ToolInput["command"].(string); ok && cmd != "" {
			domains := ExtractDomainsFromCommand(cmd)
			for _, domain := range domains {
				isNew, err := h.domainChecker.IsNewlyRegistered(r.Context(), domain)
				if err != nil {
					// Silenced on purpose: a single domain's check failing doesn't abort the
					// whole request. But it means the reviewer sees whatever the classifier
					// decides afterward (no-match/explicit-rule) with no indication a domain
					// check was attempted and came back inconclusive for this domain.
					log.Warn("[ApprovalHandler] domain age check error", "domain", domain, "err", err)
					continue
				}
				if isNew {
					threshDays := int(h.domainChecker.NewDomainThreshold().Hours() / 24)
					reason := fmt.Sprintf("Domain %q was registered within the last %d days — possible phishing or supply-chain risk.", domain, threshDays)
					log.ForSession(sessionID).Info("[ApprovalHandler] escalating — newly-registered domain", "tool", payload.ToolName, "domain", domain, "escalation_category", "domain-age")
					domainEscalation := classifier.ClassificationResult{
						Decision:  classifier.Escalate,
						RiskLevel: classifier.RiskHigh,
						RuleID:    classifier.RuleIDNewDomainCheck,
						RuleName:  "New Domain Check",
						Reason:    reason,
					}
					if h.analyticsStore != nil {
						h.analyticsStore.RecordFromResult(payload, domainEscalation, sessionID, "", 0, source)
					}
					// Fall through to manual review queue (do NOT return here).
					escalation = domainEscalation
					classified = true
					goto createApproval
				}
			}
		}
	}

	// AskUserQuestion: fire an informational notification so the user knows to check the
	// terminal, then defer to the native terminal dialog (empty body). No approval_id
	// is included in the notification metadata so the UI renders a plain ❓ toast with
	// no Approve/Deny buttons.
	if strings.EqualFold(payload.ToolName, "AskUserQuestion") {
		log.ForSession(sessionID).Info("[ApprovalHandler] AskUserQuestion — notifying and deferring to native dialog")
		h.broadcastQuestionNotification(sessionID, payload)
		h.writeDeferDecision(w)
		return
	}

	// Classify the request: auto-allow/deny if a rule matches; escalate to manual review otherwise.
	if h.classifier != nil {
		start := time.Now()
		classCtx := h.classifier.BuildContext(payload.Cwd)
		if h.liveFinder != nil {
			if inst := h.liveFinder.FindLiveInstance(sessionID); inst != nil {
				// Read via Snapshot(), not raw fields: PRStatusPoller mutates these same
				// fields on its own goroutine under inst.mu (session/instance.go's mu
				// doc comment mandates Snapshot() for reads outside the actor).
				ghInfo := inst.Snapshot().GitHub
				if ghInfo.GitHubPRNumber > 0 {
					classCtx.CIStatus = ghInfo.GitHubCheckConclusion
					// Staleness guard (Task 1.1.2b): a cached conclusion older than 2x the
					// poller's configured interval may no longer reflect the branch's real CI
					// state. Treat it as unknown rather than risk gating an irreversible
					// auto-approve (RequireCIPassing) on stale data.
					if time.Since(ghInfo.LastPRStatusCheck) > 2*h.pollInterval {
						classCtx.CIStatus = ""
					}
				}
				// Independent of PR/CI state — populate whenever a live instance is
				// found so MinSessionIdleMinutes rules can evaluate. Left at the Go
				// zero value (0) when no live instance is found (fail-closed contract,
				// see ClassificationContext.SessionIdleMinutes's doc comment).
				classCtx.SessionIdleMinutes = int(inst.GetTimeSinceLastMeaningfulOutput().Minutes())
			}
		}
		result := h.classifier.Classify(payload, classCtx)
		durationMs := time.Since(start).Milliseconds()

		if h.analyticsStore != nil {
			// Normalize RuleID the same way the default: branch below does before recording,
			// so the analytics breakdown and the review-queue card agree on category for an
			// unrecognized decision — result.RuleID is "" here (no rule lookup occurred), which
			// would otherwise bucket as EscalationNoMatch instead of EscalationUnexpected.
			recordResult := result
			if result.Decision != classifier.AutoAllow && result.Decision != classifier.AutoDeny && result.Decision != classifier.Escalate {
				recordResult.RuleID = classifier.RuleIDUnexpectedDecision
			}
			h.analyticsStore.RecordFromResult(payload, recordResult, sessionID, "", durationMs, source)
		}

		switch result.Decision {
		case classifier.AutoAllow:
			log.ForSession(sessionID).Info("[ApprovalHandler] auto-allowed", "tool", payload.ToolName, "rule", result.RuleID)
			if h.autoApprovalLog != nil {
				filePath, _ := payload.ToolInput["file_path"].(string)
				_ = h.autoApprovalLog.AppendAutoApproved(sessionID, "", payload.ToolName, filePath, result.RuleID, result.RuleName, result.Source, "allow")
			}
			h.writeDecision(w, "allow", "")
			return
		case classifier.AutoDeny:
			msg := result.Reason
			if result.Alternative != "" {
				msg = fmt.Sprintf("%s %s", msg, result.Alternative)
			}
			log.ForSession(sessionID).Info("[ApprovalHandler] auto-denied", "tool", payload.ToolName, "rule", result.RuleID, "msg", msg)
			if h.autoApprovalLog != nil {
				filePath, _ := payload.ToolInput["file_path"].(string)
				_ = h.autoApprovalLog.AppendAutoApproved(sessionID, "", payload.ToolName, filePath, result.RuleID, result.RuleName, result.Source, "deny")
			}
			h.writeDecision(w, "deny", msg)
			return
		case classifier.Escalate:
			escalation = result
			classified = true
			// Fall through to manual review queue (createApproval label below).
		default:
			// Unrecognized classifier.ClassificationDecision (e.g. a future 4th value). Fail safe
			// toward manual review rather than silently falling through with escalation unset —
			// this switch's missing-case behavior is exactly the bug this feature fixes; guard
			// against it recurring for any future decision value.
			log.Warn("[ApprovalHandler] unrecognized classifier decision, escalating for manual review", "decision", result.Decision)
			// Pre-mortem P3: route through the synthetic RuleIDUnexpectedDecision sentinel so
			// CategorizeEscalationRuleID buckets this as EscalationUnexpected, not EscalationNoMatch
			// (result.RuleID is almost certainly "" here, since no rule lookup occurred) — an internal
			// classifier bug must not silently render normal "no rule matched" copy or offer the
			// Create Rule CTA as if this were a real coverage gap. Override RuleID before the
			// assignment (not after) so escalation is never observably set without it.
			result.RuleID = classifier.RuleIDUnexpectedDecision
			escalation = result
			classified = true
		}
	}

createApproval:

	// Autonomous LLM approval: if the session is autonomous and a headless pool is configured,
	// ask the LLM to approve or deny instead of queuing for human review.
	if h.headlessPool != nil && h.autonomousChecker != nil && h.autonomousChecker(sessionID) {
		var sessionTail string
		if h.queueChecker != nil && sessionID != "unknown" {
			if inst := h.queueChecker.FindInstance(sessionID); inst != nil {
				sessionTail, _ = inst.Preview()
			}
		}
		query := buildApprovalQuery(payload.ToolName, payload.ToolInput, sessionTail)
		const approvalSystemPrompt = `You are a security reviewer for an autonomous coding session.
Evaluate the requested tool call and decide if it is safe to approve.
Reply with APPROVE: <reason> if safe, or DENY: <reason> if risky.`
		resp, llmErr := h.headlessPool.CallBlocking(
			r.Context(),
			headless.FeatureKeyAutonomousApproval,
			approvalSystemPrompt,
			query,
			headless.CallOptions{WorkDir: payload.Cwd},
			session.CostSinkForSessionUUID(h.storage, sessionID),
		)
		if llmErr == nil {
			resp = strings.TrimSpace(resp)
			if strings.HasPrefix(resp, "APPROVE:") {
				reason := strings.TrimSpace(strings.TrimPrefix(resp, "APPROVE:"))
				log.ForSession(sessionID).Info("[ApprovalHandler] autonomous LLM approved", "tool", payload.ToolName, "reason", reason)
				h.writeDecision(w, "allow", "")
				return
			} else if strings.HasPrefix(resp, "DENY:") {
				reason := strings.TrimSpace(strings.TrimPrefix(resp, "DENY:"))
				log.ForSession(sessionID).Info("[ApprovalHandler] autonomous LLM denied", "tool", payload.ToolName, "reason", reason)
				h.writeDecision(w, "deny", reason)
				return
			}
			log.ForSession(sessionID).Warn("[ApprovalHandler] autonomous LLM gave unexpected response, falling through to human queue", "tool", payload.ToolName, "resp", resp)
		} else {
			log.ForSession(sessionID).Warn("[ApprovalHandler] autonomous LLM call failed, falling through to human queue", "tool", payload.ToolName, "err", llmErr)
		}
	}

	// Create a pending approval record
	approvalID := uuid.New().String()
	riskLevel := ""
	if classified {
		riskLevel = riskLevelString(escalation.RiskLevel)
	}
	approval := &PendingApproval{
		ID:                 approvalID,
		SessionID:          sessionID,
		ClaudeSessionID:    payload.SessionID,
		ToolName:           payload.ToolName,
		ToolInput:          payload.ToolInput,
		Cwd:                payload.Cwd,
		PermissionMode:     payload.PermissionMode,
		CreatedAt:          time.Now(),
		EscalationReason:   truncateEscalationReason(classifier.EscalationReasonText(escalation)),
		EscalationCategory: string(classifier.CategorizeEscalationRuleID(escalation.RuleID)),
		RiskLevel:          riskLevel,
		// Use the configured timeout (default 4 minutes), strictly less than the 5-minute hook timeout.
		ExpiresAt: time.Now().Add(h.approvalTimeout()),
	}

	if err := h.store.Create(approval); err != nil {
		log.Error("[ApprovalHandler] failed to store approval", "err", err)
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
			log.ForSession(sessionID).Info("[ApprovalHandler] triggered immediate queue check")
		}
	}

	log.ForSession(sessionID).Info("[ApprovalHandler] waiting for decision", "approval_id", approvalID, "tool", payload.ToolName)

	// Block until user decides, server times out, or connection closes
	var decision ApprovalDecision
	select {
	case decision = <-approval.decisionCh:
		// User responded via ResolveApproval RPC
		log.ForSession(sessionID).Info("[ApprovalHandler] approval resolved", "approval_id", approvalID, "behavior", decision.Behavior)
	case <-time.After(h.approvalTimeout()):
		// Server-side timeout (before the hook's 5-minute timeout).
		h.store.Remove(approvalID)
		h.stampResolved(approvalID, sessionID, "timeout")
		log.ForSession(sessionID).Info("[ApprovalHandler] approval timed out", "approval_id", approvalID, "source", source)
		if source == "pi" {
			// Fail closed, deliberately (pi-support MAJOR 3): unlike Claude's
			// curl hook, pi's ssq-approval.ts extension has no native terminal
			// permission dialog to fall back to, so an empty 200 here would
			// leave the tool call in limbo at the mercy of the extension's own
			// fetch()-throws-on-empty-body behavior — an accident of the
			// client, not a contract. Deny explicitly instead.
			h.writeDecision(w, "deny", "stapler-squad approval timed out")
			return
		}
		// Claude: return an empty HTTP response so the hook script gets no
		// hookSpecificOutput and Claude Code falls back to its native terminal
		// permission dialog. This lets the user still approve/deny in the
		// terminal rather than being silently allowed or denied.
		w.WriteHeader(http.StatusOK)
		return
	case <-r.Context().Done():
		// Claude Code disconnected (e.g., stapler-squad restarted, network issue).
		// The client is already gone, so there is no response to write for
		// either source — decision is intentionally left unset/unused here.
		h.store.Remove(approvalID)
		h.stampResolved(approvalID, sessionID, "canceled")
		log.ForSession(sessionID).Info("[ApprovalHandler] approval context canceled", "approval_id", approvalID)
		return // Don't write to disconnected client
	}

	h.writeDecision(w, decision.Behavior, decision.Message)
}

// piExtensionHealthPingPayload is the JSON body ssqApprovalExtensionTemplate
// (cmd/ssq-hooks/main.go) POSTs to /api/hooks/pi-extension-loaded, both at
// extension-load time and on every periodic re-ping (Story 4.2.3). Cwd is the
// only field: the ping fires before any tool_call handler runs, so there is no
// event/ctx object carrying a session ID the way HandlePermissionRequest's
// payload does — cwd-prefix matching (the same fallback resolveSessionID
// already uses for a missing/unmatched X-CS-Session-ID header) is the only
// session-identifying signal available at that point.
type piExtensionHealthPingPayload struct {
	Cwd string `json:"cwd"`
}

// HandlePiExtensionLoaded records one pi approval-extension health ping
// (pi-support Epic 4.2). The ping is best-effort and fire-and-forget from the
// extension's side (see ssqApprovalExtensionTemplate's doc comment) — this
// handler always responds 200 regardless of whether the session could be
// resolved or the tracker recorded anything, so a malformed/unresolvable ping
// never surfaces as an error to the extension.
//
// Feature-flag gate as the handler's first line (same idiom as
// GitHubWebhookHandler.Handle / GenericWebhookHandler.Handle): with
// pi-support off, this never touches piHealthTracker's in-memory map — that
// mirrors every other pi surface (resume injection, UI preset, status
// source, extension injection/enforcement), which all check the flag before
// acting (see project_plans/pi-support/implementation/plan.md's Risk
// Control section).
//
// +http: POST /api/hooks/pi-extension-loaded hooks:pi-extension-loaded
func (h *ApprovalHandler) HandlePiExtensionLoaded(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !config.LoadConfig().GetFeatureFlag(config.FeaturePiSupport) {
		http.NotFound(w, r)
		return
	}

	var payload piExtensionHealthPingPayload
	_ = json.NewDecoder(r.Body).Decode(&payload) // best-effort; a malformed body still gets a 200

	if h.piHealthTracker != nil {
		sessionID := h.resolveSessionID(r.Header.Get("X-CS-Session-ID"), payload.Cwd)
		if sessionID != "" {
			h.piHealthTracker.RecordPing(sessionID)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// resolveSessionName returns the human-readable title for sessionID using the
// in-memory queueChecker (no DB side-effects). Falls back to sessionID itself
// when the instance cannot be found (e.g. external sessions, race at startup).
func (h *ApprovalHandler) resolveSessionName(sessionID string) string {
	if h.queueChecker != nil {
		if inst := h.queueChecker.FindInstance(sessionID); inst != nil {
			return inst.Title
		}
	}
	return sessionID
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

	title := fmt.Sprintf("Permission Required: %s", sanitizeNotificationText(approval.ToolName))
	message := buildApprovalMessage(approval)

	event := events.NewNotificationEvent(
		sessionID,
		h.resolveSessionName(sessionID),
		approval.ID, // Use approval ID as notification ID for correlation
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT),
		title,
		message,
		metadata,
	)
	h.eventBus.Publish(event)

	// Slack notification (Epic 1.3, Story 1.3.2): nil-guarded, matching every
	// other optional Set* dependency in this file. NotifyApprovalPending
	// performs its own dispatchAsync wrapping internally (Story 1.2.3's
	// ownership model) — no separate goroutine needed at this call site.
	if h.slackNotifier != nil {
		cfg := config.LoadConfig()
		dashboardURL := cfg.Slack.DashboardBaseURL
		if dashboardURL == "" && h.dashboardBaseURLFn != nil {
			dashboardURL = h.dashboardBaseURLFn()
		}
		h.slackNotifier.NotifyApprovalPending(context.Background(), cfg, approval, h.resolveSessionName(sessionID), dashboardURL)
	}
}

// maxNotificationMessageLen is the maximum number of runes to include in a
// notification toast message before truncating with "...".
const maxNotificationMessageLen = 120

// broadcastQuestionNotification fires an INPUT_REQUIRED notification when Claude uses
// AskUserQuestion. It omits approval_id from metadata so no Approve/Deny buttons are shown —
// only a ❓ toast directing the user to respond in the terminal.
func (h *ApprovalHandler) broadcastQuestionNotification(sessionID string, payload classifier.PermissionRequestPayload) {
	message := "Check the terminal to respond."
	if prompt, ok := payload.ToolInput["prompt"].(string); ok && prompt != "" {
		message = truncateString(prompt, maxNotificationMessageLen)
	}

	event := events.NewNotificationEvent(
		sessionID,
		h.resolveSessionName(sessionID),
		uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
		"Claude has a question",
		message,
		nil,
	)
	h.eventBus.Publish(event)
}

// truncateString returns s truncated to at most maxRunes Unicode code points,
// appending "..." when truncation occurs. Safe for any UTF-8 content.
func truncateString(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}

// maxEscalationReasonLen bounds PendingApproval.EscalationReason. An explicit
// rule's Reason is free text a rule author can set to any length, and
// persistToDiskLocked re-marshals and writes ALL pending approvals to disk on
// every single Create/Resolve while holding the write lock — an unbounded
// string here would scale that cost with rule-author verbosity, not just
// entry count.
const maxEscalationReasonLen = 500

// truncateEscalationReason caps s at maxEscalationReasonLen runes.
func truncateEscalationReason(s string) string {
	return truncateString(s, maxEscalationReasonLen)
}

// sanitizeNotificationText strips newlines and non-printable characters from
// user- or model-controlled strings before embedding them in OS notification
// titles, preventing newline injection into the OS notification tray.
func sanitizeNotificationText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r < 32 {
			return ' '
		}
		return r
	}, s)
}

// buildApprovalMessage builds the human-readable message for an approval notification.
func buildApprovalMessage(approval *PendingApproval) string {
	if cmd, ok := approval.ToolInput["command"].(string); ok && cmd != "" {
		return truncateString(cmd, maxNotificationMessageLen)
	}
	if filePath, ok := approval.ToolInput["file_path"].(string); ok && filePath != "" {
		return filePath
	}
	return fmt.Sprintf("Claude needs permission to use %s", approval.ToolName)
}

// resolveSessionID returns the stable session ID (UUID) for a given raw header
// value and cwd. It tries the header first (matching by title or UUID via
// stableIDForData/matchesIDData), then falls back to cwd prefix matching.
// Uses ListInstanceData to avoid constructing Instance objects and spawning
// tmux subprocesses for every stopped session.
func (h *ApprovalHandler) resolveSessionID(headerVal, cwd string) string {
	if h.storage == nil {
		return headerVal
	}
	instances, err := h.storage.ListInstanceData()
	if err != nil {
		return headerVal
	}
	if headerVal != "" {
		for _, d := range instances {
			if matchesIDData(d, headerVal) {
				return stableIDForData(d)
			}
		}
	}
	// Fall back to cwd prefix matching
	bestID := ""
	bestLen := 0
	for _, d := range instances {
		if p := d.Path; p != "" && strings.HasPrefix(cwd, p) && len(p) > bestLen {
			bestID = stableIDForData(d)
			bestLen = len(p)
		}
		if wd := d.WorkingDir; wd != "" && strings.HasPrefix(cwd, wd) && len(wd) > bestLen {
			bestID = stableIDForData(d)
			bestLen = len(wd)
		}
	}
	return bestID
}

// stableIDForData mirrors Instance.GetStableID for raw InstanceData.
func stableIDForData(d session.InstanceData) string {
	if d.UUID != "" {
		return d.UUID
	}
	return d.Title
}

// matchesIDData mirrors Instance.MatchesID for raw InstanceData without constructing
// an Instance or spawning any subprocesses. Matches by UUID, title, or computed
// tmux session name (prefix + sanitized title). The tmux-name branch requires a
// non-empty TmuxPrefix to avoid a title-only match that would bypass UUID lookup.
//
// TODO: move stableIDForData and matchesIDData to session.InstanceData methods so
// the matching semantics are colocated with the type they operate on.
func matchesIDData(d session.InstanceData, id string) bool {
	if d.UUID != "" && d.UUID == id {
		return true
	}
	if d.Title == id {
		return true
	}
	// Tmux-name match requires a prefix; without it d.TmuxPrefix+title == title
	// which duplicates the title check above and allows UUID-less resolution.
	if d.TmuxPrefix == "" {
		return false
	}
	// Derive via the canonical sanitizer rather than re-implementing it here —
	// a hand-rolled copy silently drifts from tmux.NewSessionName's rules (#162).
	return tmux.NewSessionName(d.Title, d.TmuxPrefix).String() == id
}

// writeDeferDecision returns an empty HTTP 200 with no body.
// Claude Code interprets the absence of hookSpecificOutput as "no decision made by the hook"
// and falls back to its native terminal permission dialog.
func (h *ApprovalHandler) writeDeferDecision(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
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
		log.Warn("[ApprovalHandler] failed to write decision response", "err", err)
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
					log.Info("[ApprovalStore] cleaned up expired approvals", "count", len(expired), "ids", expired)
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

const (
	hookTimeout = 300 // seconds — must be ≤ Claude Code's 5-minute hook timeout
)

// hookApprovalURL returns the current PermissionRequest hook callback URL. It delegates to
// hook_injector.go's hookEndpoints(hookBaseURLFn) — the single source of truth for hook URLs,
// shared with InjectHooksConfig — rather than maintaining a second, parallel lazy-base-URL
// mechanism. Resolved fresh on every call (never cached), so all usage sites in
// InjectHookConfig below reflect whatever base URL is current at their point of use rather
// than a value baked in at server- or package-construction time.
func hookApprovalURL() string {
	return hookEndpoints(getHookBaseURLFn())[HookPermissionApproval]
}

// InjectHookConfig writes (or merges) the stapler-squad PermissionRequest HTTP hook
// into <rootDir>/.claude/settings.local.json.
//
// If the file already contains a hook pointing to hookApprovalURL(), it is left unchanged.
// If the file exists but lacks our hook, the hook is prepended to PermissionRequest.
// If the file does not exist, it is created with just our hook config.
//
// Local-only: rootDir must be a path on THIS host (os.ReadFile/os.WriteFile).
// For a remote session's rootDir (a path that only exists on the remote
// host), use InjectHookConfigRemote instead -- calling this on a remote
// path silently produces no hook at all (os.ReadFile treats the missing
// local path as "file doesn't exist" and this then writes a settings file
// nobody remote will ever read), which was the ssh-remote-workspaces Phase
// 5 bug this pair of functions exists to fix. See ADR-003's addendum.
func InjectHookConfig(rootDir, sessionTitle string) error {
	claudeDir := filepath.Join(rootDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	// Serializes the read-merge-write sequence below against InjectHooksConfig and
	// RemoveHooksConfig, which independently read-modify-write the same settingsPath —
	// see settingsFileLocks' doc comment in mcp_injector.go for the lost-update hazard
	// this closes.
	defer lockSettingsPath(settingsPath)()

	url := hookApprovalURL()
	// Desired hook entry for this session.
	// settings.local.json only supports "command" type hooks; use curl to POST to the approval URL.
	curlCmd := fmt.Sprintf(
		"curl -s --max-time %d -X POST '%s' -H 'Content-Type: application/json' -H 'X-CS-Session-ID: %s' -d @-",
		hookTimeout, url, sessionTitle,
	)
	entry := hookEntry{Type: "command", Command: curlCmd, Timeout: hookTimeout}

	// Read existing settings (if any).
	data, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", settingsPath, err)
	}

	out, alreadyPresent, err := mergeHookEntryIntoSettings(data, entry, func(command string) bool {
		return hookCommandReferencesURL(command, url)
	})
	if err != nil {
		return err
	}
	if alreadyPresent {
		log.Debug("[InjectHookConfig] hook already present", "path", settingsPath)
		return nil
	}

	// Re-parse out back into a raw map so this shares writeSettingsAtomic's
	// unique-tmp-filename write with hook_injector.go's settings.local.json
	// writers, instead of the fixed settingsPath+".tmp" name this used to write via a
	// direct os.WriteFile: two concurrent writers of the same rootDir's settings.local.json
	// (e.g. this and InjectHooksConfig racing on session creation) could
	// clobber each other's temp file mid-write and rename in a corrupt result. See
	// writeSettingsAtomic's doc comment for the identical hazard it already fixes.
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(out, &raw); err != nil {
		return fmt.Errorf("re-parse merged settings: %w", err)
	}
	if err := writeSettingsAtomic(settingsPath, claudeDir, raw); err != nil {
		return err
	}
	log.Info("[InjectHookConfig] wrote hook config", "path", settingsPath, "session", sessionTitle)
	return nil
}

// mergeHookEntryIntoSettings computes the merged settings.local.json bytes
// for a single PermissionRequest hook entry, given existingData (the raw
// bytes currently on disk/remote-host, or nil/empty if the file doesn't
// exist yet) and the pre-built entry to inject. alreadyPresentFn decides
// whether an existing command-type hook already matches entry -- callers
// pass a URL-based match (hookCommandReferencesURL) for the local HTTP path
// or a socket-based match (hookCommandTargetsSocket) for the remote path,
// since the two paths generate differently-shaped commands for the same
// logical hook.
//
// Extracted out of InjectHookConfig so InjectHookConfig (local, writes via
// os.WriteFile) and InjectHookConfigRemote (writes via a piped `sh -c` over
// tmux.CommandRunner) share exactly ONE implementation of the merge/repair
// logic rather than two that could silently drift -- ssh-remote-workspaces
// Phase 5 correction, see ADR-003's addendum.
//
// Returns the final, fully-marshaled settings JSON (ready to write
// verbatim) and whether entry's hook was already present (in which case out
// is nil and the caller should skip writing anything).
func mergeHookEntryIntoSettings(existingData []byte, entry hookEntry, alreadyPresentFn func(command string) bool) (out []byte, alreadyPresent bool, err error) {
	raw := parseSettingsWithRepair(existingData, "mergeHookEntryIntoSettings")

	existingGroups := permissionRequestGroupsFromSettings(raw)
	if hookAlreadyPresentInGroups(existingGroups, alreadyPresentFn) {
		return nil, true, nil
	}

	prGroups := mergePermissionRequestGroups(existingGroups, entry)

	hooksMap := map[string]json.RawMessage{}
	if hooksRaw, ok := raw["hooks"]; ok {
		_ = json.Unmarshal(hooksRaw, &hooksMap)
	}
	prJSON, err := json.Marshal(prGroups)
	if err != nil {
		return nil, false, fmt.Errorf("marshal PermissionRequest hooks: %w", err)
	}
	hooksMap["PermissionRequest"] = json.RawMessage(prJSON)

	hooksJSON, err := json.Marshal(hooksMap)
	if err != nil {
		return nil, false, fmt.Errorf("marshal hooks map: %w", err)
	}
	raw["hooks"] = json.RawMessage(hooksJSON)

	out, err = json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal settings: %w", err)
	}
	return out, false, nil
}

// parseSettingsWithRepair unmarshals existingData as a settings.local.json top-level map,
// attempting repairSettingsJSON's targeted fix for common corruption before falling back to a
// fresh, empty config. logPrefix names the caller in the resulting log lines (mirrors
// InjectHooksConfig's own identical repair step in hook_injector.go's
// readExistingHooksSettings -- kept as two call sites, not a shared helper, since one operates
// on a file path and the other on already-read bytes from a remote host).
func parseSettingsWithRepair(existingData []byte, logPrefix string) map[string]json.RawMessage {
	raw := map[string]json.RawMessage{}
	if len(existingData) == 0 {
		return raw
	}
	if err := json.Unmarshal(existingData, &raw); err != nil {
		log.Warn("["+logPrefix+"] invalid JSON, attempting repair", "err", err)
		repaired, repairErr := repairSettingsJSON(existingData)
		if repairErr != nil {
			log.Warn("["+logPrefix+"] could not repair settings, resetting to minimal config", "err", repairErr)
			return map[string]json.RawMessage{}
		}
		log.Info("[" + logPrefix + "] repaired settings file")
		_ = json.Unmarshal(repaired, &raw) // best-effort; raw may still be partial
	}
	return raw
}

// permissionRequestGroupsFromSettings extracts raw's "hooks" -> "PermissionRequest"
// hookMatcherGroup list, returning nil (not an error) if either level is absent or malformed --
// every caller already treats "no existing entries" and "couldn't parse existing entries"
// identically.
func permissionRequestGroupsFromSettings(raw map[string]json.RawMessage) []hookMatcherGroup {
	hooksRaw, ok := raw["hooks"]
	if !ok {
		return nil
	}
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
		return nil
	}
	prRaw, ok := hooks["PermissionRequest"]
	if !ok {
		return nil
	}
	var groups []hookMatcherGroup
	_ = json.Unmarshal(prRaw, &groups)
	return groups
}

// hookAlreadyPresentInGroups reports whether any command-type hook in groups matches
// alreadyPresentFn.
func hookAlreadyPresentInGroups(groups []hookMatcherGroup, alreadyPresentFn func(command string) bool) bool {
	for _, g := range groups {
		for _, h := range g.Hooks {
			if h.Type == "command" && alreadyPresentFn(h.Command) {
				return true
			}
		}
	}
	return false
}

// mergePermissionRequestGroups prepends a group containing entry ahead of existingGroups, while
// migrating away old http-type entries that point at our own URL (hookApprovalURL()) -- a group
// left with zero hooks after that filtering is dropped entirely rather than kept empty.
func mergePermissionRequestGroups(existingGroups []hookMatcherGroup, entry hookEntry) []hookMatcherGroup {
	group := hookMatcherGroup{Hooks: []hookEntry{entry}}
	var kept []hookMatcherGroup
	for _, g := range existingGroups {
		filtered := g.Hooks[:0]
		for _, h := range g.Hooks {
			if h.URL != hookApprovalURL() {
				filtered = append(filtered, h)
			}
		}
		if len(filtered) > 0 {
			g.Hooks = filtered
			kept = append(kept, g)
		}
	}
	return append([]hookMatcherGroup{group}, kept...)
}

// InjectHookConfigRemote is InjectHookConfig's remote-host counterpart
// (ssh-remote-workspaces Phase 5, ADR-003): it reads, merges, and writes
// back <rootDir>/.claude/settings.local.json on the OTHER end of runner
// instead of this process's local filesystem, and routes the generated
// PermissionRequest hook at target's Unix socket (via
// remoteApprovalHookCommand's socat pipeline, the same mechanism
// InjectHooksConfig's WithRemoteHookTarget option already builds for the
// plural entry point) instead of hookApprovalURL()'s HTTP endpoint --
// curl-over-HTTP can never reach this process from a different host the way
// it can from rootDir on this same machine.
//
// rootDir is the session's remote-host working-directory root (its
// worktree path on the remote host, e.g. instance.GetEffectiveRootDir());
// sessionID is used only for log messages here (the X-CS-Session-ID value
// actually delivered to ApprovalHandler comes from RemoteApprovalRelayTarget.
// StableSessionID, embedded by the relay itself, not from this function).
func InjectHookConfigRemote(ctx context.Context, runner tmux.CommandRunner, rootDir, sessionID string, target RemoteHookTarget) error {
	claudeDir := path.Join(rootDir, ".claude")
	settingsPath := path.Join(claudeDir, "settings.local.json")

	// Read existing settings: empty output, no error, if the file doesn't exist yet (mirrors
	// os.ReadFile+os.IsNotExist's "missing means empty" semantics) -- but unlike an
	// unconditional `cat ... 2>/dev/null || true`, a file that DOES exist but can't be read
	// (permission denied, etc.) still surfaces as a real error here rather than being silently
	// treated as empty and then clobbered by the write below. `[ -e path ]` gates whether cat
	// runs at all; if it does, cat's own exit code is the script's exit code (the last command
	// in an `sh -c` script determines its overall status), which runner.Run reports as err.
	readScript := "if [ -e " + posixShellQuoteRemote(settingsPath) + " ]; then cat " + posixShellQuoteRemote(settingsPath) + "; fi"
	data, err := runner.Run(ctx, "", "sh", "-c", readScript)
	if err != nil {
		return fmt.Errorf("read remote %s: %w", settingsPath, err)
	}

	curlCmd := remoteApprovalHookCommand(target)
	entry := hookEntry{Type: "command", Command: curlCmd, Timeout: hookTimeout}

	out, alreadyPresent, err := mergeHookEntryIntoSettings(data, entry, func(command string) bool {
		return hookCommandTargetsSocket(command, target.SocketPath)
	})
	if err != nil {
		return err
	}
	if alreadyPresent {
		log.Debug("[InjectHookConfigRemote] hook already present", "path", settingsPath, "session", sessionID)
		return nil
	}

	writeScript := "mkdir -p " + posixShellQuoteRemote(claudeDir) + " && cat > " + posixShellQuoteRemote(settingsPath)
	stdin, stdout, wait, err := runner.Start(ctx, "", "sh", "-c", writeScript)
	if err != nil {
		return fmt.Errorf("start remote write for %s: %w", settingsPath, err)
	}
	// wait() must run on every exit path once Start has succeeded -- per
	// SSHRunner.Start's doc comment, skipping it leaks the connection pool's
	// reference count (acquire/Release never balance) and leaves the remote
	// session/channel open. waited tracks whether the explicit call below
	// (the happy path, which also surfaces the remote script's real exit
	// error) already ran it, so this deferred call is purely a safety net
	// for an earlier return -- a stdin.Write/Close failure -- and never
	// double-invokes wait() on the same path.
	waited := false
	defer func() {
		if !waited {
			_ = wait()
		}
	}()
	if _, err := stdin.Write(out); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("write remote settings %s: %w", settingsPath, err)
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("close remote settings stdin for %s: %w", settingsPath, err)
	}
	if stdout != nil {
		_, _ = io.Copy(io.Discard, stdout) // drain, best-effort; the script has no meaningful stdout
	}
	waited = true
	if err := wait(); err != nil {
		return fmt.Errorf("remote write %s failed: %w", settingsPath, err)
	}
	log.Info("[InjectHookConfigRemote] wrote hook config", "path", settingsPath, "session", sessionID)
	return nil
}

// posixShellQuoteRemote POSIX-single-quotes s so it survives the remote
// `sh -c` scripts InjectHookConfigRemote builds, unmodified regardless of
// embedded spaces or shell metacharacters. Package-local rather than
// exporting session/git's equivalent posixShellQuote (a different package)
// or session/tmux's unexported shellQuote (also a different package, and
// itself unexported there) -- same three-line escaping strategy (close
// quote, literal quote, reopen quote), independently duplicated because
// it's not worth promoting to a shared export for one caller on each side,
// per session/git/remote_worktree.go's own posixShellQuote doc comment
// making the identical call.
func posixShellQuoteRemote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
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
