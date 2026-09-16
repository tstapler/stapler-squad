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
	"time"
)

// CallOptions configures an individual pool call with overrides.
type CallOptions struct {
	// WorkDir sets the subprocess working directory (for git operations). Callers
	// MUST validate this is an absolute, existing directory before passing it here —
	// os/exec.Cmd.Dir has a well-documented quirk where a non-existent Dir makes the
	// resulting fork/exec error name the EXECUTABLE path, not the directory (e.g.
	// "fork/exec /home/user/.local/bin/claude: no such file or directory"), which
	// looks exactly like the binary is missing even though the real problem is a bad
	// working directory. See BUG-062 (server/services/backlog_service_triage.go's
	// TriggerTriage) for a live incident this caused and the validation added there.
	WorkDir string
	// Model overrides the pool's DefaultModel for this call only.
	Model string
	// TimeoutSecs is unused by Pool directly — callers wrap ctx with WithTimeout.
	TimeoutSecs int
	// AllowedTools scopes a WorkDir-bearing call to a specific comma-separated
	// tool list (e.g. "Read,Grep,Glob"), mirroring session.InstanceOptions.AllowedTools.
	// Only applied when WorkDir is also set; ignored otherwise.
	AllowedTools string
	// PermissionMode scopes a WorkDir-bearing call to a specific --permission-mode
	// value, mirroring session.InstanceOptions.PermissionMode. Only applied when
	// WorkDir is also set; ignored otherwise.
	PermissionMode string
	// DisallowedTools scopes a WorkDir-bearing call to an explicit denylist
	// (comma-separated, e.g. "Bash(rm:*),Write,Edit"), passed through to the
	// claude CLI's --disallowedTools flag. Mirrors AllowedTools/PermissionMode:
	// only applied when WorkDir is also set; ignored otherwise. Used alongside
	// AllowedTools as belt-and-suspenders — an explicit denylist of destructive
	// Bash prefixes and write-capable tools on top of a scoped allowlist.
	DisallowedTools string
}

// firstCallJSONResult is the JSON schema of the terminal `"type":"result"` line
// from claude -p --output-format stream-json --verbose. The cost field is
// total_cost_usd, not cost_usd (which doesn't exist) — verified against the
// live CLI.
type firstCallJSONResult struct {
	SessionID string  `json:"session_id"`
	Result    string  `json:"result"`
	IsError   bool    `json:"is_error"`
	CostUSD   float64 `json:"total_cost_usd"`
}

// claudeFallbackDirs lists standard install locations to check for the claude
// binary if it's not found via the process's PATH. Checked in order; the
// first executable match wins. Mirrors the standard system dirs
// scripts/install-service.sh appends to the systemd unit's PATH.
var claudeFallbackDirs = []string{
	"/usr/local/sbin",
	"/usr/local/bin",
	"/opt/homebrew/bin",
	"/usr/sbin",
	"/usr/bin",
	"/sbin",
	"/bin",
}

