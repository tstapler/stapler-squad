package executor

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CircuitState represents the current state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Fail-fast mode
	CircuitHalfOpen                     // Probing for recovery
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "CLOSED"
	case CircuitOpen:
		return "OPEN"
	case CircuitHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

// ErrCircuitOpen is returned when a call is rejected because the circuit is open.
var ErrCircuitOpen = fmt.Errorf("circuit breaker is open")

// Resettable is implemented by executors that support resetting their internal
// failure state (e.g., after a successful recovery from a dependency outage).
// Use a type assertion to this interface rather than asserting to *CircuitBreakerExecutor
// directly, so that mock or wrapper executors can also participate in resets.
type Resettable interface {
	Reset()
}

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	FailureThreshold int           // Number of consecutive failures to trip the breaker
	RecoveryTimeout  time.Duration // Time to wait before probing in HALF-OPEN
	// IsFailure classifies whether a command result counts as a circuit breaker failure.
	// Receives the command class, combined output (nil for Run calls), and the error.
	// If nil, any non-nil error is treated as a failure (default behavior).
	IsFailure func(commandClass string, output []byte, err error) bool
}

// DefaultCircuitBreakerConfig returns the default configuration.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  30 * time.Second,
	}
}

// Clock is an interface for time operations (injectable for testing).
type Clock interface {
	Now() time.Time
}

// realClock is the default Clock implementation using time.Now.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// circuitBreaker tracks state for a single command-class.
type circuitBreaker struct {
	mu                  sync.Mutex
	state               CircuitState
	consecutiveFailures int
	lastStateChange     time.Time
	probeInFlight       bool // true when a HALF-OPEN probe is currently executing
	config              CircuitBreakerConfig
	clock               Clock
}

// allowRequest checks whether a request should be allowed through.
// The caller MUST hold cb.mu.
func (cb *circuitBreaker) allowRequest() bool {
	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		elapsed := cb.clock.Now().Sub(cb.lastStateChange)
		if elapsed >= cb.config.RecoveryTimeout {
			cb.state = CircuitHalfOpen
			cb.lastStateChange = cb.clock.Now()
			cb.probeInFlight = true
			return true
		}
		return false
	case CircuitHalfOpen:
		// In HALF-OPEN, only the single probe that caused the transition is allowed.
		// Any additional requests are rejected until the probe completes.
		if !cb.probeInFlight {
			// No probe in flight yet -- allow one.
			cb.probeInFlight = true
			return true
		}
		return false
	default:
		return false
	}
}

// recordResult records the outcome of an executed request.
// The caller MUST hold cb.mu.
func (cb *circuitBreaker) recordResult(success bool) {
	if success {
		cb.consecutiveFailures = 0
		if cb.state != CircuitClosed {
			cb.state = CircuitClosed
			cb.lastStateChange = cb.clock.Now()
		}
		cb.probeInFlight = false
	} else {
		cb.consecutiveFailures++
		switch cb.state {
		case CircuitClosed:
			if cb.consecutiveFailures >= cb.config.FailureThreshold {
				cb.state = CircuitOpen
				cb.lastStateChange = cb.clock.Now()
			}
		case CircuitHalfOpen:
			// Failed probe: revert to OPEN and restart the recovery timer.
			cb.state = CircuitOpen
			cb.lastStateChange = cb.clock.Now()
			cb.probeInFlight = false
		}
	}
}

// reset resets this circuit breaker to closed state.
// The caller must NOT hold cb.mu.
func (cb *circuitBreaker) reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.consecutiveFailures = 0
	cb.lastStateChange = cb.clock.Now()
	cb.probeInFlight = false
}

// Reset resets all circuit breakers in this executor to closed state.
// Call after successfully recovering an external dependency (e.g. tmux server restart).
func (e *CircuitBreakerExecutor) Reset() {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, cb := range e.breakers {
		cb.reset()
	}
}

// CircuitBreakerExecutor wraps an Executor with per-command-class circuit breakers.
type CircuitBreakerExecutor struct {
	delegate Executor
	config   CircuitBreakerConfig
	clock    Clock

	mu       sync.RWMutex
	breakers map[string]*circuitBreaker
}

// NewCircuitBreakerExecutor creates a new CircuitBreakerExecutor wrapping the delegate.
func NewCircuitBreakerExecutor(delegate Executor, config CircuitBreakerConfig) *CircuitBreakerExecutor {
	return &CircuitBreakerExecutor{
		delegate: delegate,
		config:   config,
		clock:    realClock{},
		breakers: make(map[string]*circuitBreaker),
	}
}

// NewCircuitBreakerExecutorWithClock creates a CircuitBreakerExecutor with an injectable clock (for testing).
func NewCircuitBreakerExecutorWithClock(delegate Executor, config CircuitBreakerConfig, clock Clock) *CircuitBreakerExecutor {
	return &CircuitBreakerExecutor{
		delegate: delegate,
		config:   config,
		clock:    clock,
		breakers: make(map[string]*circuitBreaker),
	}
}

