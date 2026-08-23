package session

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/handoffsummary"
	"github.com/tstapler/stapler-squad/session/headless"
)

// HandoffSummaryStatus mirrors SessionSummaryStatus's shape (session_summary_service.go)
// for the handoff-summary row's status column.
type HandoffSummaryStatus string

const (
	HandoffSummaryStatusPending    HandoffSummaryStatus = "pending"
	HandoffSummaryStatusGenerating HandoffSummaryStatus = "generating"
	HandoffSummaryStatusReady      HandoffSummaryStatus = "ready"
	HandoffSummaryStatusError      HandoffSummaryStatus = "error"
)

// handoffSummaryTimeout bounds the GenerateHandoffSummary pool call, identical
// shape to session_summary_service.go's llmNarrativeTimeout (Story 1.5.1
// Adversarial-review Blocker #1: the call must have a hard deadline so a
// blocked pool call cannot hang GenerateAndPersist, and must still resolve the
// row to ERROR + release the dedup guard on timeout). Declared as a var (not
// const), matching llmNarrativeTimeout's actual declaration despite the task
// description's "name the const" wording — llmNarrativeTimeout is a var
// specifically so tests can temporarily lower it to exercise the timeout path
// without waiting the full 60s; a real const would force this package's
// timeout test to block for a full minute.
var handoffSummaryTimeout = 60 * time.Second

// activeTaskHeading is the literal markdown heading GenerateHandoffSummary's
// output always ends with (session/headless/features.go's
// handoffSummarySystemPrompt) — used to best-effort extract the active task
// text for the ActiveTask column.
const activeTaskHeading = "## Active Task"

// HandoffSummaryGenerator is the domain-level orchestrator that owns the
// headless pool, the ent client, and the in-process dedup map for handoff
// summary generation. GenerateAndPersist mirrors
// SessionSummaryGenerator.GenerateAndPersist's structure (async dispatch,
// in-flight dedup, panic recovery, status-transitioning upserts).
type HandoffSummaryGenerator struct {
	entClient *ent.Client
	pool      headless.PoolClient

	// inFlight is a sync.Map[string]*sync.Mutex keyed by session_id, mirroring
	// SessionSummaryGenerator.inFlight. Collapses concurrent/duplicate triggers
	// per session.
	inFlight sync.Map
}

// NewHandoffSummaryGenerator creates a HandoffSummaryGenerator.
func NewHandoffSummaryGenerator(entClient *ent.Client, pool headless.PoolClient) *HandoffSummaryGenerator {
	return &HandoffSummaryGenerator{
		entClient: entClient,
		pool:      pool,
	}
}

// FindRowBySessionID queries the HandoffSummary row for sessionID directly.
// Wraps ent's not-found error as ErrNotFound, mirroring
// SessionSummaryGenerator.FindRowBySessionID.
func (g *HandoffSummaryGenerator) FindRowBySessionID(ctx context.Context, sessionID string) (*ent.HandoffSummary, error) {
	row, err := g.entClient.HandoffSummary.Query().Where(handoffsummary.SessionID(sessionID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: handoff summary for session=%s", ErrNotFound, sessionID)
		}
		return nil, err
	}
	return row, nil
}

// tryAcquire attempts to acquire the in-process per-session guard for
// sessionUUID. Returns (release, true) on success — the caller must call
// release() exactly once, typically via defer. Returns (nil, false) if a
// generation is already in flight for this session. Mirrors
// SessionSummaryGenerator.tryAcquire exactly.
func (g *HandoffSummaryGenerator) tryAcquire(sessionUUID string) (release func(), ok bool) {
	muI, _ := g.inFlight.LoadOrStore(sessionUUID, &sync.Mutex{})
	m := muI.(*sync.Mutex)
	if !m.TryLock() {
		return nil, false
	}
	return func() {
		m.Unlock()
		// Remove the map entry once this generation is done, so a session that
		// triggers generation once doesn't leave a permanent *sync.Mutex behind
		// for the process's lifetime. CompareAndDelete only removes it if it's
		// still the same mutex we acquired — if a concurrent tryAcquire already
		// raced in a fresh LoadOrStore between our Unlock and this line, that
		// entry belongs to a different in-flight generation and must be left
		// alone.
		g.inFlight.CompareAndDelete(sessionUUID, m)
	}, true
}

