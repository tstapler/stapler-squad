// Package workflows provides the WorkflowScheduler for cron-based session automation.
package workflows

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// SessionServiceInterface is the minimal interface the scheduler needs from SessionService.
// Defined here to avoid a circular import: server/workflows does not import server/services.
type SessionServiceInterface interface {
	CreateSession(ctx context.Context, req *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error)
}

// AdmissionGate is the narrow consumer interface Scheduler needs to check the shared
// backlog-work-item WIP cap before firing a trigger-created session (webhook-triggers
// Epic 1.3 — closes the pre-existing bypass where FireNow called CreateSession directly,
// skipping the same MaxConcurrentBacklogWorkItems check BacklogService's own spawn path
// enforces). Defined here (consumer-defined), not in server/services, to avoid a
// server/workflows → server/services import — per .claude/rules/interface-pollution-
// checklist.md. Satisfied by *services.BacklogService's Admit method.
type AdmissionGate interface {
	// Admit reports whether a new trigger-fired session may be created right now.
	Admit(ctx context.Context) (bool, error)
}

// triggerFireEventRecorder is the narrow interface Scheduler needs to persist a
// trigger-fire audit row. Satisfied by session.TriggerFireEventRepository (only the
// Create method is needed here, so the consumer interface stays narrow).
type triggerFireEventRecorder interface {
	Create(ctx context.Context, input session.TriggerFireEventInput) error
}

// triggerRateLimiterGate is the narrow interface Scheduler needs for per-Workflow rate
// limiting (webhook-triggers Epic 2.4.2) — satisfied by *services.TriggerRateLimiter's
// Allow method. Defined here (consumer-defined), not in server/services, to avoid a
// server/workflows -> server/services import — per .claude/rules/interface-pollution-
// checklist.md.
type triggerRateLimiterGate interface {
	// Allow reports whether a fire for workflowID is permitted right now.
	Allow(workflowID uuid.UUID) bool
}

// Scheduler manages cron-based workflow execution.
type Scheduler struct {
	c          *cron.Cron
	repo       session.WorkflowRepository
	sessionSvc SessionServiceInterface
	eventBus   *events.EventBus
	mu         sync.Mutex
	entryMap   map[string]cron.EntryID // workflowID → cron.EntryID

	// admissionGate is consulted before every CreateSession call inside FireNow. nil
	// (the default) disables the check — matches this codebase's existing nil-safe
	// optional-dependency convention. Wired via SetAdmissionGate.
	admissionGate AdmissionGate
	// fireEventRepo persists TriggerFireEvent audit rows (Epic 1.2). nil (the default)
	// disables audit-row persistence — Scheduler still functions, it just doesn't leave
	// a durable record. Wired via SetTriggerFireEventRepo.
	fireEventRepo triggerFireEventRecorder
	// rateLimiter enforces a per-Workflow fire rate (Epic 2.4.2). nil (the default)
	// disables rate limiting. Wired via SetRateLimiter.
	rateLimiter triggerRateLimiterGate
}

// NewScheduler creates a new WorkflowScheduler.
func NewScheduler(repo session.WorkflowRepository, sessionSvc SessionServiceInterface, eventBus *events.EventBus) *Scheduler {
	return &Scheduler{
		c: cron.New(
			cron.WithParser(cron.NewParser(
				cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
			)),
		),
		repo:       repo,
		sessionSvc: sessionSvc,
		eventBus:   eventBus,
		entryMap:   make(map[string]cron.EntryID),
	}
}

// SetAdmissionGate wires the WIP-cap admission check. A setter (not a NewScheduler
// parameter) so existing construction call sites in server/dependencies.go stay
// minimally diffed — see Task 1.3.1a.
func (s *Scheduler) SetAdmissionGate(g AdmissionGate) {
	s.admissionGate = g
}

// SetTriggerFireEventRepo wires the trigger-fire audit trail (Epic 1.2). A setter for
// the same reason as SetAdmissionGate.
func (s *Scheduler) SetTriggerFireEventRepo(repo triggerFireEventRecorder) {
	s.fireEventRepo = repo
}

// SetRateLimiter wires the per-Workflow fire rate limit (Epic 2.4.2). A setter for the
// same reason as SetAdmissionGate.
func (s *Scheduler) SetRateLimiter(limiter triggerRateLimiterGate) {
	s.rateLimiter = limiter
}

