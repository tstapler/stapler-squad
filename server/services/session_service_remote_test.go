package services

// session_service_remote_test.go covers ssh-remote-workspaces Phase 4, Epic 4.2,
// Story 4.2.1's CreateSession remote-target mode-specific block: an unknown
// remote name (Task 4.2.1g), a full success round trip verified via a second,
// independent SSH dial (the Story's AC1 scenario), and partial-failure
// compensating cleanup + connection-drop-during-cleanup (Task 4.2.1f).
//
// These start real in-process SSH servers (github.com/gliderlabs/ssh) that
// execute real shell commands -- session_service.go's remote block runs real
// `git branch`/`git worktree add`/`tmux ...` invocations, and there is no
// meaningful way to verify CreateSession's orchestration of them without a
// real shell and real git/tmux binaries on the other end. This mirrors
// session/git/remote_worktree_test.go and session/tmux/ssh_test_server_test.go's
// own real-exec test servers -- duplicated here (not imported) since _test.go
// files aren't importable across package boundaries, same as
// remote_service_test.go's startRemoteTestSSHServer already documents.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	gliderssh "github.com/gliderlabs/ssh"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/sshremote"
)

// remoteSessionTestSSHServer is this file's own minimal in-process SSH server,
// separate from remote_service_test.go's startRemoteTestSSHServer (whose
// handler just exits(0) -- fine for TOFU-flow tests, useless here since these
// tests need real git/tmux command execution).
type remoteSessionTestSSHServer struct {
	Addr    string
	HostKey ssh.PublicKey
}

// realExecSessionSSHHandler runs s.RawCommand() through a real shell, wiring
// stdin/stdout/stderr directly to the SSH session -- mirrors
// session/git/ssh_test_server_test.go's testGitSSHHandler and
// session/tmux/ssh_test_server_test.go's realExecSSHHandler.
func realExecSessionSSHHandler(s gliderssh.Session) {
	raw := s.RawCommand()
	if raw == "" {
		_ = s.Exit(0)
		return
	}
	cmd := exec.CommandContext(s.Context(), "sh", "-c", raw)
	cmd.Stdin = s
	cmd.Stdout = s
	cmd.Stderr = s.Stderr()
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			_ = s.Exit(exitErr.ExitCode())
			return
		}
		_ = s.Exit(1)
		return
	}
	_ = s.Exit(0)
}

// startRemoteSessionTestSSHServer starts a real-exec in-process SSH server
// listening on an OS-assigned loopback port, torn down via t.Cleanup.
func startRemoteSessionTestSSHServer(t *testing.T) *remoteSessionTestSSHServer {
	t.Helper()
	return startRemoteSessionTestSSHServerWithHandler(t, realExecSessionSSHHandler, nil)
}

// startTmuxFailingTestSSHServer is like startRemoteSessionTestSSHServer, but
// any command line naming the tmux binary fails immediately (exit 1) while
// every other command (git, test, mkdir, sh) still executes for real --
// simulating a remote host whose SSH connection and git tooling are healthy
// but whose tmux is broken/missing (Task 4.2.1f's "tmux setup fails, worktree
// cleanup succeeds" case).
func startTmuxFailingTestSSHServer(t *testing.T) *remoteSessionTestSSHServer {
	t.Helper()
	handler := func(s gliderssh.Session) {
		if strings.Contains(s.RawCommand(), "tmux") {
			_ = s.Exit(1)
			return
		}
		realExecSessionSSHHandler(s)
	}
	return startRemoteSessionTestSSHServerWithHandler(t, handler, nil)
}

