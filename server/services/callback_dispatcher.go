package services

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// maxInFlightCallbacks bounds the number of concurrent outbound-callback delivery
// goroutines (AC10). Fixed cap, not configurable — revisit if a real deployment
// needs it (ponytail: no evidence yet that 20 is too low for any known call
// site's fan-out).
const maxInFlightCallbacks = 20

// callbackRetryAttempts is how many times CallbackDispatcher tries to deliver a
// single event before giving up and logging a failure.
const callbackRetryAttempts = 3

// callbackAttemptTimeout bounds a single HTTP POST attempt (including its own
// SSRF re-validation).
const callbackAttemptTimeout = 5 * time.Second

// callbackRetryBackoff is the delay before retry attempt N (1-indexed): a simple
// linear backoff (500ms, 1s), not exponential — three attempts over a few seconds
// is enough to ride out a transient blip without holding the semaphore slot for long.
const callbackRetryBackoff = 500 * time.Millisecond

// CallbackDispatcher fires outbound HTTP callbacks for the three lifecycle events
// FR7 covers: "session_complete", "session_stale", "queue_item_created". Built
// fresh from stdlib net/http — same shape as the (unimplemented) SlackNotifier
// design from project_plans/slack-review-notifications, which has no shipped code
// to reuse (0 matches repo-wide, verified). Concrete type, not an interface — one
// implementation, per .claude/rules/interface-pollution-checklist.md; the
// session package's CallbackDispatcher interface (session/callback_dispatcher.go)
// exists only because session cannot import this package.
//
// Dispatch is always non-blocking (FR8): a non-blocking select on a
// semaphore-sized channel either reserves a slot immediately or drops the
// dispatch and logs a warning (AC10) — it never queues beyond the cap and never
// makes the caller wait. Actual delivery (up to callbackRetryAttempts attempts,
// each independently timeout-bounded and SSRF-revalidated via
// ValidateCallbackURL — send-time half of AC11) happens in a goroutine launched
// after the slot is reserved.
//
// Delivery failures and dropped dispatches are logged with the event type only —
// never the target URL — per the redaction requirement (a URL can carry embedded
// credentials in its userinfo component).
type CallbackDispatcher struct {
	client *http.Client
	cfg    *config.Config

	// cfgMu guards concurrent reads of cfg.Callbacks (via resolveURL) against
	// CallbackConfigService.UpdateCallbackConfig's writes to that SAME
	// *config.Config instance — wired via ConfigMu()/server/dependencies.go's
	// SetSharedCallbackConfig call. Mirrors BacklogService.cfgMu (PR #199 review
	// F1) applied to the identical stale-pointer bug: cfg here was a snapshot
	// loaded once at process start with no writer ever touching it, so a saved
	// callback URL had zero runtime effect on Dispatch until a process restart.
	cfgMu sync.RWMutex

	inFlight chan struct{}

	// validateURL performs the send-time SSRF check (Task 5.2.1f). Always
	// ValidateCallbackURL in production (set by NewCallbackDispatcher); tests in
	// this package construct a CallbackDispatcher literal directly with a
	// permissive stub here so they can exercise real delivery/retry/redaction
	// behavior against an httptest.Server, whose address is necessarily loopback
	// and would otherwise always fail the real check — ValidateCallbackURL itself
	// is exercised directly and exhaustively in webhook_ssrf_test.go.
	validateURL func(ctx context.Context, rawURL string) error
}

