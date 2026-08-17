package tmux

import (
	"context"
	"fmt"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/singleflight"
)

// SSHTarget identifies a single named SSH remote: a stable name to key the
// shared connection-pool entry by (corresponding to config.RemoteConfig.Name
// once Phase 3 wires remote configuration), plus the "host:port" address
// actually dialed over TCP. Bundled into one type rather than left as two
// adjacent strings (see .claude/rules/primitive-obsession-checklist.md):
// unlike CommandRunner's dir/name (which fail loudly and immediately at
// exec time if swapped -- a directory path is never a valid program name),
// swapping Name and Addr here compiles silently and both still "look like"
// plausible strings in the wrong slot, which is exactly the silent-
// plausible-wrongness a newtype exists to prevent.
type SSHTarget struct {
	// Name is the pool/registry key -- every SSHRunner dialing the same
	// remote must use the same Name for pooling to actually share a
	// connection.
	Name string
	// Addr is the "host:port" dialed over TCP.
	Addr string
}

// pooledSSHClient is one entry in SSHClientPool: a dialed *ssh.Client plus
// the count of callers currently holding a reference to it.
type pooledSSHClient struct {
	client   *ssh.Client
	refCount int
}

// SSHClientPool is a reference-counted registry of one shared *ssh.Client
// per remote name, per the ssh-remote-workspaces plan's Design Decision
// (Epic 2.1): SSHRunner instances for the same remote share a single dialed
// connection, with each command/session opened as a new SSH *channel* on
// that connection rather than a new TCP+SSH handshake. Without this, a
// burst of concurrent session creation collides with sshd's default
// MaxStartups throttle (pre-mortem.md Failure #1, P1) -- see
// ssh_pool_test.go's load test for the empirical proof this pool is
// actually shared under a throttled listener.
//
// Concurrent GetOrDial calls for the same not-yet-dialed target.Name
// coalesce onto a single in-flight dial via singleflight rather than racing
// independent dials. A pooled client is torn down only in two cases: an
// explicit Remove (the "remote-config removal" trigger) or a detected dead
// connection (Client.Wait() returning, spawned as a background watcher at
// register time) -- the last Release does NOT tear the client down, since
// the whole point of pooling is that the connection outlives any single
// caller's use of it.
type SSHClientPool struct {
	mu          sync.Mutex
	entries     map[string]*pooledSSHClient
	group       singleflight.Group
	subscribers map[string][]chan *ssh.Client // +checklocks:mu
}

// NewSSHClientPool returns an empty pool.
func NewSSHClientPool() *SSHClientPool {
	return &SSHClientPool{entries: make(map[string]*pooledSSHClient)}
}

// Subscribe returns a channel that receives name's pooled *ssh.Client every
// time register() installs one for it -- both the very first dial and every
// subsequent redial after an Evict/dead-connection cycle. This is the
// reconnect signal session/sshremote's RemoteApprovalRelay subscribes to in
// order to re-open its direct-streamlocal channel against a fresh
// connection after the underlying one drops and redials, per
// project_plans/ssh-remote-workspaces/implementation/plan.md Story 5.1.2 /
// Task 5.1.2a. Nothing in this package previously needed to observe a
// reconnect after the fact -- SSHRunner only ever calls GetOrDial again
// on-demand -- so this is new, minimal surface, not a pre-existing hook.
//
// The returned channel is buffered (capacity 1) and lossy by design: a
// subscriber that hasn't drained the previous notification only cares about
// the MOST RECENT client, so a notify that finds a full channel drops the
// stale value and pushes the fresh one instead of blocking the notifier
// (register, called from GetOrDial's caller) or growing unbounded.
//
// If a client is already pooled for name when Subscribe is called, the
// channel is primed with it immediately -- a subscriber that attaches after
// the first dial doesn't have to wait for the NEXT reconnect to learn about
// the current client.
//
// The returned unsubscribe func must be called once the subscriber is done;
// it removes the channel from the pool's subscriber list so future
// notifies don't leak a send to a channel nobody reads anymore.
func (p *SSHClientPool) Subscribe(name string) (<-chan *ssh.Client, func()) {
	ch := make(chan *ssh.Client, 1)

	p.mu.Lock()
	if p.subscribers == nil {
		p.subscribers = make(map[string][]chan *ssh.Client)
	}
	p.subscribers[name] = append(p.subscribers[name], ch)
	if e, ok := p.entries[name]; ok {
		ch <- e.client
	}
	p.mu.Unlock()

	unsubscribe := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		subs := p.subscribers[name]
		for i, c := range subs {
			if c == ch {
				p.subscribers[name] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
	return ch, unsubscribe
}

// notifySubscribers delivers client to every current subscriber of name,
// using the same non-blocking drop-stale-then-push pattern Subscribe's doc
// comment describes. Must not be called while holding p.mu.
func (p *SSHClientPool) notifySubscribers(name string, client *ssh.Client) {
	p.mu.Lock()
	subs := append([]chan *ssh.Client(nil), p.subscribers[name]...)
	p.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- client:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- client:
			default:
			}
		}
	}
}

// Peek returns the currently pooled client for name without dialing,
// reporting false if no live entry exists. Callers that already hold a
// reference (via a prior GetOrDial) can use this as a cheap hot-path check
// that does not touch the singleflight machinery or count as a dial
// attempt for backoff/circuit-breaker purposes.
func (p *SSHClientPool) Peek(name string) (*ssh.Client, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[name]
	if !ok {
		return nil, false
	}
	return e.client, true
}

