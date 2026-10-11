package tokens

import "time"

// ParseResult holds aggregated token data extracted from one JSONL file.
// Privacy: only tool names, skill names (short strings), and token counts.
// Message content is never stored.
type ParseResult struct {
	SessionUUID  string
	ProjectPath  string   // decoded from project dir name (best-effort)
	PrimaryModel string   // most-used model in this session
	Models       []string // all distinct models observed

	TotalInput    int64
	TotalOutput   int64
	CacheCreation int64
	CacheRead     int64
	MessageCount  int

	TurnTimeline     []TurnStats // per-assistant-message stats for burn rate chart
	ToolUsage        map[string]ToolTokenStats
	SkillActivations []SkillActivation
	// ContextHealth holds derived counts over the trailing assistant turns; no message content.
	ContextHealth ContextHealthSignals
	CompactEvents []CompactEvent // compaction boundaries in transcript order

	ParsedAt    time.Time
	FileModTime time.Time // used for cache invalidation
}

// TurnStats is per-assistant-message token data (for timeline/burn-rate chart).
type TurnStats struct {
	Timestamp     time.Time
	Model         string
	Input         int64
	Output        int64
	CacheCreation int64
	CacheRead     int64
	ToolNames     []string // tool_use block names in this message
}

// ContextTokens is the number of tokens occupying the context window on this
// turn: everything sent as prompt, cached or not.
func (t TurnStats) ContextTokens() int64 {
	return t.Input + t.CacheRead + t.CacheCreation
}

// CompactEvent records one conversation compaction (/compact or auto-compact).
type CompactEvent struct {
	Timestamp time.Time
	Trigger   string // "manual" or "auto"; empty when the transcript omits it
	// TurnIndex is the number of timeline turns preceding the boundary, so
	// TurnTimeline[TurnIndex] is the first post-compaction turn when present.
	TurnIndex    int
	TokensBefore int64
	TokensAfter  int64
}

// TokensFreed is the context reduction achieved by the compaction, never negative.
func (c CompactEvent) TokensFreed() int64 {
	if c.TokensBefore > c.TokensAfter {
		return c.TokensBefore - c.TokensAfter
	}
	return 0
}

// ToolTokenStats aggregates attribution for one tool name.
// Token attribution is message-level (not per-tool-call); CallCount is exact.
type ToolTokenStats struct {
	ToolName  string
	CallCount int
	// MCPServer is non-empty when tool follows mcp__<server>__<tool> pattern.
	MCPServer string
	// CostUsd is populated by AttributeToolCosts (tool-type-level session sum,
	// per ADR-001), not by the parser. Zero until a caller explicitly attributes
	// costs onto this struct.
	CostUsd float64
}

// SkillActivation records a detected skill or command invocation.
type SkillActivation struct {
	Name      string // e.g. "code-review", "/plan:feature"
	TurnIndex int    // which human turn triggered it
	IsCommand bool   // true for /command, false for skill name
}

// TokenStoreReader is the read-only interface InsightsService needs from a TokenStore.
// Defined as an interface so test fakes can be injected without constructing a real store.
type TokenStoreReader interface {
	GetAll() []*ParseResult
	GetByUUID(uuid string) *ParseResult
	IsLoading() bool
	Subscribe() <-chan *ParseResult
	Unsubscribe(ch <-chan *ParseResult)
}

// ModelPricing holds per-model token prices in USD per million tokens.
type ModelPricing struct {
	ModelFamily        string  // normalized key, e.g. "claude-sonnet-4"
	InputPricePerMTok  float64 // USD per 1M input tokens
	OutputPricePerMTok float64 // USD per 1M output tokens
	CacheWritePerMTok  float64 // USD per 1M cache-write tokens
	CacheReadPerMTok   float64 // USD per 1M cache-read tokens
	EffectiveDate      string  // ISO date of last price update
}

// PricingTable maps normalized model family names to pricing.
// Hardcoded defaults; overridable via config JSON.
type PricingTable struct {
	Prices     map[string]ModelPricing
	LoadedAt   time.Time
	ConfigPath string // empty = hardcoded only
}
