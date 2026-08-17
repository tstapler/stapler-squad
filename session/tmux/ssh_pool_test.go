package tmux

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestSSHClientPool_GetOrDial_SharesOneClientAcrossCallers verifies
// concurrent GetOrDial calls for the same not-yet-dialed target coalesce
// onto a single dial rather than racing independent ones, and that every
// caller receives the exact same *ssh.Client.
func TestSSHClientPool_GetOrDial_SharesOneClientAcrossCallers(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "concurrent-remote", Addr: srv.Addr}

	const callers = 10
	clients := make([]*ssh.Client, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			clients[i], errs[i] = pool.GetOrDial(ctx, target, &cfg)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: GetOrDial() error: %v", i, err)
		}
	}
	for i := 1; i < callers; i++ {
		if clients[i] != clients[0] {
			t.Errorf("caller %d got a different *ssh.Client than caller 0 -- dials were not coalesced", i)
		}
	}
	if got := pool.RefCount(target.Name); got != callers {
		t.Errorf("RefCount(%q) = %d, want %d (one per GetOrDial call)", target.Name, got, callers)
	}
}

// TestSSHClientPool_Release_DoesNotCloseClient verifies Release only
// decrements the reference count -- the shared connection survives even
// after every caller has released it, per the Design Decision ("the last
// channel closing does not tear down the client").
func TestSSHClientPool_Release_DoesNotCloseClient(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "release-remote", Addr: srv.Addr}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}
	pool.Release(target.Name)

	if got := pool.RefCount(target.Name); got != 0 {
		t.Errorf("RefCount() after Release = %d, want 0", got)
	}
	if _, ok := pool.Peek(target.Name); !ok {
		t.Fatal("client was evicted after refcount reached zero, want it to stay pooled")
	}
	// The client must still work: open a real session on it.
	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("NewSession() on a released-but-not-removed client: %v", err)
	}
	_ = sess.Close()
}

// TestSSHClientPool_Remove_ClosesClientRegardlessOfRefCount verifies the
// explicit-removal teardown trigger closes the connection even while
// callers still hold references.
func TestSSHClientPool_Remove_ClosesClientRegardlessOfRefCount(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "remove-remote", Addr: srv.Addr}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	if err := pool.Remove(target.Name); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if _, ok := pool.Peek(target.Name); ok {
		t.Error("Peek() found an entry after Remove()")
	}
	if _, err := client.NewSession(); err == nil {
		t.Error("NewSession() on a Remove()d client succeeded, want an error (connection should be closed)")
	}
}

// TestSSHClientPool_DeadConnection_EvictedAndRedialed verifies a dead
// connection is detected (via the Client.Wait() watcher) and the next
// GetOrDial call redials rather than reusing or hanging on the dead entry.
func TestSSHClientPool_DeadConnection_EvictedAndRedialed(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "dead-remote", Addr: srv.Addr}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client1, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}
	_ = client1.Close() // simulate the connection dying

	waitForPoolEviction(t, pool, target.Name)

	client2, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() after dead-connection eviction: %v", err)
	}
	if client2 == client1 {
		t.Error("GetOrDial() returned the same dead client instead of redialing")
	}
	if _, err := client2.NewSession(); err != nil {
		t.Errorf("NewSession() on the redialed client: %v", err)
	}
}

// TestSSHClientPool_Subscribe_PrimesWithCurrentClient verifies a subscriber
// that attaches after a client is already pooled immediately receives it,
// without needing to wait for a future reconnect.
func TestSSHClientPool_Subscribe_PrimesWithCurrentClient(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "subscribe-primed", Addr: srv.Addr}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	ch, unsubscribe := pool.Subscribe(target.Name)
	defer unsubscribe()

	select {
	case got := <-ch:
		if got != client {
			t.Error("Subscribe() primed channel with a different *ssh.Client than the pooled one")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe() did not prime the channel with the already-pooled client")
	}
}

