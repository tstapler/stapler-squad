package headless

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// CallOptions configures an individual pool call with overrides.
type CallOptions struct {
	// WorkDir sets the subprocess working directory (for git operations).
	WorkDir string
	// Model overrides the pool's DefaultModel for this call only.
	Model string
	// TimeoutSecs is unused by Pool directly — callers wrap ctx with WithTimeout.
	TimeoutSecs int
}

// firstCallJSONResult is the JSON schema returned by claude -p --output-format json.
type firstCallJSONResult struct {
	SessionID string  `json:"session_id"`
	Result    string  `json:"result"`
	IsError   bool    `json:"is_error"`
	CostUSD   float64 `json:"cost_usd"`
}

// claudeFallbackDirs lists standard install locations to check for the claude
// binary if it's not found via the process's PATH. Checked in order; the
// first executable match wins.
var claudeFallbackDirs = []string{
	"/usr/local/bin",
	"/opt/homebrew/bin",
}

// findClaudeBinary locates the claude binary, trying lookPath (the process's
// PATH) first and falling back to well-known install locations relative to
// homeDir if that fails.
//
// This exists because a service manager's baked-in PATH can go stale
// independently of the interactive shell's PATH that a developer actually
// uses: systemd user units snapshot PATH at install time with no fallback
// (unlike the macOS LaunchAgent plist, which explicitly appends Homebrew and
// system paths — see scripts/install-service.sh). If claude is later
// reinstalled to a new location (nvm/asdf switch, a fresh `pip install
// --user`/npm global install) without a subsequent `make install-service`,
// the bare exec.LookPath("claude") call below would otherwise fail silently:
// NewPool returns ErrClaudeNotFound, the headless pool is left nil (only a
// log warning — see server/dependencies.go), and backlog triage quietly
// no-ops with no user-visible error.
// fallbackDirs takes an explicit parameter (rather than reading
// claudeFallbackDirs directly) so tests can inject a controlled set of
// candidate directories without mutating package-level state.
func findClaudeBinary(lookPath func(string) (string, error), homeDir string, fallbackDirs []string) (string, error) {
	if bin, err := lookPath("claude"); err == nil {
		return bin, nil
	}

	dirs := fallbackDirs
	if homeDir != "" {
		dirs = append([]string{filepath.Join(homeDir, ".local", "bin")}, dirs...)
	}
	for _, dir := range dirs {
		candidate := filepath.Join(dir, "claude")
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("claude not found in PATH or fallback locations %v", dirs)
}

// NewPool constructs a Pool by looking up the claude binary in PATH, falling
// back to well-known install locations if PATH lookup fails.
// Returns ErrClaudeNotFound if the binary is not found anywhere.
func NewPool(cfg PoolConfig) (*Pool, error) {
	homeDir, _ := os.UserHomeDir()
	bin, err := findClaudeBinary(exec.LookPath, homeDir, claudeFallbackDirs)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrClaudeNotFound, err)
	}
	applyDefaults(&cfg)
	runner := &ProcessRunner{claudeBin: bin}
	return newPoolWithRunner(cfg, runner, bin), nil
}

// NewPoolWithRunner constructs a Pool with a custom runner (no PATH lookup).
// Used in tests to inject a FakeRunner.
func NewPoolWithRunner(cfg PoolConfig, runner ClaudeRunner) *Pool {
	applyDefaults(&cfg)
	return newPoolWithRunner(cfg, runner, "claude")
}

func applyDefaults(cfg *PoolConfig) {
	if cfg.MaxCallsPerSession <= 0 {
		cfg.MaxCallsPerSession = defaultMaxCalls
	}
	if cfg.MaxConcurrentSessions <= 0 {
		cfg.MaxConcurrentSessions = defaultMaxConcurrent
	}
}

func newPoolWithRunner(cfg PoolConfig, runner ClaudeRunner, claudeBin string) *Pool {
	return &Pool{
		claudeBin:      claudeBin,
		cfg:            cfg,
		runner:         runner,
		sessions:       make(map[FeatureKey]*sessionState),
		keyMu:          make(map[FeatureKey]*sync.Mutex),
		concurrencySem: make(chan struct{}, cfg.MaxConcurrentSessions),
	}
}

