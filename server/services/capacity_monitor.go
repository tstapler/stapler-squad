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
	pricing         *tokens.PricingTable
	roleResolver    SessionRoleResolver
	terminator      SessionTerminator
	now             func() time.Time

	mu              sync.RWMutex
	current         map[string]ProviderLimits
	sessionLimits   map[string]ProviderLimits  // keyed by session title
	lastWarningTime map[string]time.Time       // keyed by session title to rate-limit events
	idleWaitState   map[string]idleWaitTracker // keyed by session UUID (see checkIdleWaitLoop)
	roleCache       map[string]string          // session UUID -> backlog role (hits only)
	handledOnce     map[string]bool            // "<kind>:<session UUID>" one-shot guardrail actions
}

// SessionRoleResolver returns the backlog session role ("triage", "work", ...)
// of the session with the given UUID, or "" if it isn't backlog-linked.
type SessionRoleResolver func(ctx context.Context, sessionUUID string) string

// SessionTerminator stops a guardrail-tripped session and records reason so
// the orchestrator can pick the item back up. Nil means notify-only.
type SessionTerminator func(ctx context.Context, inst *session.Instance, reason string) error

// pipelineRoles are sessions the orchestrator spawned for a bounded stage; a
// cost-budget breach stops them rather than only warning.
var pipelineRoles = map[string]bool{
	session.SessionRoleTriage:   true,
	session.SessionRoleReview:   true,
	session.SessionRoleDiagnose: true,
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
	RoleResolver    SessionRoleResolver
	Terminator      SessionTerminator
}

