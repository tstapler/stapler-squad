package inbox

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// D10: a redirect must neither be followed nor carry the token off-host.
func TestClient_should_NotFollowRedirectAndNotSendToken_When_InboxRepliesWithRedirectToAnotherHost(t *testing.T) {
	var offHost atomic.Int32
	evil := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { offHost.Add(1) }))
	defer evil.Close()
	f := newFakeInbox(t, func(w http.ResponseWriter, r *http.Request) {
		// Both the documented evil.test name and a live listener, so a followed redirect is observable.
		w.Header().Set("Location", "http://evil.test/")
		if r.URL.Path == "/v1/subscriptions" {
			w.Header().Set("Location", evil.URL)
		}
		w.WriteHeader(http.StatusTemporaryRedirect)
	})
	c := clientFor(t, f, tokenFile(t))

	_, err := c.Events(context.Background(), 0)
	require.ErrorIs(t, err, ErrTransport)
	_, err = c.Subscriptions(context.Background())
	require.ErrorIs(t, err, ErrTransport)

	assert.Zero(t, offHost.Load(), "redirect target must never be contacted")
	reqs := f.requests()
	require.Len(t, reqs, 2, "exactly one request per call, no follow-up")
	for _, r := range reqs {
		assert.Equal(t, "Bearer "+sentinelToken, r.Header.Get("Authorization"))
	}
}

// Every error path and the stdlib logger must stay free of the bearer token,
// even when the server echoes the Authorization header back.
func TestClient_should_NeverLeakToken_When_EveryFailurePathRunsUnderLogCapture(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	echo := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			// Non-JSON echo (a proxy page): serverMessage drops it.
			_, _ = w.Write([]byte("<html>" + r.Header.Get("Authorization") + "</html>"))
		}
	}
	// Echo inside error.message is relayed by serverMessage; redactToken must scrub it.
	jsonEcho := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			msg, _ := json.Marshal(map[string]any{"error": map[string]string{"message": "bad auth " + r.Header.Get("Authorization")}})
			_, _ = w.Write(msg)
		}
	}
	handlers := []http.HandlerFunc{echo(401), echo(500), echo(400), jsonEcho(401), jsonEcho(400), reply(200, `{"items":`), reply(307, "")}
	errs := make([]error, 0, 2*len(handlers))
	for _, h := range handlers {
		f := newFakeInbox(t, h)
		_, err := clientFor(t, f, tokenFile(t)).Events(context.Background(), 5)
		errs = append(errs, err)
		errs = append(errs, clientFor(t, f, tokenFile(t)).Ack(context.Background(), "inb_1"))
	}
	for _, err := range errs {
		if err == nil { // Ack ignores the body, so a malformed 200 is a success
			continue
		}
		assert.NotContains(t, err.Error(), sentinelToken)
	}
	assert.NotContains(t, buf.String(), sentinelToken)
}