// GetOrDial returns the shared *ssh.Client for target, dialing a new one
// (bounded by ctx) if none is pooled yet. Every call -- whether it triggers
// the dial or coalesces onto an in-flight/existing one -- increments
// target.Name's reference count by one; callers should pair this with a
// Release once they're done with the client for now.
func (p *SSHClientPool) GetOrDial(ctx context.Context, target SSHTarget, config *ssh.ClientConfig) (*ssh.Client, error) {
	if c, ok := p.Peek(target.Name); ok {
		p.acquire(target.Name)
		return c, nil
	}

	v, err, _ := p.group.Do(target.Name, func() (interface{}, error) {
		// Re-check: another goroutine may have finished dialing and
		// registered the entry between our Peek above and entering Do.
		if c, ok := p.Peek(target.Name); ok {
			return c, nil
		}
		client, dialErr := dialSSHContext(ctx, target.Addr, config)
		if dialErr != nil {
			return nil, dialErr
		}
		p.register(target.Name, client)
		return client, nil
	})
	if err != nil {
		return nil, err
	}
	client, ok := v.(*ssh.Client)
	if !ok {
		return nil, fmt.Errorf("ssh pool: unexpected singleflight result type %T for %s", v, target.Name)
	}
	p.acquire(target.Name)
	return client, nil
}

// register stores a newly-dialed client for name and starts a background
// watcher that evicts the entry once the connection dies, so the next
// GetOrDial call redials rather than reusing (or hanging on) a dead client
// indefinitely.
func (p *SSHClientPool) register(name string, client *ssh.Client) {
	p.mu.Lock()
	p.entries[name] = &pooledSSHClient{client: client}
	p.mu.Unlock()

	p.notifySubscribers(name, client)

	go func() {
		// Client.Wait blocks until the underlying connection closes, by any
		// means (remote close, network death, or our own Remove/Evict
		// calling client.Close()). Its return is exactly the "detected dead
		// connection" teardown trigger from the Design Decision.
		_ = client.Wait()
		p.evictIfCurrent(name, client)
	}()
}

func (p *SSHClientPool) acquire(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.entries[name]; ok {
		e.refCount++
	}
}

// Release decrements the reference count for name. It never closes the
// underlying client itself -- per the Design Decision, the last
// channel/session closing does not tear down the shared connection; only
// Remove (explicit remote-config removal) or a detected dead connection
// does.
func (p *SSHClientPool) Release(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.entries[name]; ok && e.refCount > 0 {
		e.refCount--
	}
}

// RefCount reports the current reference count for name (0 if not pooled).
// Exposed for tests and observability, not for teardown decisions.
func (p *SSHClientPool) RefCount(name string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.entries[name]; ok {
		return e.refCount
	}
	return 0
}

// Remove force-closes and evicts the pooled client for name regardless of
// reference count -- the explicit-remote-config-removal teardown trigger.
// A no-op returning nil if name has no pooled entry.
func (p *SSHClientPool) Remove(name string) error {
	p.mu.Lock()
	e, ok := p.entries[name]
	if ok {
		delete(p.entries, name)
	}
	p.mu.Unlock()
	if !ok {
		return nil
	}
	return e.client.Close()
}

// Evict removes the pooled entry for name only if its current client is
// exactly client, force-closing it. A no-op if name has already been
// replaced by a newer dial (avoids evicting a fresh, unrelated connection
// out from under a concurrent caller). Used by SSHRunner when a session-
// level call fails in a way that indicates the pooled connection itself is
// dead, to accelerate the next redial rather than waiting on Client.Wait().
func (p *SSHClientPool) Evict(name string, client *ssh.Client) {
	p.evictIfCurrent(name, client)
}

func (p *SSHClientPool) evictIfCurrent(name string, client *ssh.Client) {
	p.mu.Lock()
	e, ok := p.entries[name]
	if !ok || e.client != client {
		p.mu.Unlock()
		return
	}
	delete(p.entries, name)
	p.mu.Unlock()
	_ = client.Close()
}

// dialSSHContext dials addr over TCP and performs the SSH handshake against
// config, with the whole operation bounded by ctx -- golang.org/x/crypto/ssh
// has no native context support (research/pitfalls.md §4), so ctx.Done() is
// raced against the handshake in a goroutine, force-closing the raw TCP
// connection on expiry to unblock it rather than leaking the goroutine
// silently. This is safe to force-close on timeout because, at this point,
// the connection has not yet been registered in the pool and is not shared
// with any other caller.
func dialSSHContext(ctx context.Context, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ssh: tcp dial %s: %w", addr, err)
	}

	type handshakeResult struct {
		client *ssh.Client
		err    error
	}
	resCh := make(chan handshakeResult, 1)
	go func() {
		sshConn, chans, reqs, hsErr := ssh.NewClientConn(conn, addr, config)
		if hsErr != nil {
			resCh <- handshakeResult{err: hsErr}
			return
		}
		resCh <- handshakeResult{client: ssh.NewClient(sshConn, chans, reqs)}
	}()

	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("ssh: handshake %s: %w", addr, res.err)
		}
		return res.client, nil
	case <-ctx.Done():
		_ = conn.Close() // force-close to unblock the handshake goroutine
		return nil, fmt.Errorf("ssh: dial %s: %w", addr, ctx.Err())
	}
}