// recordFireEvent persists a TriggerFireEvent audit row if fireEventRepo is wired.
// Failures are logged, not propagated — an audit-trail write failure must never block
// or mask the caller's own fire-path error.
func (s *Scheduler) recordFireEvent(ctx context.Context, wf *ent.Workflow, outcome, deliveryID, sessionID, errMsg string) {
	if s.fireEventRepo == nil {
		return
	}
	input := session.TriggerFireEventInput{
		Outcome:      outcome,
		DeliveryID:   deliveryID,
		SessionID:    sessionID,
		ErrorMessage: errMsg,
	}
	if wf != nil {
		id := wf.ID
		input.WorkflowID = &id
	}
	if err := s.fireEventRepo.Create(ctx, input); err != nil {
		slug := ""
		if wf != nil {
			slug = wf.Slug
		}
		log.Warn("[WorkflowScheduler] failed to record trigger fire event",
			"slug", slug, "outcome", outcome, "err", err)
	}
}

// Start loads all enabled workflows and begins cron processing.
// Stops when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	if s.repo == nil {
		log.Warn("[WorkflowScheduler] repo is nil, scheduler disabled")
		return
	}

	// One-time backfill of trigger_type on rows that predate the field (Task 1.1.1d).
	backfillTriggerTypes(ctx, s.repo)

	wfs, err := s.repo.ListEnabled(ctx)
	if err != nil {
		log.Error("[WorkflowScheduler] failed to load enabled workflows on start", "err", err)
	} else {
		s.mu.Lock()
		for _, wf := range wfs {
			if err := s.addCronEntry(wf); err != nil {
				log.Warn("[WorkflowScheduler] failed to register cron entry on start", "slug", wf.Slug, "err", err)
			}
		}
		s.mu.Unlock()
		log.Info("[WorkflowScheduler] loaded enabled workflows", "count", len(wfs))
	}

	s.c.Start()
	// Stop is registered as a shutdown hook in server.go; no goroutine needed here.
}

// Stop halts the cron engine. Called as a shutdown hook.
func (s *Scheduler) Stop() {
	stopCtx := s.c.Stop()
	select {
	case <-stopCtx.Done():
		log.Info("[WorkflowScheduler] stopped cleanly")
	case <-time.After(8 * time.Second):
		log.Warn("[WorkflowScheduler] stop timed out — in-flight cron jobs may have leaked")
	}
}

// Reload registers or re-registers a workflow's cron job.
// Called after create/update. If cron_enabled is false, removes any existing entry.
func (s *Scheduler) Reload(ctx context.Context, wf *ent.Workflow) error {
	if wf == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	id := wf.ID.String()

	// Remove existing entry if present.
	if entryID, ok := s.entryMap[id]; ok {
		s.c.Remove(entryID)
		delete(s.entryMap, id)
	}

	// Defense in depth (Task 1.1.1e / pre-mortem P1 #2): also require trigger_type ==
	// "cron" so a mismatched row (e.g. trigger_type="webhook" with cron_enabled=true,
	// left over from a stale form default or a direct DB write) can never register as
	// BOTH a cron entry and a webhook route.
	if !wf.CronEnabled || wf.TriggerType != "cron" {
		return nil
	}

	return s.addCronEntry(wf)
}

// Remove removes a workflow's cron job by workflow ID string.
// Safe to call when no entry exists (no-op).
func (s *Scheduler) Remove(workflowID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entryMap[workflowID]; ok {
		s.c.Remove(entryID)
		delete(s.entryMap, workflowID)
	}
	return nil
}