// startMaxChannelsTestSSHServer is like startRemoteSessionTestSSHServer, but
// rejects (at the SSH protocol level, ssh.ResourceShortage) any
// session-channel-open request past the maxChannels'th ever opened on the
// connection -- an otherwise perfectly healthy connection that simply cannot
// carry more commands, closely simulating a connection that dies partway
// through a multi-command sequence. Used for Task 4.2.1f's "connection drops
// during compensating cleanup" case: unlike startMaxSessionsTestServer
// (session/tmux/ssh_test_server_test.go), which limits CONCURRENT open
// channels and releases the budget as each closes, this counter is
// cumulative and never released -- SSHRunner opens and closes one channel
// per command, so a concurrent-limit of any size would never actually reject
// a purely sequential command stream like this one.
func startMaxChannelsTestSSHServer(t *testing.T, maxChannels int) *remoteSessionTestSSHServer {
	t.Helper()
	var opened atomic.Int64
	channelHandlers := map[string]gliderssh.ChannelHandler{
		"session": func(srv *gliderssh.Server, conn *ssh.ServerConn, newChan ssh.NewChannel, ctx gliderssh.Context) {
			if opened.Add(1) > int64(maxChannels) {
				_ = newChan.Reject(ssh.ResourceShortage, "max channels reached (test)")
				return
			}
			gliderssh.DefaultSessionHandler(srv, conn, newChan, ctx)
		},
	}
	return startRemoteSessionTestSSHServerWithHandler(t, realExecSessionSSHHandler, channelHandlers)
}

func startRemoteSessionTestSSHServerWithHandler(t *testing.T, handler gliderssh.Handler, channelHandlers map[string]gliderssh.ChannelHandler) *remoteSessionTestSSHServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	hostSigner, err := ssh.NewSignerFromSigner(priv)
	require.NoError(t, err)

	srv := &gliderssh.Server{
		Handler: handler,
		PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool {
			return true
		},
		ChannelHandlers: channelHandlers,
	}
	srv.AddHostKey(hostSigner)

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})

	return &remoteSessionTestSSHServer{Addr: ln.Addr().String(), HostKey: hostSigner.PublicKey()}
}

// remoteSessionTestClientAuth returns an ssh.AuthMethod for a throwaway
// client keypair (the test servers' PublicKeyHandler accepts any key).
func remoteSessionTestClientAuth(t *testing.T) ssh.AuthMethod {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromSigner(priv)
	require.NoError(t, err)
	return ssh.PublicKeys(signer)
}

// initRemoteSessionTestRepo creates a real local git repository at dir with
// an initial commit on "main", standing in for the "existing repo already on
// the remote host" resolvedPath (req.Msg.Path) CreateSession's remote block
// expects -- the test SSH server executes real shell commands against the
// local filesystem, so a real local repo IS the "remote" repo.
func initRemoteSessionTestRepo(t *testing.T, dir string) {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	readme := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("test repo\n"), 0o644))
	_, err = wt.Add("README.md")
	require.NoError(t, err)
	sig := &object.Signature{Name: "Test", Email: "test@localhost", When: time.Now()}
	_, err = wt.Commit("initial commit", &gogit.CommitOptions{Author: sig})
	require.NoError(t, err)
}

// remoteSessionFixture wires a SessionService with real remote-session
// support: a config.json (isolated via STAPLER_SQUAD_TEST_DIR) naming one
// remote pointed at a test SSH server, a real KnownHostsStore that trusts
// that server's host key, and a real KeyStore (backed by go-keyring's mock)
// holding a generated identity for it.
type remoteSessionFixture struct {
	*forkTestFixture
	repoPath string
	basePath string
}

// newRemoteSessionFixture builds the fixture described above against srv,
// naming the remote "test-remote" with basePath as its configured base_path.
func newRemoteSessionFixture(t *testing.T, srv *remoteSessionTestSSHServer) *remoteSessionFixture {
	t.Helper()
	keyring.MockInit()

	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

	repoPath := t.TempDir()
	initRemoteSessionTestRepo(t, repoPath)
	basePath := t.TempDir()

	knownHosts, err := sshremote.NewKnownHostsStore()
	require.NoError(t, err)
	require.NoError(t, knownHosts.Trust(srv.Addr, srv.HostKey))

	keyStore := sshremote.NewKeyStore()
	_, err = keyStore.GenerateAndStoreIdentity(context.Background(), "test-remote")
	require.NoError(t, err)

	cfg := &config.Config{
		BranchPrefix: "session/",
		Remotes: []config.RemoteConfig{
			{Name: "test-remote", Host: srv.Addr, User: "testuser", BasePath: basePath, IdentityRef: "test-remote"},
		},
	}
	require.NoError(t, config.SaveConfig(cfg))

	fix := setupForkTestFixture(t)
	fix.svc.SetRemoteDeps(keyStore, knownHosts)

	return &remoteSessionFixture{forkTestFixture: fix, repoPath: repoPath, basePath: basePath}
}

