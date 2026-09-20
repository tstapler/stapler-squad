package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// geminiAvailabilityTTL bounds how long a cached Available() result is trusted
// before GeminiCaller re-probes the binary's presence via lookPath. Short
// enough to notice an uninstall within a bounded window (resolves
// adversarial-review Blocker #3), long enough that a hot call path never pays
// for a fresh exec.LookPath on every single call.
const geminiAvailabilityTTL = 30 * time.Second

// geminiRunner abstracts how the gemini binary is invoked, mirroring
// ClaudeRunner's real/fake split (see fake_runner.go) so tests can stub the
// subprocess without touching PATH or writing a real script to disk.
type geminiRunner interface {
	Run(ctx context.Context, binPath string, args []string, workDir string) (stdout []byte, err error)
}

// realGeminiRunner shells out to the real gemini binary. args are built
// entirely from internal fixed flags plus opts.Model/the combined prompt —
// never from unsanitized user input passed through a shell — so no shell
// metacharacter risk exists here (there is no shell in the exec path at all).
type realGeminiRunner struct{}

func (realGeminiRunner) Run(ctx context.Context, binPath string, args []string, workDir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binPath, args...) //nolint:gosec // binPath/args are internally constructed, not shell-interpreted
	cmd.Dir = workDir
	return cmd.Output()
}

// geminiTokenUsage mirrors one model's "tokens" object in `gemini -p
// --output-format json`'s stats.models[<family>] shape.
type geminiTokenUsage struct {
	Prompt     int64 `json:"prompt"`
	Candidates int64 `json:"candidates"`
	Total      int64 `json:"total"`
	Cached     int64 `json:"cached"`
}

type geminiModelStats struct {
	Tokens geminiTokenUsage `json:"tokens"`
}

type geminiStats struct {
	Models map[string]geminiModelStats `json:"models"`
}

type geminiErrorObj struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// geminiResult is the documented JSON schema of `gemini -p ...
// --output-format json`'s stdout.
type geminiResult struct {
	Response string          `json:"response"`
	Stats    geminiStats     `json:"stats"`
	Error    *geminiErrorObj `json:"error"`
}

// GeminiCaller implements headless.PoolClient by shelling out to the `gemini`
// CLI. Unlike Pool (Claude), it has no session/history reuse in v1 — every
// call is a fresh, self-contained subprocess — and it bounds its own
// concurrent subprocess count independently of Pool's semaphore, since it is
// not built on Pool and inherits none of that pool's concurrency bound
// (adversarial-review Blocker #2).
type GeminiCaller struct {
	binPath        string
	pricing        *tokens.PricingTable
	concurrencySem chan struct{}
	runner         geminiRunner

	// lookPath/now are overridden by tests for Available()'s TTL behavior;
	// production callers always get exec.LookPath/time.Now via NewGeminiCaller.
	lookPath func(string) (string, error)
	now      func() time.Time

	availMu         sync.Mutex
	availLastCheck  time.Time
	availLastResult bool

	unpricedMu             sync.Mutex
	loggedUnpricedFamilies map[string]bool
}

// compile-time check that *GeminiCaller satisfies PoolClient.
var _ PoolClient = (*GeminiCaller)(nil)

// NewGeminiCaller constructs a GeminiCaller. maxConcurrent <= 0 defaults to
// defaultMaxConcurrent (5) — the same literal already hardcoded for Claude's
// Pool at server/dependencies.go's headless.PoolConfig{MaxConcurrentSessions:
// 5} — so both pools share one operator-legible concurrency expectation even
// though they are separate semaphores.
func NewGeminiCaller(binPath string, pricing *tokens.PricingTable, maxConcurrent int) *GeminiCaller {
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrent
	}
	return &GeminiCaller{
		binPath:                binPath,
		pricing:                pricing,
		concurrencySem:         make(chan struct{}, maxConcurrent),
		runner:                 realGeminiRunner{},
		lookPath:               exec.LookPath,
		now:                    time.Now,
		loggedUnpricedFamilies: make(map[string]bool),
	}
}

