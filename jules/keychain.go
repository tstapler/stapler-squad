package jules

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/sync/singleflight"

	"github.com/tstapler/stapler-squad/log"
)

// keychainService is this package's OS-keychain service namespace --
// deliberately distinct from github/keychain.go's "stapler-squad" service
// and session/sshremote/keystore.go's "stapler-squad-ssh": a different
// credential domain, no shared entries.
const keychainService = "stapler-squad-jules"

// keychainAccount is the single entry KeyringTokenSource reads/writes under
// keychainService -- there is only ever one Jules API key per machine, so
// (unlike sshremote's per-remote keying) no per-caller account suffix is
// needed.
const keychainAccount = "api-key"

// defaultKeyringTimeout bounds how long a synchronous keyring read/write
// waits before giving up, mirroring session/sshremote/keystore.go:36's
// defaultIdentityTimeout: on a headless system, go-keyring's Linux Secret
// Service backend can hang indefinitely on a D-Bus unlock prompt that never
// appears.
const defaultKeyringTimeout = 5 * time.Second

// defaultCacheTTL bounds how long a resolved key is served from memory
// before the next call re-checks the keyring. The Jules key is re-resolved
// on every outbound HTTP request (client.go's newRequest) and by the poller
// once per open session per tick, so without a cache a healthy keychain
// would still be hit far more often than necessary.
const defaultCacheTTL = 5 * time.Minute

// defaultCircuitCooldown bounds how long the circuit breaker stays open
// after a timeout before allowing exactly one more probe.
const defaultCircuitCooldown = 5 * time.Minute

// keyringGet, keyringSet, and keyringDelete are test seams over the
// package-level github.com/zalando/go-keyring functions. Production code
// never reassigns these.
var (
	keyringGet    = keyring.Get    //nolint:gochecknoglobals // test seam, see doc comment above
	keyringSet    = keyring.Set    //nolint:gochecknoglobals // test seam, see doc comment above
	keyringDelete = keyring.Delete //nolint:gochecknoglobals // test seam, see doc comment above
)

// KeyringTokenSource resolves and stores the Jules API key in the OS
// keychain under keychainService/keychainAccount. It implements
// JulesTokenSource (client.go).
//
// Unlike session/sshremote/keystore.go's package-level keyringMu (which
// serializes every future keyring call behind a single hung one, forever),
// KeyringTokenSource pairs a short-TTL cache with a bounded single-probe
// circuit breaker plus singleflight-coalesced synchronous reads: at most one
// goroutine is ever blocked inside the underlying keyring call at a time --
// whether that's the single background probe run while the circuit is open,
// or the shared synchronous call that concurrent cache-miss callers (e.g. the
// dispatch HTTP path and the poller racing right after the cache TTL
// expires) fan into via sfGroup -- regardless of call volume. See
// project_plans/google-jules-integration/implementation/plan.md Epic 1.2,
// Task 1.2.1a (pre-mortem P1 #4) for the incident this design closes.
type KeyringTokenSource struct {
	timeout  time.Duration
	cacheTTL time.Duration
	cooldown time.Duration

	// now abstracts time.Now for tests.
	now func() time.Time

	// onProbeStart, when non-nil, is called at the top of the background
	// probe goroutine launched in step (2) of APIKey. Test-only: lets tests
	// observe exactly how many probe goroutines ran without asserting on
	// runtime.NumGoroutine(), which is flake-prone under -race/parallel
	// tests.
	onProbeStart func()

	// sfGroup coalesces concurrent synchronous resolves (step (3) of APIKey)
	// into a single in-flight keyring call: every caller that misses the
	// cache at the same time shares one raceKeyringOp instead of each
	// spawning its own. Keyed by keychainAccount (this package only ever
	// resolves one account), so a single shared Group is fine.
	sfGroup singleflight.Group

	// stateMu guards only the four fields below -- cheap, never held across
	// the actual keyring call.
	stateMu          sync.Mutex
	cachedKey        JulesAPIKey
	cachedAt         time.Time
	circuitOpenUntil time.Time
	probeInFlight    bool
}

// KeyringTokenSourceOption configures a KeyringTokenSource at construction.
type KeyringTokenSourceOption func(*KeyringTokenSource)

// withKeyringTimeout overrides the per-call keyring timeout budget. Test-only.
func withKeyringTimeout(d time.Duration) KeyringTokenSourceOption {
	return func(s *KeyringTokenSource) { s.timeout = d }
}

// withCacheTTL overrides how long a resolved key is served from memory. Test-only.
func withCacheTTL(d time.Duration) KeyringTokenSourceOption {
	return func(s *KeyringTokenSource) { s.cacheTTL = d }
}