// remoteHasSessionViaIndependentDial dials srv independently of fix.svc's own
// runner (a fresh *ssh.Client) and reports whether tmux reports sessionName
// as present on that host -- AC1's "verified via a second, independent SSH
// dial in the test" requirement, proving the session exists on the remote
// host's tmux server rather than merely trusting CreateSession's own success
// return. serverSocket must be the same -L socket the SessionService under
// test applies to every Instance it creates (SessionService.testTmuxServerSocket,
// readable here since this file shares the services package) -- under
// config.IsTestMode() each SessionService gets its own isolated per-instance
// tmux -L socket distinct from the package-wide test-isolation fallback, so
// checking the unscoped default server would always report the session absent.
func remoteHasSessionViaIndependentDial(t *testing.T, srv *remoteSessionTestSSHServer, sessionName, serverSocket string) bool {
	t.Helper()
	client, err := ssh.Dial("tcp", srv.Addr, &ssh.ClientConfig{
		User:            "testuser",
		Auth:            []ssh.AuthMethod{remoteSessionTestClientAuth(t)},
		HostKeyCallback: ssh.FixedHostKey(srv.HostKey),
		Timeout:         5 * time.Second,
	})
	require.NoError(t, err)
	defer client.Close()

	sess, err := client.NewSession()
	require.NoError(t, err)
	defer sess.Close()

	cmd := "tmux has-session -t " + sessionName
	if serverSocket != "" {
		cmd = "tmux -L " + serverSocket + " has-session -t " + sessionName
	}
	err = sess.Run(cmd)
	return err == nil
}

// TestCreateSession_RemoteTarget_UnknownRemoteName_ReturnsInvalidArgument is
// Task 4.2.1g's core case: an unknown remote_name must fail fast with
// CodeInvalidArgument, naming the remote, before any worktree/tmux work
// begins -- no SSH server is even started for this test, since resolveRemoteTarget
// must reject the request before attempting to dial anything.
func TestCreateSession_RemoteTarget_UnknownRemoteName_ReturnsInvalidArgument(t *testing.T) {
	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
	require.NoError(t, config.SaveConfig(&config.Config{}))

	fix := setupForkTestFixture(t)

	_, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "remote-unknown-name",
		Path:        t.TempDir(),
		Branch:      "feature-x",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		Remote:      &sessionv1.RemoteTarget{RemoteName: "does-not-exist"},
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	require.Contains(t, connectErr.Message(), "does-not-exist")

	data, listErr := fix.storage.ListInstanceData()
	require.NoError(t, listErr)
	for _, d := range data {
		require.NotEqual(t, "remote-unknown-name", d.Title, "no instance should be persisted when the remote name doesn't resolve")
	}
}

