package headless

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/executor"
)

// poolBackend adapts any PoolClient (claude Pool, GeminiCaller) to Backend.
type poolBackend struct {
	PoolClient
	name  string
	caps  Caps
	avail func() bool
}

func (b *poolBackend) Name() string       { return b.name }
func (b *poolBackend) Capabilities() Caps { return b.caps }
func (b *poolBackend) Available() bool    { return b.avail == nil || b.avail() }

// ClaudeCaps: the claude CLI honors every capability.
var ClaudeCaps = Caps{Resume: true, SystemPrompt: true, ToolRestriction: true, WorkDir: true}

// NewClaudeBackend wraps the claude pool. Returns nil for a nil pool so callers
// never register a typed-nil interface.
func NewClaudeBackend(pool *Pool) Backend {
	if pool == nil {
		return nil
	}
	return &poolBackend{PoolClient: pool, name: BackendClaude, caps: ClaudeCaps}
}

// NewGeminiBackend wraps a GeminiCaller. Gemini has no separate system prompt
// flag (it is prepended), no tool restriction and no resume.
func NewGeminiBackend(g *GeminiCaller) Backend {
	if g == nil {
		return nil
	}
	return &poolBackend{PoolClient: g, name: BackendGemini, caps: Caps{WorkDir: true}, avail: g.Available}
}

// --- consolette -----------------------------------------------------------

const consoletteProbeTTL = 10 * time.Second

// ConsoletteBackend runs the claude CLI against an Anthropic-compatible router
// by setting ANTHROPIC_BASE_URL. The URL is read from live settings on every
// call; one Pool is cached per URL.
type ConsoletteBackend struct {
	settings func() BackendSettings
	newPool  func(baseURL string) (PoolClient, error)
	probe    func(ctx context.Context, baseURL string) bool
	now      func() time.Time

	mu       sync.Mutex
	pools    map[string]PoolClient
	probeURL string
	probeAt  time.Time
	probeOK  bool
}

var _ Backend = (*ConsoletteBackend)(nil)

// NewConsoletteBackend builds the production consolette backend: pools are
// claude CLI pools with ANTHROPIC_BASE_URL set; availability is an HTTP reachability probe.
func NewConsoletteBackend(settings func() BackendSettings, cfg PoolConfig) *ConsoletteBackend {
	return newConsoletteBackend(settings, func(baseURL string) (PoolClient, error) {
		return NewConsolettePool(cfg, baseURL)
	}, httpReachable)
}

func newConsoletteBackend(settings func() BackendSettings, newPool func(string) (PoolClient, error), probe func(context.Context, string) bool) *ConsoletteBackend {
	return &ConsoletteBackend{settings: settings, newPool: newPool, probe: probe, now: time.Now, pools: map[string]PoolClient{}}
}

// NewConsolettePool builds a claude Pool whose subprocesses talk to baseURL.
func NewConsolettePool(cfg PoolConfig, baseURL string) (*Pool, error) {
	homeDir, _ := os.UserHomeDir()
	bin, err := findClaudeBinary(exec.LookPath, homeDir, claudeFallbackDirs)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrClaudeNotFound, err)
	}
	applyDefaults(&cfg)
	runner := &ProcessRunner{claudeBin: bin, extraEnv: []string{"ANTHROPIC_BASE_URL=" + baseURL}}
	return newPoolWithRunner(cfg, runner, bin), nil
}

func (c *ConsoletteBackend) Name() string       { return BackendConsolette }
func (c *ConsoletteBackend) Capabilities() Caps { return ClaudeCaps }

func (c *ConsoletteBackend) baseURL() string { return c.settings().ConsoletteURL() }