// findClaudeBinary locates the claude binary: lookPath first (the process's
// PATH, normally exec.LookPath), then homeDir's ".local/bin", then
// fallbackDirs, in that order. The first executable regular file wins.
//
// This exists because a service manager's baked-in PATH can go stale
// independently of the interactive shell's PATH a developer actually uses:
// systemd user units snapshot PATH at install time with no fallback (unlike
// the macOS LaunchAgent plist, which explicitly appends Homebrew and system
// paths — see scripts/install-service.sh). If claude is later reinstalled to
// a new location (nvm/asdf switch, a fresh `pip install --user`/npm global
// install) without a subsequent `make install-service`, lookPath alone would
// otherwise fail silently: NewPool returns ErrClaudeNotFound, the headless
// pool is left nil (only a log warning — see server/dependencies.go), and
// backlog triage quietly no-ops with no user-visible error.
//
// lookPath and fallbackDirs are explicit parameters (rather than calling
// exec.LookPath and reading claudeFallbackDirs directly) so tests can inject
// controlled values without mutating package-level state.
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
		if statErr == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
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
		// First call: stream-json output (one JSON object per line — system init,
		// assistant messages, tool_use/tool_result, and a terminal "result" event
		// carrying session_id/is_error/result/total_cost_usd) rather than a single
		// blocking JSON object. This gives call() a real per-line activity signal
		// for idleTimeout detection (pool.go) instead of an opaque "wait for the
		// whole subprocess to finish or don't" — see call()'s isFirstCall branch.
		// --verbose is required: the CLI rejects --print with
		// --output-format=stream-json otherwise (confirmed empirically against
		// the live binary, not documented anywhere clearly enough to trust
		// without checking).
		isFirstCall = true
		args = []string{"-p", "--output-format", "stream-json", "--verbose", "--system-prompt", systemPrompt, "--exclude-dynamic-system-prompt-sections"}
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
	// Bounded separately by maxQueueWait — shorter than any real caller's own
	// budget — so a call stuck behind other concurrent calls fails fast with a
	// distinct ErrPoolSaturated instead of silently consuming its entire call
	// budget just waiting for a slot (see that constant's doc comment).
	queueCtx, queueCancel := context.WithTimeout(ctx, maxQueueWait)
	defer queueCancel()
	select {
	case p.concurrencySem <- struct{}{}:
	case <-queueCtx.Done():
		p.decrementCallCount(key)
		close(ch)
		if err := ctx.Err(); err != nil {
			// The caller's own ctx (not just our added queue-wait cap) is what
			// expired/was cancelled first — preserve that real signal (e.g.
			// shutdown, or a caller whose own budget is shorter than
			// maxQueueWait) rather than masking it as pool saturation.
			return ch, err
		}
		return ch, fmt.Errorf("headless pool: %w", ErrPoolSaturated)
	}

	stdout, stop, err := runner.Run(ctx, args, stdinReader)
	if err != nil {
		<-p.concurrencySem // release on startup failure
		p.decrementCallCount(key)
		p.recordErrorAndMaybeRotate(key)
		close(ch)
		return ch, fmt.Errorf("headless runner start: %w: %w", ErrSubprocessStart, err)
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

		// sendFinal delivers a terminal chunk (typically a ctx-cancellation error)
		// from a code path where ctx is already Done — send() above cannot be used
		// there since its own select would immediately take the <-ctx.Done() branch
		// and silently discard the chunk instead of delivering it. ch is buffered
		// (16) and this is always the single terminal chunk on its path, so the
		// buffered send below never blocks in practice; the default case is a
		// safety net, not an expected outcome.
		sendFinal := func(chunk StreamChunk) {
			select {
			case ch <- chunk:
			default:
			}
		}

		cio := callIO{stop: stop, send: send, sendFinal: sendFinal}
		if isFirstCall {
			p.readFirstCallStream(ctx, key, stdout, cio)
			return
		}
		p.readResumedCallStream(ctx, key, stdout, cio)
	}()

	return ch, nil
}

// callIO bundles the plumbing readFirstCallStream/readResumedCallStream (and
// their helpers) need to terminate a call and deliver chunks, so passing it
// around doesn't blow past the parameter-count gate.
type callIO struct {
	stop      func() error
	send      func(StreamChunk) bool
	sendFinal func(StreamChunk)
}

// streamLine is one line of subprocess stdout read by startLineScanner, or a
// terminal scan error if the underlying read itself failed.
type streamLine struct {
	text string
	err  error
}

// startLineScanner reads stdout line by line in a background goroutine so
// readFirstCallStream/readResumedCallStream can select on a line arriving
// against ctx cancellation and idleTimeout, instead of blocking on a
// synchronous Scan() that ctx cancellation can't interrupt. drain must be
// called once the caller stops reading from the returned channel so the
// goroutine can exit after stdout closes rather than leaking.
func startLineScanner(stdout io.Reader) (lines <-chan streamLine, drain func()) {
	ch := make(chan streamLine, 16)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(stdout)
		// The one-time "system init" line lists every tool/MCP server/skill/
		// plugin and can exceed bufio.Scanner's 64KB default token size on a
		// richly-configured install — confirmed empirically against a live
		// call. 10MB is a generous ceiling with no real downside here (one
		// line, once per call).
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for scanner.Scan() {
			ch <- streamLine{text: scanner.Text()}
		}
		if err := scanner.Err(); err != nil {
			ch <- streamLine{err: err}
		}
	}()
	return ch, func() {
		for range ch { //nolint:revive // draining, not iterating for values
		}
	}
}

// isResultLine reports whether line is a well-formed stream-json envelope
// whose top-level "type" field is "result" — a structural check rather than a
// substring match, so a tool_use/tool_result payload that happens to contain
// the literal text `"type":"result"` can never be mistaken for the terminal
// event.
func isResultLine(line string) bool {
	var envelope struct {
		Type string `json:"type"`
	}
	return json.Unmarshal([]byte(line), &envelope) == nil && envelope.Type == "result"
}

// resetIdleTimer safely resets t after draining an already-fired channel —
// the standard safe-reset dance for a timer whose Stop() can return false
// because it already fired.
func resetIdleTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

