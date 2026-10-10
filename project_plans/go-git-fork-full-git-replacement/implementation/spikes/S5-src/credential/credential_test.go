package credential

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

func hostOf(t *testing.T, raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func TestClient_should_RoundTripStubHelperAndCloneWithZeroGitSpawns(t *testing.T) {
	sh := newShim(t)
	root, head := seedBare(t)
	srv, st := gitHTTP(t, root, "stubuser", "stub-dummy-pw", "")
	host := hostOf(t, srv.URL)
	actions := stubHelper(t, host, "stubuser", "stub-dummy-pw")

	h, err := ResolveHelper("s5stub")
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{Helpers: []Helper{h}}
	res, err := p.Fill(context.Background(), Request{Protocol: "http", Host: host})
	if err != nil || res.From != "helper" || res.Password != "stub-dummy-pw" {
		t.Fatalf("fill: %+v %v", res, err)
	}
	dir := t.TempDir()
	repo, err := git.PlainClone(dir, false, &git.CloneOptions{URL: srv.URL + "/repo.git", Auth: NewHostAuth(host, res.Credential)})
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := repo.Head()
	if ref.Hash().String() != head {
		t.Fatalf("head %s != %s", ref.Hash(), head)
	}
	if sp := sh.Spawns(); len(sp) != 0 {
		t.Fatalf("expected zero git spawns, got %v", sp)
	}
	b, _ := os.ReadFile(actions)
	if strings.TrimSpace(string(b)) != "get" {
		t.Fatalf("helper actions: %q", b)
	}
	if st.WithAuth.Load() == 0 {
		t.Fatal("server never saw auth")
	}
}

func TestClone_should_ReturnAuthRequired_When_NoOrWrongCredential(t *testing.T) {
	root, _ := seedBare(t)
	srv, _ := gitHTTP(t, root, "u", "right-dummy", "")
	host := hostOf(t, srv.URL)
	for name, auth := range map[string]transport.AuthMethod{
		"none":  nil,
		"wrong": NewHostAuth(host, Credential{"u", "wrong-dummy"}),
	} {
		_, err := git.PlainClone(t.TempDir(), false, &git.CloneOptions{URL: srv.URL + "/repo.git", Auth: auth})
		if !errors.Is(err, transport.ErrAuthenticationRequired) || !IsAuthFailure(err) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestHostAuth_should_NotAttachCredential_When_RequestHostDiffers(t *testing.T) {
	a := NewHostAuth("ghe.example.invalid", Credential{"u", "p"})
	r, _ := http.NewRequest("GET", "https://github.com/x", nil)
	a.SetAuth(r)
	if r.Header.Get("Authorization") != "" {
		t.Fatal("credential leaked to other host")
	}
	r, _ = http.NewRequest("GET", "https://ghe.example.invalid/x", nil)
	a.SetAuth(r)
	if r.Header.Get("Authorization") == "" {
		t.Fatal("credential not attached to own host")
	}
	if strings.Contains(a.String(), "p") && strings.Contains(a.String(), ":p") {
		t.Fatal("String() leaks password")
	}
}

func TestProvider_should_ScopeTokenSourcesByHostAndFallBackToHelper(t *testing.T) {
	newShim(t)
	stubHelper(t, "helper-only.invalid", "hu", "helper-dummy-pw")
	h, _ := ResolveHelper("s5stub")
	p := &Provider{
		Sources: []TokenSource{{Name: "keychain", Lookup: func(host string) (string, string, bool) {
			switch host {
			case "github.com":
				return "x-access-token", "dummy-gh-com-token", true
			case "ghe.example.invalid":
				return "x-access-token", "dummy-ghe-token", true
			}
			return "", "", false
		}}},
		Helpers: []Helper{h},
	}
	ctx := context.Background()
	r, _ := p.Fill(ctx, Request{Protocol: "https", Host: "ghe.example.invalid"})
	if r.Password != "dummy-ghe-token" || r.From != "keychain" {
		t.Fatalf("ghe: %+v", r)
	}
	r, _ = p.Fill(ctx, Request{Protocol: "https", Host: "helper-only.invalid"})
	if r.From != "helper" || r.Password != "helper-dummy-pw" {
		t.Fatalf("helper fallback: %+v", r)
	}
	if _, err := p.Fill(ctx, Request{Protocol: "https", Host: "unknown.invalid"}); err == nil {
		t.Fatal("expected no credential")
	}
}

func TestHelper_should_KillProcessGroup_When_HelperTimesOut(t *testing.T) {
	dir := t.TempDir()
	pidf := filepath.Join(dir, "pid")
	script := filepath.Join(dir, "git-credential-hang")
	os.WriteFile(script, []byte("#!/bin/sh\nsleep 60 &\necho $! > "+pidf+"\nwait\n"), 0o755)
	h, err := ResolveHelper(script)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = h.Get(context.Background(), Request{Protocol: "https", Host: "x.invalid"}, 300*time.Millisecond)
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("expected timeout quickly, got %v after %v", err, time.Since(start))
	}
	b, _ := os.ReadFile(pidf)
	var pid int
	for _, c := range strings.TrimSpace(string(b)) {
		pid = pid*10 + int(c-'0')
	}
	time.Sleep(100 * time.Millisecond)
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("grandchild %d survived", pid)
	}
}

func TestRealHelpers_should_ExecDirectlyWithZeroGitSpawns_OnDummyHosts(t *testing.T) {
	sh := newShim(t)
	// The system helper is not on PATH on macOS; found via KnownExecDirs without `git --exec-path`.
	os.Setenv("PATH", strings.ReplaceAll(os.Getenv("PATH"), execPath, "")) // belt and braces
	osx, err := ResolveHelper("osxkeychain")
	if err != nil {
		t.Skipf("osxkeychain helper not installed: %v", err)
	}
	c, err := osx.Get(context.Background(), Request{Protocol: "https", Host: "s5-dummy.invalid"}, 5*time.Second)
	t.Logf("osxkeychain get on dummy host -> user=%q pw-empty=%v err=%v argv=%v", c.Username, c.Password == "", err, osx.Argv)
	if c.Password != "" {
		t.Fatal("unexpected credential for dummy host")
	}

	ghdir := t.TempDir()
	os.WriteFile(filepath.Join(ghdir, "hosts.yml"), []byte("ghe.s5.invalid:\n    user: dummyuser\n    oauth_token: dummy-ghe-token-0000\n    git_protocol: https\n"), 0o600)
	t.Setenv("GH_CONFIG_DIR", ghdir)
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
		t.Setenv(k, "")
	}
	gh, err := ResolveHelper("!gh auth git-credential")
	if err != nil {
		t.Fatal(err)
	}
	c, err = gh.Get(context.Background(), Request{Protocol: "https", Host: "ghe.s5.invalid"}, 10*time.Second)
	t.Logf("gh get on dummy GHE host -> user=%q token-matches=%v err=%v", c.Username, c.Password == "dummy-ghe-token-0000", err)
	if c.Password != "dummy-ghe-token-0000" {
		t.Fatalf("gh helper did not return dummy token: err=%v", err)
	}
	c, _ = gh.Get(context.Background(), Request{Protocol: "https", Host: "other.s5.invalid"}, 10*time.Second)
	if c.Password != "" {
		t.Fatal("gh returned a credential for an unconfigured host")
	}
	if sp := sh.Spawns(); len(sp) != 0 {
		t.Fatalf("git spawned: %v", sp)
	}
}

