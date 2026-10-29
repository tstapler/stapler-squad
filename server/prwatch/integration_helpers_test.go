package prwatch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

// fakeHTTPInbox is the shared integration-test double for gh-signal's agent
// inbox (protocol v1) on 127.0.0.1. Unlike fakeInbox (the in-memory port fake)
// it speaks the real wire format, so tests drive the real inbox.Client.
//
// Like the real inbox, GET /v1/events is non-destructive: an item is redelivered
// until it is acked. Routes are keyed by their ServeMux pattern, e.g.
// "GET /v1/events", "POST /v1/events/{id}/ack", "DELETE /v1/subscriptions/{id}",
// "POST /v1/watch", "POST /v1/subscriptions", "GET /v1/subscriptions",
// "GET /v1/health".
//
// API: newFakeHTTPInbox(t); URL, Token, TokenPath, Config(consumer);
// Push, Pending, Acked, Subscriptions; SetWatchCoverage, SetWatchUnsupported;
// SetDelay, FailNext, FailAlways, ClearFailures; Requests, RequestsFor,
// EventsRequests.
type fakeHTTPInbox struct {
	URL       string // http://127.0.0.1:<port>
	Token     string
	TokenPath string // 0600 file holding Token

	mu          sync.Mutex
	srv         *httptest.Server
	items       []inbox.Item
	acked       map[string]bool
	subs        []inbox.Subscription
	coverage    inbox.Coverage
	reason      string
	noWatch     bool
	delays      map[string]time.Duration
	failures    map[string]*fakeHTTPFailure
	requests    []fakeHTTPRequest
	eventsCalls int
}

type fakeHTTPFailure struct {
	status int
	left   int // -1 means forever
}

// fakeHTTPRequest is one request as the server saw it (body read in full).
type fakeHTTPRequest struct {
	Method  string
	Path    string
	Pattern string
	Query   map[string][]string
	Body    []byte
	Bearer  string // value after "Bearer ", empty if absent
}

func newFakeHTTPInbox(t *testing.T) *fakeHTTPInbox {
	t.Helper()
	f := &fakeHTTPInbox{
		Token:    "test-token-" + t.Name(),
		acked:    map[string]bool{},
		coverage: inbox.CoverageExisting,
		delays:   map[string]time.Duration{},
		failures: map[string]*fakeHTTPFailure{},
	}
	f.TokenPath = filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(f.TokenPath, []byte(f.Token+"\n"), 0o600))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", f.wrap(false, f.health))
	mux.HandleFunc("POST /v1/subscriptions", f.wrap(true, f.subscribe))
	mux.HandleFunc("POST /v1/watch", f.wrap(true, f.watch))
	mux.HandleFunc("GET /v1/subscriptions", f.wrap(true, f.listSubscriptions))
	mux.HandleFunc("DELETE /v1/subscriptions/{id}", f.wrap(true, f.unsubscribe))
	mux.HandleFunc("GET /v1/events", f.wrap(true, f.events))
	mux.HandleFunc("POST /v1/events/{id}/ack", f.wrap(true, f.ack))
	f.srv = httptest.NewServer(mux) // listens on 127.0.0.1
	t.Cleanup(f.srv.Close)
	f.URL = f.srv.URL
	return f
}

// Config returns an inbox.Config pointed at this fake for the given consumer.
func (f *fakeHTTPInbox) Config(consumerID string) inbox.Config {
	return inbox.Config{URL: f.URL, TokenPath: f.TokenPath, ConsumerID: consumerID}
}

// Push queues a pending item. Zero ID/SubscriptionID/CreatedAt are filled in;
// Event.Version defaults to 1.
func (f *fakeHTTPInbox) Push(item inbox.Item) inbox.Item {
	f.mu.Lock()
	defer f.mu.Unlock()
	if item.ID == "" {
		item.ID = "inb-" + strconv.Itoa(len(f.items)+1)
	}
	if item.Event.ID == "" {
		item.Event.ID = "evt-" + item.ID
	}
	if item.Event.Version == 0 {
		item.Event.Version = 1
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Unix(1_700_000_000, 0).UTC().Add(time.Duration(len(f.items)) * time.Second)
	}
	f.items = append(f.items, item)
	return item
}

// Pending returns the IDs of items not yet acked, oldest first.
func (f *fakeHTTPInbox) Pending() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, it := range f.items {
		if !f.acked[it.ID] {
			out = append(out, it.ID)
		}
	}
	return out
}

// Acked returns the IDs of acked items, in item order.
func (f *fakeHTTPInbox) Acked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, it := range f.items {
		if f.acked[it.ID] {
			out = append(out, it.ID)
		}
	}
	return out
}

// Subscriptions returns a copy of the active and inactive subscriptions.
func (f *fakeHTTPInbox) Subscriptions() []inbox.Subscription {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]inbox.Subscription(nil), f.subs...)
}

// SetWatchCoverage sets what POST /v1/watch reports; reason is only sent for
// CoverageUnavailable.
func (f *fakeHTTPInbox) SetWatchCoverage(c inbox.Coverage, reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.coverage, f.reason = c, reason
}

