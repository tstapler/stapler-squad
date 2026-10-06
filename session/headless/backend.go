package headless

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Canonical backend names accepted in BackendSettings.
const (
	BackendClaude     = "claude"
	BackendConsolette = "consolette"
	BackendAgy        = "agy"
	BackendGemini     = "gemini"
	BackendOpenCode   = "opencode"
)

// DefaultConsoletteBaseURL is the local free-tier router (Anthropic-compatible).
const DefaultConsoletteBaseURL = "http://127.0.0.1:47000"

// Caps declares what a Backend can honor. A request whose needs exceed a
// backend's Caps never reaches it (see Selector.Resolve).
type Caps struct {
	Resume          bool // continues a prior conversation (--resume)
	SystemPrompt    bool // separate system prompt (not just prepended text)
	ToolRestriction bool // enforces AllowedTools/DisallowedTools/PermissionMode
	WorkDir         bool // runs with a working directory
}

// Missing returns the names of capabilities in need that c lacks.
func (c Caps) Missing(need Caps) []string {
	var m []string
	if need.Resume && !c.Resume {
		m = append(m, "resume")
	}
	if need.SystemPrompt && !c.SystemPrompt {
		m = append(m, "system_prompt")
	}
	if need.ToolRestriction && !c.ToolRestriction {
		m = append(m, "tool_restriction")
	}
	if need.WorkDir && !c.WorkDir {
		m = append(m, "workdir")
	}
	return m
}

// CapsNeeded derives the capabilities a call requires from its inputs.
func CapsNeeded(systemPrompt string, opts CallOptions) Caps {
	return Caps{
		SystemPrompt:    strings.TrimSpace(systemPrompt) != "",
		WorkDir:         opts.WorkDir != "",
		ToolRestriction: opts.AllowedTools != "" || opts.DisallowedTools != "" || opts.PermissionMode != "",
	}
}

// Backend is one LLM provider for headless calls: the existing PoolClient call
// shape plus identity, capability flags and a live availability probe.
type Backend interface {
	PoolClient
	Name() string
	Capabilities() Caps
	// Available re-probes whether the backend can serve a call right now.
	Available() bool
}

// ErrNoBackend is returned when neither the selected backend nor the claude
// fallback can satisfy a call.
var ErrNoBackend = errors.New("headless: no backend can serve this call")

// BackendSettings is the live-editable backend configuration. Zero value means
// "claude everywhere, built-in model handling".
type BackendSettings struct {
	// Default is the backend for features with no override. Empty = claude.
	Default string
	// PerFeature maps a FeatureKey (or the prefix before ':' of a dynamic key
	// such as "gate-custom-check:<id>") to a backend name.
	PerFeature map[string]string
	// ConsoletteBaseURL overrides DefaultConsoletteBaseURL.
	ConsoletteBaseURL string
	// AnthropicBaseURL overrides the HTTP Anthropic client base URL.
	AnthropicBaseURL string
	// ModelMaps maps backend name -> model alias -> backend-specific model ID.
	// Merged over built-in defaults; an explicit empty string value means
	// "let the backend choose its own model".
	ModelMaps map[string]map[string]string
}

// builtinModelMaps are the defaults when no configured map covers an alias.
// Only backends that cannot take Claude model IDs need entries; claude and
// consolette pass names through unchanged unless a map says otherwise.
var builtinModelMaps = map[string]map[string]string{
	BackendGemini:   {"haiku": "gemini-2.5-flash", "sonnet": "gemini-2.5-pro", "opus": "gemini-2.5-pro"},
	BackendAgy:      {},
	BackendOpenCode: {},
}

// passthroughModels lists backends that accept Claude model names as-is.
var passthroughModels = map[string]bool{BackendClaude: true, BackendConsolette: true}