type countingRT struct {
	rt http.RoundTripper
	n  atomic.Int64
}

func (c *countingRT) RoundTrip(r *http.Request) (*http.Response, error) {
	c.n.Add(1)
	return c.rt.RoundTrip(r)
}

func restoreProtocols(t *testing.T) {
	old := map[string]transport.Transport{}
	for k, v := range client.Protocols {
		old[k] = v
	}
	t.Cleanup(func() {
		for k := range client.Protocols {
			delete(client.Protocols, k)
		}
		for k, v := range old {
			client.Protocols[k] = v
		}
	})
}

func TestInstallProtocol_should_RouteGoGitThroughCustomHTTPClient(t *testing.T) {
	restoreProtocols(t)
	root, head := seedBare(t)
	srv, _ := gitHTTP(t, root, "u", "p-dummy", "")
	host := hostOf(t, srv.URL)
	rt := &countingRT{rt: http.DefaultTransport}
	Install(NewHTTPClient(rt, 16))
	repo, err := git.PlainClone(t.TempDir(), false, &git.CloneOptions{URL: srv.URL + "/repo.git", Auth: NewHostAuth(host, Credential{"u", "p-dummy"})})
	if err != nil {
		t.Fatal(err)
	}
	if h, _ := repo.Head(); h.Hash().String() != head {
		t.Fatal("head mismatch")
	}
	if rt.n.Load() == 0 {
		t.Fatal("custom RoundTripper was not used")
	}
	t.Logf("custom RoundTripper saw %d requests", rt.n.Load())

	// Known limitation: per-endpoint TLS options clone the transport and need *http.Transport.
	// With the transport cache enabled go-git v5.19.2 PANICS (unchecked assertion at
	// plumbing/transport/http/common.go:323); with the cache off it returns an error.
	tlsClone := func() (err error, panicked any) {
		defer func() { panicked = recover() }()
		_, err = git.PlainClone(t.TempDir(), false, &git.CloneOptions{URL: srv.URL + "/repo.git", InsecureSkipTLS: true, Auth: NewHostAuth(host, Credential{"u", "p-dummy"})})
		return
	}
	_, pn := tlsClone()
	t.Logf("custom non-*http.Transport + InsecureSkipTLS + cache=16 -> panic=%v", pn)
	if pn == nil {
		t.Fatal("expected the documented panic")
	}
	Install(NewHTTPClient(rt, 0))
	err, pn = tlsClone()
	t.Logf("custom non-*http.Transport + InsecureSkipTLS + cache=0 -> err=%v panic=%v", err, pn)
	if pn != nil || err == nil || !strings.Contains(err.Error(), "expected underlying client transport") {
		t.Fatalf("expected a clean error with cache off, got err=%v panic=%v", err, pn)
	}
}

