package services

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

type CapacityMonitor struct {
	clients         map[string]ProviderLimitsClient
	config          config.CapacityConfig
	eventBus        *events.EventBus
	poller          InstancePoller
	tokenStore      tokens.TokenStoreReader
	sessionSwitcher SessionSwitcher
	compactor       SessionCompactor

	mu              sync.RWMutex
	current         map[string]ProviderLimits
	sessionLimits   map[string]ProviderLimits  // keyed by session title
	lastWarningTime map[string]time.Time       // keyed by session title to rate-limit events
	idleWaitState   map[string]idleWaitTracker // keyed by session UUID (see checkIdleWaitLoop)
}

type InstancePoller interface {
	GetInstances() []*session.Instance
}

type SessionSwitcher interface {
	UpdateSessionProgram(ctx context.Context, sessionID string, newProgram string) error
}

// SessionCompactor sends driver-generated content (here, always "/compact")
// to a live session's pane. Injected as a function value, rather than calling
// session.SubmitContentWithEnter directly, so tests can assert on invocations
// without needing a real tmux-backed *session.Instance. The production
// default is session.SubmitContentWithEnter itself (wired at construction).
type SessionCompactor func(ctx context.Context, inst *session.Instance, content string) error

// CapacityMonitorParams bundles NewCapacityMonitor's dependencies. Compactor
// may be left nil — it defaults to session.SubmitContentWithEnter.
type CapacityMonitorParams struct {
	Config          config.CapacityConfig
	EventBus        *events.EventBus
	Poller          InstancePoller
	TokenStore      tokens.TokenStoreReader
	SessionSwitcher SessionSwitcher
	Compactor       SessionCompactor
}

func NewCapacityMonitor(p CapacityMonitorParams) *CapacityMonitor {
	compactor := p.Compactor
	if compactor == nil {
		// session.SubmitContentWithEnter takes the unexported paneSubmitter
		// interface, not *session.Instance, so it can't be assigned to
		// SessionCompactor directly — wrap it. *session.Instance satisfies
		// paneSubmitter, so this call resolves fine even though the two
		// function *types* don't match for a direct assignment.
		compactor = func(ctx context.Context, inst *session.Instance, content string) error {
			return session.SubmitContentWithEnter(ctx, inst, content)
		}
	}
	return &CapacityMonitor{
		clients:         make(map[string]ProviderLimitsClient),
		config:          p.Config,
		eventBus:        p.EventBus,
		poller:          p.Poller,
		tokenStore:      p.TokenStore,
		sessionSwitcher: p.SessionSwitcher,
		compactor:       compactor,
		current:         make(map[string]ProviderLimits),
		sessionLimits:   make(map[string]ProviderLimits),
		lastWarningTime: make(map[string]time.Time),
		idleWaitState:   make(map[string]idleWaitTracker),
	}
}

func (m *CapacityMonitor) RegisterClient(name string, client ProviderLimitsClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[name] = client
}

