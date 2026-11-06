package credential

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

const badToken = "ghp_DUMMYDUMMYDUMMYDUMMY0000000000000000"

type fbEnv struct {
	http  *srvStats
	ssh   *sshServer
	fetch *Fetcher
	repo  *git.Repository
	hURL  string
}

// newFB: HTTP server rejects badToken, SSH server accepts the agent key.
func newFB(t *testing.T, fallback bool, agentAllowed bool, rw *Rewriter) *fbEnv {
	root, _ := seedBare(t)
	srv, hst := gitHTTP(t, root, "u", "real-token-dummy", "")
	agentKey := startAgent(t)
	var allowed = agentKey
	if !agentAllowed {
		allowed = nil
	}
	ss := newSSHServer(t, root, allowed)
	writeKnownHosts(t, knownHostsLine(ss.Addr, ss.HostKey.PublicKey()))
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	hURL := srv.URL + "/repo.git"
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{hURL}, Fetch: []config.RefSpec{"+refs/heads/*:refs/remotes/origin/*"}}); err != nil {
		t.Fatal(err)
	}
	if rw == nil {
		rw, _ = LoadRewriter()
	}
	f := &Fetcher{
		Provider: &Provider{Sources: []TokenSource{{Name: "keychain", Lookup: func(string) (string, string, bool) { return "u", badToken, true }}}},
		Rewriter: rw, Fallback: fallback,
		SSHURLFn: func(string) (string, bool) { return "ssh://git@" + ss.Addr + "/repo.git", true },
	}
	return &fbEnv{hst, ss, f, repo, hURL}
}

func TestFallback_should_RetryOnceViaSSH_When_SettingTrue_AndNeverSendTokenToSSHHost(t *testing.T) {
	e := newFB(t, true, true, nil)
	err := e.fetch.Fetch(context.Background(), e.repo, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if e.fetch.HTTPAttempts != 1 || e.fetch.SSHAttempts != 1 {
		t.Fatalf("attempts http=%d ssh=%d", e.fetch.HTTPAttempts, e.fetch.SSHAttempts)
	}
	if e.http.WithAuth.Load() == 0 {
		t.Fatal("HTTPS attempt never carried the (bad) token; test vacuous")
	}
	for _, a := range e.http.AuthHeaders { // sanity: header is basic auth of the dummy token
		if !strings.HasPrefix(a, "Basic ") {
			t.Fatalf("unexpected header %q", a)
		}
	}
	if len(e.ssh.Passwords) != 0 {
		t.Fatalf("SSH server received password auth: %v", e.ssh.Passwords)
	}
	if e.ssh.Execs.Load() != 1 || e.ssh.Conns.Load() < 1 {
		t.Fatalf("ssh execs=%d conns=%d", e.ssh.Execs.Load(), e.ssh.Conns.Load())
	}
	for _, u := range e.ssh.Users {
		if strings.Contains(u, "ghp_") || strings.Contains(u, "u:") {
			t.Fatalf("token material in SSH user: %q", u)
		}
	}
	refs, _ := e.repo.References()
	n := 0
	refs.ForEach(func(r *plumbingRef) error { n++; return nil })
	if n == 0 {
		t.Fatal("fetch via SSH brought no refs")
	}
	if strings.Contains(e.fetch.lastSSHURL, "@") && strings.Contains(e.fetch.lastSSHURL, badToken) {
		t.Fatal("token in ssh URL")
	}
}

func TestFallback_should_NotRetryOverSSH_When_SettingFalse(t *testing.T) {
	e := newFB(t, false, true, nil)
	err := e.fetch.Fetch(context.Background(), e.repo, "origin")
	if !IsAuthFailure(err) {
		t.Fatalf("want auth failure, got %v", err)
	}
	if e.fetch.SSHAttempts != 0 || e.ssh.Conns.Load() != 0 {
		t.Fatalf("ssh attempted: %d / %d", e.fetch.SSHAttempts, e.ssh.Conns.Load())
	}
}

func TestFallback_should_StopAfterOneSSHAttempt_When_SSHAlsoFails(t *testing.T) {
	e := newFB(t, true, false, nil)
	err := e.fetch.Fetch(context.Background(), e.repo, "origin")
	if err == nil {
		t.Fatal("expected failure")
	}
	if e.fetch.HTTPAttempts != 1 || e.fetch.SSHAttempts != 1 {
		t.Fatalf("not single-retry: http=%d ssh=%d", e.fetch.HTTPAttempts, e.fetch.SSHAttempts)
	}
	t.Logf("both-fail error: %v", err)
}

func TestFallback_should_NotRetry_When_FailureIsNotAuth(t *testing.T) {
	root, _ := seedBare(t)
	srv, _ := gitHTTP(t, root, "u", "ok-dummy", "")
	repo, _ := git.PlainInit(t.TempDir(), false)
	repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{srv.URL + "/missing.git"}})
	rw, _ := LoadRewriter()
	f := &Fetcher{Provider: &Provider{Sources: []TokenSource{{Name: "k", Lookup: func(string) (string, string, bool) { return "u", "ok-dummy", true }}}},
		Rewriter: rw, Fallback: true, SSHURLFn: func(string) (string, bool) { return "ssh://git@127.0.0.1:1/x", true }}
	err := f.Fetch(context.Background(), repo, "origin")
	t.Logf("missing-repo error: %v", err)
	if err == nil || IsAuthFailure(err) || f.SSHAttempts != 0 {
		t.Fatalf("err=%v ssh=%d", err, f.SSHAttempts)
	}
}

func TestFetch_should_UseSSHURL_When_InsteadOfRewritesHTTPS(t *testing.T) {
	// Build env first to learn ports, then craft a global config rewriting the HTTP base to SSH.
	e := newFB(t, false, true, nil)
	httpBase := e.hURL[:strings.LastIndex(e.hURL, "/")+1]
	cfgPath := t.TempDir() + "/gc"
	writeFile(t, cfgPath, fmt.Sprintf("[url \"ssh://git@%s/\"]\n\tinsteadOf = %s\n", e.ssh.Addr, httpBase))
	rw, _ := LoadRewriter(cfgPath)
	e.fetch.Rewriter = rw
	if err := e.fetch.Fetch(context.Background(), e.repo, "origin"); err != nil {
		t.Fatal(err)
	}
	if e.fetch.HTTPAttempts != 0 || e.fetch.SSHAttempts != 1 || e.http.Requests.Load() != 0 {
		t.Fatalf("http=%d ssh=%d httpreqs=%d", e.fetch.HTTPAttempts, e.fetch.SSHAttempts, e.http.Requests.Load())
	}
}

var _ transport.AuthMethod