func cloneThroughRedirect(t *testing.T, sameHostnameOtherPort bool, install bool) (a, b *srvStats, err error) {
	if install {
		restoreProtocols(t)
		Install(NewHTTPClient(http.DefaultTransport, 16))
	}
	root, _ := seedBare(t)
	srvB, stB := gitHTTP(t, root, "", "", "")
	target := srvB.URL
	if !sameHostnameOtherPort {
		target = "http://localhost:" + strings.Split(srvB.URL, ":")[2] // different Hostname()
	}
	srvA, stA := gitHTTP(t, root, "u", "p-dummy", target)
	host := hostOf(t, srvA.URL)
	_, err = git.PlainClone(t.TempDir(), false, &git.CloneOptions{URL: srvA.URL + "/repo.git", Auth: NewHostAuth(host, Credential{"u", "p-dummy"})})
	return stA, stB, err
}

func TestRedirect_should_NotForwardAuthorization_When_HostnameDiffers(t *testing.T) {
	for _, install := range []bool{false, true} {
		a, b, err := cloneThroughRedirect(t, false, install)
		t.Logf("custom-client=%v hostname-differs: err=%v A.withAuth=%d B.withAuth=%d", install, err, a.WithAuth.Load(), b.WithAuth.Load())
		if a.WithAuth.Load() == 0 {
			t.Fatal("A never got auth, test is vacuous")
		}
		if b.WithAuth.Load() != 0 {
			t.Fatalf("install=%v: Authorization forwarded to different hostname", install)
		}
	}
}

// Documents the gap: stock net/http compares Hostname() only, so a redirect to
// the same host on another port still carries Authorization.
func TestRedirect_should_ForwardOnlyWithStockClient_When_SameHostnameOtherPort(t *testing.T) {
	a, b, err := cloneThroughRedirect(t, true, false)
	t.Logf("STOCK go-git v5.19.2 same-hostname/other-port: err=%v A.withAuth=%d B.withAuth=%d", err, a.WithAuth.Load(), b.WithAuth.Load())
	if b.WithAuth.Load() == 0 {
		t.Log("stock client did NOT forward (gap not reproduced)")
	}
	stockForwarded := b.WithAuth.Load() > 0

	a, b, err = cloneThroughRedirect(t, true, true)
	t.Logf("CUSTOM CheckRedirect same-hostname/other-port: err=%v A.withAuth=%d B.withAuth=%d", err, a.WithAuth.Load(), b.WithAuth.Load())
	if b.WithAuth.Load() != 0 {
		t.Fatalf("custom client forwarded Authorization (stock forwarded=%v)", stockForwarded)
	}
}

func TestPush_should_Succeed_When_HelperAuthOverCustomClient(t *testing.T) {
	restoreProtocols(t)
	Install(NewHTTPClient(http.DefaultTransport, 16))
	root, _ := seedBare(t)
	srv, _ := gitHTTP(t, root, "u", "p-dummy", "")
	host := hostOf(t, srv.URL)
	auth := NewHostAuth(host, Credential{"u", "p-dummy"})
	dir := t.TempDir()
	repo, err := git.PlainClone(dir, false, &git.CloneOptions{URL: srv.URL + "/repo.git", Auth: auth})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644)
	runGit(t, dir, "add", "b.txt")
	runGit(t, dir, "commit", "-qm", "b")
	if err := repo.Push(&git.PushOptions{Auth: auth}); err != nil {
		t.Fatal(err)
	}
	if got := runGit(t, filepath.Join(root, "repo.git"), "rev-parse", "main"); got != runGit(t, dir, "rev-parse", "HEAD") {
		t.Fatalf("push did not land: %s", got)
	}
	_ = githttp.BasicAuth{}
}
