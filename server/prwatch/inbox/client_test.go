package inbox

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recorded struct {
	Method, Path, RawQuery string
	Header                 http.Header
	Body                   string
}

// fakeInbox records every request and answers via handler (nil handler = 200 {}).
type fakeInbox struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []recorded
}

func newFakeInbox(t *testing.T, handler http.HandlerFunc) *fakeInbox {
	t.Helper()
	f := &fakeInbox{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), string(body)})
		f.mu.Unlock()
		if handler == nil {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		handler(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeInbox) requests() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.reqs...)
}

func clientFor(t *testing.T, f *fakeInbox, tokenPath string) *Client {
	t.Helper()
	c, err := NewClient(Config{URL: f.URL, TokenPath: tokenPath, ConsumerID: "squad:host", Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	return c
}

func tokenFile(t *testing.T) string { return writeToken(t, 0o600, sentinelToken+"\n") }

func reply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestClient_should_ReturnTypedErrors_When_ServerReturns401And5xxAndRefusesConnectionAndHangs(t *testing.T) {
	tok := tokenFile(t)
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		want    error
	}{
		{"401", reply(401, `{"error":{"code":"unauthorized","message":"bad token"}}`), ErrUnauthorized},
		{"500", reply(500, `{"error":{"code":"internal_error","message":"boom"}}`), Err5xx},
		{"502 proxy page", reply(502, `<html>Bad Gateway</html>`), Err5xx},
		{"400", reply(400, `{"error":{"code":"invalid_request","message":"nope"}}`), ErrRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := clientFor(t, newFakeInbox(t, tc.handler), tok).Subscriptions(context.Background())
			assert.ErrorIs(t, err, tc.want)
			assert.NotContains(t, err.Error(), sentinelToken)
		})
	}

	t.Run("refused connection", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := ln.Addr().String()
		ln.Close()
		c, err := NewClient(Config{URL: "http://" + addr, TokenPath: tok, ConsumerID: "c"})
		require.NoError(t, err)
		_, err = c.Events(context.Background(), 10)
		assert.ErrorIs(t, err, ErrTransport)
	})

	t.Run("hang past timeout", func(t *testing.T) {
		release := make(chan struct{})
		f := newFakeInbox(t, func(http.ResponseWriter, *http.Request) { <-release })
		defer close(release)
		start := time.Now()
		_, err := clientFor(t, f, tok).Events(context.Background(), 10)
		assert.ErrorIs(t, err, ErrTransport)
		assert.Less(t, time.Since(start), 2*time.Second)
	})
}