// isInFlight is a non-blocking probe used by ReconcileStaleness to distinguish
// "still actively generating in this process" from "genuinely stuck (e.g. the
// process restarted mid-generation)". Mirrors SessionSummaryGenerator.isInFlight
// exactly.
func (g *HandoffSummaryGenerator) isInFlight(sessionUUID string) bool {
	muI, ok := g.inFlight.Load(sessionUUID)
	if !ok {
		return false
	}
	m := muI.(*sync.Mutex)
	if m.TryLock() {
		m.Unlock()
		return false
	}
	return true
}

// ReconcileStaleness flips a row stuck in GENERATING for longer than
// staleGenerationTimeout (session_summary_service.go's constant, reused as-is
// rather than redeclared — both types live in package session) to ERROR,
// unless this process's in-memory guard is still held for that session (a
// long-running call, not a genuinely stuck row). Called from the RPC read
// path (server/services.HandoffSummaryService.GetHandoffSummary,
// Adversarial-review Blocker #3), not a background sweep — mirrors
// SessionSummaryGenerator.ReconcileStaleness's TOCTOU-safe predicated update;
// see that method's doc comment for the race this guards against.
func (g *HandoffSummaryGenerator) ReconcileStaleness(ctx context.Context, row *ent.HandoffSummary) *ent.HandoffSummary {
	if row == nil || row.Status != string(HandoffSummaryStatusGenerating) || row.GenerationStartedAt == nil {
		return row
	}
	if time.Since(*row.GenerationStartedAt) <= staleGenerationTimeout {
		return row
	}
	if g.isInFlight(row.SessionID) {
		return row
	}

	const staleMessage = "generation did not complete (server restart or hung call)"
	affected, err := g.entClient.HandoffSummary.Update().
		Where(
			handoffsummary.SessionID(row.SessionID),
			handoffsummary.Status(string(HandoffSummaryStatusGenerating)),
			handoffsummary.GenerationStartedAtEQ(*row.GenerationStartedAt),
		).
		SetStatus(string(HandoffSummaryStatusError)).
		SetErrorStage("stale").
		SetErrorMessage(staleMessage).
		Save(ctx)
	if err != nil {
		log.ForSession(row.SessionID).Warn("[HandoffSummary] failed to reconcile stale GENERATING row", "err", err)
		return row
	}
	if affected == 0 {
		// Someone else (a fresh GenerateAndPersist call, or a concurrent
		// reconcile) already transitioned this row out of the state we read.
		// Return the row as originally read, not a freshly-error'd one.
		log.ForSession(row.SessionID).Info("[HandoffSummary] skipped stale-GENERATING reconcile: row already transitioned")
		return row
	}

	row.Status = string(HandoffSummaryStatusError)
	row.ErrorStage = "stale"
	row.ErrorMessage = staleMessage
	return row
}