// TestCreateSession_RemoteTarget_CreatesRemoteWorktreeAndTmuxSession is Story
// 4.2.1's AC1 scenario: a CreateSession request naming a configured remote
// creates the worktree and tmux session on that remote host, not locally.
// Skipped under -short (starts a real tmux session, mirroring
// TestSpawnReviewSession_SetsBacklogCategory in session_service_test.go).
func TestCreateSession_RemoteTarget_CreatesRemoteWorktreeAndTmuxSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts a real tmux session")
	}
	srv := startRemoteSessionTestSSHServer(t)
	fix := newRemoteSessionFixture(t, srv)

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "remote-create-worktree",
		Path:        fix.repoPath,
		Branch:      "feature-x",
		Program:     "sh",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		Remote:      &sessionv1.RemoteTarget{RemoteName: "test-remote"},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)

	sessionName := "staplersquad_remote-create-worktree"
	require.Eventually(t, func() bool {
		return remoteHasSessionViaIndependentDial(t, srv, sessionName, fix.svc.testTmuxServerSocket)
	}, 10*time.Second, 200*time.Millisecond, "remote tmux session must exist on the remote host")

	worktreePath := filepath.Join(fix.basePath, "remote-create-worktree")
	info, statErr := os.Stat(worktreePath)
	require.NoError(t, statErr, "remote worktree directory must exist under the configured base_path")
	require.True(t, info.IsDir())

	// The instance's own ExecutionTarget must report remote, per AC1.
	// ExecutionTarget is in-memory-only (json:"-": a live SSH connection can't
	// be serialized), so this reads the live instance CreateSession registered
	// with the poller rather than storage.
	var found *session.Instance
	for _, inst := range fix.poller.GetInstances() {
		if inst.Title == "remote-create-worktree" {
			found = inst
			break
		}
	}
	require.NotNil(t, found, "created instance must be registered with the poller")
	require.True(t, found.ExecutionTarget != nil && found.ExecutionTarget.IsRemote(),
		"Instance.ExecutionTarget.IsRemote() must be true for a remote-created session")
}

// TestCreateSession_RemoteTarget_TmuxSetupFails_CleansUpWorktree is Task
// 4.2.1f's first case: RemoteWorktreeOps.CreateWorktree succeeds but the
// subsequent remote tmux session-setup step fails -- CreateSession must
// attempt (and here, succeed at) a best-effort RemoveWorktree before
// surfacing the original error, so the remote worktree directory does not
// survive a failed creation attempt.
func TestCreateSession_RemoteTarget_TmuxSetupFails_CleansUpWorktree(t *testing.T) {
	srv := startTmuxFailingTestSSHServer(t)
	fix := newRemoteSessionFixture(t, srv)

	_, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "remote-tmux-fails",
		Path:        fix.repoPath,
		Branch:      "feature-y",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		Remote:      &sessionv1.RemoteTarget{RemoteName: "test-remote"},
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInternal, connectErr.Code())
	require.NotContains(t, connectErr.Message(), "orphaned",
		"cleanup succeeded here -- the error must not claim the path may be orphaned")

	worktreePath := filepath.Join(fix.basePath, "remote-tmux-fails")
	_, statErr := os.Stat(worktreePath)
	require.True(t, os.IsNotExist(statErr), "remote worktree directory must have been removed by the compensating cleanup")
}

// TestCreateSession_RemoteTarget_ConnectionDropDuringCleanup_SurfacesOrphanWarning
// is Task 4.2.1f's second case: the connection degrades enough that neither
// the remote tmux session-setup step nor the compensating RemoveWorktree
// call can complete. The surfaced error must explicitly name the path as
// possibly orphaned rather than failing silently or claiming success.
//
// maxChannels=5 allows exactly the 5 real, successful SSH channels
// CreateSession's remote block opens before attempting to start the remote
// tmux session (git branch, git worktree add's base_path test -d, git
// worktree add itself, EnsureRemoteSession's has-session check, and its own
// remote test -d workDir check -- see tmux.go's EnsureRemoteSession), then
// rejects every channel after that: the new-session attempt, its
// has-session recheck, and the compensating RemoveWorktree's git worktree
// remove all fail.
func TestCreateSession_RemoteTarget_ConnectionDropDuringCleanup_SurfacesOrphanWarning(t *testing.T) {
	srv := startMaxChannelsTestSSHServer(t, 5)
	fix := newRemoteSessionFixture(t, srv)

	_, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "remote-connection-drops",
		Path:        fix.repoPath,
		Branch:      "feature-z",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		Remote:      &sessionv1.RemoteTarget{RemoteName: "test-remote"},
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInternal, connectErr.Code())
	require.Contains(t, connectErr.Message(), "orphaned",
		"error must explicitly name the remote path as possibly orphaned, not fail silently")
	worktreePath := filepath.Join(fix.basePath, "remote-connection-drops")
	require.Contains(t, connectErr.Message(), worktreePath)
}