// sendAccumulatedTextThenErr sends text (if non-empty) as a Text chunk, then
// finalErr as the terminal Err chunk — the shared shape for every first-call
// failure path that still has real partial output worth persisting (see
// readFirstCallStream/finishFirstCall).
func sendAccumulatedTextThenErr(send func(StreamChunk) bool, text string, finalErr error) {
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		if !send(StreamChunk{Text: trimmed}) {
			return
		}
	}
	send(StreamChunk{Err: finalErr, Done: true})
}

// recordErrorAndMaybeRotate records a call failure for key and rotates the
// session if the circuit-breaker threshold has been reached — the shared
// shape repeated across every error path below.
func (p *Pool) recordErrorAndMaybeRotate(key FeatureKey) {
	if tripBreaker := p.recordError(key); tripBreaker {
		p.rotateSession(key)
	}
}

// terminateStream stops the subprocess, drains any buffered lines, and sends
// err as the terminal chunk — the shared shutdown path for idle-timeout and
// ctx-cancellation on both the first-call and resumed-call scan loops.
func terminateStream(cio callIO, drainLines func(), err error) {
	_ = cio.stop()
	drainLines()
	cio.sendFinal(StreamChunk{Err: fmt.Errorf("headless call ended: %w", err), Done: true})
}

// firstCallScanResult is what scanFirstCallLines collected before the stream
// ended normally (not via idle timeout, ctx cancellation, or the output cap).
type firstCallScanResult struct {
	allText    string
	resultLine string
}

// firstCallScanState is the mutable per-line state threaded through
// scanFirstCallLines — bundled into a struct so handleFirstCallLine doesn't
// blow past the parameter-count gate.
type firstCallScanState struct {
	idleTimer  *time.Timer
	allText    *strings.Builder
	resultLine *string
	drainLines func()
}

// handleFirstCallLine processes one successfully-scanned line: resets the
// idle timer, accumulates allText, enforces the cumulative output cap, and
// records resultLine on the first line whose structural type is "result".
// Returns true once the output cap has been exceeded and the terminal chunk
// already sent — the caller must stop looping in that case.
func (p *Pool) handleFirstCallLine(state firstCallScanState, lr streamLine, cio callIO) bool {
	resetIdleTimer(state.idleTimer, idleTimeout)
	state.allText.WriteString(lr.text)
	state.allText.WriteByte('\n')
	if state.allText.Len() > maxFirstCallOutputBytes {
		// Cumulative cap independent of the per-line scanner buffer cap
		// (startLineScanner): a long-running, non-idle stream could otherwise
		// grow this buffer unbounded before idleTimeout would catch it.
		terminateStream(cio, state.drainLines, ErrOutputCapExceeded)
		return true
	}
	if *state.resultLine == "" && isResultLine(lr.text) {
		*state.resultLine = lr.text
	}
	return false
}

// scanFirstCallLines runs the idle-timeout-guarded scan loop for a first call
// (--output-format stream-json): system init, assistant messages,
// tool_use/tool_result, and a terminal "result" event. ok is false if the
// call was already terminated on this path (idle timeout, ctx cancellation,
// or the output cap) — the final chunk has already been sent via
// cio.sendFinal, and the caller must not send anything further.
func (p *Pool) scanFirstCallLines(ctx context.Context, key FeatureKey, stdout io.Reader, cio callIO) (result firstCallScanResult, ok bool) {
	lines, drainLines := startLineScanner(stdout)

	var allText strings.Builder
	var resultLine string
	state := firstCallScanState{idleTimer: time.NewTimer(idleTimeout), allText: &allText, resultLine: &resultLine, drainLines: drainLines}
	defer state.idleTimer.Stop()

	for {
		select {
		case lr, more := <-lines:
			if !more {
				return firstCallScanResult{allText: allText.String(), resultLine: resultLine}, true
			}
			if lr.err != nil {
				// A subprocess killed mid-write (e.g. OOM-killed) can still have
				// left real, useful output in allText before the read failed —
				// deliver it rather than discarding it (see sendAccumulatedTextThenErr).
				p.recordErrorAndMaybeRotate(key)
				sendAccumulatedTextThenErr(cio.send, allText.String(), lr.err)
				return firstCallScanResult{}, false
			}
			if p.handleFirstCallLine(state, lr, cio) {
				return firstCallScanResult{}, false
			}
		case <-state.idleTimer.C:
			// No new output line for idleTimeout: a real, distinct-from-a-
			// hard-deadline "this call is stalled" signal — see idleTimeout's
			// doc comment (pool.go).
			terminateStream(cio, drainLines, ErrIdleTimeout)
			return firstCallScanResult{}, false
		case <-ctx.Done():
			// Kill subprocess to unblock the scan, then wait for it to exit.
			terminateStream(cio, drainLines, ctx.Err())
			return firstCallScanResult{}, false
		}
	}
}