// TestSSHClientPool_Subscribe_NotifiesOnRedial verifies a subscriber
// receives the fresh *ssh.Client once a dead connection is evicted and
// redialed -- the reconnect signal session/sshremote's RemoteApprovalRelay
// depends on (plan.md Task 5.1.2a).
func TestSSHClientPool_Subscribe_NotifiesOnRedial(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "subscribe-redial", Addr: srv.Addr}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client1, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	ch, unsubscribe := pool.Subscribe(target.Name)
	defer unsubscribe()

	// Drain the priming notification for client1 before triggering the
	// redial, so the next receive on ch unambiguously corresponds to the
	// post-redial client.
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive the priming notification for client1")
	}

	_ = client1.Close() // simulate the connection dying
	waitForPoolEviction(t, pool, target.Name)

	client2, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() after dead-connection eviction: %v", err)
	}

	select {
	case got := <-ch:
		if got != client2 {
			t.Error("Subscribe() notified with a different *ssh.Client than the redialed one")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe() did not notify after redial")
	}
}

// TestSSHClientPool_Subscribe_UnsubscribeStopsFutureNotifies verifies
// calling the unsubscribe func removes the channel from future notify
// fan-out, so a subscriber that's done doesn't leak a permanently-blocked
// (or silently-dropped) send target.
func TestSSHClientPool_Subscribe_UnsubscribeStopsFutureNotifies(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "subscribe-unsub", Addr: srv.Addr}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client1, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	ch, unsubscribe := pool.Subscribe(target.Name)
	// Drain the priming notification.
	<-ch
	unsubscribe()

	_ = client1.Close()
	waitForPoolEviction(t, pool, target.Name)
	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() after dead-connection eviction: %v", err)
	}

	select {
	case v, ok := <-ch:
		t.Fatalf("received notify after unsubscribe: v=%v ok=%v", v, ok)
	case <-time.After(200 * time.Millisecond):
		// Expected: no notification after unsubscribe.
	}
}

// TestSSHClientPool_GetOrDial_RespectsCtxTimeout verifies GetOrDial itself
// is ctx-bounded (via dialSSHContext), matching Task 2.1.1f's requirement
// at the pool layer where the actual dial happens.
func TestSSHClientPool_GetOrDial_RespectsCtxTimeout(t *testing.T) {
	addr, _ := startStallingListener(t)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "stalling-remote", Addr: addr}
	cfg := ssh.ClientConfig{
		User: "test",
		Auth: []ssh.AuthMethod{testClientAuth(t)},
		HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error {
			return errors.New("unexpected: host key callback invoked during hung-handshake test")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := pool.GetOrDial(ctx, target, &cfg)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("GetOrDial() error = nil, want a context-deadline error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("GetOrDial() took %v, want well under its 500ms ctx budget plus scheduling slack", elapsed)
	}
}

// TestSSHClientPool_LoadTest_SharedUnderMaxStartupsThrottle is Task
// 2.1.0b's load test: creating a burst of concurrent sessions against one
// remote must stay well under a realistic MaxStartups-style throttle,
// because the pool coalesces every concurrent caller for the same
// not-yet-dialed remote name onto a single dial -- proven here by asserting
// the number of distinct TCP connections opened does not scale with
// session count (exactly one connection for 15-20 concurrent "sessions"),
// against a throttled listener that would reject connections past a limit
// far below that session count if pooling were not actually happening.
func TestSSHClientPool_LoadTest_SharedUnderMaxStartupsThrottle(t *testing.T) {
	const (
		sessionCount = 18 // within the plan's stated 15-20 range
		maxStartups  = 5  // deliberately far below sessionCount
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	throttled := newMaxStartupsListener(ln, maxStartups)
	srv := startTestSSHServerOnListener(t, throttled)

	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "burst-remote", Addr: srv.Addr}

	var wg sync.WaitGroup
	errs := make([]error, sessionCount)
	for i := 0; i < sessionCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := pool.GetOrDial(ctx, target, &cfg)
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("session %d: GetOrDial() error: %v (a shared pool should never hit the %d-connection throttle for %d sessions)", i, err, maxStartups, sessionCount)
		}
	}
	if got := pool.RefCount(target.Name); got != sessionCount {
		t.Errorf("RefCount(%q) = %d, want %d", target.Name, got, sessionCount)
	}
	if accepted := throttled.accepted.Load(); accepted != 1 {
		t.Errorf("distinct TCP connections accepted = %d, want exactly 1 -- the pool is not actually sharing a connection across %d sessions", accepted, sessionCount)
	}
	if rejected := throttled.rejected.Load(); rejected != 0 {
		t.Errorf("throttle rejected %d connection attempts, want 0", rejected)
	}
}