// FireNow immediately fires a workflow outside of cron schedule.
// Returns the created session ID. Used by RunWorkflow RPC and internal cron trigger.
//
// A thin wrapper around FireTrigger (Task 3.2.1a): builds the {{input}}-substituted
// prompt from wf.Command/wf.InputTemplate/arg exactly as before, then delegates all
// admission/rate-limit/CreateSession/audit logic to FireTrigger with deliveryID=""
// (FireNow's manual/cron callers have no webhook delivery to attribute the fire to).
func (s *Scheduler) FireNow(ctx context.Context, wf *ent.Workflow, arg string) (string, error) {
	// Build prompt: command is the primary instruction (required). If inputTemplate is
	// set it is appended after interpolation. If only arg is present and command has no
	// {{input}} placeholder, append the arg as additional context.
	var parts []string
	if wf.Command != "" {
		cmdPart := wf.Command
		if arg != "" && strings.Contains(cmdPart, "{{input}}") {
			cmdPart = strings.ReplaceAll(cmdPart, "{{input}}", arg)
		}
		parts = append(parts, cmdPart)
	}
	argInjectedIntoCommand := wf.Command != "" && strings.Contains(wf.Command, "{{input}}")
	if wf.InputTemplate != "" {
		tmplPart := wf.InputTemplate
		if arg != "" {
			tmplPart = strings.ReplaceAll(tmplPart, "{{input}}", arg)
		}
		parts = append(parts, tmplPart)
	} else if arg != "" && !argInjectedIntoCommand {
		parts = append(parts, arg)
	}
	prompt := strings.Join(parts, "\n\n")

	return s.FireTrigger(ctx, wf, prompt, "")
}

// FireTrigger fires wf with an already-constructed prompt (renderedPrompt), running the
// shared post-prompt-construction logic every trigger type converges on (Task 3.2.1a):
// per-Workflow rate limit, WIP-cap admission gate, CreateSession, and a last_fired_at
// bump on success. Returns the created session ID.
//
// deliveryID identifies the inbound webhook delivery that caused this fire, or "" for
// FireNow's manual/cron callers. It is significant to the audit trail: webhook callers
// (server/services/webhook_trigger_common.go's claimAndFireTrigger /
// renderAndFireTrigger) already claim a "pending" TriggerFireEvent row for
// (wf.ID, deliveryID) via TriggerFireEventRepository.Create *before* calling
// FireTrigger, and update that same row's outcome themselves once FireTrigger returns
// — so FireTrigger must not attempt its own Create for a non-empty deliveryID, which
// would collide with the already-claimed row (ErrDuplicateDelivery) and log a spurious
// warning without fixing the row's outcome anyway. For deliveryID == "" (FireNow's
// callers, which never pre-claim a row), FireTrigger's own recordFireEvent call is the
// only place a rate-limit/admission-gate rejection ever gets an audit trail — preserved
// unchanged from FireNow's pre-Phase-3 behavior.
func (s *Scheduler) FireTrigger(ctx context.Context, wf *ent.Workflow, renderedPrompt string, deliveryID string) (string, error) {
	if s.sessionSvc == nil {
		return "", fmt.Errorf("session service not available")
	}

	recordGateRejection := func(reason string) {
		if deliveryID != "" {
			return
		}
		s.recordFireEvent(ctx, wf, "fired_failed", deliveryID, "", reason)
	}

	// Per-Workflow rate limit (Epic 2.4.2): a noisy/malicious trigger source can't spawn
	// unbounded sessions. Checked before the admission gate so a throttled fire is
	// distinguishable (in logs/audit) from one rejected by the WIP cap.
	if s.rateLimiter != nil && !s.rateLimiter.Allow(wf.ID) {
		reason := "rate limit exceeded"
		log.Warn("[WorkflowScheduler] FireTrigger: rate limited", "slug", wf.Slug, "reason", reason)
		recordGateRejection(reason)
		return "", fmt.Errorf("rate limit exceeded for workflow %q", wf.Slug)
	}

	// WIP-cap admission check (Epic 1.3): every trigger-fired session must pass the
	// same MaxConcurrentBacklogWorkItems gate BacklogService's own spawn path enforces
	// — previously bypassed entirely here. Rejected (not queued, not silently
	// dropped): logged and persisted as a fired_failed TriggerFireEvent.
	if s.admissionGate != nil {
		admitted, admitErr := s.admissionGate.Admit(ctx)
		if admitErr != nil || !admitted {
			reason := "WIP limit reached"
			if admitErr != nil {
				reason = admitErr.Error()
			}
			log.Warn("[WorkflowScheduler] FireTrigger: admission rejected", "slug", wf.Slug, "reason", reason)
			recordGateRejection(reason)
			return "", fmt.Errorf("admission rejected for workflow %q: %s", wf.Slug, reason)
		}
	}

	title := fmt.Sprintf("%s — %s", wf.Name, time.Now().Format("2006-01-02 15:04"))

	sessionType := sessionTypeToProto(session.SessionType(wf.SessionType))

	// Append --model flag when a model is specified and the program is claude (or defaulting to claude).
	program := wf.AgentType
	if wf.Model != "" {
		isClaudeProgram := program == "" || program == "claude"
		if isClaudeProgram {
			program = "claude --model " + wf.Model
		}
	}

	// Deliberately mirrors a manually-created CreateSessionRequest field-for-field
	// aside from WorkflowId (attribution): no auto_yes/skip_defaults or other
	// bypass/elevated-permission field is set here, so a trigger-fired session goes
	// through the exact same approval/review path a manually-created one does (Goal 4 —
	// see TestFireTrigger_NeverSetsAutoApproveFlag).
	req := connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:         title,
		Path:          wf.TargetDirectory,
		Program:       program,
		InitialPrompt: renderedPrompt,
		SessionType:   sessionType,
		WorkflowId:    wf.ID.String(),
	})
	resp, err := s.sessionSvc.CreateSession(ctx, req)
	if err != nil {
		log.Error("[WorkflowScheduler] FireTrigger: failed to create session", "slug", wf.Slug, "err", err)
		return "", fmt.Errorf("create session for workflow %q: %w", wf.Slug, err)
	}

	sessionID := ""
	if sess := resp.Msg.GetSession(); sess != nil {
		sessionID = sess.GetId()
	}
	log.Info("[WorkflowScheduler] fired workflow", "slug", wf.Slug, "session_id", sessionID, "title", title)

	// Bump last_fired_at on every successful fire (Phase 4's missed-fire detection,
	// Task 4.1.1b, reads this to compute whether an expected cron occurrence was
	// missed while the process was down). Best-effort: a failed bump is logged, not
	// propagated — it must never turn an otherwise-successful fire into an error.
	if s.repo != nil {
		now := time.Now()
		if _, updErr := s.repo.Update(ctx, wf.ID, session.WorkflowUpdateInput{LastFiredAt: &now}); updErr != nil {
			log.Warn("[WorkflowScheduler] failed to update last_fired_at", "slug", wf.Slug, "err", updErr)
		}
	}

	return sessionID, nil
}