func TestClient_should_ReturnDecodeError_When_ResponseIsMalformedJSON(t *testing.T) {
	tok := tokenFile(t)
	for name, body := range map[string]string{
		"malformed":   `{"items": [`,
		"wrong type":  `{"items": "nope"}`,
		"not json":    `<html>`,
		"items shape": `{"items": [{"id": 5}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := clientFor(t, newFakeInbox(t, reply(200, body)), tok).Events(context.Background(), 0)
			assert.ErrorIs(t, err, ErrDecode)
		})
	}
}

func TestClient_should_BoundResponseSize_When_BodyExceedsCap(t *testing.T) {
	f := newFakeInbox(t, reply(200, `{"items":[],"pad":"`+strings.Repeat("x", 4096)+`"}`))
	c, err := NewClient(Config{URL: f.URL, TokenPath: tokenFile(t), ConsumerID: "c", MaxBytes: 1024})
	require.NoError(t, err)
	_, err = c.Events(context.Background(), 0)
	assert.ErrorIs(t, err, ErrDecode)
}

func TestClient_should_PickUpRotatedToken_When_FileChangesBetweenBatches(t *testing.T) {
	f := newFakeInbox(t, nil)
	tok := tokenFile(t)
	c := clientFor(t, f, tok)

	_, _ = c.Subscriptions(context.Background())
	require.NoError(t, os.WriteFile(tok, []byte("rotated-TOKEN-2\n"), 0o600))
	_, _ = c.Subscriptions(context.Background())

	reqs := f.requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "Bearer "+sentinelToken, reqs[0].Header.Get("Authorization"))
	assert.Equal(t, "Bearer rotated-TOKEN-2", reqs[1].Header.Get("Authorization"))
}

func TestClient_should_SendTokenOnlyInAuthorizationHeader_When_CallingAuthenticatedEndpoints(t *testing.T) {
	f := newFakeInbox(t, nil)
	c := clientFor(t, f, tokenFile(t))
	ctx := context.Background()

	_, _ = c.Watch(ctx, "o/r", 1)
	_, _ = c.Subscribe(ctx, "o/r", 1)
	_, _ = c.Subscriptions(ctx)
	_, _ = c.Events(ctx, 50)
	_ = c.Ack(ctx, "item-1")
	_, _ = c.Unsubscribe(ctx, "sub-1")

	reqs := f.requests()
	require.Len(t, reqs, 6)
	for _, r := range reqs {
		assert.Equal(t, "Bearer "+sentinelToken, r.Header.Get("Authorization"), r.Path)
		assert.NotContains(t, r.Path+r.RawQuery+r.Body, sentinelToken)
		for k, vs := range r.Header {
			if k != "Authorization" {
				assert.NotContains(t, strings.Join(vs, ","), sentinelToken, k)
			}
		}
	}
}

func TestClient_should_OnlyCallDocumentedV1Endpoints_When_ExercisedEndToEnd(t *testing.T) {
	f := newFakeInbox(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/watch":
			_, _ = w.Write([]byte(`{"coverage":"registered","subscription":{"id":"s1","consumer_id":"squad:host","repository":"o/r","pull_request":7,"active":true}}`))
		case "POST /v1/subscriptions":
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"created":true,"subscription":{"id":"s1","repository":"o/r","pull_request":7,"active":true}}`))
		case "GET /v1/subscriptions":
			_, _ = w.Write([]byte(`{"subscriptions":[{"id":"s1","repository":"o/r","pull_request":7,"active":true}]}`))
		case "GET /v1/events":
			_, _ = w.Write([]byte(`{"items":[{"id":"i1","subscription_id":"s1","event":{"id":"e1","kind":"ci","head_sha":null}}]}`))
		case "POST /v1/events/i1/ack":
			_, _ = w.Write([]byte(`{"acknowledged":true,"id":"i1"}`))
		case "DELETE /v1/subscriptions/s1":
			_, _ = w.Write([]byte(`{"removed":true,"id":"s1"}`))
		case "GET /v1/health":
			_, _ = w.Write([]byte(`{"status":"ok","protocol_version":1}`))
		default:
			t.Errorf("undocumented call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
	c := clientFor(t, f, tokenFile(t))
	ctx := context.Background()

	w, err := c.Watch(ctx, "o/r", 7)
	require.NoError(t, err)
	assert.Equal(t, CoverageRegistered, w.Coverage)
	sub, err := c.Subscribe(ctx, "o/r", 7)
	require.NoError(t, err)
	assert.Equal(t, "s1", sub.ID)
	subs, err := c.Subscriptions(ctx)
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	items, err := c.Events(ctx, 200)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "e1", items[0].Event.ID)
	require.NoError(t, c.Ack(ctx, items[0].ID))
	removed, err := c.Unsubscribe(ctx, "s1")
	require.NoError(t, err)
	assert.True(t, removed)
	h, err := c.Health(ctx)
	require.NoError(t, err)
	assert.Equal(t, "ok", h.Status)

	reqs := f.requests()
	assert.Equal(t, "consumer_id=squad%3Ahost&limit=200", reqs[3].RawQuery)
	var ack map[string]string
	require.NoError(t, json.Unmarshal([]byte(reqs[4].Body), &ack))
	assert.Equal(t, "squad:host", ack["consumer_id"])
	assert.Empty(t, reqs[6].Header.Get("Authorization"), "health is unauthenticated")
}

func TestClient_should_ReturnWatchUnsupported_When_WatchEndpointIs404(t *testing.T) {
	f := newFakeInbox(t, reply(404, `{"error":{"code":"not_found","message":"no route"}}`))
	c := clientFor(t, f, tokenFile(t))
	_, err := c.Watch(context.Background(), "o/r", 1)
	assert.ErrorIs(t, err, ErrWatchUnsupported)
	assert.ErrorIs(t, c.Ack(context.Background(), "x"), ErrRejected)
}

func TestClient_should_ExposeNoIngestOrHookOperations_When_PortIsInspected(t *testing.T) {
	var methods []string
	typ := reflect.TypeOf(&Client{})
	for i := 0; i < typ.NumMethod(); i++ {
		methods = append(methods, typ.Method(i).Name)
	}
	assert.ElementsMatch(t, []string{"Watch", "Subscribe", "Subscriptions", "Events", "Ack", "Unsubscribe", "Health"}, methods)
}

func TestClient_should_NotFollowRedirect_When_InboxRepliesWithRedirect(t *testing.T) {
	var hits int
	var mu sync.Mutex
	evil := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
	}))
	defer evil.Close()
	f := newFakeInbox(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL, http.StatusTemporaryRedirect)
	})
	_, err := clientFor(t, f, tokenFile(t)).Events(context.Background(), 0)
	assert.ErrorIs(t, err, ErrTransport)
	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, hits)
}

// The token file is read only after the URL passes, so a refused URL leaves
// both the network and the file untouched.
func TestNewClient_should_NotReadTokenFile_When_URLRefused(t *testing.T) {
	// A missing token path would surface ErrTokenMissing if it were consulted.
	_, err := NewClient(Config{URL: "http://127.0.0.1.evil.com:1", TokenPath: "/nonexistent/token", ConsumerID: "c"})
	require.ErrorIs(t, err, ErrNonLoopbackURL)
	assert.NotErrorIs(t, err, ErrTokenMissing)

	tok := tokenFile(t)
	before, err := os.Stat(tok)
	require.NoError(t, err)
	_, err = NewClient(Config{URL: "http://evil.example", TokenPath: tok, ConsumerID: "c"})
	require.ErrorIs(t, err, ErrNonLoopbackURL)
	after, err := os.Stat(tok)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime())
}