func NewCapacityMonitor(p CapacityMonitorParams) *CapacityMonitor {
	compactor := p.Compactor
	if compactor == nil {
		// session.SubmitContentWithEnter takes the unexported paneSubmitter
		// interface, not *session.Instance, so it can't be assigned to
		// SessionCompactor directly — wrap it. *session.Instance satisfies
		// paneSubmitter, so this call resolves fine even though the two
		// function *types* don't match for a direct assignment.
		//
		// The closure is the chain's acquirer (Story 5.0): a busy lease is an
		// error the capacity monitor already logs and retries next cycle.
		compactor = func(ctx context.Context, inst *session.Instance, content string) error {
			lease, ok := inst.TryTerminalWriteLease(session.LeaseWriterOther)
			if !ok {
				return session.ErrLeaseBusy
			}
			return session.SubmitContentWithEnter(ctx, inst, lease, content)
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
		pricing:         tokens.DefaultPricingTable(),
		roleResolver:    p.RoleResolver,
		terminator:      p.Terminator,
		now:             time.Now,
		roleCache:       make(map[string]string),
		handledOnce:     make(map[string]bool),
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
	active := make(map[string]bool, len(instances))
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		// Use a lock-free snapshot read instead of accessing inst.Status directly.
		snap := inst.Snapshot()
		if snap.Status != session.Active {
			continue
		}
		active[snap.UUID] = true

		m.evaluateInstance(ctx, inst)
	}
	m.pruneSessionState(active)
}

// pruneSessionState drops per-session guardrail state for sessions that are no
// longer active, bounding map growth and letting a resumed session re-arm.
func (m *CapacityMonitor) pruneSessionState(active map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for uuid := range m.idleWaitState {
		if !active[uuid] {
			delete(m.idleWaitState, uuid)
		}
	}
	for uuid := range m.roleCache {
		if !active[uuid] {
			delete(m.roleCache, uuid)
		}
	}
	for key := range m.handledOnce {
		if !active[key[strings.LastIndex(key, ":")+1:]] {
			delete(m.handledOnce, key)
		}
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
	if anthropicParseRes != nil {
		// Cache reads dominate long sessions (93% of the #882 incident); the
		// input/output-only estimate above can never trip a budget on them.
		// Unpriced models keep the fallback — never understate cost.
		if cost, unpriced := m.pricing.EstimateCost(anthropicParseRes); len(unpriced) == 0 && cost > limits.EstimatedCostUSD {
			limits.EstimatedCostUSD = cost
		}
	}
	role := m.sessionRole(ctx, snap.UUID)

	m.mu.Lock()
	m.sessionLimits[snap.Title] = limits
	m.mu.Unlock()

	// 3. Check for an idle-wait loop (independent of the capacity thresholds
	// below — a session can be nowhere near its context/cost ceiling and
	// still be burning money re-reading a growing cached history on every
	// trivial "still waiting" wake-up turn; see #882).
	if anthropicParseRes != nil {
		m.checkIdleWaitLoop(ctx, inst, snap, anthropicParseRes, role)
		m.checkSessionTokenCeiling(snap, anthropicParseRes)
	}

	// 4. Pipeline-role cost budget: stop and hand back instead of warning.
	if pipelineRoles[role] && m.overCostBudget(limits, role) {
		m.stopForGuardrail(ctx, inst, snap, "cost_budget_exceeded", "Cost Budget Exceeded",
			fmt.Sprintf("%s (%s) reached $%.2f against its $%.2f budget; stopping it.", snap.Title, role, limits.EstimatedCostUSD, m.config.CostBudgetFor(role)))
		return
	}

	// 5. Check thresholds.
	reason := m.checkThresholds(limits, role)
	if reason == "" {
		return
	}

	m.handleTransitionTrigger(ctx, inst, reason, limits)
}

func (m *CapacityMonitor) overCostBudget(limits ProviderLimits, role string) bool {
	budget := m.config.CostBudgetFor(role)
	return budget > 0 && limits.EstimatedCostUSD >= budget
}

// sessionRole resolves and caches a session's backlog role ("" if none).
func (m *CapacityMonitor) sessionRole(ctx context.Context, sessionUUID string) string {
	if m.roleResolver == nil || sessionUUID == "" {
		return ""
	}
	m.mu.RLock()
	cached, ok := m.roleCache[sessionUUID]
	m.mu.RUnlock()
	if ok {
		return cached
	}
	role := m.roleResolver(ctx, sessionUUID)
	if role != "" {
		m.mu.Lock()
		m.roleCache[sessionUUID] = role
		m.mu.Unlock()
	}
	return role
}

// firstTime reports true once per key, so a persistent condition acts and
// notifies exactly once.
func (m *CapacityMonitor) firstTime(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handledOnce[key] {
		return false
	}
	m.handledOnce[key] = true
	return true
}

// stopForGuardrail publishes one high-priority notification and, when a
// terminator is wired, stops the session so the orchestrator takes it back.
// One-shot per (kind, session): it never re-arms.
func (m *CapacityMonitor) stopForGuardrail(ctx context.Context, inst *session.Instance, snap *session.InstanceSnapshot, kind, title, msg string) {
	if !m.firstTime("stopped:" + kind + ":" + snap.UUID) {
		return
	}
	log.Warn("CapacityMonitor: guardrail tripped", "session", snap.Title, "kind", kind)
	if m.firstTime("notified:" + kind + ":" + snap.UUID) {
		m.eventBus.Publish(events.NewNotificationEvent(
			snap.UUID, snap.Title,
			fmt.Sprintf("%s-%d", kind, time.Now().Unix()),
			8, // NOTIFICATION_TYPE_WARNING
			3, // NOTIFICATION_PRIORITY_HIGH
			title, msg,
			map[string]string{"type": kind},
		))
	}
	if m.terminator == nil {
		return
	}
	if err := m.terminator(ctx, inst, kind); err != nil {
		log.Error("CapacityMonitor: terminating session failed, will retry next poll", "session", snap.Title, "kind", kind, "err", err)
		// Un-mark so the next poll retries; the notification stays one-shot.
		m.mu.Lock()
		delete(m.handledOnce, "stopped:"+kind+":"+snap.UUID)
		m.mu.Unlock()
	}
}

// checkSessionTokenCeiling surfaces a critical SESSION_TOKEN_CEILING finding
// (otherwise visible only on the Insights page) as one notification per session.
func (m *CapacityMonitor) checkSessionTokenCeiling(snap *session.InstanceSnapshot, parseRes *tokens.ParseResult) {
	for _, f := range tokens.ComputeFindings(parseRes, m.pricing) {
		if f.Type != tokens.FindingSessionTokenCeiling || f.Severity != tokens.SeverityCritical {
			continue
		}
		if !m.firstTime("token_ceiling:" + snap.UUID) {
			return
		}
		m.eventBus.Publish(events.NewNotificationEvent(
			snap.UUID, snap.Title,
			fmt.Sprintf("token-ceiling-%d", time.Now().Unix()),
			8, 3,
			"Session Token Ceiling Exceeded",
			fmt.Sprintf("%s: %s", snap.Title, f.Message),
			map[string]string{"type": "session_token_ceiling"},
		))
		return
	}
}

func (m *CapacityMonitor) checkThresholds(limits ProviderLimits, role string) string {
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
	if m.overCostBudget(limits, role) {
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

// idleWaitRun measures the trailing run of idle-wait turns: its length and
// the span between its first and last turn. If since is non-zero, turns at or
// before it are excluded — used to count only the turns that happened after
// a prior intervention, so a loop that never actually broke isn't merged
// with the run that triggered the first compact.
func idleWaitRun(timeline []tokens.TurnStats, since time.Time) idleRun {
	var run idleRun
	for i := len(timeline) - 1; i >= 0; i-- {
		turn := timeline[i]
		if !since.IsZero() && !turn.Timestamp.After(since) {
			break
		}
		if !isIdleWaitTurn(turn) {
			break
		}
		if run.count == 0 {
			run.last = turn.Timestamp
		}
		run.first = turn.Timestamp
		run.count++
		run.sawSignalTool = run.sawSignalTool || len(turn.ToolNames) > 0
	}
	return run
}

// idleRun describes a trailing run of idle-wait turns. sawSignalTool is true
// when at least one turn used a wait tool (Monitor, ListAgents, ...), which
// separates "waiting on background work" from a plain tool-less chat.
type idleRun struct {
	count         int
	first, last   time.Time
	sawSignalTool bool
}

// idleRunFreshness is how recent a run's last turn must be for the run to
// count as live; a finished transcript tail from a parked session must not
// trigger a /compact. Wake cadence in the #882 incident was a few minutes.
const idleRunFreshness = 15 * time.Minute

// consecutiveIdleWaitTurns is idleWaitRun's count alone.
func consecutiveIdleWaitTurns(timeline []tokens.TurnStats, since time.Time) int {
	return idleWaitRun(timeline, since).count
}

// wallClockMinTurns is the fewest idle turns that can trip the wall-clock
// bound; a lone text-only turn from a session parked at the prompt must not
// count as a loop however long ago it was.
const wallClockMinTurns = 3

// idleWaitExceeded reports whether the trailing idle run trips the turn
// ceiling or the wall-clock bound.
func (m *CapacityMonitor) idleWaitExceeded(timeline []tokens.TurnStats, since time.Time, ceiling int) (int, bool) {
	run := idleWaitRun(timeline, since)
	if !run.sawSignalTool || m.now().Sub(run.last) > idleRunFreshness {
		return run.count, false
	}
	if run.count >= ceiling {
		return run.count, true
	}
	maxSpan := time.Duration(m.config.IdleWaitMaxMinutes) * time.Minute
	return run.count, maxSpan > 0 && run.count >= wallClockMinTurns && run.last.Sub(run.first) >= maxSpan
}

// checkIdleWaitLoop detects and responds to the idle-wait-loop pattern from
// #882. First trip (turn ceiling or wall-clock bound): send /compact to
// collapse the ballooning cached history. If the pattern trips again strictly
// after that intervention, take the terminal action once — notify and, for a
// backlog-linked session, stop it and hand back to the orchestrator — and
// never act again for that loop (the tracker stays until real work clears it).
// Keyed by session UUID (not conversation UUID, which can rotate across
// resume/history-transfer, and not Title, which a user can rename).
func (m *CapacityMonitor) checkIdleWaitLoop(ctx context.Context, inst *session.Instance, snap *session.InstanceSnapshot, parseRes *tokens.ParseResult, role string) {
	ceiling := m.config.IdleWaitCeilingFor(role)
	if ceiling <= 0 {
		return
	}

	m.mu.Lock()
	tracker, hasTracker := m.idleWaitState[snap.UUID]
	m.mu.Unlock()

	if hasTracker && !tracker.lastInterventionAt.IsZero() {
		m.checkIdleWaitLoopPostIntervention(ctx, inst, snap, parseRes, tracker, ceiling, role)
		return
	}

	count, exceeded := m.idleWaitExceeded(parseRes.TurnTimeline, time.Time{}, ceiling)
	if !exceeded {
		return
	}
	m.compactIdleWaitLoop(ctx, inst, snap, count)
	m.mu.Lock()
	m.idleWaitState[snap.UUID] = idleWaitTracker{lastInterventionAt: time.Now()}
	m.mu.Unlock()
}

// checkIdleWaitLoopPostIntervention handles the case where a prior /compact
// already fired: take the terminal action if the idle run tripped again
// strictly after that compact, or clear the tracker if a real turn happened
// since — the loop broke on its own, so a future run is fresh.
func (m *CapacityMonitor) checkIdleWaitLoopPostIntervention(ctx context.Context, inst *session.Instance, snap *session.InstanceSnapshot, parseRes *tokens.ParseResult, tracker idleWaitTracker, ceiling int, role string) {
	if realWorkSince(parseRes.TurnTimeline, tracker.lastInterventionAt) {
		m.mu.Lock()
		delete(m.idleWaitState, snap.UUID)
		delete(m.handledOnce, "stopped:idle_wait_loop_escalation:"+snap.UUID)
		delete(m.handledOnce, "notified:idle_wait_loop_escalation:"+snap.UUID)
		delete(m.handledOnce, "idle_wait_loop_escalation:"+snap.UUID)
		m.mu.Unlock()
		return
	}

	postCount, exceeded := m.idleWaitExceeded(parseRes.TurnTimeline, tracker.lastInterventionAt, ceiling)
	if !exceeded {
		return
	}
	// Only backlog-linked sessions are stopped; a human's interactive session
	// gets the notification alone.
	if role == "" {
		inst = nil
	}
	m.escalateIdleWaitLoop(ctx, inst, snap, postCount)
}

// realWorkSince reports whether any non-idle turn happened after since.
func realWorkSince(timeline []tokens.TurnStats, since time.Time) bool {
	for i := len(timeline) - 1; i >= 0 && timeline[i].Timestamp.After(since); i-- {
		if !isIdleWaitTurn(timeline[i]) {
			return true
		}
	}
	return false
}

// compactIdleWaitLoop sends /compact to inst and publishes an informational
// notification. Errors are logged, not returned — this runs from the
// background poll loop, and a failed compact attempt should not block the
// next poll cycle (the tracker still records the attempt, so a repeat
// trip after this point still escalates rather than retrying /compact
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

// escalateIdleWaitLoop is the one-shot terminal action for a loop that
// survived /compact. A nil inst means notify-only (no session to stop).
func (m *CapacityMonitor) escalateIdleWaitLoop(ctx context.Context, inst *session.Instance, snap *session.InstanceSnapshot, idleTurns int) {
	const kind = "idle_wait_loop_escalation"
	msg := fmt.Sprintf("%s is still looping on idle-wait turns after an auto-compact (%d more consecutive turns) — likely stuck waiting on the same background task.", snap.Title, idleTurns)
	if inst == nil {
		if !m.firstTime(kind + ":" + snap.UUID) {
			return
		}
		m.eventBus.Publish(events.NewNotificationEvent(
			snap.UUID, snap.Title, fmt.Sprintf("%s-%d", kind, time.Now().Unix()),
			8, 3, "Idle-Wait Loop Persisting", msg+" Needs a look.",
			map[string]string{"type": kind, "idle_turns": fmt.Sprintf("%d", idleTurns)},
		))
		return
	}
	m.stopForGuardrail(ctx, inst, snap, kind, "Idle-Wait Loop Stopped", msg+" The session is being stopped and handed back to the orchestrator.")
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
