package credential

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"github.com/go-git/go-git/v5/plumbing"
	"io"
	"net"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var (
	realGit  string
	execPath string
	realUP   string
)

func TestMain(m *testing.M) {
	// Pin the system git: on the author's machine `git` on PATH is the ~/.local/bin ssh-fallback wrapper.
	realGit = "/usr/bin/git"
	if _, err := os.Stat(realGit); err != nil {
		realGit, _ = exec.LookPath("git")
	}
	out, _ := exec.Command(realGit, "--exec-path").Output()
	execPath = strings.TrimSpace(string(out))
	realUP, _ = exec.LookPath("git-upload-pack")
	home, _ := os.MkdirTemp("", "s5home")
	os.Setenv("HOME", home)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	os.Setenv("GIT_TERMINAL_PROMPT", "0")
	os.Setenv("S5_REAL_PATH", os.Getenv("PATH"))
	os.WriteFile(filepath.Join(home, "gitconfig"), []byte("[user]\n\tname = s5\n\temail = s5@example.invalid\n[init]\n\tdefaultBranch = main\n"), 0o644)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func runGit(t testing.TB, dir string, args ...string) string {
	t.Helper()
	c := exec.Command(realGit, args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "PATH="+os.Getenv("S5_REAL_PATH"))
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// shim puts logging `git` and `git-upload-pack` first on PATH; any spawn of
// either by the process under test is appended to the log.
type shim struct{ log string }

func newShim(t *testing.T) *shim {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "spawns.log")
	for name, target := range map[string]string{"git": realGit, "git-upload-pack": realUP} {
		body := fmt.Sprintf("#!/bin/sh\necho \"%s $*\" >> \"$S5_SHIM_LOG\"\nexec %s \"$@\"\n", name, target)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("S5_SHIM_LOG", log)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return &shim{log: log}
}

func (s *shim) Spawns() []string {
	b, _ := os.ReadFile(s.log)
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// seedBare creates a bare repo "repo.git" under root with one commit on main
// and returns the commit hash.
func seedBare(t *testing.T) (root, head string) {
	t.Helper()
	root = t.TempDir()
	work := t.TempDir()
	runGit(t, work, "init", "-q")
	os.WriteFile(filepath.Join(work, "a.txt"), []byte("hello\n"), 0o644)
	runGit(t, work, "add", "a.txt")
	runGit(t, work, "commit", "-qm", "init")
	head = runGit(t, work, "rev-parse", "HEAD")
	runGit(t, root, "clone", "-q", "--bare", work, "repo.git")
	runGit(t, filepath.Join(root, "repo.git"), "config", "http.receivepack", "true")
	return root, head
}

type srvStats struct {
	Requests, WithAuth, Unauth atomic.Int64
	mu                         sync.Mutex
	AuthHeaders                []string
}

// gitHTTP serves root via git http-backend. If user!="" basic auth is
// required. onAuth sees every Authorization header received.
func gitHTTP(t *testing.T, root, user, pass string, redirectTo string) (*httptest.Server, *srvStats) {
	t.Helper()
	st := &srvStats{}
	cg := &cgi.Handler{
		Path: filepath.Join(execPath, "git-http-backend"),
		Env:  []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1", "REMOTE_USER=s5", "PATH=" + os.Getenv("S5_REAL_PATH"), "HOME=/nonexistent", "GIT_CONFIG_NOSYSTEM=1"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st.Requests.Add(1)
		h := r.Header.Get("Authorization")
		if h != "" {
			st.WithAuth.Add(1)
			st.mu.Lock()
			st.AuthHeaders = append(st.AuthHeaders, h)
			st.mu.Unlock()
		}
		if user != "" {
			u, p, ok := r.BasicAuth()
			if !ok || u != user || p != pass {
				st.Unauth.Add(1)
				w.Header().Set("WWW-Authenticate", `Basic realm="s5"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if redirectTo != "" && strings.HasSuffix(r.URL.Path, "/info/refs") {
			http.Redirect(w, r, redirectTo+r.URL.Path+"?"+r.URL.RawQuery, http.StatusFound)
			return
		}
		cg.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, st
}

// sshServer is a minimal SSH server that executes git-upload-pack for
// public-key-authenticated clients. It records every auth attempt.
type sshServer struct {
	Addr      string
	HostKey   ssh.Signer
	Passwords []string
	Users     []string
	Execs     atomic.Int64
	Conns     atomic.Int64
	mu        sync.Mutex
}

func newSSHServer(t *testing.T, root string, allowed ssh.PublicKey) *sshServer {
	return newSSHServerAt(t, root, allowed, "127.0.0.1:0")
}

func newSSHServerAt(t *testing.T, root string, allowed ssh.PublicKey, listen string) *sshServer {
	t.Helper()
	_, hk, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, _ := ssh.NewSignerFromKey(hk)
	s := &sshServer{HostKey: hostSigner}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			s.mu.Lock()
			s.Passwords = append(s.Passwords, string(pw))
			s.mu.Unlock()
			return nil, fmt.Errorf("no passwords")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			s.mu.Lock()
			s.Users = append(s.Users, c.User())
			s.mu.Unlock()
			if allowed != nil && bytes.Equal(k.Marshal(), allowed.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("key not allowed")
		},
	}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		t.Fatal(err)
	}
	s.Addr = ln.Addr().String()
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			s.Conns.Add(1)
			go s.serve(nc, cfg, root)
		}
	}()
	return s
}

func (s *sshServer) serve(nc net.Conn, cfg *ssh.ServerConfig, root string) {
	defer nc.Close()
	conn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		if nch.ChannelType() != "session" {
			nch.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, creqs, _ := nch.Accept()
		go func() {
			for rq := range creqs {
				if rq.Type != "exec" {
					rq.Reply(false, nil)
					continue
				}
				cmdline := string(rq.Payload[4:])
				rq.Reply(true, nil)
				s.Execs.Add(1)
				path := strings.Trim(strings.TrimPrefix(cmdline, "git-upload-pack "), "'")
				c := exec.Command(realUP, filepath.Join(root, path))
				c.Env = []string{"PATH=" + os.Getenv("S5_REAL_PATH")}
				stdin, _ := c.StdinPipe()
				c.Stdout = ch
				c.Stderr = ch.Stderr()
				c.Start()
				go func() { io.Copy(stdin, ch); stdin.Close() }()
				c.Wait()
				ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
				ch.Close()
			}
		}()
	}
}

// startAgent serves an in-process ssh-agent on a unix socket and points
// SSH_AUTH_SOCK at it. Returns the key held by the agent.
func startAgent(t *testing.T) ssh.PublicKey {
	t.Helper()
	dir, _ := os.MkdirTemp("/tmp", "s5a")
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	kr := agent.NewKeyring()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := kr.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(priv)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(kr, c)
		}
	}()
	t.Setenv("SSH_AUTH_SOCK", sock)
	return signer.PublicKey()
}

func knownHostsLine(addr string, k ssh.PublicKey) string {
	h, p, _ := net.SplitHostPort(addr)
	return fmt.Sprintf("[%s]:%s %s", h, p, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k))))
}

func writeKnownHosts(t *testing.T, lines ...string) {
	t.Helper()
	home := os.Getenv("HOME")
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// stubHelper installs a git-credential-s5stub script (no git use) that
// answers `get` for host and logs actions (never the password) to log.
func stubHelper(t *testing.T, host, user, pass string) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "actions.log")
	body := fmt.Sprintf(`#!/bin/sh
echo "$1" >> %q
in=$(cat)
if [ "$1" = get ]; then
  case "$in" in *"host=%s"*) printf 'username=%s\npassword=%s\n';; esac
fi
`, logPath, host, user, pass)
	if err := os.WriteFile(filepath.Join(dir, "git-credential-s5stub"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return logPath
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

type plumbingRef = plumbing.Reference