// addCronEntry adds a cron entry for the given workflow. Must be called with mu held.
// Guards trigger_type itself (not just relying on callers) since both Start's
// ListEnabled loop and Reload funnel through here — see Task 1.1.1f's Scheduler.Start
// dual-registration-guard test.
func (s *Scheduler) addCronEntry(wf *ent.Workflow) error {
	if !wf.CronEnabled || wf.TriggerType != "cron" {
		return fmt.Errorf("workflow %q is not a cron trigger (cron_enabled=%v trigger_type=%q), not registering",
			wf.Slug, wf.CronEnabled, wf.TriggerType)
	}
	if wf.CronExpression == "" {
		return fmt.Errorf("workflow %q has cron_enabled=true but empty cron_expression", wf.Slug)
	}

	wfCopy := wf // capture for closure
	entryID, err := s.c.AddFunc(wf.CronExpression, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if _, fireErr := s.FireNow(ctx, wfCopy, ""); fireErr != nil {
			log.Error("[WorkflowScheduler] cron job failed", "slug", wfCopy.Slug, "err", fireErr)
		}
	})
	if err != nil {
		return fmt.Errorf("register cron entry for workflow %q: %w", wf.Slug, err)
	}
	s.entryMap[wf.ID.String()] = entryID
	return nil
}

// ValidateCronExpression validates a 5-field cron expression.
// Exported so workflow_service.go can use it without importing the cron library directly.
func ValidateCronExpression(expr string) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	return err
}

// sessionTypeToProto converts a session.SessionType string to the proto enum.
func sessionTypeToProto(st session.SessionType) sessionv1.SessionType {
	switch st {
	case session.SessionTypeDirectory:
		return sessionv1.SessionType_SESSION_TYPE_DIRECTORY
	case session.SessionTypeNewWorktree:
		return sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE
	case session.SessionTypeExistingWorktree:
		return sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE
	case session.SessionTypeNewProject:
		return sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT
	case session.SessionTypeOneOff:
		return sessionv1.SessionType_SESSION_TYPE_ONE_OFF
	default:
		return sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED
	}
}