// NewCallbackDispatcher creates a CallbackDispatcher reading callback URLs and the
// webhook_triggers feature flag from cfg (the live, shared *config.Config
// instance — Dispatch re-reads cfg.Callbacks on every call, so a config update
// via CallbackConfigService takes effect on the very next dispatch without a
// process restart).
func NewCallbackDispatcher(cfg *config.Config) *CallbackDispatcher {
	return &CallbackDispatcher{
		client: &http.Client{
			// SSRF fix (sdd:6-verify Layer 3 security review): the zero-value
			// http.Client follows up to 10 redirects transparently. Without this
			// override, a callback target that itself passes ValidateCallbackURL
			// could respond with a 3xx to a loopback/link-local/metadata address,
			// and the client would silently follow it — completely bypassing the
			// send-time SSRF check ValidateCallbackURL performs against the
			// *original* URL. Refuse redirects outright; a webhook-callback POST
			// has no legitimate need to follow one, and re-validating each hop
			// (the alternative) adds complexity for a case that shouldn't occur
			// in practice.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		cfg:         cfg,
		inFlight:    make(chan struct{}, maxInFlightCallbacks),
		validateURL: ValidateCallbackURL,
	}
}

// Dispatch fires eventType's configured callback URL with payload as a JSON POST
// body. A no-op when: d is nil, the webhook_triggers feature flag is off (Task
// 8.2.1b — defense in depth beyond route-registration gating), eventType has no
// URL configured, or the in-flight semaphore is already at maxInFlightCallbacks
// (dropped + logged, AC10). Never blocks the caller.
func (d *CallbackDispatcher) Dispatch(eventType string, payload any) {
	if d == nil || d.cfg == nil {
		return
	}
	if !d.cfg.GetFeatureFlag("webhook_triggers") {
		return
	}
	url := d.resolveURL(eventType)
	if url == "" {
		return
	}

	select {
	case d.inFlight <- struct{}{}:
	default:
		log.Warn("[CallbackDispatcher] dispatch dropped, at capacity", "event", eventType)
		return
	}

	go d.deliver(eventType, url, payload)
}

// resolveURL maps eventType to its configured callback URL ("" if unset or
// eventType is unrecognized). Reads cfg.Callbacks under cfgMu's read lock so a
// concurrent CallbackConfigService.UpdateCallbackConfig write is always either
// fully visible or not yet applied — never torn.
func (d *CallbackDispatcher) resolveURL(eventType string) string {
	d.cfgMu.RLock()
	defer d.cfgMu.RUnlock()
	switch eventType {
	case "session_complete":
		return d.cfg.Callbacks.OnSessionCompleteURL
	case "session_stale":
		return d.cfg.Callbacks.OnSessionStaleURL
	case "queue_item_created":
		return d.cfg.Callbacks.OnQueueItemCreatedURL
	default:
		return ""
	}
}

// ConfigMu exposes the mutex guarding cfg.Callbacks so
// CallbackConfigService.UpdateCallbackConfig can write a saved callback URL
// directly into this dispatcher's live *config.Config instance without a
// process restart — wired via SetSharedCallbackConfig/server/dependencies.go.
// See cfgMu's doc comment.
func (d *CallbackDispatcher) ConfigMu() *sync.RWMutex {
	return &d.cfgMu
}

// deliver runs in its own goroutine (launched by Dispatch after the semaphore
// slot is reserved) and always releases that slot before returning.
func (d *CallbackDispatcher) deliver(eventType, url string, payload any) {
	defer func() { <-d.inFlight }()

	body, err := json.Marshal(payload)
	if err != nil {
		log.Warn("[CallbackDispatcher] failed to marshal payload", "event", eventType, "err", err)
		return
	}

	for attempt := 1; attempt <= callbackRetryAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(time.Duration(attempt-1) * callbackRetryBackoff)
		}

		ctx, cancel := context.WithTimeout(context.Background(), callbackAttemptTimeout)

		// Send-time half of AC11: DNS can change between save time and this
		// attempt (and between successive attempts), so re-validate every time
		// rather than trusting the config-save-time check alone (TOCTOU /
		// DNS-rebinding, plan.md pitfalls §5). A validation failure aborts
		// delivery entirely (no further retries) — retrying against a target
		// that just failed an SSRF check would only give a DNS-rebinding
		// attacker more attempts to land the malicious answer.
		if err := d.validateURL(ctx, url); err != nil {
			cancel()
			log.Warn("[CallbackDispatcher] callback URL failed SSRF validation, aborting delivery", "event", eventType, "attempt", attempt, "err", err)
			return
		}

		ok := d.attempt(ctx, url, body)
		cancel()
		if ok {
			return
		}
	}

	log.Warn("[CallbackDispatcher] delivery failed after retries", "event", eventType, "attempts", callbackRetryAttempts)
}

// attempt performs a single POST of body to url, reporting whether it succeeded
// (2xx response). Never returns the response body or any part of url to the
// caller — attempt itself must not log (its callers decide what to log, and
// must never log url).
func (d *CallbackDispatcher) attempt(ctx context.Context, url string, body []byte) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
