package services

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/codes"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/telemetry"
)

// maxCreationResolutionTimeout is the default value of
// SessionService.creationResolutionTimeout: it bounds the Background
// Resolution Pipeline's entire run (Epic 2.2, Story 2.2.1) -- GitHub-URL
// resolution through worktree/tmux startup -- independent of the
// originating RPC's own deadline/cancellation. 10 minutes matches the
// Stale-Creation Sweeper's default threshold (Epic 4.1's
// CreationStaleConfig.ThresholdMinutesOrDefault) so a pipeline that is
// genuinely still working right up to the sweeper's own cutoff gets its own
// definitive Failed write instead of racing the sweeper for the same
// outcome.
const maxCreationResolutionTimeout = 10 * time.Minute

// creationPipelineParams bundles everything runBackgroundResolutionPipeline
// needs, captured in CreateSession's synchronous prefix before the pipeline
// goroutine is dispatched via trackCleanup. Grouped into one struct rather
// than a same-typed-parameter pile -- see
// .claude/rules/primitive-obsession-checklist.md.
type creationPipelineParams struct {
	instance *session.Instance
	// epoch is instance.CreationEpoch(), captured once at spawn time, before
	// any phase runs -- the only value the pipeline's one terminal write
	// (commitTerminalStatus) presents back to win the race against a
	// concurrent cancel/retry (Story 2.2.3).
	epoch uint64
	// instanceTitle/instanceRootDir mirror CreateSession's own pre-goroutine
	// captures (avoids capturing req.Msg, which may be GC'd) -- instanceRootDir
	// is further updated in-pipeline once a deferred GitHub URL resolves to
	// its real clone path.
	instanceTitle   string
	instanceRootDir string

	deferredGitHubURL       bool
	deferredGitHubSourceURL string
	deferredEnterpriseHosts []string
}

// pipelineOutcome bundles the three values a terminal write needs: the
// session.Status to force, the human-readable FailureReason to persist
// alongside it (empty for a success), and the session.creation.outcome
// metrics label. See terminal's doc comment in
// runBackgroundResolutionPipeline for why this is a struct rather than a
// same-typed (status, string, string) parameter pile.
type pipelineOutcome struct {
	status         session.Status
	failureReason  string
	metricsOutcome string
}