// sendFirstCallSuccess stores the session ID for future resume calls and
// delivers the successful result's text and cost.
func (p *Pool) sendFirstCallSuccess(key FeatureKey, result firstCallJSONResult, cio callIO) {
	if result.SessionID != "" {
		p.storeSessionID(key, result.SessionID)
	}
	p.recordSuccess(key)
	if result.Result != "" {
		if !cio.send(StreamChunk{Text: result.Result}) {
			return
		}
	}
	cio.send(StreamChunk{Done: true, CostUSD: result.CostUSD})
}

// finishFirstCall interprets a first call's scan result once the stream has
// ended normally: parses the terminal result line and sends the resulting
// success/error chunk(s).
func (p *Pool) finishFirstCall(key FeatureKey, scanRes firstCallScanResult, cio callIO) {
	if scanRes.resultLine == "" {
		// The stream ended (subprocess exited) without ever producing a
		// terminal "result" event — treat the whole accumulated output as
		// plain text, mirroring the old format's "not valid JSON" fallback.
		p.recordErrorAndMaybeRotate(key)
		sendAccumulatedTextThenErr(cio.send, scanRes.allText, errors.New("stream-json: subprocess exited with no terminal result event"))
		return
	}

	var result firstCallJSONResult
	if jsonErr := json.Unmarshal([]byte(scanRes.resultLine), &result); jsonErr != nil {
		p.recordErrorAndMaybeRotate(key)
		sendAccumulatedTextThenErr(cio.send, scanRes.allText, fmt.Errorf("first-call result-line JSON parse: %w", jsonErr))
		return
	}

	// claude -p sets is_error=true when the LLM returns an error response.
	if result.IsError {
		p.recordErrorAndMaybeRotate(key)
		cio.send(StreamChunk{Err: fmt.Errorf("claude reported error: %s", strings.TrimSpace(result.Result)), Done: true})
		return
	}

	p.sendFirstCallSuccess(key, result, cio)
}

// readFirstCallStream drives a first call's stream-json scan to completion
// and reports the outcome via cio.
func (p *Pool) readFirstCallStream(ctx context.Context, key FeatureKey, stdout io.Reader, cio callIO) {
	scanRes, ok := p.scanFirstCallLines(ctx, key, stdout, cio)
	if !ok {
		return
	}
	// Guard against a ctx cancellation race after the scan completes normally.
	if ctx.Err() != nil {
		cio.sendFinal(StreamChunk{Err: fmt.Errorf("headless call ended: %w", ctx.Err()), Done: true})
		return
	}
	p.finishFirstCall(key, scanRes, cio)
}

// handleResumedCallLine processes one line for readResumedCallStream:
// resets the idle timer and forwards the line as a Text chunk, or reports a
// scan error (recordSuccess on a clean io.EOF, recordError/rotate
// otherwise). Returns true once the call has ended and the caller must stop
// looping.
func (p *Pool) handleResumedCallLine(key FeatureKey, idleTimer *time.Timer, lr streamLine, cio callIO) bool {
	if lr.err != nil {
		if errors.Is(lr.err, io.EOF) {
			p.recordSuccess(key)
			cio.send(StreamChunk{Done: true})
			return true
		}
		p.recordErrorAndMaybeRotate(key)
		cio.send(StreamChunk{Err: lr.err, Done: true})
		return true
	}
	resetIdleTimer(idleTimer, idleTimeout)
	return !cio.send(StreamChunk{Text: lr.text})
}