// commandClass extracts a stable key from the command for circuit breaker grouping.
// Examples: "git diff ..." -> "git-diff", "tmux capture-pane ..." -> "tmux-capture-pane"
// Flags that consume a value argument (e.g. "-L socketName", "-S socketPath") are skipped
// so the subcommand is correctly identified even when such flags precede it.
func commandClass(cmd *exec.Cmd) string {
	if cmd == nil || len(cmd.Args) == 0 {
		return "unknown"
	}
	parts := []string{filepath.Base(cmd.Args[0])}
	args := cmd.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			parts = append(parts, arg)
			break
		}
		// Skip flags that consume the next argument (e.g. -L <socket>, -S <path>)
		if (arg == "-L" || arg == "-S") && i+1 < len(args) {
			i++ // skip the flag's value too
		}
	}
	return strings.Join(parts, "-")
}

// getOrCreateBreaker returns the circuit breaker for the given command class, creating if needed.
func (e *CircuitBreakerExecutor) getOrCreateBreaker(class string) *circuitBreaker {
	e.mu.RLock()
	cb, ok := e.breakers[class]
	e.mu.RUnlock()
	if ok {
		return cb
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if cb, ok = e.breakers[class]; ok {
		return cb
	}
	cb = &circuitBreaker{
		state:           CircuitClosed,
		lastStateChange: e.clock.Now(),
		config:          e.config,
		clock:           e.clock,
	}
	e.breakers[class] = cb
	return cb
}

// Run executes the command through the circuit breaker, delegating to the wrapped executor.
// Returns ErrCircuitOpen if the breaker for this command class is open.
func (e *CircuitBreakerExecutor) Run(cmd *exec.Cmd) error {
	class := commandClass(cmd)
	cb := e.getOrCreateBreaker(class)

	cb.mu.Lock()
	if !cb.allowRequest() {
		cb.mu.Unlock()
		return ErrCircuitOpen
	}
	// BUG-003 prevention: In HALF-OPEN, allowRequest sets probeInFlight=true.
	// Concurrent callers see probeInFlight and are rejected in allowRequest
	// without blocking. We release the lock during execution so other callers
	// can check the flag and fail fast rather than waiting.
	cb.mu.Unlock()

	err := e.delegate.Run(cmd)

	cb.mu.Lock()
	cb.recordResult(!e.isFailure(class, nil, err))
	cb.mu.Unlock()

	return err
}

// Output executes the command and returns its output through the circuit breaker.
// Returns ErrCircuitOpen if the breaker for this command class is open.
func (e *CircuitBreakerExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	class := commandClass(cmd)
	cb := e.getOrCreateBreaker(class)

	cb.mu.Lock()
	if !cb.allowRequest() {
		cb.mu.Unlock()
		return nil, ErrCircuitOpen
	}
	// BUG-003 prevention: Same approach as Run -- probeInFlight flag rejects
	// concurrent callers without blocking.
	cb.mu.Unlock()

	output, err := e.delegate.Output(cmd)

	cb.mu.Lock()
	cb.recordResult(!e.isFailure(class, output, err))
	cb.mu.Unlock()

	return output, err
}

// CombinedOutput executes the command and returns its combined stdout+stderr through the circuit breaker.
// Returns ErrCircuitOpen if the breaker for this command class is open.
func (e *CircuitBreakerExecutor) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	class := commandClass(cmd)
	cb := e.getOrCreateBreaker(class)

	cb.mu.Lock()
	if !cb.allowRequest() {
		cb.mu.Unlock()
		return nil, ErrCircuitOpen
	}
	cb.mu.Unlock()

	output, err := e.delegate.CombinedOutput(cmd)

	cb.mu.Lock()
	cb.recordResult(!e.isFailure(class, output, err))
	cb.mu.Unlock()

	return output, err
}

// isFailure returns true if the command result should count as a circuit breaker failure.
// Delegates to config.IsFailure if set, otherwise treats any non-nil error as a failure.
func (e *CircuitBreakerExecutor) isFailure(class string, output []byte, err error) bool {
	if e.config.IsFailure != nil {
		return e.config.IsFailure(class, output, err)
	}
	return err != nil
}

// AllBreakers returns a snapshot of all circuit breakers for observability.
func (e *CircuitBreakerExecutor) AllBreakers() map[string]CircuitBreakerSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make(map[string]CircuitBreakerSnapshot, len(e.breakers))
	for k, cb := range e.breakers {
		cb.mu.Lock()
		result[k] = CircuitBreakerSnapshot{
			State:               cb.state,
			ConsecutiveFailures: cb.consecutiveFailures,
			LastStateChange:     cb.lastStateChange,
			Config:              cb.config,
		}
		cb.mu.Unlock()
	}
	return result
}

// CircuitBreakerSnapshot holds a point-in-time view of a circuit breaker's state.
type CircuitBreakerSnapshot struct {
	State               CircuitState
	ConsecutiveFailures int
	LastStateChange     time.Time
	Config              CircuitBreakerConfig
}