// combinedGeminiPrompt joins systemPrompt/userPrompt the way `gemini -p`
// expects a single prompt argument (Gemini's CLI has no separate
// --system-prompt flag the way claude -p does).
func combinedGeminiPrompt(systemPrompt, userPrompt string) string {
	if systemPrompt == "" {
		return userPrompt
	}
	return systemPrompt + "\n\n" + userPrompt
}

// warnIgnoredClaudeOnlyOptions logs a Warn naming any Claude-CLI-flag-shaped
// CallOptions field that GeminiCaller cannot honor (`gemini -p` has no
// --allowedTools/--permission-mode/--disallowedTools equivalent) — resolves
// architecture-review Concern (Liskov/ISP mismatch). The call still proceeds
// using only WorkDir/Model: dropping an unenforceable restriction is a
// visible, logged degradation, never a silent one, and never an error (the
// call still has a valid, if less-restricted, shape without it).
func warnIgnoredClaudeOnlyOptions(opts CallOptions) {
	var ignored []string
	if opts.AllowedTools != "" {
		ignored = append(ignored, "AllowedTools")
	}
	if opts.PermissionMode != "" {
		ignored = append(ignored, "PermissionMode")
	}
	if opts.DisallowedTools != "" {
		ignored = append(ignored, "DisallowedTools")
	}
	if len(ignored) > 0 {
		log.Warn("gemini caller: ignoring claude-only CallOptions fields", "fields", ignored)
	}
}

// validateGeminiWorkDir mirrors TriggerTriage's eager filepath.IsAbs+os.Stat
// guard (BUG-062 precedent, server/services/backlog_service_trigger_triage.go)
// rather than rediscovering it: os/exec.Cmd.Dir has a well-documented quirk
// where a non-existent Dir makes the resulting fork/exec error name the
// EXECUTABLE path, not the directory, which looks exactly like the gemini
// binary is missing even when the real problem is a bad working directory.
func validateGeminiWorkDir(workDir string) error {
	if workDir == "" {
		return nil
	}
	if !filepath.IsAbs(workDir) {
		return fmt.Errorf("gemini caller: work dir %q is not an absolute path", workDir)
	}
	if fi, statErr := os.Stat(workDir); statErr != nil || !fi.IsDir() {
		return fmt.Errorf("gemini caller: work dir %q does not exist or is not a directory", workDir)
	}
	return nil
}

// acquireGeminiSlot bounds the queue-wait-then-fail-fast on g.concurrencySem,
// mirroring Pool.call()'s own maxQueueWait/ErrPoolSaturated discipline
// (BUG-093 precedent) — a call stuck behind other concurrent gemini
// subprocesses fails fast with a distinct, recognizable error instead of
// spawning an unbounded Nth subprocess or hanging indefinitely on the
// caller's own (much longer) budget. release must be called once the slot is
// no longer needed; it is nil when err is non-nil.
func (g *GeminiCaller) acquireGeminiSlot(ctx context.Context) (release func(), err error) {
	queueCtx, queueCancel := context.WithTimeout(ctx, maxQueueWait)
	defer queueCancel()
	select {
	case g.concurrencySem <- struct{}{}:
		return func() { <-g.concurrencySem }, nil
	case <-queueCtx.Done():
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("gemini caller: %w", ErrPoolSaturated)
	}
}