// acquireSession reads current session state for key and builds the subprocess args
// (flags only — the user prompt is passed via stdin by the caller, not in args).
// It increments callCount under lock before returning.
// Returns isFirstCall=true when this call should use --output-format json.
//
// IMPORTANT: the per-key mutex is held only long enough to read/write state —
// it is NOT held during subprocess execution to avoid deadlocks.
func (p *Pool) acquireSession(key FeatureKey, systemPrompt, model string) (isFirstCall bool, args []string) {
	p.mu.Lock()
	keyMu := p.acquireKeyMu(key)
	if _, ok := p.sessions[key]; !ok {
		p.sessions[key] = &sessionState{}
	}
	p.mu.Unlock()

	keyMu.Lock()
	defer keyMu.Unlock()

	p.mu.Lock()
	state := p.sessions[key]

	// Determine if we need a fresh session (first call or rotation due to errors/max calls).
	needsRotation := state.sessionID == "" ||
		state.callCount >= p.cfg.MaxCallsPerSession ||
		state.consecutiveErrors >= maxConsecutiveErrors

	if needsRotation && state.sessionID != "" {
		// Reset the state in place.
		state.sessionID = ""
		state.callCount = 0
		state.consecutiveErrors = 0
	}

	sessionID := state.sessionID
	state.callCount++
	p.mu.Unlock()

	// Effective model: per-call override > pool default.
	effectiveModel := model
	if effectiveModel == "" {
		effectiveModel = p.cfg.DefaultModel
	}

	if sessionID == "" {
		// First call: JSON output to capture session_id.
		isFirstCall = true
		args = []string{"-p", "--output-format", "json", "--system-prompt", systemPrompt, "--exclude-dynamic-system-prompt-sections"}
		if effectiveModel != "" {
			args = append(args, "--model", effectiveModel)
		}
	} else {
		// Resumed call: plain output (line-at-a-time streaming).
		isFirstCall = false
		args = []string{"-p", "--resume", sessionID, "--exclude-dynamic-system-prompt-sections"}
	}

	return isFirstCall, args
}

// storeSessionID stores the session ID captured from a first-call JSON response.
func (p *Pool) storeSessionID(key FeatureKey, sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if state, ok := p.sessions[key]; ok {
		state.sessionID = sessionID
		state.consecutiveErrors = 0
	}
}

// recordSuccess resets the consecutive error counter for key.
func (p *Pool) recordSuccess(key FeatureKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if state, ok := p.sessions[key]; ok {
		state.consecutiveErrors = 0
	}
}

// recordError increments the consecutive error counter for key.
// Returns true if the circuit breaker threshold has been reached.
func (p *Pool) recordError(key FeatureKey) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if state, ok := p.sessions[key]; ok {
		state.consecutiveErrors++
		return state.consecutiveErrors >= maxConsecutiveErrors
	}
	return false
}

// decrementCallCount reverses a premature callCount increment for key.
// Called when the semaphore acquire is cancelled or runner.Run fails before any
// output is produced, so the call slot is not consumed.
func (p *Pool) decrementCallCount(key FeatureKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if state, ok := p.sessions[key]; ok && state.callCount > 0 {
		state.callCount--
	}
}

// Call starts a streaming headless LLM call for the given feature key.
// It returns a channel that receives StreamChunk values. The channel is closed
// when the subprocess exits (or the context is cancelled).
//
// The caller should drain the channel until Done=true or Err!=nil.
func (p *Pool) Call(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string) (<-chan StreamChunk, error) {
	return p.call(ctx, key, systemPrompt, userPrompt, p.cfg.DefaultModel, p.runner)
}

// call is the internal implementation shared by Call and CallWithOptions.
// model is the effective model override; runner is the subprocess launcher to use.
func (p *Pool) call(ctx context.Context, key FeatureKey, systemPrompt, userPrompt, model string, runner ClaudeRunner) (<-chan StreamChunk, error) {
	isFirstCall, args := p.acquireSession(key, systemPrompt, model)

	// Pass the user prompt via stdin so it does not appear in /proc/<pid>/cmdline.
	// This prevents leaking diff content (which may contain sensitive paths or tokens)
	// to any process listing tool on the host.
	stdinReader := strings.NewReader(userPrompt)

	ch := make(chan StreamChunk, 16)

	// Acquire concurrency semaphore with context awareness so callers are not
	// permanently blocked when ctx is cancelled while the semaphore is full.
	select {
	case p.concurrencySem <- struct{}{}:
	case <-ctx.Done():
		p.decrementCallCount(key)
		close(ch)
		return ch, ctx.Err()
	}

	stdout, stop, err := runner.Run(ctx, args, stdinReader)
	if err != nil {
		<-p.concurrencySem // release on startup failure
		p.decrementCallCount(key)
		if tripBreaker := p.recordError(key); tripBreaker {
			p.rotateSession(key)
		}
		close(ch)
		return ch, fmt.Errorf("headless runner start: %w", err)
	}

	go func() {
		defer close(ch)
		defer func() { <-p.concurrencySem }()
		defer func() { _ = stop() }()

		send := func(chunk StreamChunk) bool {
			select {
			case ch <- chunk:
				return true
			case <-ctx.Done():
				return false
			}
		}

		if isFirstCall {
			// First call: accumulate all output in a helper goroutine so that ctx
			// cancellation can terminate the subprocess and unblock the read.
			readDone := make(chan struct{})
			var data []byte
			var readErr error
			go func() {
				defer close(readDone)
				data, readErr = io.ReadAll(stdout)
			}()

			select {
			case <-readDone:
				// Normal completion — fall through to JSON parsing.
			case <-ctx.Done():
				// Kill subprocess to unblock the ReadAll goroutine, then wait.
				_ = stop()
				<-readDone
				return
			}

			if readErr != nil && !errors.Is(readErr, io.EOF) {
				if tripBreaker := p.recordError(key); tripBreaker {
					p.rotateSession(key)
				}
				send(StreamChunk{Err: readErr, Done: true})
				return
			}

			// Guard against a ctx cancellation race after ReadAll completes.
			if ctx.Err() != nil {
				return
			}

			var result firstCallJSONResult
			if jsonErr := json.Unmarshal(data, &result); jsonErr != nil {
				// Not valid JSON: treat the whole output as plain text.
				text := strings.TrimSpace(string(data))
				if text != "" {
					if !send(StreamChunk{Text: text}) {
						return
					}
				}
				if tripBreaker := p.recordError(key); tripBreaker {
					p.rotateSession(key)
				}
				send(StreamChunk{Err: fmt.Errorf("first-call JSON parse: %w", jsonErr), Done: true})
				return
			}

			// claude -p sets is_error=true when the LLM returns an error response.
			if result.IsError {
				if tripBreaker := p.recordError(key); tripBreaker {
					p.rotateSession(key)
				}
				send(StreamChunk{Err: fmt.Errorf("claude reported error: %s", strings.TrimSpace(result.Result)), Done: true})
				return
			}

			// Store the session ID for future resume calls.
			if result.SessionID != "" {
				p.storeSessionID(key, result.SessionID)
			}
			p.recordSuccess(key)
			if result.Result != "" {
				if !send(StreamChunk{Text: result.Result}) {
					return
				}
			}
			send(StreamChunk{Done: true, CostUSD: result.CostUSD})
			return
		}

		// Resumed call: stream line by line.
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}
			line := scanner.Text()
			if !send(StreamChunk{Text: line}) {
				return
			}
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
			if tripBreaker := p.recordError(key); tripBreaker {
				p.rotateSession(key)
			}
			send(StreamChunk{Err: err, Done: true})
			return
		}
		p.recordSuccess(key)
		send(StreamChunk{Done: true})
	}()

	return ch, nil
}