// toHandoffMessages converts []ClaudeConversationMessage (session package's
// transcript message shape) into []headless.HandoffTranscriptMessage
// (headless.GenerateHandoffSummary's input shape). A small local conversion
// helper, not a shared type, because session/headless cannot import session
// (session already imports session/headless — see
// headless.HandoffTranscriptMessage's doc comment).
func toHandoffMessages(messages []ClaudeConversationMessage) []headless.HandoffTranscriptMessage {
	out := make([]headless.HandoffTranscriptMessage, len(messages))
	for i, msg := range messages {
		out[i] = headless.HandoffTranscriptMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return out
}

// extractActiveTask returns the text following the literal "## Active Task"
// heading in summaryText, best-effort. Returns "" if the heading is missing —
// never an error.
func extractActiveTask(summaryText string) string {
	idx := strings.Index(summaryText, activeTaskHeading)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(summaryText[idx+len(activeTaskHeading):])
}

// upsertHandoffSummaryError writes an ERROR row for sessionUUID, preserving
// sessionTitle and generationStartedAt. Used by every error-return path in
// GenerateAndPersist so the row never sticks in GENERATING.
func (g *HandoffSummaryGenerator) upsertHandoffSummaryError(ctx context.Context, sessionUUID, sessionTitle, stage, message string, generationStartedAt time.Time) error {
	return g.entClient.HandoffSummary.Create().
		SetID(uuid.New().String()).
		SetSessionID(sessionUUID).
		SetSessionTitle(sessionTitle).
		SetStatus(string(HandoffSummaryStatusError)).
		SetGenerationStartedAt(generationStartedAt).
		OnConflictColumns(handoffsummary.FieldSessionID).
		Update(func(u *ent.HandoffSummaryUpsert) {
			u.SetStatus(string(HandoffSummaryStatusError))
			u.SetErrorStage(stage)
			u.SetErrorMessage(message)
			u.SetGenerationStartedAt(generationStartedAt)
		}).
		Exec(ctx)
}

// failStage upserts an ERROR row for sourceSessionID at the given stage,
// logging the upsert failure (if any) and then the stage failure itself.
// Extracted from GenerateAndPersist, which repeated this exact
// upsert-log-log shape at every error-return path (transcript load,
// transcript parse, generation, and persist). asError selects Error-level
// logging for the stage-failure line (used by the persist stage, since LLM
// cost has already been spent by that point) instead of the default Warn
// level the other stages use — matching each call site's original log level.
func (g *HandoffSummaryGenerator) failStage(ctx context.Context, sourceSessionID, sourceSessionTitle, stage string, err error, generationStartedAt time.Time, asError bool) {
	if upsertErr := g.upsertHandoffSummaryError(ctx, sourceSessionID, sourceSessionTitle, stage, err.Error(), generationStartedAt); upsertErr != nil {
		log.ForSession(sourceSessionID).Error(fmt.Sprintf("[HandoffSummary] failed to persist %s-stage error row", stage), "err", upsertErr)
	}
	msg := fmt.Sprintf("[HandoffSummary] GENERATING -> ERROR (%s stage)", stage)
	if asError {
		log.ForSession(sourceSessionID).Error(msg, "err", err)
	} else {
		log.ForSession(sourceSessionID).Warn(msg, "err", err)
	}
}

// GenerateAndPersist runs the full handoff-summary pipeline: read the source
// session's transcript, window/budget it, call the LLM to compact the middle
// portion into a handoff summary, and persist via status-transitioning
// upserts (PENDING -> GENERATING -> READY, or -> ERROR on failure). Always
// invoked as a detached goroutine — never call this synchronously.
func (g *HandoffSummaryGenerator) GenerateAndPersist(ctx context.Context, sourceSessionID, sourceSessionTitle string) {
	release, ok := g.tryAcquire(sourceSessionID)
	if !ok {
		return
	}
	defer release()

	// Panic safety, mirrors SessionSummaryGenerator.GenerateAndPersist: this
	// method always runs as a detached goroutine, so an unrecovered panic here
	// would crash the entire server process. Go runs deferred functions LIFO,
	// so this recover (registered second) fires before release() (registered
	// first) — the guard is never left permanently locked by a panic.
	defer func() {
		if r := recover(); r != nil {
			log.WarningLog().Printf("[HandoffSummary] GenerateAndPersist panicked (recovered) for session=%s: %v", sourceSessionID, r)
		}
	}()

	now := time.Now()

	// Interim GENERATING upsert.
	if err := g.entClient.HandoffSummary.Create().
		SetID(uuid.New().String()).
		SetSessionID(sourceSessionID).
		SetSessionTitle(sourceSessionTitle).
		SetStatus(string(HandoffSummaryStatusGenerating)).
		SetGenerationStartedAt(now).
		OnConflictColumns(handoffsummary.FieldSessionID).
		Update(func(u *ent.HandoffSummaryUpsert) {
			u.SetSessionTitle(sourceSessionTitle)
			u.SetStatus(string(HandoffSummaryStatusGenerating))
			u.SetGenerationStartedAt(now)
		}).
		Exec(ctx); err != nil {
		log.ForSession(sourceSessionID).Error("[HandoffSummary] failed to write interim GENERATING row", "err", err)
		return
	}
	log.ForSession(sourceSessionID).Info("[HandoffSummary] PENDING -> GENERATING")

	history, err := NewClaudeSessionHistoryFromClaudeDir()
	if err != nil {
		g.failStage(ctx, sourceSessionID, sourceSessionTitle, "transcript", err, now, false)
		return
	}

	messages, err := history.GetMessagesFromConversationFile(sourceSessionID, 0)
	if err != nil {
		g.failStage(ctx, sourceSessionID, sourceSessionTitle, "transcript", err, now, false)
		return
	}

	window := buildTranscriptWindow(messages)
	// Head/Tail are carried verbatim in message count, but each message's
	// content still needs the same per-message byte cap Middle gets --
	// otherwise a single oversized Head/Tail message (e.g. a large pasted
	// file in the first turn) bypasses the excerpt budget entirely.
	window.Head = pruneMessages(window.Head)
	// totalTranscriptBytes is computed from the raw, pre-pruning messages
	// slice (Head+Middle+Tail combined) so the excerpt budget scales with how
	// much conversation there actually is to compress -- see
	// newSummaryBudget's doc comment.
	totalTranscriptBytes := sumContentBytes(messages)
	window.Middle = applySummaryBudget(window.Middle, newSummaryBudget(config.LoadConfig().HandoffSummary, totalTranscriptBytes))
	window.Tail = pruneMessages(window.Tail)

	genCtx, cancel := context.WithTimeout(ctx, handoffSummaryTimeout)
	summaryText, err := headless.GenerateHandoffSummary(genCtx, g.pool, sourceSessionTitle, toHandoffMessages(window.Head), toHandoffMessages(window.Middle), toHandoffMessages(window.Tail))
	cancel()
	if err != nil {
		g.failStage(ctx, sourceSessionID, sourceSessionTitle, "generation", err, now, false)
		return
	}

	activeTask := extractActiveTask(summaryText)
	finalNow := time.Now()

	if err := g.entClient.HandoffSummary.Create().
		SetID(uuid.New().String()).
		SetSessionID(sourceSessionID).
		SetSessionTitle(sourceSessionTitle).
		SetStatus(string(HandoffSummaryStatusReady)).
		SetSummaryText(summaryText).
		SetActiveTask(activeTask).
		SetMiddleMessagesSummarized(len(window.Middle)).
		SetGenerationStartedAt(now).
		SetGeneratedAt(finalNow).
		OnConflictColumns(handoffsummary.FieldSessionID).
		UpdateNewValues().
		Update(func(u *ent.HandoffSummaryUpsert) {
			// A prior failed generation for this session may have left
			// error_message/error_stage set — clear them so a successful
			// (READY) row never carries forward stale error state.
			u.ClearErrorMessage()
			u.ClearErrorStage()
		}).
		Exec(ctx); err != nil {
		// LLM cost already spent, row still says GENERATING — attempt a
		// best-effort fallback write rather than silently leaving it stuck.
		g.failStage(ctx, sourceSessionID, sourceSessionTitle, "persist", err, now, true)
		return
	}

	log.ForSession(sourceSessionID).Info("[HandoffSummary] GENERATING -> READY")
}
