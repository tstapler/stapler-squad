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

	_ "github.com/mattn/go-sqlite3"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
)

type CapacityMonitor struct {
	clients         map[string]ProviderLimitsClient
	config          config.CapacityConfig
	eventBus        *events.EventBus
	poller          InstancePoller
	tokenStore      tokens.TokenStoreReader
	sessionSwitcher SessionSwitcher

	mu              sync.RWMutex
	current         map[string]ProviderLimits
	sessionLimits   map[string]ProviderLimits // keyed by session title
	lastWarningTime map[string]time.Time       // keyed by session title to rate-limit events
}

type InstancePoller interface {
	GetInstances() []*session.Instance
}

type SessionSwitcher interface {
	UpdateSessionProgram(ctx context.Context, sessionID string, newProgram string) error
}

func NewCapacityMonitor(
	cfg config.CapacityConfig,
	eventBus *events.EventBus,
	poller InstancePoller,
	tokenStore tokens.TokenStoreReader,
	switcher SessionSwitcher,
) *CapacityMonitor {
	return &CapacityMonitor{
		clients:         make(map[string]ProviderLimitsClient),
		config:          cfg,
		eventBus:        eventBus,
		poller:          poller,
		tokenStore:      tokenStore,
		sessionSwitcher: switcher,
		current:         make(map[string]ProviderLimits),
		sessionLimits:   make(map[string]ProviderLimits),
		lastWarningTime: make(map[string]time.Time),
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
		if inst == nil || inst.Status != session.Active {
			continue
		}

		m.evaluateInstance(ctx, inst)
	}
}

func (m *CapacityMonitor) evaluateInstance(ctx context.Context, inst *session.Instance) {
	provider := "anthropic"
	program := strings.ToLower(inst.Program)
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
	limits.Model = inst.Program

	uuid := inst.GetClaudeConversationUUID()
	if uuid == "" {
		return
	}

	// 1. Gather session usage tokens.
	var input, output int64
	var contextUsed int

	if provider == "anthropic" {
		if parseRes := m.tokenStore.GetByUUID(uuid); parseRes != nil {
			input = parseRes.TotalInput
			output = parseRes.TotalOutput
			if len(parseRes.TurnTimeline) > 0 {
				contextUsed = int(parseRes.TurnTimeline[len(parseRes.TurnTimeline)-1].Input)
			}
		}
	} else if provider == "google" {
		var err error
		input, output, contextUsed, err = m.queryGeminiUsageFromDB(uuid)
		if err != nil {
			log.Debug("CapacityMonitor: failed to query gemini DB usage", "session", inst.Title, "err", err)
		}
	}

	limits.SessionInputTokens = int(input)
	limits.SessionOutputTokens = int(output)
	limits.ContextTokensUsed = contextUsed
	limits.ContextTokensMax = client.ModelContextWindow(inst.Program)

	// 2. Estimate cost.
	limits.EstimatedCostUSD = m.estimateCost(inst.Program, input, output)

	m.mu.Lock()
	m.sessionLimits[inst.Title] = limits
	m.mu.Unlock()

	// 3. Check thresholds.
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
	m.mu.Lock()
	lastWarn := m.lastWarningTime[inst.Title]
	m.mu.Unlock()

	// Rate-limit warnings to once per 5 minutes per session.
	if time.Since(lastWarn) < 5*time.Minute {
		return
	}

	m.mu.Lock()
	m.lastWarningTime[inst.Title] = time.Now()
	m.mu.Unlock()

	// Determine transition target.
	var nextCLI, nextModel string
	for _, target := range m.config.ProviderPriority {
		if strings.ToLower(target.CLI) != strings.ToLower(inst.Program) {
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
		inst.Title, reason, limits.ContextTokensUsed, limits.ContextTokensMax, nextCLI)

	log.Warn("CapacityMonitor: trigger warning", "session", inst.Title, "reason", reason, "next", nextCLI)

	// Publish notification event to frontend.
	m.eventBus.Publish(events.NewNotificationEvent(
		inst.UUID,
		inst.Title,
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
		log.Info("CapacityMonitor: performing auto-transition", "session", inst.Title, "to", nextCLI)
		go func() {
			if err := m.sessionSwitcher.UpdateSessionProgram(context.Background(), inst.Title, nextCLI); err != nil {
				log.Error("CapacityMonitor: auto-transition failed", "session", inst.Title, "to", nextCLI, "err", err)
			}
		}()
	}
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

	db, err := sql.Open("sqlite3", dbPath)
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