// runBackgroundResolutionPipeline is the Background Resolution Pipeline
// (Epic 2.2): the single trackCleanup-dispatched goroutine that runs, in
// order, GitHub-URL resolution -> the alias/default-based defaults merge and
// branch/session-type inference -> worktree setup -> tmux startup,
// publishing creation_progress between phases and making exactly one
// terminal status write at the end via commitTerminalStatus (Story 2.2.3).
// Alias *existence* is never checked here -- it already ran synchronously in
// CreateSession's prefix (Epic 2.1).
//
// rpcCtx is the originating RPC's context, used only to build the detached
// Background Resolution Context below via context.WithoutCancel -- never
// awaited or read from again, so the RPC returning (or its client
// disconnecting) cannot cancel in-progress resolution
// (research/pitfalls.md §2).
func (s *SessionService) runBackgroundResolutionPipeline(rpcCtx context.Context, p creationPipelineParams) {
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(rpcCtx), s.creationResolutionTimeoutOrDefault())
	defer cancel()
	// Stored before any phase runs so a concurrent Cancel RPC (Epic 3.2) can
	// stop this specific pipeline invocation, even mid-clone.
	p.instance.SetCreationCancelFunc(cancel)

	ctx, span := telemetry.StartLinkedBackgroundSpan(bgCtx, "session.create.resolve")
	defer span.End()

	startedAt := time.Now()
	instanceRootDir := p.instanceRootDir

	// terminal is the pipeline's one terminal-write call site (Story 2.2.3):
	// every exit path (success, per-phase failure, timeout, panic recovery)
	// funnels through here so commitTerminalStatus/span/metrics can never
	// drift relative to one another. It only records the metric/publishes
	// the event when commitTerminalStatus reports applied == true -- a
	// superseded writer (stale epoch, cancel/retry already won) must never
	// double-count against the writer that actually won.
	//
	// Takes one pipelineOutcome value rather than a bare (status, string,
	// string) parameter pile -- see
	// .claude/rules/primitive-obsession-checklist.md -- since status,
	// failureReason, and the metrics outcome label are three distinct
	// concepts that could otherwise be silently transposed at a call site.
	terminal := func(o pipelineOutcome) {
		storage := s.GetStorage()
		if storage == nil {
			log.Error("[session pipeline] no concrete *session.Storage available for terminal write",
				"session", p.instanceTitle, "status", o.status)
			return
		}
		// The terminal write itself must not be attempted against ctx once
		// ctx is already Done -- e.g. exactly the timeout-exceeded case this
		// write exists to record: ctx (derived from bgCtx) has already
		// expired by the time this closure runs, and an ent Save() call
		// against an expired context fails outright ("context deadline
		// exceeded") before it can even evaluate the epoch predicate,
		// permanently stranding the row at Creating instead of recording
		// the Failed outcome. writeCtx is deliberately detached (its own
		// bounded timeout, not derived from ctx's cancellation) so the one
		// write that must always be able to land, can.
		writeCtx, writeCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer writeCancel()
		applied := commitTerminalStatus(writeCtx, storage, p.instance, p.epoch, o.status, o.failureReason)
		if !applied {
			span.AddEvent("terminal_write_skipped_stale_epoch")
			log.Info("[session pipeline] terminal write skipped, epoch already advanced",
				"session", p.instanceTitle, "epoch", p.epoch, "status", o.status)
			return
		}
		if o.failureReason != "" {
			span.SetStatus(codes.Error, o.failureReason)
		}
		RecordSessionCreationMetrics(ctx, o.metricsOutcome, time.Since(startedAt))
		s.eventBus.Publish(events.NewSessionUpdatedEvent(p.instance, []string{"status", "creation_progress"}))
	}

	// Panic safety (Task 2.2.2d): a detached goroutine's unrecovered panic
	// crashes the process. Recover, log, and make a terminal Failed write --
	// never leave the instance stuck at Creating forever.
	defer func() {
		if r := recover(); r != nil {
			log.Error("[session pipeline] panic recovered, writing terminal Failed",
				"session", p.instanceTitle, "panic", r)
			span.AddEvent("panic_recovered")
			terminal(pipelineOutcome{session.Failed, "StartupError", SessionCreationOutcomeFailed})
		}
	}()

	// setPhase transitions to a new Creation Phase: publishes progress,
	// records a span event, and persists via storage.UpdateInstance --
	// deliberately not storage.SaveInstances, which silently skips any
	// instance where !inst.Started() (true for every phase before the
	// worktree/tmux phase below), so the Stale-Creation Sweeper's
	// restart-survival guarantee actually holds across every phase, not
	// only the terminal write.
	setPhase := func(msg string) {
		if s.creationPhaseHook != nil {
			s.creationPhaseHook(msg)
		}
		p.instance.SetCreationProgress(msg)
		span.AddEvent(msg)
		s.eventBus.Publish(events.NewSessionUpdatedEvent(p.instance, []string{"creation_progress"}))
		if storage := s.GetStorage(); storage != nil {
			if err := storage.UpdateInstance(p.instance); err != nil {
				log.Warn("[session pipeline] failed to persist creation_progress", "session", p.instanceTitle, "phase", msg, "err", err)
			}
		}
	}

	// Phase: ResolvingGitHubURL / CloningRepository.
	if p.deferredGitHubURL {
		setPhase("Resolving GitHub URL...")
		setPhase("Cloning repository...")
		localPath, ref, resolveErr := s.githubResolver(ctx, p.deferredGitHubSourceURL, p.deferredEnterpriseHosts)
		if resolveErr != nil {
			log.Error("[session pipeline] GitHub URL resolution failed", "session", p.instanceTitle, "url", p.deferredGitHubSourceURL, "err", resolveErr)
			setPhase(fmt.Sprintf("Failed to resolve GitHub URL: %s", resolveErr.Error()))
			terminal(pipelineOutcome{session.Failed, "GitHubResolutionError", SessionCreationOutcomeFailed})
			return
		}

		branch := ref.Branch
		var prURL string
		if ref.PRNumber > 0 {
			prURL = ref.PRURL()
		}
		p.instance.SetGitHubResolution(session.GitHubResolution{
			Path:           localPath,
			Branch:         branch,
			Owner:          ref.Owner,
			Repo:           ref.Repo,
			SourceRef:      p.deferredGitHubSourceURL,
			ClonedRepoPath: localPath,
			PRNumber:       ref.PRNumber,
			PRURL:          prURL,
		})
		instanceRootDir = p.instance.GetEffectiveRootDir()
		s.eventBus.Publish(events.NewSessionUpdatedEvent(p.instance, []string{"path", "branch", "github_owner", "github_repo"}))
		log.Info("[session pipeline] resolved deferred GitHub URL", "session", p.instanceTitle, "path", localPath, "branch", branch)
	}

	// Phase: ResolvingDefaults -- folds in branch/session-type inference.
	// The actual alias/default-based defaults merge (config.ResolveAlias /
	// config.ResolveDefaults post-existence-check work: env vars, CLI
	// flags, path, program, autoyes, alias session type) already ran
	// synchronously in CreateSession's prefix (session_service.go, near the
	// alias/default resolution block), ahead of instance construction --
	// this deviates from Task 2.2.2b's spec, which called for that merge to
	// move here. It stayed synchronous because the resolved path is needed
	// synchronously anyway (Directory-mode's existence check and instance
	// construction both read it before the pipeline goroutine spawns);
	// moving the merge alone would require gating that path behind a
	// placeholder-then-patch scheme like SetGitHubResolution, which is a
	// much larger, riskier restructuring across every session-creation mode
	// than this vestigial phase justifies. See the "Scope Deviations"
	// section of project_plans/async-session-creation/implementation/plan.md
	// (after Risk Control) for the full rationale, including why this is
	// low-practical-risk (the merge is CPU-only: struct field copies and
	// os.LookupEnv, no I/O). This phase still exists, and still publishes
	// progress + persists, so every
	// session type (GitHub-URL or plain) surfaces the same observable
	// Creation Phase sequence from plan.md's glossary.
	setPhase("Resolving defaults...")

	// A pipeline that outlives its Background Resolution Context (deadline
	// exceeded, or a concurrent Cancel RPC fired its stored CancelFunc)
	// must not proceed into worktree/tmux setup.
	if ctx.Err() != nil {
		terminal(pipelineOutcome{session.Failed, "StartupError", SessionCreationOutcomeFailed})
		return
	}

	// A pipeline whose captured epoch has already been superseded (a
	// cancel/retry raced ahead of it, per ADR-002) must not call
	// p.instance.Start(true) at all: Start() transitions the instance to
	// Active as an unconditional side effect of its own pre-existing,
	// epoch-unaware startLocked logic (session/instance.go), entirely
	// out-of-band from commitTerminalStatus's fencing. Checking here --
	// before that side effect can happen -- is what makes "no in-memory
	// state is touched" (Story 2.2.3's stale-epoch acceptance criterion)
	// actually hold; checking only at the final commitTerminalStatus call
	// would be too late, since Start() would have already flipped Status
	// by then regardless of the outcome of that check.
	if p.instance.CreationEpoch() != p.epoch {
		log.Info("[session pipeline] epoch already advanced before worktree/tmux startup, aborting without starting",
			"session", p.instanceTitle, "capturedEpoch", p.epoch)
		return
	}

	// Wire callbacks before starting so rate-limit and status-change events fire.
	s.wireCallbacks(p.instance)

	// Phase: SettingUpWorktree / StartingSession.
	setPhase("Setting up worktree...")
	setPhase("Starting session...")

	if startErr := p.instance.Start(true); startErr != nil {
		log.Error("[session pipeline] async start failed", "session", p.instanceTitle, "err", startErr)
		setPhase(fmt.Sprintf("Startup failed: %s", startErr.Error()))
		terminal(pipelineOutcome{session.Failed, "StartupError", SessionCreationOutcomeFailed})
		return
	}

	// Clear progress message now that we are about to become Active.
	p.instance.SetCreationProgress("")

	// Inject Claude Code HTTP hook config for remote approval from the web UI.
	// Non-fatal: session is fully functional even without this config.
	//
	// Remote sessions (ssh-remote-workspaces Phase 5 correction) can't use
	// InjectHookConfig at all: instanceRootDir is a path that only exists on
	// the remote host, so os.ReadFile/os.WriteFile there would either fail
	// outright or -- worse -- silently write to a local path nobody remote
	// ever reads. setupRemoteApprovalHooks routes through a
	// *sshremote.RemoteApprovalRelay + InjectHookConfigRemote instead.
	if p.instance.IsRemote() {
		if err := s.setupRemoteApprovalHooks(p.instance, instanceRootDir, p.instanceTitle); err != nil {
			log.Warn("[session pipeline] failed to set up remote approval relay", "session", p.instanceTitle, "err", err)
		}
	} else if err := InjectHookConfig(instanceRootDir, p.instanceTitle); err != nil {
		log.Warn("[session pipeline] failed to inject hook config", "session", p.instanceTitle, "err", err)
	}

	if s.backlogLifecycleListener != nil {
		s.backlogLifecycleListener.WireToInstance(p.instance)
	}
	if s.sessionSummaryGenerator != nil {
		session.WireSessionSummaryListener(s.sessionSummaryGenerator, p.instance)
	}

	// Wire the status manager and start the controller AFTER Start() returns so the
	// tmux attach-session process has had time to fully initialize. Starting the
	// controller inside Start() caused immediate PTY EIO because tmux hadn't
	// stabilized yet. This mirrors the pattern used by loadInstancesWithWiring.
	if s.statusManager != nil {
		p.instance.SetStatusManager(s.statusManager)
		if ctrlErr := p.instance.StartController(); ctrlErr != nil {
			log.Warn("[session pipeline] failed to start controller after wiring", "session", p.instanceTitle, "err", ctrlErr)
		}
	}

	// Start the session driver goroutine so UI-created sessions receive their
	// initial prompt (typed into the session terminal once the session reaches Ready).
	// StartSessionDriver is idempotent (CAS guard) -- safe to call even if a driver
	// was already started by another code path.
	session.StartSessionDriver(p.instance, instanceRootDir)

	if p.instance.AutonomousMode && s.headlessPool != nil {
		costOpt := session.NoopDriverOption
		if concreteStorage := s.GetStorage(); concreteStorage != nil {
			costOpt = session.WithCostSink(session.CostSinkForSessionUUID(concreteStorage, p.instance.UUID))
		}
		driver := session.NewAutonomousDriver(p.instance, s.headlessPool, p.instance.Prompt, 0, costOpt)
		driver.RegisterCompletionCallback(s.autonomousSvc.onAutonomousDriverComplete)
		if driverErr := driver.Start(s.autonomousSvc.driverCtx()); driverErr != nil {
			log.Warn("[session pipeline] failed to start autonomous driver", "session", p.instanceTitle, "err", driverErr)
		} else {
			s.autonomousSvc.registerDriver(p.instanceTitle, driver)
		}
	} else if p.instance.AutonomousMode {
		log.Warn("[session pipeline] autonomous_mode requested but headlessPool is nil", "session", p.instanceTitle)
	}

	// Persist the full instance row (branch/worktree/etc. resolved during
	// Start(), e.g. an existing-worktree remote session's discovered branch
	// name) before the terminal write -- commitTerminalStatus's own
	// UpdateInstanceIfEpoch only sets status/failure_reason/updated_at (an
	// intentionally narrow, epoch-gated conditional UPDATE, see its doc
	// comment), not the rest of the row. Mirrors the pre-Epic-2.2 code's
	// final storage.SaveInstances([]*session.Instance{instance}) call, which
	// this UpdateInstance takes the place of now that the instance is
	// definitely Started() (SaveInstances would otherwise be equally
	// correct here, but UpdateInstance has no Started()-gate to reason
	// about).
	if storage := s.GetStorage(); storage != nil {
		if err := storage.UpdateInstance(p.instance); err != nil {
			log.Warn("[session pipeline] failed to persist instance after successful start", "session", p.instanceTitle, "err", err)
		}
	}

	terminal(pipelineOutcome{session.Active, "", SessionCreationOutcomeSuccess})
	log.Info("[session pipeline] async start complete", "session", p.instanceTitle)
	if p.instance.RestartedFromSessionID != "" {
		log.Info("[HandoffSummary] restart session created", "source_session", p.instance.RestartedFromSessionID, "new_session", p.instance.UUID)
	}
}

// creationResolutionTimeoutOrDefault returns s.creationResolutionTimeout,
// falling back to maxCreationResolutionTimeout for any SessionService
// constructed without going through NewSessionService/
// NewSessionServiceWithSearchEngine (defensive default; every production and
// test constructor sets this field explicitly).
func (s *SessionService) creationResolutionTimeoutOrDefault() time.Duration {
	if s.creationResolutionTimeout <= 0 {
		return maxCreationResolutionTimeout
	}
	return s.creationResolutionTimeout
}