// TranslateModel maps model (a Claude alias or ID) to the ID backend expects.
// Unmapped names pass through only for backends that understand Claude IDs;
// for the rest they become "" so the backend uses its own default rather than
// failing on an unknown model.
func (s BackendSettings) TranslateModel(backend, model string) string {
	if model == "" {
		return ""
	}
	key := strings.ToLower(model)
	for _, maps := range []map[string]map[string]string{s.ModelMaps, builtinModelMaps} {
		if m, ok := maps[backend]; ok {
			if v, ok := m[model]; ok {
				return v
			}
			if v, ok := m[familyAlias(key)]; ok {
				return v
			}
		}
	}
	if passthroughModels[backend] {
		return model
	}
	return ""
}

// familyAlias reduces a concrete Claude ID (claude-sonnet-4-5-...) to its
// family alias (sonnet) so one map entry covers every version.
func familyAlias(lowerModel string) string {
	for _, fam := range []string{"haiku", "sonnet", "opus"} {
		if strings.Contains(lowerModel, fam) {
			return fam
		}
	}
	return lowerModel
}

// featureSettingKey strips a dynamic suffix ("gate-custom-check:abc" -> "gate-custom-check").
func featureSettingKey(key FeatureKey) string {
	s := string(key)
	if i := strings.IndexByte(s, ':'); i > 0 {
		return s[:i]
	}
	return s
}

// KnownBackendNames lists every backend name the selector understands.
func KnownBackendNames() []string {
	return []string{BackendClaude, BackendConsolette, BackendAgy, BackendGemini, BackendOpenCode}
}

// ConsoletteURL returns the effective consolette base URL (no trailing slash).
func (s BackendSettings) ConsoletteURL() string {
	if u := strings.TrimSpace(s.ConsoletteBaseURL); u != "" {
		return strings.TrimRight(u, "/")
	}
	return DefaultConsoletteBaseURL
}

// OverridableFeatureKeys lists the features a user may route to a different
// backend. "gate-custom-check" covers the dynamic "gate-custom-check:<id>" keys.
func OverridableFeatureKeys() []string {
	return []string{
		string(FeatureKeyReview), string(FeatureKeySummarize), string(FeatureKeyAC),
		string(FeatureKeyPRDescription), string(FeatureKeyCommitMessage), string(FeatureKeyCustom),
		string(FeatureKeyAutonomousFix), string(FeatureKeyAutonomousApproval),
		string(FeatureKeyTriage), string(FeatureKeyBacklogIntentParse),
		string(FeatureKeySessionCompletionSummary), string(FeatureKeyHandoffSummary),
		string(FeatureKeySessionTagging), string(FeatureKeyUnfinishedWorkSummary),
		string(FeatureKeyInstanceResume), string(FeatureKeyRulesGeneration), "gate-custom-check",
	}
}

// Validate rejects unknown backend names and feature keys.
func (s BackendSettings) Validate() error {
	known := map[string]bool{}
	for _, n := range KnownBackendNames() {
		known[n] = true
	}
	if s.Default != "" && !known[s.Default] {
		return fmt.Errorf("unknown default backend %q", s.Default)
	}
	for name, raw := range map[string]string{"consolette": s.ConsoletteBaseURL, "anthropic": s.AnthropicBaseURL} {
		if err := validateBaseURL(raw); err != nil {
			return fmt.Errorf("%s base URL: %w", name, err)
		}
	}
	features := map[string]bool{}
	for _, f := range OverridableFeatureKeys() {
		features[f] = true
	}
	for f, b := range s.PerFeature {
		if !features[f] {
			return fmt.Errorf("unknown feature key %q", f)
		}
		if !known[b] {
			return fmt.Errorf("unknown backend %q for feature %q", b, f)
		}
	}
	for b := range s.ModelMaps {
		if !known[b] {
			return fmt.Errorf("model map for unknown backend %q", b)
		}
	}
	return nil
}

// validateBaseURL accepts empty (use the default) or an absolute http(s) URL with a host.
func validateBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%q must be an absolute http(s) URL", raw)
	}
	return nil
}
