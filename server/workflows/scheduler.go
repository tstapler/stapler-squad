// Package workflows provides the WorkflowScheduler for cron-based session automation.
package workflows

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
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

// Scheduler manages cron-based workflow execution.
type Scheduler struct {
	c          *cron.Cron
	repo       session.WorkflowRepository
	sessionSvc SessionServiceInterface
	eventBus   *events.EventBus
	mu         sync.Mutex
	entryMap   map[string]cron.EntryID // workflowID → cron.EntryID
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

// Start loads all enabled workflows and begins cron processing.
// Stops when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	if s.repo == nil {
		log.Warn("[WorkflowScheduler] repo is nil, scheduler disabled")
		return
	}

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

	if !wf.CronEnabled {
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
func (s *Scheduler) FireNow(ctx context.Context, wf *ent.Workflow, arg string) (string, error) {
	if s.sessionSvc == nil {
		return "", fmt.Errorf("session service not available")
	}

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

	title := fmt.Sprintf("%s — %s", wf.Name, time.Now().Format("2006-01-02 15:04"))
	oneOff := wf.SessionType == session.SessionTypeOneOff

	req := connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:         title,
		Path:          wf.TargetDirectory,
		Program:       wf.AgentType,
		InitialPrompt: prompt,
		OneOff:        oneOff,
	})
	resp, err := s.sessionSvc.CreateSession(ctx, req)
	if err != nil {
		log.Error("[WorkflowScheduler] FireNow: failed to create session", "slug", wf.Slug, "err", err)
		return "", fmt.Errorf("create session for workflow %q: %w", wf.Slug, err)
	}

	sessionID := ""
	if sess := resp.Msg.GetSession(); sess != nil {
		sessionID = sess.GetId()
	}
	log.Info("[WorkflowScheduler] fired workflow", "slug", wf.Slug, "session_id", sessionID, "title", title)
	return sessionID, nil
}

// addCronEntry adds a cron entry for the given workflow. Must be called with mu held.
func (s *Scheduler) addCronEntry(wf *ent.Workflow) error {
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