// withCircuitCooldown overrides how long the circuit stays open after a
// timeout before allowing one more probe. Test-only.
func withCircuitCooldown(d time.Duration) KeyringTokenSourceOption {
	return func(s *KeyringTokenSource) { s.cooldown = d }
}

// withClock overrides the time source used for cache/circuit bookkeeping. Test-only.
func withClock(now func() time.Time) KeyringTokenSourceOption {
	return func(s *KeyringTokenSource) { s.now = now }
}

// withProbeStartHook injects a callback invoked at the start of the
// background probe goroutine. Test-only.
func withProbeStartHook(f func()) KeyringTokenSourceOption {
	return func(s *KeyringTokenSource) { s.onProbeStart = f }
}

// NewKeyringTokenSource builds a KeyringTokenSource with production defaults.
func NewKeyringTokenSource(opts ...KeyringTokenSourceOption) *KeyringTokenSource {
	s := &KeyringTokenSource{
		timeout:  defaultKeyringTimeout,
		cacheTTL: defaultCacheTTL,
		cooldown: defaultCircuitCooldown,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// APIKey resolves the Jules API key, in order: (1) serve from cache if
// still within cacheTTL; (2) if the circuit is open, launch at most one
// background probe (never blocking this call) and return
// ErrJulesKeychainPaused immediately; (3) otherwise run a timeout-raced
// synchronous keyring read for this call.
func (s *KeyringTokenSource) APIKey(ctx context.Context) (JulesAPIKey, error) {
	now := s.now()

	s.stateMu.Lock()
	if !s.cachedAt.IsZero() && now.Sub(s.cachedAt) < s.cacheTTL {
		key := s.cachedKey
		s.stateMu.Unlock()
		return key, nil
	}

	if now.Before(s.circuitOpenUntil) {
		launchProbe := !s.probeInFlight
		if launchProbe {
			s.probeInFlight = true
		}
		s.stateMu.Unlock()
		if launchProbe {
			go s.runProbe()
		}
		return "", fmt.Errorf("%w: retrying after cooldown", ErrJulesKeychainPaused)
	}
	s.stateMu.Unlock()

	return s.resolveSync(ctx)
}

// syncResolveResult is the value type resolveSync's singleflight-shared
// closure returns, since singleflight.Group.Do only carries a single
// interface{} plus error -- timedOut needs to travel alongside val/err to
// every waiter, not just the leader.
type syncResolveResult struct {
	val      string
	err      error
	timedOut bool
}

// resolveSync runs a single timeout-raced synchronous keyring read, per step
// (3) of APIKey, shared via sfGroup across every goroutine that calls it
// concurrently: only the first caller ("leader") actually invokes
// raceKeyringOp, and every concurrent caller ("follower") blocks on that same
// call and receives its result, rather than each spawning its own
// non-cancellable goroutine against the keyring. All post-processing side
// effects (opening the circuit, the paused-warning log, and the cache write)
// run inside the shared closure too, so they execute exactly once per
// underlying keyring call -- not once per caller -- mirroring
// source_registry.go's refresh(), which does its cache mutation inside its
// own Do closure for the same reason. On timeout it opens the circuit and
// returns the timeout error to these callers only -- it does not itself
// return ErrJulesKeychainPaused; that is reserved for calls made while the
// circuit is already open. The follower's ctx is not honored (only the
// leader's is, since the underlying call is already shared) -- acceptable
// here because all callers share the same s.timeout budget regardless of ctx.
func (s *KeyringTokenSource) resolveSync(ctx context.Context) (JulesAPIKey, error) {
	v, _, _ := s.sfGroup.Do(keychainAccount, func() (any, error) {
		val, err, timedOut := s.raceKeyringOp(ctx, func() (string, error) {
			return keyringGet(keychainService, keychainAccount)
		})

		switch {
		case timedOut:
			s.openCircuit()
			log.Warn("jules keychain paused", "reason", "timeout")
		case err == nil:
			s.stateMu.Lock()
			s.cachedKey = JulesAPIKey(val)
			s.cachedAt = s.now()
			// Reset in case this call is the re-entry after a cooldown
			// elapsed (circuitOpenUntil is already in the past by now and
			// would never evaluate true again, but zeroing it makes "the
			// circuit is closed" explicit rather than merely stale) --
			// mirrors runProbe's success path.
			s.circuitOpenUntil = time.Time{}
			s.stateMu.Unlock()
		}

		return syncResolveResult{val: val, err: err, timedOut: timedOut}, nil
	})
	res := v.(syncResolveResult) //nolint:errcheck // sfGroup.Do's own closure never returns a non-nil error, only res.err

	if res.timedOut {
		return "", res.err
	}
	if res.err != nil {
		if errors.Is(res.err, keyring.ErrNotFound) {
			return "", fmt.Errorf("jules: %w", ErrJulesNotConfigured)
		}
		return "", fmt.Errorf("jules: reading API key from keychain: %w", res.err)
	}

	return JulesAPIKey(res.val), nil
}

// runProbe is the single background goroutine APIKey launches when the
// circuit is open and no other probe is already in flight. It runs the same
// timeout-raced keyring read as resolveSync: on success it populates the
// cache and closes the circuit; on any failure (timeout or otherwise) it
// re-extends circuitOpenUntil and logs once at Warn. Either way it clears
// probeInFlight when it returns, so a later call can launch the next probe
// once the (possibly re-extended) cooldown elapses.
func (s *KeyringTokenSource) runProbe() {
	if s.onProbeStart != nil {
		s.onProbeStart()
	}

	val, err, _ := s.raceKeyringOp(context.Background(), func() (string, error) {
		return keyringGet(keychainService, keychainAccount)
	})

	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.probeInFlight = false
	if err != nil {
		s.circuitOpenUntil = s.now().Add(s.cooldown)
		log.Warn("jules keychain paused", "reason", "probe failed", "err", err)
		return
	}
	s.cachedKey = JulesAPIKey(val)
	s.cachedAt = s.now()
	s.circuitOpenUntil = time.Time{}
}

// openCircuit sets circuitOpenUntil to now + cooldown.
func (s *KeyringTokenSource) openCircuit() {
	s.stateMu.Lock()
	s.circuitOpenUntil = s.now().Add(s.cooldown)
	s.stateMu.Unlock()
}

// resetCacheAndCircuit invalidates the cache and closes the circuit, used by
// SetJulesAPIKey/DeleteJulesAPIKey so a freshly-entered key is tried
// immediately rather than waiting out a stale cooldown.
func (s *KeyringTokenSource) resetCacheAndCircuit() {
	s.stateMu.Lock()
	s.cachedAt = time.Time{}
	s.circuitOpenUntil = time.Time{}
	s.stateMu.Unlock()
}

// SetJulesAPIKey stores key in the OS keychain, bypassing the cache/circuit
// read path -- a write must reach the keyring or fail honestly. It resets
// the cache and circuit afterward regardless of outcome, per Task 1.2.1a, so
// a subsequent APIKey call re-checks the keyring rather than serving a stale
// cached value or an unrelated open circuit.
func (s *KeyringTokenSource) SetJulesAPIKey(ctx context.Context, key JulesAPIKey) error {
	defer s.resetCacheAndCircuit()

	// string(key) is a plain type conversion, not a call to JulesAPIKey's
	// unexported reveal() method -- keychain.go must not add a second
	// reveal() call site (jules/secrets_guard_test.go asserts reveal()
	// appears exactly once, inside client.go's newRequest).
	_, err, timedOut := s.raceKeyringOp(ctx, func() (string, error) {
		return "", keyringSet(keychainService, keychainAccount, string(key))
	})
	if timedOut {
		return err
	}
	if err != nil {
		return fmt.Errorf("jules: writing API key to keychain: %w", err)
	}
	return nil
}

// DeleteJulesAPIKey removes the stored Jules API key from the OS keychain.
func (s *KeyringTokenSource) DeleteJulesAPIKey(ctx context.Context) error {
	defer s.resetCacheAndCircuit()

	_, err, timedOut := s.raceKeyringOp(ctx, func() (string, error) {
		return "", keyringDelete(keychainService, keychainAccount)
	})
	if timedOut {
		return err
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("jules: deleting API key from keychain: %w", err)
	}
	return nil
}

// raceKeyringOp runs op in a goroutine and races it against a ctx bounded to
// s.timeout, mirroring session/sshremote/keystore.go:171's raceKeyringOp
// exactly, including its documented gap: op cannot be force-aborted
// mid-flight (go-keyring exposes no cancellation), so on timeout the
// background goroutine keeps running until the real (or simulated) keyring
// call returns on its own. The third return value reports whether the
// timeout branch was taken, so callers can distinguish "the keychain hung"
// from "the keychain answered with an error" (e.g. ErrNotFound) without
// string-matching the error.
func (s *KeyringTokenSource) raceKeyringOp(ctx context.Context, op func() (string, error)) (val string, err error, timedOut bool) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	type result struct {
		val string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, opErr := op()
		ch <- result{val: v, err: opErr}
	}()

	select {
	case r := <-ch:
		return r.val, r.err, false
	case <-ctx.Done():
		return "", fmt.Errorf("jules: keychain operation timed out after %s: %w", s.timeout, ctx.Err()), true
	}
}