func (m *CapacityMonitor) Start(ctx context.Context) {
	// Query initially on start.
	m.poll(ctx)

	pollInterval := time.Duration(m.config.PollIntervalSeconds) * time.Second
	if pollInterval <= 0 {
		pollInterval = 60 * time.Second
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.poll(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (m *CapacityMonitor) UpdateFromResponseHeaders(provider string, headers http.Header) {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, ok := m.clients[provider]
	if !ok {
		return
	}

	cur := m.current[provider]
	updated := client.UpdateFromResponseHeaders(headers, cur)
	m.current[provider] = updated
}

func (m *CapacityMonitor) poll(ctx context.Context) {
	m.mu.Lock()
	clientsCopy := make(map[string]ProviderLimitsClient, len(m.clients))
	for k, v := range m.clients {
		clientsCopy[k] = v
	}
	m.mu.Unlock()

	for name, client := range clientsCopy {
		m.mu.Lock()
		cur := m.current[name]
		m.mu.Unlock()

		// Optimize Anthropic polling: don't make probe calls if we recently got response headers.
		if name == "anthropic" && !cur.FetchedAt.IsZero() && time.Since(cur.FetchedAt) < time.Duration(m.config.PollIntervalSeconds)*time.Second {
			continue
		}

		limits, err := client.QueryLimits(ctx)
		if err != nil {
			log.Warn("CapacityMonitor: failed to poll limits", "provider", name, "err", err)
			continue
		}

		m.mu.Lock()
		m.current[name] = limits
		m.mu.Unlock()
	}

	m.evaluate(ctx)
}

func (m *CapacityMonitor) evaluate(ctx context.Context) {
	if m.poller == nil {
		return
	}

	instances := m.poller.GetInstances()
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		// Use a lock-free snapshot read instead of accessing inst.Status directly.
		if inst.Snapshot().Status != session.Active {
			continue
		}

		m.evaluateInstance(ctx, inst)
	}
}

func (m *CapacityMonitor) evaluateInstance(ctx context.Context, inst *session.Instance) {
	// Lock-free snapshot read: inst.Program was previously read without any lock.
	snap := inst.Snapshot()

	provider := "anthropic"
	program := strings.ToLower(snap.Program)
	if strings.Contains(program, "agy") || strings.Contains(program, "antigravity") || strings.Contains(program, "gemini") {
		provider = "google"
	} else if strings.Contains(program, "openai") || strings.Contains(program, "opencode") {
		provider = "openai"
	}

	m.mu.Lock()
	client, hasClient := m.clients[provider]
	provLimits := m.current[provider]
	m.mu.Unlock()

	if !hasClient {
		return
	}

	limits := provLimits
	limits.Model = snap.Program

	uuid := inst.GetClaudeConversationUUID()
	if uuid == "" {
		return
	}

	// 1. Gather session usage tokens.
	var input, output int64
	var contextUsed int
	var anthropicParseRes *tokens.ParseResult

	switch provider {
	case "anthropic":
		if parseRes := m.tokenStore.GetByUUID(uuid); parseRes != nil {
			anthropicParseRes = parseRes
			input = parseRes.TotalInput
			output = parseRes.TotalOutput
			if len(parseRes.TurnTimeline) > 0 {
				contextUsed = int(parseRes.TurnTimeline[len(parseRes.TurnTimeline)-1].Input)
			}
		}
	case "google":
		var err error
		input, output, contextUsed, err = m.queryGeminiUsageFromDB(uuid)
		if err != nil {
			log.Debug("CapacityMonitor: failed to query gemini DB usage", "session", snap.Title, "err", err)
		}
	}

	limits.SessionInputTokens = int(input)
	limits.SessionOutputTokens = int(output)
	limits.ContextTokensUsed = contextUsed
	limits.ContextTokensMax = client.ModelContextWindow(snap.Program)

	// 2. Estimate cost.
	limits.EstimatedCostUSD = m.estimateCost(snap.Program, input, output)

	m.mu.Lock()
	m.sessionLimits[snap.Title] = limits
	m.mu.Unlock()

	// 3. Check for an idle-wait loop (independent of the capacity thresholds
	// below — a session can be nowhere near its context/cost ceiling and
	// still be burning money re-reading a growing cached history on every
	// trivial "still waiting" wake-up turn; see #882).
	if anthropicParseRes != nil {
		m.checkIdleWaitLoop(ctx, inst, snap, anthropicParseRes)
	}

	// 4. Check thresholds.
	reason := m.checkThresholds(limits)
	if reason == "" {
		return
	}

	m.handleTransitionTrigger(ctx, inst, reason, limits)
}

func (m *CapacityMonitor) checkThresholds(limits ProviderLimits) string {
	// Check context window pct.
	if limits.ContextTokensMax > 0 && limits.ContextTokensUsed > 0 {
		pct := float64(limits.ContextTokensUsed) / float64(limits.ContextTokensMax)
		if pct >= m.config.ContextWindowWarnPct {
			return fmt.Sprintf("context_limit_%.0f_percent", pct*100)
		}
	}

	// Check rate limit remaining.
	if limits.RequestsLimit > 0 && limits.RequestsRemaining >= 0 && limits.RequestsRemaining <= m.config.RateLimitWarnRemaining {
		return "rate_limit_exhausted"
	}

	// Check budget.
	if m.config.CostBudgetUSD > 0 && limits.EstimatedCostUSD >= m.config.CostBudgetUSD {
		return "cost_budget_exceeded"
	}

	return ""
}

func (m *CapacityMonitor) handleTransitionTrigger(ctx context.Context, inst *session.Instance, reason string, limits ProviderLimits) {
	// Lock-free snapshot read for inst.Title, inst.Program, inst.UUID.
	snap := inst.Snapshot()

	m.mu.Lock()
	lastWarn := m.lastWarningTime[snap.Title]
	m.mu.Unlock()

	// Rate-limit warnings to once per 5 minutes per session.
	if time.Since(lastWarn) < 5*time.Minute {
		return
	}

	m.mu.Lock()
	m.lastWarningTime[snap.Title] = time.Now()
	m.mu.Unlock()

	// Determine transition target.
	var nextCLI, nextModel string
	for _, target := range m.config.ProviderPriority {
		if !strings.EqualFold(target.CLI, snap.Program) {
			nextCLI = target.CLI
			nextModel = target.Model
			break
		}
	}

	if nextCLI == "" {
		nextCLI = "agy"
		nextModel = "gemini-2.0-flash"
	}

	msg := fmt.Sprintf("Capacity Warning for %s: %s (Context: %d/%d). Suggesting switch to %s.",
		snap.Title, reason, limits.ContextTokensUsed, limits.ContextTokensMax, nextCLI)

	log.Warn("CapacityMonitor: trigger warning", "session", snap.Title, "reason", reason, "next", nextCLI)

	// Publish notification event to frontend.
	m.eventBus.Publish(events.NewNotificationEvent(
		snap.UUID,
		snap.Title,
		fmt.Sprintf("cap-%d", time.Now().Unix()),
		2, // warning priority
		2, // warning priority
		"Capacity Alert",
		msg,
		map[string]string{
			"type":       "capacity_alert",
			"reason":     reason,
			"suggest_to": nextCLI,
			"model":      nextModel,
		},
	))

	// Perform auto transition if enabled.
	if m.config.TransitionMode == config.TransitionModeAuto {
		log.Info("CapacityMonitor: performing auto-transition", "session", snap.Title, "to", nextCLI)
		go func() {
			if err := m.sessionSwitcher.UpdateSessionProgram(context.Background(), snap.Title, nextCLI); err != nil {
				log.Error("CapacityMonitor: auto-transition failed", "session", snap.Title, "to", nextCLI, "err", err)
			}
		}()
	}
}

// idleWaitTracker remembers the last time we intervened on a session's
// idle-wait loop, so a repeat crossing of the ceiling after that point can be
// told apart from the same still-open run continuing (see checkIdleWaitLoop).
type idleWaitTracker struct {
	lastInterventionAt time.Time
}

// idleWaitSignalTools are the only tool_use names a turn may contain and
// still count as "idle waiting," alongside a turn with no tool calls at all
// (a plain "still waiting..." status reply). Any other tool name — Read,
// Write, Edit, Bash, Grep, etc. — means the turn did real work and breaks
// the run. Deliberately a narrow allowlist, not a denylist: an unrecognized
// future tool name should NOT silently count as idle.
var idleWaitSignalTools = map[string]bool{
	"Monitor":     true,
	"ListAgents":  true,
	"TaskStop":    true,
	"SendMessage": true,
}

// isIdleWaitTurn reports whether a turn matches the "still waiting on
// background work" shape observed in the incident behind #882: a session
// spawns background subagents, then gets woken by task-notification events
// and produces a short status turn instead of doing new work — safe
// individually, but unbounded, it re-reads the entire cached conversation on
// every wake and can run for hundreds of turns without anyone noticing.
func isIdleWaitTurn(turn tokens.TurnStats) bool {
	for _, name := range turn.ToolNames {
		if !idleWaitSignalTools[name] {
			return false
		}
	}
	return true
}

// consecutiveIdleWaitTurns counts idle-wait turns from the end of timeline
// backward, stopping at the first non-idle turn. If since is non-zero, turns
// at or before it are excluded — used to count only the turns that happened
// after a prior intervention, so a loop that never actually broke isn't
// merged with the run that triggered the first compact.
func consecutiveIdleWaitTurns(timeline []tokens.TurnStats, since time.Time) int {
	count := 0
	for i := len(timeline) - 1; i >= 0; i-- {
		turn := timeline[i]
		if !since.IsZero() && !turn.Timestamp.After(since) {
			break
		}
		if !isIdleWaitTurn(turn) {
			break
		}
		count++
	}
	return count
}

// checkIdleWaitLoop detects and responds to the idle-wait-loop pattern from
// #882. First crossing of the configured ceiling: send /compact to collapse
// the ballooning cached history (non-destructive — no in-progress background
// work is lost). If the pattern recurs for another full ceiling's worth of
// turns strictly after that intervention (compacting alone didn't stop it —
// e.g. still waiting on the same slow background task), escalate via a
// higher-priority notification instead of compacting forever, so an operator
// gets visibility. Keyed by session UUID (not conversation UUID, which can
// rotate across resume/history-transfer, and not Title, which a user can
// rename) so intervention state survives both.
func (m *CapacityMonitor) checkIdleWaitLoop(ctx context.Context, inst *session.Instance, snap *session.InstanceSnapshot, parseRes *tokens.ParseResult) {
	ceiling := m.config.IdleWaitTurnCeiling
	if ceiling <= 0 {
		return
	}

	m.mu.Lock()
	tracker, hasTracker := m.idleWaitState[snap.UUID]
	m.mu.Unlock()

	if hasTracker && !tracker.lastInterventionAt.IsZero() {
		m.checkIdleWaitLoopPostIntervention(snap, parseRes, tracker, ceiling)
		return
	}

	count := consecutiveIdleWaitTurns(parseRes.TurnTimeline, time.Time{})
	if count < ceiling {
		return
	}
	m.compactIdleWaitLoop(ctx, inst, snap, count)
	m.mu.Lock()
	m.idleWaitState[snap.UUID] = idleWaitTracker{lastInterventionAt: time.Now()}
	m.mu.Unlock()
}

// checkIdleWaitLoopPostIntervention handles the case where a prior /compact
// already fired for this session: escalate if the idle-wait run continued
// for another full ceiling's worth of turns strictly after that compact, or
// clear the tracker if a real (non-idle-wait) turn happened since — the loop
// broke on its own, so a future run should be treated as fresh.
func (m *CapacityMonitor) checkIdleWaitLoopPostIntervention(snap *session.InstanceSnapshot, parseRes *tokens.ParseResult, tracker idleWaitTracker, ceiling int) {
	postCount := consecutiveIdleWaitTurns(parseRes.TurnTimeline, tracker.lastInterventionAt)
	if postCount >= ceiling {
		m.escalateIdleWaitLoop(snap, postCount)
		m.mu.Lock()
		delete(m.idleWaitState, snap.UUID)
		m.mu.Unlock()
		return
	}

	n := len(parseRes.TurnTimeline)
	if n == 0 {
		return
	}
	last := parseRes.TurnTimeline[n-1]
	if last.Timestamp.After(tracker.lastInterventionAt) && !isIdleWaitTurn(last) {
		m.mu.Lock()
		delete(m.idleWaitState, snap.UUID)
		m.mu.Unlock()
	}
}

// compactIdleWaitLoop sends /compact to inst and publishes an informational
// notification. Errors are logged, not returned — this runs from the
// background poll loop, and a failed compact attempt should not block the
// next poll cycle (the tracker still records the attempt, so a repeat
// crossing after this point still escalates rather than retrying /compact
// forever against a session that won't accept input).
func (m *CapacityMonitor) compactIdleWaitLoop(ctx context.Context, inst *session.Instance, snap *session.InstanceSnapshot, idleTurns int) {
	log.Info("CapacityMonitor: idle-wait loop detected, sending /compact",
		"session", snap.Title, "idle_turns", idleTurns, "ceiling", m.config.IdleWaitTurnCeiling)

	if err := m.compactor(ctx, inst, "/compact"); err != nil {
		log.Warn("CapacityMonitor: /compact for idle-wait loop failed", "session", snap.Title, "err", err)
	}

	m.eventBus.Publish(events.NewNotificationEvent(
		snap.UUID,
		snap.Title,
		fmt.Sprintf("idle-wait-compact-%d", time.Now().Unix()),
		10, // NOTIFICATION_TYPE_INFO
		1,  // NOTIFICATION_PRIORITY_LOW
		"Idle-Wait Loop Compacted",
		fmt.Sprintf("%s spent %d consecutive turns waiting on background work — auto-compacted to stop re-reading a growing cached history on every wake.", snap.Title, idleTurns),
		map[string]string{
			"type":       "idle_wait_loop_compact",
			"idle_turns": fmt.Sprintf("%d", idleTurns),
		},
	))
}

// escalateIdleWaitLoop publishes a higher-priority notification when the
// idle-wait loop recurs after an auto-compact, using the same event-bus
// mechanism this file already uses for capacity warnings — not the
// item-keyed BacklogStuckState/StuckReason machinery, which needs a new
// StuckReason value wired through proto and domain together.
func (m *CapacityMonitor) escalateIdleWaitLoop(snap *session.InstanceSnapshot, idleTurns int) {
	log.Warn("CapacityMonitor: idle-wait loop recurred after compact, escalating",
		"session", snap.Title, "idle_turns", idleTurns)

	m.eventBus.Publish(events.NewNotificationEvent(
		snap.UUID,
		snap.Title,
		fmt.Sprintf("idle-wait-escalate-%d", time.Now().Unix()),
		8, // NOTIFICATION_TYPE_WARNING
		3, // NOTIFICATION_PRIORITY_HIGH
		"Idle-Wait Loop Persisting",
		fmt.Sprintf("%s is still looping on idle-wait turns after an auto-compact (%d more consecutive turns) — likely stuck waiting on the same background task. Needs a look.", snap.Title, idleTurns),
		map[string]string{
			"type":       "idle_wait_loop_escalation",
			"idle_turns": fmt.Sprintf("%d", idleTurns),
		},
	))
}

func (m *CapacityMonitor) queryGeminiUsageFromDB(uuid string) (input, output int64, lastInput int, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, 0, 0, err
	}

	dbPath := filepath.Join(home, ".gemini", "antigravity-cli", "conversations", uuid+".db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return 0, 0, 0, nil
	}

	db, err := sql.Open("sqlite", sqlitedsn.New(dbPath).Build())
	if err != nil {
		return 0, 0, 0, err
	}
	defer db.Close()

	// Query cumulative sizes. In gen_metadata, size represents the request size in bytes.
	// We divide by 4 to get approximate tokens.
	var totalSize int64
	err = db.QueryRow("SELECT COALESCE(SUM(size), 0) FROM gen_metadata").Scan(&totalSize)
	if err != nil {
		return 0, 0, 0, err
	}

	// Fetch last size.
	var lastSize int
	_ = db.QueryRow("SELECT size FROM gen_metadata ORDER BY idx DESC LIMIT 1").Scan(&lastSize)

	totalTokens := totalSize / 4
	lastTokens := lastSize / 4

	// Estimate: 80% input, 20% output.
	return int64(float64(totalTokens) * 0.8), int64(float64(totalTokens) * 0.2), lastTokens, nil
}

func (m *CapacityMonitor) estimateCost(model string, input, output int64) float64 {
	inputPrice := 3.0   // sonnet default
	outputPrice := 15.0 // sonnet default

	model = strings.ToLower(model)
	if strings.Contains(model, "opus") {
		inputPrice = 15.0
		outputPrice = 75.0
	} else if strings.Contains(model, "haiku") {
		inputPrice = 0.25
		outputPrice = 1.25
	} else if strings.Contains(model, "flash") {
		inputPrice = 0.075
		outputPrice = 0.3
	} else if strings.Contains(model, "pro") {
		inputPrice = 1.25
		outputPrice = 5.0
	}

	return (float64(input)*inputPrice + float64(output)*outputPrice) / 1_000_000.0
}

func (m *CapacityMonitor) GetCurrentLimits() map[string]ProviderLimits {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]ProviderLimits, len(m.current))
	for k, v := range m.current {
		out[k] = v
	}
	return out
}

func (m *CapacityMonitor) GetSessionLimits(title string) (ProviderLimits, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	limits, ok := m.sessionLimits[title]
	return limits, ok
}