// readResumedCallStream scans a resumed call's plain-text stdout line by
// line, forwarding each line as its own Text chunk. Shares
// scanFirstCallLines's idle-timer-per-line protection: before this, only a
// session's first call had any progress detection, leaving every later call
// in a session — most real call volume — defended only by ctx's own (much
// larger) deadline.
func (p *Pool) readResumedCallStream(ctx context.Context, key FeatureKey, stdout io.Reader, cio callIO) {
	lines, drainLines := startLineScanner(stdout)
	idleTimer := time.NewTimer(idleTimeout)
	defer idleTimer.Stop()

	for {
		select {
		case lr, more := <-lines:
			if !more {
				p.recordSuccess(key)
				cio.send(StreamChunk{Done: true})
				return
			}
			if p.handleResumedCallLine(key, idleTimer, lr, cio) {
				return
			}
		case <-idleTimer.C:
			terminateStream(cio, drainLines, ErrIdleTimeout)
			return
		case <-ctx.Done():
			terminateStream(cio, drainLines, ctx.Err())
			return
		}
	}
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

		// Acquire parent semaphore so WorkDir calls count toward the overall cap,
		// bounded by maxQueueWait like call()'s own acquire — the oneShot pool
		// below never blocks on its own semaphore, so without this bound here
		// the BUG-093 queue-wait fix would never actually apply to a WorkDir
		// caller (triage, review, PR creation, approval classification).
		queueCtx, queueCancel := context.WithTimeout(ctx, maxQueueWait)
		defer queueCancel()
		select {
		case p.concurrencySem <- struct{}{}:
		case <-queueCtx.Done():
			ch := make(chan StreamChunk)
			close(ch)
			if err := ctx.Err(); err != nil {
				return ch, err
			}
			return ch, fmt.Errorf("headless pool: %w", ErrPoolSaturated)
		}

		dirRunner := pr.WithWorkDir(opts.WorkDir)
		if opts.AllowedTools != "" || opts.PermissionMode != "" || opts.DisallowedTools != "" {
			dirRunner = dirRunner.WithToolAccess(opts.AllowedTools, opts.PermissionMode, opts.DisallowedTools)
		}
		oneShot := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 1, DefaultModel: opts.Model}, dirRunner)
		innerCh, err := oneShot.Call(ctx, key, systemPrompt, userPrompt)
		if err != nil {
			<-p.concurrencySem
			return innerCh, err
		}

		// Proxy the inner channel, releasing the parent semaphore when done.
		// A plain blocking send (not a select racing ctx.Done()) is deliberate:
		// drainChannelWithCost's reader always ranges over outCh until it's
		// closed, regardless of ctx state, so this never blocks in practice —
		// and once ctx is Done, a racing select would have a real chance of
		// picking the ctx.Done() case over a ready send and silently dropping
		// the inner goroutine's terminal error chunk (see call()'s sendFinal),
		// which is exactly the failure this proxy must not reintroduce.
		outCh := make(chan StreamChunk, 16)
		go func() {
			defer close(outCh)
			defer func() { <-p.concurrencySem }()
			for chunk := range innerCh {
				outCh <- chunk
			}
		}()
		return outCh, nil
	}

	// No WorkDir override: use the pool's session reuse path, forwarding opts.Model.
	return p.call(ctx, key, systemPrompt, userPrompt, opts.Model, p.runner)
}

// CostSink receives the USD cost of a completed CallBlocking call. Every call site
// must supply one — see DiscardCost for the explicit, greppable opt-out for a call
// with nowhere to persist cost. This replaced a `(string, float64, error)` return
// shape that let several pipeline call sites silently drop real cost data via `_`.
type CostSink func(usd float64)

// DiscardCost is the explicit opt-out for a CallBlocking call with nowhere to
// persist cost (e.g. a capability self-check). Grep this name to find every call
// site not wired into cost tracking.
func DiscardCost(float64) {}

// CallBlocking makes a single blocking headless call and returns the result text
// and any error. opts is the single place to pass WorkDir/Model/AllowedTools/
// PermissionMode; the zero value reproduces the simplest call shape. sink is
// always invoked with the cost in USD reported by claude, parsed from the JSON
// result at no extra cost — pass DiscardCost if the caller has nowhere to put it.
func (p *Pool) CallBlocking(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions, sink CostSink) (string, error) {
	ch, err := p.CallWithOptions(ctx, key, systemPrompt, userPrompt, opts)
	if err != nil {
		return "", err
	}
	text, cost, err := drainChannelWithCost(ch)
	if sink != nil {
		sink(cost)
	}
	return text, err
}

// drainChannelWithCost collects all StreamChunk text from ch until Done=true or
// Err!=nil, along with the CostUSD reported on the Done chunk.
func drainChannelWithCost(ch <-chan StreamChunk) (string, float64, error) {
	var sb strings.Builder
	var costUSD float64
	for chunk := range ch {
		if chunk.Err != nil {
			return sb.String(), costUSD, chunk.Err
		}
		if chunk.Text != "" {
			sb.WriteString(chunk.Text)
		}
		if chunk.Done {
			costUSD = chunk.CostUSD
			break
		}
	}
	for range ch {
	}
	return sb.String(), costUSD, nil
}
