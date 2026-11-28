package sessioncreate

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/otel/codes"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/approvals"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/autonomous"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/driver"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/summaries"
	"github.com/tstapler/stapler-squad/telemetry"
)

// DefaultResolutionTimeout is the default value of
// Deps.ResolutionTimeout: it bounds the Background
// Resolution Pipeline's entire run (Epic 2.2, Story 2.2.1) -- GitHub-URL
// resolution through worktree/tmux startup -- independent of the
// originating RPC's own deadline/cancellation. 10 minutes matches the
// Stale-Creation Sweeper's default threshold (Epic 4.1's
// CreationStaleConfig.ThresholdMinutesOrDefault) so a pipeline that is
// genuinely still working right up to the sweeper's own cutoff gets its own
// definitive Failed write instead of racing the sweeper for the same
// outcome.
const DefaultResolutionTimeout = 10 * time.Minute

// PipelineParams bundles everything RunResolutionPipeline
// needs, captured in CreateSession's synchronous prefix before the pipeline
// goroutine is dispatched via trackCleanup. Grouped into one struct rather
// than a same-typed-parameter pile -- see
// .claude/rules/primitive-obsession-checklist.md.
type PipelineParams struct {
	Instance *instance.Instance
	// epoch is instance.CreationEpoch(), captured once at spawn time, before
	// any phase runs -- the only value the pipeline's one terminal write
	// (CommitTerminalStatus) presents back to win the race against a
	// concurrent cancel/retry (Story 2.2.3).
	Epoch uint64
	// instanceTitle/instanceRootDir mirror CreateSession's own pre-goroutine
	// captures (avoids capturing req.Msg, which may be GC'd) -- instanceRootDir
	// is further updated in-pipeline once a deferred GitHub URL resolves to
	// its real clone path.
	Title   string
	RootDir string

	DeferredGitHubURL       bool
	DeferredGitHubSourceURL string
	DeferredEnterpriseHosts []string
}

// pipelineOutcome bundles the three values a terminal write needs: the
// session.Status to force, the human-readable FailureReason to persist
// alongside it (empty for a success), and the session.creation.outcome
// metrics label. See terminal's doc comment in
// RunResolutionPipeline for why this is a struct rather than a
// same-typed (status, string, string) parameter pile.
type pipelineOutcome struct {
	status         domain.Status
	failureReason  string
	metricsOutcome string
}

