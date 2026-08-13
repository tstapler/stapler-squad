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

	// modelFamilies maps a family alias (e.g. "sonnet") to the concrete model ID
	// it currently resolves to. Defaults to DefaultModelFamilies(); overridden via
	// SetModelFamilies (see LoadModelFamilyOverride).
	modelFamilies map[string]string
}

// NewScheduler creates a new WorkflowScheduler.
func NewScheduler(repo session.WorkflowRepository, sessionSvc SessionServiceInterface, eventBus *events.EventBus) *Scheduler {
	return &Scheduler{
		c: cron.New(
			cron.WithParser(cron.NewParser(
				cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
			)),
		),
		repo:          repo,
		sessionSvc:    sessionSvc,
		eventBus:      eventBus,
		entryMap:      make(map[string]cron.EntryID),
		modelFamilies: DefaultModelFamilies(),
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

// SetModelFamilies replaces the family alias → concrete model ID map used to
// resolve a workflow's Model field at fire time. Wired at startup from
// LoadModelFamilyOverride when an override file is present (see
// server/dependencies.go); falls back to DefaultModelFamilies() otherwise.
func (s *Scheduler) SetModelFamilies(families map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelFamilies = families
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
		now := time.Now()
		s.mu.Lock()
		for _, wf := range wfs {
			if err := s.addCronEntry(wf); err != nil {
				log.Warn("[WorkflowScheduler] failed to register cron entry on start", "slug", wf.Slug, "err", err)
				continue
			}
			// Missed-fire detection (Task 4.1.1b): only meaningful for workflows that
			// just successfully registered as cron entries — addCronEntry already
			// applies the trigger_type=="cron" + non-empty cron_expression gate.
			checkMissedCronFire(wf, now)
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
	prompt := buildTemplatedPrompt(wf, arg)
	return s.FireTrigger(ctx, wf, prompt, "")
}

// buildTemplatedPrompt renders wf.Command/wf.InputTemplate with arg substituted
// for {{input}}, the same interpolation FireNow has always used: command is the
// primary instruction (required), inputTemplate is appended after interpolation
// if set, and if only arg is present with no {{input}} placeholder anywhere it's
// appended as additional context. Factored out of FireNow so
// FireTriggerChained (webhook-triggers Phase 6) can reuse it for chain-fire's
// "arg" (the completed prior item's summary) without duplicating the
// {{input}} substitution rules.
func buildTemplatedPrompt(wf *ent.Workflow, arg string) string {
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
	return strings.Join(parts, "\n\n")
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
	return s.fireTrigger(ctx, wf, renderedPrompt, deliveryID, 0)
}

// FireTriggerChained fires wf as the next hop in a pipeline chain (webhook-
// triggers Phase 6, FR10/AC5): priorItemSummary (typically built via
// session.BuildSessionInitialPrompt over the just-completed BacklogItem) is
// interpolated into wf's own prompt template the same way FireNow's arg is
// (see buildTemplatedPrompt), and chainDepth is threaded onto the created
// session's TriggeredByChainDepth attribution field (Epic 6.3). Never claims
// a TriggerFireEvent row itself (deliveryID=""), same as FireNow — a
// rate-limit/admission-gate rejection still gets its own audit row via
// fireTrigger's recordGateRejection.
func (s *Scheduler) FireTriggerChained(ctx context.Context, wf *ent.Workflow, priorItemSummary string, chainDepth int32) (string, error) {
	prompt := buildTemplatedPrompt(wf, priorItemSummary)
	return s.fireTrigger(ctx, wf, prompt, "", chainDepth)
}

// fireTrigger is FireTrigger/FireTriggerChained's shared implementation —
// chainDepth is 0 for every non-chained caller (FireTrigger/FireNow) and
// item.TriggeredByChainDepth+1 for FireTriggerChained.
func (s *Scheduler) fireTrigger(ctx context.Context, wf *ent.Workflow, renderedPrompt string, deliveryID string, chainDepth int32) (string, error) {
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

	// Resolve a family alias (e.g. "family:sonnet") to a concrete model ID.
	// Fails closed: an unknown/retired alias aborts the fire rather than
	// passing the broken "family:xxx" string through to the CLI.
	s.mu.Lock()
	families := s.modelFamilies
	s.mu.Unlock()
	resolvedModel, modelErr := ResolveModel(families, wf.Model)
	if modelErr != nil {
		log.Error("[WorkflowScheduler] FireNow: model resolution failed", "slug", wf.Slug, "model", wf.Model, "err", modelErr)
		return "", fmt.Errorf("resolve model for workflow %q: %w", wf.Slug, modelErr)
	}

	// Append --model flag when a model is specified and the program is claude (or defaulting to claude).
	program := wf.AgentType
	if resolvedModel != "" {
		isClaudeProgram := program == "" || program == "claude"
		if isClaudeProgram {
			program = "claude --model " + resolvedModel
		}
	}

	// Deliberately mirrors a manually-created CreateSessionRequest field-for-field
	// aside from WorkflowId/TriggeredByChainDepth (attribution only, webhook-triggers
	// Epic 6.3): no auto_yes/skip_defaults or other bypass/elevated-permission field
	// is set here, so a trigger-fired session goes through the exact same
	// approval/review path a manually-created one does (Goal 4 — see
	// TestFireTrigger_NeverSetsAutoApproveFlag).
	req := connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:                 title,
		Path:                  wf.TargetDirectory,
		Program:               program,
		InitialPrompt:         renderedPrompt,
		SessionType:           sessionType,
		WorkflowId:            wf.ID.String(),
		TriggeredByChainDepth: chainDepth,
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

// missedFireCronParser parses cron expressions for checkMissedCronFire (Task 4.1.1b),
// mirroring the field mask NewScheduler/ValidateCronExpression use (minute hour dom month
// dow, no seconds field).
var missedFireCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// checkMissedCronFire logs a warning if wf's cron schedule has an occurrence between its
// CreatedAt and now that last_fired_at doesn't account for — i.e. the process was down
// (or otherwise failed to fire) across an expected cron occurrence. Detection/logging
// only: it never replay-fires the missed occurrence (Task 4.1.1b / AC2's straddled-
// restart scenario).
func checkMissedCronFire(wf *ent.Workflow, now time.Time) {
	schedule, err := missedFireCronParser.Parse(wf.CronExpression)
	if err != nil {
		log.Warn("[WorkflowScheduler] missed-fire check: failed to parse cron expression", "slug", wf.Slug, "err", err)
		return
	}

	expected, found := mostRecentCronOccurrence(schedule, wf.CreatedAt, now)
	if !found {
		// No scheduled occurrence has fallen due since the workflow was created —
		// either it's brand new, or its schedule genuinely hasn't come around yet.
		// Bounding the search by CreatedAt (rather than e.g. epoch or a fixed
		// lookback) is what prevents a false positive here: there is nothing to
		// compare last_fired_at against.
		return
	}

	if wf.LastFiredAt == nil || wf.LastFiredAt.Before(expected) {
		log.Warn("[WorkflowScheduler] missed cron fire", "slug", wf.Slug, "expected_at", expected)
	}
}

// mostRecentCronOccurrence returns the latest scheduled occurrence of schedule in
// (notBefore, now], or found=false if no such occurrence exists.
//
// robfig/cron's Schedule interface exposes only Next(t) — the next occurrence strictly
// after t — with no "previous occurrence" method. Naively walking forward one occurrence
// at a time from notBefore would work but costs one Schedule.Next call per occurrence
// since notBefore, which is unbounded for an old workflow with a frequent schedule (e.g.
// a workflow created a year ago on a per-minute cron would take ~525,600 calls just to
// reach the present). Instead this searches backward from `now` with an exponentially
// widening window: try the most recent minute, then the most recent 2 minutes, 4, 8, ...
// — at each width, latestOccurrenceInRange does a cheap forward Next-walk confined to
// that window only. This costs O(log(gap)) probes to find the right window, plus a walk
// bounded by the occurrences inside the true (small, since Phase 3 bumps last_fired_at on
// every successful fire) gap rather than the workflow's entire lifetime. The search is
// bounded below by notBefore (the workflow's CreatedAt at the only call site) — see
// checkMissedCronFire's comment for why that also solves the "don't false-positive on a
// brand-new workflow" requirement without separate period arithmetic.
func mostRecentCronOccurrence(schedule cron.Schedule, notBefore, now time.Time) (occurrence time.Time, found bool) {
	if !notBefore.Before(now) {
		return time.Time{}, false
	}

	for window := time.Minute; ; window *= 2 {
		lowerBound := now.Add(-window)
		if !lowerBound.After(notBefore) {
			lowerBound = notBefore
		}

		if occ, ok := latestOccurrenceInRange(schedule, lowerBound, now); ok {
			return occ, true
		}
		if !lowerBound.After(notBefore) {
			// Already searched the full permitted range (lowerBound was clamped to
			// notBefore this iteration) and found nothing — no occurrence exists.
			return time.Time{}, false
		}
	}
}

// latestOccurrenceInRange returns the latest occurrence of schedule in (from, upTo], or
// found=false if none exists.
func latestOccurrenceInRange(schedule cron.Schedule, from, upTo time.Time) (occurrence time.Time, found bool) {
	next := schedule.Next(from)
	if next.After(upTo) {
		return time.Time{}, false
	}
	last := next
	for {
		n := schedule.Next(last)
		if n.After(upTo) {
			return last, true
		}
		last = n
	}
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
