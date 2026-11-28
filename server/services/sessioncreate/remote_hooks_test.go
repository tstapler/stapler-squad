package sessioncreate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/sshtransport"
)

type unusedPermissionHandler struct{}

func (unusedPermissionHandler) HandlePermissionRequest(http.ResponseWriter, *http.Request) {}

// relayLoops counts the running poll loops of remote approval relays.
func relayLoops() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "sshremote.(*RemoteApprovalRelay).run(")
}

// loopbackRemote is an execution target whose SSH server runs in this process
// and answers every command with handler. It never leaves loopback.
func loopbackRemote(t *testing.T, handler gliderssh.Handler) (instance.RemoteExecutionTarget, *sshtransport.SSHClientPool) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromSigner(priv)
	require.NoError(t, err)
	srv := &gliderssh.Server{Handler: handler, PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool { return true }}
	srv.AddHostKey(signer)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	pool := sshtransport.NewSSHClientPool()
	runner := sshtransport.NewSSHRunner(sshtransport.SSHTarget{Name: t.Name(), Addr: ln.Addr().String()},
		ssh.ClientConfig{User: "u", Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: ssh.FixedHostKey(signer.PublicKey())},
		sshtransport.WithSSHClientPool(pool))
	require.NoError(t, runner.Dial(context.Background()))
	return instance.NewRemoteExecutionTarget(instance.RemoteTarget{Name: t.Name()}, runner), pool
}

func TestSetupRemoteApprovalHooks_ShouldStopTheRelay_When_HookInjectionFails(t *testing.T) {
	// Not t.Parallel(): counts relay goroutines of the whole process.
	var duringInjection atomic.Int32
	target, pool := loopbackRemote(t, func(s gliderssh.Session) {
		duringInjection.Store(int32(relayLoops())) //nolint:gosec // a goroutine count
		_ = s.Exit(1)
	})
	f := newFixture(t)
	f.deps.SSHClientPool = func() *sshtransport.SSHClientPool { return pool }
	f.deps.PermissionHandler = func() sshremote.PermissionRequestHandler { return unusedPermissionHandler{} }
	inst, err := instance.NewInstance(instance.InstanceOptions{Title: "relay-stop", Path: t.TempDir(), ExecutionTarget: target})
	require.NoError(t, err)

	err = f.handler().setupRemoteApprovalHooks(inst, "/srv/work", inst.Title)

	require.ErrorContains(t, err, "inject remote hook config")
	require.Equal(t, int32(1), duringInjection.Load(), "the relay must have been running while the hook was injected")
	assert.Equal(t, 0, relayLoops(), "the relay must be stopped when injection fails")
}