// runGemini invokes the subprocess and parses its JSON response, returning a
// non-nil error for a subprocess start/exit failure, malformed JSON, or an
// `error` object in the response — never a silently-successful empty result.
func (g *GeminiCaller) runGemini(ctx context.Context, systemPrompt, userPrompt string, opts CallOptions) (geminiResult, error) {
	args := []string{"-p", combinedGeminiPrompt(systemPrompt, userPrompt), "--output-format", "json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	stdout, runErr := g.runner.Run(ctx, g.binPath, args, opts.WorkDir)
	if runErr != nil && len(stdout) == 0 {
		return geminiResult{}, fmt.Errorf("gemini caller: subprocess failed: %w", runErr)
	}

	var result geminiResult
	if jsonErr := json.Unmarshal(stdout, &result); jsonErr != nil {
		return geminiResult{}, fmt.Errorf("gemini caller: malformed JSON response: %w", jsonErr)
	}
	if result.Error != nil {
		return geminiResult{}, fmt.Errorf("gemini reported error: %s", result.Error.Message)
	}
	return result, nil
}

// CallBlocking implements PoolClient. key is accepted for interface
// compatibility but unused: GeminiCaller has no session/history reuse in v1
// (see the Unresolved Questions in plan.md) — every call is a fresh,
// self-contained subprocess.
func (g *GeminiCaller) CallBlocking(ctx context.Context, _ FeatureKey, systemPrompt, userPrompt string, opts CallOptions, sink CostSink) (string, error) {
	if err := validateGeminiWorkDir(opts.WorkDir); err != nil {
		return "", err
	}
	warnIgnoredClaudeOnlyOptions(opts)

	release, err := g.acquireGeminiSlot(ctx)
	if err != nil {
		return "", err
	}
	defer release()

	result, err := g.runGemini(ctx, systemPrompt, userPrompt, opts)
	if err != nil {
		return "", err
	}

	cost, priced := g.computeCost(result.Stats.Models)
	if sink != nil {
		sink(cost, priced)
	}
	return result.Response, nil
}

// computeCost sums the priced portion of models' token usage against the
// pricing table. priced is false whenever ANY model family in the response is
// unpriced — a mixed response is still marked unpriced overall, since the
// reported total would otherwise understate the true cost (Task 3.1.1d).
func (g *GeminiCaller) computeCost(models map[string]geminiModelStats) (cost float64, priced bool) {
	var unpricedFamilies []string
	for family, stats := range models {
		pricing, ok := g.pricing.LookupByModel(family)
		if !ok {
			unpricedFamilies = append(unpricedFamilies, family)
			continue
		}
		cost += float64(stats.Tokens.Prompt) / 1_000_000.0 * pricing.InputPricePerMTok
		cost += float64(stats.Tokens.Candidates) / 1_000_000.0 * pricing.OutputPricePerMTok
	}
	if len(unpricedFamilies) > 0 {
		sort.Strings(unpricedFamilies)
		g.warnUnpricedFamilies(unpricedFamilies)
	}
	return cost, len(unpricedFamilies) == 0
}

// warnUnpricedFamilies logs a Warn for each family not previously logged by
// this GeminiCaller, then marks it logged — deduped across calls so a given
// unpriced family produces exactly one log line for the life of this caller,
// not one per request. Mirrors InsightsService.warnNewUnpricedFamilies'
// per-process-lifetime dedup convention (server/services/insights_service.go),
// scoped to this instance rather than a package-level singleton.
func (g *GeminiCaller) warnUnpricedFamilies(families []string) {
	g.unpricedMu.Lock()
	defer g.unpricedMu.Unlock()
	for _, family := range families {
		if !g.loggedUnpricedFamilies[family] {
			g.loggedUnpricedFamilies[family] = true
			log.Warn("gemini caller: unpriced model family observed", "family", family)
		}
	}
}

// Available reports whether the gemini binary can currently be found,
// short-TTL-cached (geminiAvailabilityTTL) so a hot call path doesn't pay for
// a fresh lookPath syscall on every call, but bounded fresh enough to catch
// an uninstall within a bounded window (resolves adversarial-review Blocker
// #3, consumed by Story 2.3.1's resolveHeadlessCaller re-probe). Claude's
// *Pool intentionally does not implement this: it has no equivalent
// "goes missing mid-run" failure mode the way an optional, less-central
// adapter like Gemini does — the claude binary is a hard runtime dependency
// already assumed present everywhere Pool is used.
func (g *GeminiCaller) Available() bool {
	g.availMu.Lock()
	defer g.availMu.Unlock()
	if g.now().Sub(g.availLastCheck) < geminiAvailabilityTTL {
		return g.availLastResult
	}
	_, err := g.lookPath(g.binPath)
	g.availLastResult = err == nil
	g.availLastCheck = g.now()
	return g.availLastResult
}