// RunResolutionPipeline is the Background Resolution Pipeline
// (Epic 2.2): the single trackCleanup-dispatched goroutine that runs, in
// order, GitHub-URL resolution -> the alias/default-based defaults merge and
// branch/session-type inference -> worktree setup -> tmux startup,
// publishing creation_progress between phases and making exactly one
// terminal status write at the end via CommitTerminalStatus (Story 2.2.3).
// Alias *existence* is never checked here -- it already ran synchronously in
// CreateSession's prefix (Epic 2.1).
//
// rpcCtx is the originating RPC's context, used only to build the detached
// Background Resolution Context below via context.WithoutCancel -- never
// awaited or read from again, so the RPC returning (or its client
// disconnecting) cannot cancel in-progress resolution
// (research/pitfalls.md §2).
//
//nolint:gocognit,gocyclo,funlen // pre-existing complexity moved verbatim from server/services; reducing it is separate work
func (h *Handler) RunResolutionPipeline(rpcCtx context.Context, p PipelineParams) {
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(rpcCtx), h.resolutionTimeoutOrDefault())
	defer cancel()
	// Stored before any phase runs so a concurrent Cancel RPC (Epic 3.2) can
	// stop this specific pipeline invocation, even mid-clone.
	p.Instance.SetCreationCancelFunc(cancel)

	ctx, span := telemetry.StartLinkedBackgroundSpan(bgCtx, "session.create.resolve")
	defer span.End()

	startedAt := time.Now()

	// terminal is the pipeline's one terminal-write call site (Story 2.2.3):
	// every exit path (success, per-phase failure, timeout, panic recovery)
	// funnels through here so CommitTerminalStatus/span/metrics can never
	// drift relative to one another. It only records the metric/publishes
	// the event when CommitTerminalStatus reports applied == true -- a
	// superseded writer (stale epoch, cancel/retry already won) must never
	// double-count against the writer that actually won.
	//
	// Takes one pipelineOutcome value rather than a bare (status, string,
	// string) parameter pile -- see
	// .claude/rules/primitive-obsession-checklist.md -- since status,
	// failureReason, and the metrics outcome label are three distinct
	// concepts that could otherwise be silently transposed at a call site.
	terminal := func(o pipelineOutcome) {
		storage := h.d.ConcreteStorage()
		if storage == nil {
			log.Error("[session pipeline] no concrete *session.Storage available for terminal write",
				"session", p.Title, "status", o.status)
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
		applied := CommitTerminalStatus(writeCtx, storage, p.Instance, p.Epoch, o.status, o.failureReason)
		if !applied {
			span.AddEvent("terminal_write_skipped_stale_epoch")
			log.Info("[session pipeline] terminal write skipped, epoch already advanced",
				"session", p.Title, "epoch", p.Epoch, "status", o.status)
			return
		}
		if o.failureReason != "" {
			span.SetStatus(codes.Error, o.failureReason)
		}
		RecordSessionCreationMetrics(ctx, o.metricsOutcome, time.Since(startedAt))
		h.d.Bus.Publish(events.NewSessionUpdatedEvent(p.Instance, []string{"status", "creation_progress"}))
	}

	// Panic safety (Task 2.2.2d): a detached goroutine's unrecovered panic
	// crashes the process. Recover, log, and make a terminal Failed write --
	// never leave the instance stuck at Creating forever.
	defer func() {
		if r := recover(); r != nil {
			log.Error("[session pipeline] panic recovered, writing terminal Failed",
				"session", p.Title, "panic", r, "stack", string(debug.Stack()))
			span.AddEvent("panic_recovered")
			terminal(pipelineOutcome{domain.Failed, "StartupError", SessionCreationOutcomeFailed})
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
		if hook := h.d.PhaseHook(); hook != nil {
			hook(msg)
		}
		p.Instance.SetCreationProgress(msg)
		span.AddEvent(msg)
		h.d.Bus.Publish(events.NewSessionUpdatedEvent(p.Instance, []string{"creation_progress"}))
		if storage := h.d.ConcreteStorage(); storage != nil {
			if err := storage.UpdateInstance(p.Instance); err != nil {
				log.Warn("[session pipeline] failed to persist creation_progress", "session", p.Title, "phase", msg, "err", err)
			}
		}
	}

	// Phase: ResolvingGitHubURL / CloningRepository.
	if p.DeferredGitHubURL {
		setPhase("Resolving GitHub URL...")
		setPhase("Cloning repository...")
		localPath, ref, resolveErr := h.d.GitHubResolver()(ctx, p.DeferredGitHubSourceURL, p.DeferredEnterpriseHosts)
		if resolveErr != nil {
			log.Error("[session pipeline] GitHub URL resolution failed", "session", p.Title, "url", p.DeferredGitHubSourceURL, "err", resolveErr)
			setPhase(fmt.Sprintf("Failed to resolve GitHub URL: %s", resolveErr.Error()))
			terminal(pipelineOutcome{domain.Failed, "GitHubResolutionError", SessionCreationOutcomeFailed})
			return
		}

		branch := ref.Branch
		var prURL string
		if ref.PRNumber > 0 {
			prURL = ref.PRURL()
		}
		p.Instance.SetGitHubResolution(domain.GitHubResolution{
			Path:           localPath,
			Branch:         branch,
			Owner:          ref.Owner,
			Repo:           ref.Repo,
			SourceRef:      p.DeferredGitHubSourceURL,
			ClonedRepoPath: localPath,
			PRNumber:       ref.PRNumber,
			PRURL:          prURL,
		})
		h.d.Bus.Publish(events.NewSessionUpdatedEvent(p.Instance, []string{"path", "branch", "github_owner", "github_repo"}))
		log.Info("[session pipeline] resolved deferred GitHub URL", "session", p.Title, "path", localPath, "branch", branch)
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
		terminal(pipelineOutcome{domain.Failed, "StartupError", SessionCreationOutcomeFailed})
		return
	}

	// A pipeline whose captured epoch has already been superseded (a
	// cancel/retry raced ahead of it, per ADR-002) must not call
	// p.Instance.Start(true) at all: Start() transitions the instance to
	// Active as an unconditional side effect of its own pre-existing,
	// epoch-unaware startLocked logic (session/instance.go), entirely
	// out-of-band from CommitTerminalStatus's fencing. Checking here --
	// before that side effect can happen -- is what makes "no in-memory
	// state is touched" (Story 2.2.3's stale-epoch acceptance criterion)
	// actually hold; checking only at the final CommitTerminalStatus call
	// would be too late, since Start() would have already flipped Status
	// by then regardless of the outcome of that check.
	if p.Instance.CreationEpoch() != p.Epoch {
		log.Info("[session pipeline] epoch already advanced before worktree/tmux startup, aborting without starting",
			"session", p.Title, "capturedEpoch", p.Epoch)
		return
	}

	// Wire callbacks before starting so rate-limit and status-change events fire.
	h.d.WireCallbacks(p.Instance)

	// Phase: SettingUpWorktree / StartingSession.
	setPhase("Setting up worktree...")
	setPhase("Starting session...")

	if startErr := p.Instance.Start(true); startErr != nil {
		log.Error("[session pipeline] async start failed", "session", p.Title, "err", startErr)
		setPhase(fmt.Sprintf("Startup failed: %s", startErr.Error()))
		// Classify via errors.Is (sentinel matching, not string parsing) into a fixed,
		// short failureReason/metricsOutcome distinct from the generic "StartupError"
		// (worktree-envvars-hijack Task 3.3.3a) -- never interpolate the detailed
		// title/path/blocking-UUID text from startErr.Error() into these wire-level
		// values; that text is still logged in full above for debugging.
		failureReason := "StartupError"
		switch {
		case errors.Is(startErr, instance.ErrWorktreeResolutionFailed):
			failureReason = "WorktreeResolutionFailed"
		case errors.Is(startErr, instance.ErrDirectoryCollision):
			failureReason = "DirectoryCollision"
		}
		terminal(pipelineOutcome{domain.Failed, failureReason, SessionCreationOutcomeFailed})
		return
	}

	// Derived only here, after Start() has completed worktree creation --
	// any earlier point is stale for a plain SessionTypeNewWorktree session,
	// leaving InjectHookConfig/StartSessionDriver below pointed at the bare
	// repo path instead of the freshly-created worktree.
	instanceRootDir := p.Instance.GetEffectiveRootDir()

	// Clear progress message now that we are about to become Active.
	p.Instance.SetCreationProgress("")

	// Persist the full instance row (branch/worktree/etc. resolved during
	// Start()) here, before the wiring below, not after: startLocked flips
	// the in-memory Status to Active as a side effect of Start() itself,
	// out-of-band from CommitTerminalStatus's fencing, so any reader can
	// already observe Active the instant Start() returns. Persisting later
	// left a window where the row still read Creating while the instance
	// read Active -- the live-session/stale-row split ADR-002's
	// durable-first terminal write exists to rule out. None of the wiring
	// below mutates a field this call captures.
	if storage := h.d.ConcreteStorage(); storage != nil {
		if err := storage.UpdateInstance(p.Instance); err != nil {
			log.Warn("[session pipeline] failed to persist instance after successful start", "session", p.Title, "err", err)
		}
	}

	// Inject Claude Code HTTP hook config for remote approval from the web UI.
	// Non-fatal: session is fully functional even without this config.
	//
	// Remote sessions (ssh-remote-workspaces Phase 5 correction) can't use
	// InjectHookConfig at all: instanceRootDir is a path that only exists on
	// the remote host, so os.ReadFile/os.WriteFile there would either fail
	// outright or -- worse -- silently write to a local path nobody remote
	// ever reads. setupRemoteApprovalHooks routes through a
	// *sshremote.RemoteApprovalRelay + InjectHookConfigRemote instead.
	if p.Instance.IsRemote() {
		if err := h.setupRemoteApprovalHooks(p.Instance, instanceRootDir, p.Title); err != nil {
			log.Warn("[session pipeline] failed to set up remote approval relay", "session", p.Title, "err", err)
		}
	} else if err := approvals.InjectHookConfig(instanceRootDir, p.Title, p.Instance.UUID); err != nil {
		log.Warn("[session pipeline] failed to inject hook config", "session", p.Title, "err", err)
	}

	if h.d.BacklogListener() != nil {
		h.d.BacklogListener().WireToInstance(p.Instance)
	}
	if h.d.SummaryGenerator() != nil {
		summaries.WireSessionSummaryListener(h.d.SummaryGenerator(), p.Instance)
	}

	// Wire the status manager and start the controller AFTER Start() returns so the
	// tmux attach-session process has had time to fully initialize. Starting the
	// controller inside Start() caused immediate PTY EIO because tmux hadn't
	// stabilized yet. This mirrors the pattern used by loadInstancesWithWiring.
	if h.d.StatusManager() != nil {
		p.Instance.SetStatusManager(h.d.StatusManager())
		if ctrlErr := p.Instance.StartController(); ctrlErr != nil {
			log.Warn("[session pipeline] failed to start controller after wiring", "session", p.Title, "err", ctrlErr)
		}
	}

	// Start the session driver goroutine so UI-created sessions receive their
	// initial prompt (typed into the session terminal once the session reaches Ready).
	// StartSessionDriver is idempotent (CAS guard) -- safe to call even if a driver
	// was already started by another code path.
	driver.StartSessionDriver(p.Instance, instanceRootDir)

	h.StartCreationAutonomousDriver(p.Instance, p.Title)

	terminal(pipelineOutcome{domain.Active, "", SessionCreationOutcomeSuccess})
	log.Info("[session pipeline] async start complete", "session", p.Title)
	if p.Instance.RestartedFromSessionID != "" {
		log.Info("[HandoffSummary] restart session created", "source_session", p.Instance.RestartedFromSessionID, "new_session", p.Instance.UUID)
	}
}

// resolutionTimeoutOrDefault returns the host's resolution timeout, or
// DefaultResolutionTimeout for a host that leaves it unset.
func (h *Handler) resolutionTimeoutOrDefault() time.Duration {
	if d := h.d.ResolutionTimeout(); d > 0 {
		return d
	}
	return DefaultResolutionTimeout
}

// StartCreationAutonomousDriver starts and registers the autonomous driver of a
// just-created session. It does nothing unless the session asked for autonomy.
func (h *Handler) StartCreationAutonomousDriver(inst *instance.Instance, title string) {
	if inst.AutonomousMode && h.d.HeadlessPool() != nil {
		costOpt := autonomous.NoopDriverOption
		if concreteStorage := h.d.ConcreteStorage(); concreteStorage != nil {
			costOpt = autonomous.WithCostSink(session.CostSinkForSessionUUID(concreteStorage, inst.UUID))
		}
		d := autonomous.NewAutonomousDriver(inst, h.d.DriverClient(), inst.Prompt, config.LoadConfig().AutonomousMaxTurnsOrDefault(), costOpt)
		d.RegisterCompletionCallback(h.d.Autonomy().OnAutonomousDriverComplete)
		if driverErr := d.Start(h.d.Autonomy().DriverContext()); driverErr != nil {
			log.Warn("[session pipeline] failed to start autonomous driver", "session", title, "err", driverErr)
		} else {
			h.d.Autonomy().RegisterDriver(title, d)
		}
	} else if inst.AutonomousMode {
		log.Warn("[session pipeline] autonomous_mode requested but headlessPool is nil", "session", title)
	}
}