// CallWithOptions is like Call but allows overriding model and working directory.
//
// When opts.WorkDir is non-empty a fresh one-shot subprocess is used (bypassing
// session caching, which is invalid across directory changes). The parent pool's
// concurrency semaphore is still acquired so WorkDir calls count against the
// pool-level cap.
//
// When opts.WorkDir is empty, opts.Model is forwarded to the pool's acquireSession
// so the correct model is used for the first-call (session-initialisation) request.
func (p *Pool) CallWithOptions(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions) (<-chan StreamChunk, error) {
	if opts.WorkDir != "" {
		pr, ok := p.runner.(*ProcessRunner)
		if !ok {
			ch := make(chan StreamChunk)
			close(ch)
			return ch, fmt.Errorf("CallWithOptions: WorkDir requires a ProcessRunner; got %T", p.runner)
		}

		// Acquire parent semaphore so WorkDir calls count toward the overall cap.
		select {
		case p.concurrencySem <- struct{}{}:
		case <-ctx.Done():
			ch := make(chan StreamChunk)
			close(ch)
			return ch, ctx.Err()
		}

		dirRunner := pr.WithWorkDir(opts.WorkDir)
		oneShot := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 1, DefaultModel: opts.Model}, dirRunner)
		innerCh, err := oneShot.Call(ctx, key, systemPrompt, userPrompt)
		if err != nil {
			<-p.concurrencySem
			return innerCh, err
		}

		// Proxy the inner channel, releasing the parent semaphore when done.
		outCh := make(chan StreamChunk, 16)
		go func() {
			defer close(outCh)
			defer func() { <-p.concurrencySem }()
			for chunk := range innerCh {
				select {
				case outCh <- chunk:
				case <-ctx.Done():
					return
				}
			}
		}()
		return outCh, nil
	}

	// No WorkDir override: use the pool's session reuse path, forwarding opts.Model.
	return p.call(ctx, key, systemPrompt, userPrompt, opts.Model, p.runner)
}

// CallBlockingWithOptions is like CallBlocking but supports WorkDir and Model overrides.
func (p *Pool) CallBlockingWithOptions(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions) (string, error) {
	ch, err := p.CallWithOptions(ctx, key, systemPrompt, userPrompt, opts)
	if err != nil {
		return "", err
	}
	return drainChannel(ch)
}

// CallBlocking runs a headless LLM call and blocks until the result is complete.
// Returns the concatenated text from all chunks and the first non-nil error.
func (p *Pool) CallBlocking(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string) (string, error) {
	ch, err := p.Call(ctx, key, systemPrompt, userPrompt)
	if err != nil {
		return "", err
	}
	return drainChannel(ch)
}

// drainChannel collects all StreamChunk text from ch until Done=true or Err!=nil.
func drainChannel(ch <-chan StreamChunk) (string, error) {
	var sb strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			return sb.String(), chunk.Err
		}
		if chunk.Text != "" {
			sb.WriteString(chunk.Text)
		}
		if chunk.Done {
			break
		}
	}
	// Drain remaining chunks in case the channel has extras.
	for range ch {
	}
	return sb.String(), nil
}