// SetWatchUnsupported makes POST /v1/watch answer 404, like an older inbox.
func (f *fakeHTTPInbox) SetWatchUnsupported(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.noWatch = v
}

// SetDelay makes every request on the route (ServeMux pattern) sleep first.
func (f *fakeHTTPInbox) SetDelay(route string, d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delays[route] = d
}

// FailNext answers the next n requests on the route with status and the
// documented error body. Auth is still checked first.
func (f *fakeHTTPInbox) FailNext(route string, status, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[route] = &fakeHTTPFailure{status: status, left: n}
}

// FailAlways answers every request on the route with status until cleared.
func (f *fakeHTTPInbox) FailAlways(route string, status int) { f.FailNext(route, status, -1) }

// ClearFailures removes all injected failures.
func (f *fakeHTTPInbox) ClearFailures() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = map[string]*fakeHTTPFailure{}
}

// Requests returns every request received, in order.
func (f *fakeHTTPInbox) Requests() []fakeHTTPRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeHTTPRequest(nil), f.requests...)
}

// RequestsFor returns the requests received on one route (ServeMux pattern).
func (f *fakeHTTPInbox) RequestsFor(route string) []fakeHTTPRequest {
	var out []fakeHTTPRequest
	for _, r := range f.Requests() {
		if r.Pattern == route {
			out = append(out, r)
		}
	}
	return out
}

// EventsRequests counts GET /v1/events requests, including failed ones.
func (f *fakeHTTPInbox) EventsRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.eventsCalls
}

func (f *fakeHTTPInbox) wrap(auth bool, h func(http.ResponseWriter, *http.Request, []byte)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(io.LimitReader(r.Body, 1<<20))
		}
		bearer := ""
		if v := r.Header.Get("Authorization"); len(v) > 7 && v[:7] == "Bearer " {
			bearer = v[7:]
		}
		f.mu.Lock()
		f.requests = append(f.requests, fakeHTTPRequest{
			Method: r.Method, Path: r.URL.Path, Pattern: r.Pattern,
			Query: r.URL.Query(), Body: body, Bearer: bearer,
		})
		if r.Pattern == "GET /v1/events" {
			f.eventsCalls++
		}
		delay := f.delays[r.Pattern]
		fail := f.failures[r.Pattern]
		status := 0
		if fail != nil && fail.left != 0 {
			status = fail.status
			if fail.left > 0 {
				fail.left--
			}
		}
		f.mu.Unlock()

		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		if auth && bearer != f.Token {
			fakeHTTPWriteErr(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
			return
		}
		if status != 0 {
			fakeHTTPWriteErr(w, status, "injected", "injected failure")
			return
		}
		h(w, r, body)
	}
}

func fakeHTTPWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fakeHTTPWriteErr(w http.ResponseWriter, status int, code, msg string) {
	fakeHTTPWriteJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

type fakeHTTPSubscribeRequest struct {
	ConsumerID  string `json:"consumer_id"`
	Repository  string `json:"repository"`
	PullRequest int    `json:"pull_request"`
}

// upsertSub returns the active subscription for the key, creating it when
// absent; created reports whether it was (re)activated. Caller holds f.mu.
func (f *fakeHTTPInbox) upsertSub(req fakeHTTPSubscribeRequest) (sub inbox.Subscription, created bool) {
	for i := range f.subs {
		s := &f.subs[i]
		if s.ConsumerID == req.ConsumerID && s.Repository == req.Repository && s.PullRequest == req.PullRequest {
			if !s.Active {
				s.Active = true
				return *s, true
			}
			return *s, false
		}
	}
	s := inbox.Subscription{
		ID: fmt.Sprintf("sub-%d", len(f.subs)+1), ConsumerID: req.ConsumerID,
		Repository: req.Repository, PullRequest: req.PullRequest,
		CreatedAt: time.Unix(1_700_000_000, 0).UTC(), Active: true,
	}
	f.subs = append(f.subs, s)
	return s, true
}

func (f *fakeHTTPInbox) health(w http.ResponseWriter, _ *http.Request, _ []byte) {
	fakeHTTPWriteJSON(w, http.StatusOK, inbox.Health{
		Status: "ok", ProtocolVersion: 1, Database: "ok",
		Ingress: inbox.HealthIngress{Mode: "bridge", Status: "ok"},
	})
}

func (f *fakeHTTPInbox) subscribe(w http.ResponseWriter, _ *http.Request, body []byte) {
	var req fakeHTTPSubscribeRequest
	if json.Unmarshal(body, &req) != nil || req.ConsumerID == "" || req.Repository == "" {
		fakeHTTPWriteErr(w, http.StatusBadRequest, "invalid_request", "bad subscribe body")
		return
	}
	f.mu.Lock()
	sub, created := f.upsertSub(req)
	f.mu.Unlock()
	fakeHTTPWriteJSON(w, http.StatusCreated, map[string]any{"created": created, "subscription": sub})
}

func (f *fakeHTTPInbox) watch(w http.ResponseWriter, _ *http.Request, body []byte) {
	f.mu.Lock()
	unsupported := f.noWatch
	f.mu.Unlock()
	if unsupported {
		fakeHTTPWriteErr(w, http.StatusNotFound, "not_found", "no such route")
		return
	}
	var req fakeHTTPSubscribeRequest
	if json.Unmarshal(body, &req) != nil || req.ConsumerID == "" || req.Repository == "" {
		fakeHTTPWriteErr(w, http.StatusBadRequest, "invalid_request", "bad watch body")
		return
	}
	f.mu.Lock()
	sub, _ := f.upsertSub(req)
	res := inbox.WatchResult{Coverage: f.coverage, Subscription: sub}
	if f.coverage == inbox.CoverageUnavailable {
		res.Reason = f.reason
	}
	f.mu.Unlock()
	fakeHTTPWriteJSON(w, http.StatusOK, res)
}

func (f *fakeHTTPInbox) listSubscriptions(w http.ResponseWriter, r *http.Request, _ []byte) {
	consumer := r.URL.Query().Get("consumer_id")
	f.mu.Lock()
	out := []inbox.Subscription{}
	for _, s := range f.subs {
		if s.Active && s.ConsumerID == consumer {
			out = append(out, s)
		}
	}
	f.mu.Unlock()
	fakeHTTPWriteJSON(w, http.StatusOK, map[string]any{"subscriptions": out})
}

func (f *fakeHTTPInbox) unsubscribe(w http.ResponseWriter, r *http.Request, body []byte) {
	var req struct {
		ConsumerID string `json:"consumer_id"`
	}
	_ = json.Unmarshal(body, &req)
	id := r.PathValue("id")
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.subs {
		s := &f.subs[i]
		if s.ID == id && s.ConsumerID == req.ConsumerID {
			removed := s.Active
			s.Active = false
			fakeHTTPWriteJSON(w, http.StatusOK, map[string]bool{"removed": removed})
			return
		}
	}
	fakeHTTPWriteErr(w, http.StatusNotFound, "not_found", "no such subscription")
}

func (f *fakeHTTPInbox) events(w http.ResponseWriter, r *http.Request, _ []byte) {
	limit := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v < limit {
		limit = v
	}
	f.mu.Lock()
	out := []inbox.Item{}
	for _, it := range f.items {
		if !f.acked[it.ID] && len(out) < limit {
			out = append(out, it)
		}
	}
	f.mu.Unlock()
	fakeHTTPWriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (f *fakeHTTPInbox) ack(w http.ResponseWriter, r *http.Request, _ []byte) {
	id := r.PathValue("id")
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, it := range f.items {
		if it.ID == id {
			f.acked[id] = true
			fakeHTTPWriteJSON(w, http.StatusOK, map[string]bool{"acked": true})
			return
		}
	}
	fakeHTTPWriteErr(w, http.StatusNotFound, "not_found", "no such item")
}

func TestFakeHTTPInbox_should_RedeliverUntilAcked_When_DrivenByRealClient(t *testing.T) {
	t.Parallel()
	f := newFakeHTTPInbox(t)
	c, err := inbox.NewClient(f.Config("squad"))
	require.NoError(t, err)
	ctx := t.Context()

	f.Push(inbox.Item{Event: inbox.Event{Repository: "o/r", PullRequest: 7, Kind: "ci"}})
	items, err := c.Events(ctx, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	items, err = c.Events(ctx, 10)
	require.NoError(t, err)
	require.Len(t, items, 1, "GET /v1/events is non-destructive")
	require.Equal(t, 2, f.EventsRequests())

	require.NoError(t, c.Ack(ctx, items[0].ID))
	items, err = c.Events(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, []string{"inb-1"}, f.Acked())
	require.Equal(t, f.Token, f.RequestsFor("POST /v1/events/{id}/ack")[0].Bearer)
}

func TestFakeHTTPInbox_should_ScriptWatchAndFailures_When_Configured(t *testing.T) {
	t.Parallel()
	f := newFakeHTTPInbox(t)
	c, err := inbox.NewClient(f.Config("squad"))
	require.NoError(t, err)
	ctx := t.Context()

	f.SetWatchCoverage(inbox.CoverageUnavailable, "no admin")
	res, err := c.Watch(ctx, "o/r", 7)
	require.NoError(t, err)
	require.Equal(t, inbox.CoverageUnavailable, res.Coverage)
	require.Equal(t, "no admin", res.Reason)
	require.Len(t, f.Subscriptions(), 1)

	f.SetWatchUnsupported(true)
	_, err = c.Watch(ctx, "o/r", 7)
	require.ErrorIs(t, err, inbox.ErrWatchUnsupported)

	f.FailNext("GET /v1/events", http.StatusServiceUnavailable, 1)
	_, err = c.Events(ctx, 10)
	require.ErrorIs(t, err, inbox.Err5xx)
	_, err = c.Events(ctx, 10)
	require.NoError(t, err)

	_, err = c.Health(ctx)
	require.NoError(t, err)
}