// Available reports whether the router answers at the configured base URL
// (short-TTL cached so a hot path doesn't dial on every call).
func (c *ConsoletteBackend) Available() bool {
	url := c.baseURL()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.probeURL == url && c.now().Sub(c.probeAt) < consoletteProbeTTL {
		return c.probeOK
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	c.probeURL, c.probeAt, c.probeOK = url, c.now(), c.probe(ctx, url)
	return c.probeOK
}

// CallBlocking implements PoolClient against the pool for the current base URL.
func (c *ConsoletteBackend) CallBlocking(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions, sink CostSink) (string, error) {
	url := c.baseURL()
	c.mu.Lock()
	pool, ok := c.pools[url]
	if !ok {
		var err error
		if pool, err = c.newPool(url); err != nil {
			c.mu.Unlock()
			return "", err
		}
		c.pools[url] = pool
	}
	c.mu.Unlock()
	// Router-served usage is not billed against the Anthropic quota; report unpriced.
	return pool.CallBlocking(ctx, key, systemPrompt, userPrompt, opts, func(float64, bool) {
		if sink != nil {
			sink(0, false)
		}
	})
}

// httpReachable treats any HTTP response (even 404) as "router is up".
func httpReachable(ctx context.Context, baseURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

// --- generic CLI backends (agy, opencode) ----------------------------------

// cliRunFunc runs bin with args (prompt on stdin when non-nil) in dir.
type cliRunFunc func(ctx context.Context, bin string, args []string, stdin io.Reader, dir string) ([]byte, error)

// CLIBackendSpec describes a one-shot CLI agent. Mirrors services.CLIAgentSpec
// (server/services/cli_ai_client.go) for the agents the selector supports.
type CLIBackendSpec struct {
	Name        string
	Binary      string
	Args        []string
	Separator   string
	PromptAsArg bool
}

// KnownCLIBackendSpecs are the agy and opencode invocations (verified against
// the AIClient chain's CLIAgentSpec table).
var KnownCLIBackendSpecs = []CLIBackendSpec{
	{Name: BackendAgy, Binary: "agy", Args: []string{"--print"}, Separator: "\n\n---\n\n", PromptAsArg: true},
	{Name: BackendOpenCode, Binary: "opencode", Args: []string{"run"}, Separator: "\n\n", PromptAsArg: true},
}

// CLIBackend serves one-shot text calls through a local agent CLI. It has no
// resume, tool restriction or separate system prompt; output is unpriced.
type CLIBackend struct {
	spec     CLIBackendSpec
	lookPath func(string) (string, error)
	run      cliRunFunc
}

var _ Backend = (*CLIBackend)(nil)

// NewCLIBackend builds a CLIBackend for spec using real exec.
func NewCLIBackend(spec CLIBackendSpec) *CLIBackend {
	return &CLIBackend{spec: spec, lookPath: exec.LookPath, run: runCLI}
}

func (c *CLIBackend) Name() string       { return c.spec.Name }
func (c *CLIBackend) Capabilities() Caps { return Caps{WorkDir: true} }
func (c *CLIBackend) Available() bool    { _, err := c.lookPath(c.spec.Binary); return err == nil }

// CallBlocking implements PoolClient.
func (c *CLIBackend) CallBlocking(ctx context.Context, _ FeatureKey, systemPrompt, userPrompt string, opts CallOptions, sink CostSink) (string, error) {
	bin, err := c.lookPath(c.spec.Binary)
	if err != nil {
		return "", fmt.Errorf("%s backend: %w", c.spec.Name, err)
	}
	if err := validateGeminiWorkDir(opts.WorkDir); err != nil {
		return "", err
	}
	prompt := userPrompt
	if systemPrompt != "" {
		prompt = systemPrompt + c.spec.Separator + userPrompt
	}
	args := append([]string(nil), c.spec.Args...)
	var stdin io.Reader
	if c.spec.PromptAsArg {
		args = append(args, prompt)
	} else {
		stdin = strings.NewReader(prompt)
	}
	out, err := c.run(ctx, bin, args, stdin, opts.WorkDir)
	if err != nil {
		return "", fmt.Errorf("%s backend: %w", c.spec.Name, err)
	}
	if sink != nil {
		sink(0, false)
	}
	return strings.TrimSpace(string(out)), nil
}

func runCLI(ctx context.Context, bin string, args []string, stdin io.Reader, dir string) ([]byte, error) {
	opts := []executor.Option{executor.WithTimeout(5 * time.Minute)}
	if stdin != nil {
		opts = append(opts, executor.WithStdin(stdin))
	}
	if dir != "" {
		opts = append(opts, executor.WithDir(dir))
	}
	return executor.New(ctx, bin, args, opts...).Output()
}
