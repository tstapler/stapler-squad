package tmux

import (
	"errors"
	"sync"
	"time"
)

// ErrSSHCircuitOpen is returned by SSHRunner when consecutive reconnect
// attempts to a remote host have exceeded the configured failure threshold.
// It is returned immediately, without attempting another dial, until the
// backoff interval elapses -- the mechanism that prevents a flaky network
// from turning into a tight redial loop that hammers the remote sshd.
var ErrSSHCircuitOpen = errors.New("ssh: circuit open, too many consecutive reconnect failures")

// sshCircuitState mirrors executor.CircuitState's three-state shape
// (closed/open/half-open) from executor/circuit_breaker.go, scoped per
// SSHRunner instance (i.e. per remote host) rather than per command class --
// an SSHRunner already represents exactly one remote connection, so there is
// no finer-grained "class" to key on the way CircuitBreakerExecutor keys on
// command class.
type sshCircuitState int

const (
	sshCircuitClosed sshCircuitState = iota
	sshCircuitOpen
	sshCircuitHalfOpen
)

func (s sshCircuitState) String() string {
	switch s {
	case sshCircuitClosed:
		return "CLOSED"
	case sshCircuitOpen:
		return "OPEN"
	case sshCircuitHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

// sshBackoffConfig configures sshBackoff's failure threshold and exponential
// backoff schedule.
type sshBackoffConfig struct {
	// FailureThreshold is the number of consecutive dial failures that
	// trips the circuit open.
	FailureThreshold int
	// BaseInterval is the recovery wait after the first trip.
	BaseInterval time.Duration
	// MaxInterval caps the exponential backoff (0 = no cap).
	MaxInterval time.Duration
}

// defaultSSHBackoffConfig returns SSHRunner's production defaults: three
// consecutive dial failures trips the circuit, starting at a 2s recovery
// wait and doubling up to a 2-minute cap -- conservative enough that a
// flaky remote host's sshd never sees a tight redial loop, but short enough
// that a brief network blip recovers within a couple of retries.
func defaultSSHBackoffConfig() sshBackoffConfig {
	return sshBackoffConfig{
		FailureThreshold: 3,
		BaseInterval:     2 * time.Second,
		MaxInterval:      2 * time.Minute,
	}
}

// clock abstracts time.Now for deterministic backoff tests, mirroring
// executor/circuit_breaker.go's Clock interface. Defined locally (rather
// than importing executor.Clock) since it's a single-method interface with
// no other shared behavior -- a little duplication here is cheaper than a
// cross-package dependency for one method, and keeps session/tmux's SSH
// backoff independent of executor's command-execution-focused package.
type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// sshBackoff is a single-instance (per SSHRunner) open/half-open/closed
// state machine, reusing the shape of executor/circuit_breaker.go's
// circuitBreaker: allowAttempt gates whether a new dial attempt may proceed,
// recordResult reports the outcome and drives state transitions +
// exponential backoff.
type sshBackoff struct {
	mu     sync.Mutex
	config sshBackoffConfig
	clock  clock

	state                sshCircuitState
	consecutiveFailures  int
	consecutiveOpenTrips int // failed HALF-OPEN probes; drives exponential backoff
	lastStateChange      time.Time
	probeInFlight        bool
}

func newSSHBackoff(config sshBackoffConfig) *sshBackoff {
	return newSSHBackoffWithClock(config, realClock{})
}

func newSSHBackoffWithClock(config sshBackoffConfig, c clock) *sshBackoff {
	return &sshBackoff{
		config:          config,
		clock:           c,
		state:           sshCircuitClosed,
		lastStateChange: c.Now(),
	}
}

// effectiveInterval returns the current recovery wait, doubling for each
// consecutive failed probe, capped at MaxInterval. Caller MUST hold b.mu.
func (b *sshBackoff) effectiveInterval() time.Duration {
	if b.consecutiveOpenTrips == 0 {
		return b.config.BaseInterval
	}
	shift := b.consecutiveOpenTrips
	if shift > 10 {
		shift = 10 // guard against overflow; 2^10 * base is already far beyond any sane cap
	}
	interval := b.config.BaseInterval << uint(shift)
	if b.config.MaxInterval > 0 && interval > b.config.MaxInterval {
		return b.config.MaxInterval
	}
	return interval
}

// allowAttempt reports whether a new dial attempt may proceed now. It
// returns ErrSSHCircuitOpen (without blocking) if the circuit is open and
// the backoff interval has not yet elapsed, or if a HALF-OPEN probe is
// already in flight.
func (b *sshBackoff) allowAttempt() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case sshCircuitClosed:
		return nil
	case sshCircuitOpen:
		if b.clock.Now().Sub(b.lastStateChange) >= b.effectiveInterval() {
			b.state = sshCircuitHalfOpen
			b.lastStateChange = b.clock.Now()
			b.probeInFlight = true
			return nil
		}
		return ErrSSHCircuitOpen
	case sshCircuitHalfOpen:
		if !b.probeInFlight {
			b.probeInFlight = true
			return nil
		}
		return ErrSSHCircuitOpen
	default:
		return ErrSSHCircuitOpen
	}
}

// recordResult records the outcome of a GetOrDial call, driving state
// transitions and (on failure) increasing the
// backoff.
func (b *sshBackoff) recordResult(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if success {
		b.consecutiveFailures = 0
		b.consecutiveOpenTrips = 0
		if b.state != sshCircuitClosed {
			b.state = sshCircuitClosed
			b.lastStateChange = b.clock.Now()
		}
		b.probeInFlight = false
		return
	}

	b.consecutiveFailures++
	switch b.state {
	case sshCircuitClosed:
		if b.consecutiveFailures >= b.config.FailureThreshold {
			b.state = sshCircuitOpen
			b.consecutiveOpenTrips = 0
			b.lastStateChange = b.clock.Now()
		}
	case sshCircuitHalfOpen:
		b.consecutiveOpenTrips++
		b.state = sshCircuitOpen
		b.lastStateChange = b.clock.Now()
		b.probeInFlight = false
	}
}

// State returns the current circuit state, for tests/observability.
func (b *sshBackoff) State() sshCircuitState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}
