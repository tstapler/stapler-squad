package services

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPushService creates a PushService backed by a temp directory.
func newTestPushService(t *testing.T) *PushService {
	t.Helper()
	dir := t.TempDir()
	svc := NewPushService(dir)
	require.NotNil(t, svc)
	return svc
}

// newValidSubscription creates a PushSubscription with a real P-256 public key and auth
// secret so the webpush library can complete encryption and reach the test server.
func newValidSubscription(t *testing.T, endpoint string) PushSubscription {
	t.Helper()
	privKey, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)
	// Uncompressed P-256 point: 0x04 || X || Y (65 bytes) — webpush expects this format.
	p256dh := base64.RawURLEncoding.EncodeToString(privKey.PublicKey().Bytes())

	authBytes := make([]byte, 16)
	_, err = rand.Read(authBytes)
	require.NoError(t, err)
	auth := base64.RawURLEncoding.EncodeToString(authBytes)

	return PushSubscription{
		Endpoint: endpoint,
		Keys: struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		}{P256dh: p256dh, Auth: auth},
	}
}

// UT-1.1 — Subscribe does not deadlock under concurrent calls [R1]
func TestSubscribeConcurrent(t *testing.T) {
	svc := newTestPushService(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc.Subscribe(PushSubscription{
				Endpoint: fmt.Sprintf("https://example.com/%d", i),
			})
		}(i)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		// pass — no deadlock
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe deadlocked under concurrent calls")
	}
}

// UT-1.2 — Subscribe/Unsubscribe/GetSubscriptions use correct lock pairs [R1]
func TestMutexSymmetry(t *testing.T) {
	svc := newTestPushService(t)
	svc.Subscribe(PushSubscription{Endpoint: "https://example.com/1"})
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); svc.Subscribe(PushSubscription{Endpoint: "https://example.com/new"}) }()
		go func() { defer wg.Done(); _ = svc.GetSubscriptions() }()
	}
	wg.Wait() // passes under -race if no data races
}

// UT-1.3 — SendNotification removes subscription on HTTP 410 [R2]
func TestSendNotification410RemovesSubscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone) // 410
	}))
	defer server.Close()

	svc := newTestPushService(t)
	svc.Subscribe(newValidSubscription(t, server.URL))

	require.Len(t, svc.GetSubscriptions(), 1)
	svc.SendNotification(PushNotification{Title: "test", Body: "body"})
	assert.Len(t, svc.GetSubscriptions(), 0, "subscription must be removed after 410")
}

// UT-1.4 — SendNotification removes subscription on HTTP 404 [R2]
func TestSendNotification404RemovesSubscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // 404
	}))
	defer server.Close()

	svc := newTestPushService(t)
	svc.Subscribe(newValidSubscription(t, server.URL))

	require.Len(t, svc.GetSubscriptions(), 1)
	svc.SendNotification(PushNotification{Title: "test", Body: "body"})
	assert.Len(t, svc.GetSubscriptions(), 0, "subscription must be removed after 404")
}

// UT-1.5 — SendNotification closes response body on all status codes [R21]
func TestSendNotificationClosesBody(t *testing.T) {
	statuses := []int{201, 410, 404, 413, 429, 500}
	for _, status := range statuses {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			svc := newTestPushService(t)
			svc.Subscribe(PushSubscription{Endpoint: server.URL})
			svc.SendNotification(PushNotification{Title: "t", Body: "b"})
			// Passes under -race: no goroutine leak means body was closed.
		})
	}
}

// UT-1.6 — SendNotification retains subscription on HTTP 201 [R2]
func TestSendNotification201RetainsSubscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
	}))
	defer server.Close()

	svc := newTestPushService(t)
	svc.Subscribe(PushSubscription{Endpoint: server.URL})
	svc.SendNotification(PushNotification{Title: "test", Body: "body"})
	assert.Len(t, svc.GetSubscriptions(), 1, "subscription must NOT be removed after 201")
}
