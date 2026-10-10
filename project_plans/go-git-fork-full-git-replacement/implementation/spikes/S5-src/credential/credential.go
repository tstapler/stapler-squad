// Package credential is the S5 spike: a git-credential-protocol client that
// execs the helper binary directly (never `git credential`), a host-scoped
// transport.AuthMethod, and a provider chain (Go-native token sources first,
// configured helper as always-on fallback). No go-git fork involved.
package credential

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Request struct{ Protocol, Host, Path, Username, Password string }

type Credential struct{ Username, Password string }

func (r Request) encode() []byte {
	var b bytes.Buffer
	w := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s=%s\n", k, v)
		}
	}
	w("protocol", r.Protocol)
	w("host", r.Host)
	w("path", r.Path)
	w("username", r.Username)
	w("password", r.Password)
	b.WriteString("\n")
	return b.Bytes()
}

// Helper is one configured credential.helper value.
type Helper struct {
	// Argv is the exec vector. For "!cmd" helpers it is ["sh","-c",cmd]
	// (spawns sh, never git).
	Argv []string
}

// Well-known libexec dirs searched for git-credential-<name> so that resolving
// a built-in helper such as osxkeychain does not need `git --exec-path`.
var KnownExecDirs = []string{
	"/Library/Developer/CommandLineTools/usr/libexec/git-core",
	"/Applications/Xcode.app/Contents/Developer/usr/libexec/git-core",
	"/opt/homebrew/opt/git/libexec/git-core",
	"/usr/local/opt/git/libexec/git-core",
	"/usr/lib/git-core",
	"/usr/libexec/git-core",
}

// ResolveHelper maps a credential.helper config value to an exec vector,
// following git's rules: "!cmd" -> shell, absolute path -> as is,
// otherwise git-credential-<value> on PATH then KnownExecDirs.
func ResolveHelper(value string) (Helper, error) {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return Helper{}, errors.New("empty helper")
	case strings.HasPrefix(value, "!"):
		return Helper{Argv: []string{"sh", "-c", value[1:]}}, nil
	case filepath.IsAbs(value):
		return Helper{Argv: strings.Fields(value)}, nil
	}
	name, args, _ := strings.Cut(value, " ")
	bin := "git-credential-" + name
	if p, err := exec.LookPath(bin); err == nil {
		return Helper{Argv: append([]string{p}, strings.Fields(args)...)}, nil
	}
	for _, d := range KnownExecDirs {
		p := filepath.Join(d, bin)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return Helper{Argv: append([]string{p}, strings.Fields(args)...)}, nil
		}
	}
	return Helper{}, fmt.Errorf("credential helper %q not found", bin)
}

// run execs the helper with action get|store|erase. The helper runs in its own
// process group so a timeout kills grandchildren too.
func (h Helper) run(ctx context.Context, action string, req Request, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	argv := append(append([]string{}, h.Argv...), action)
	if h.Argv[0] == "sh" && len(h.Argv) >= 3 && h.Argv[1] == "-c" {
		// git appends the action to the shell snippet.
		argv = []string{"sh", "-c", h.Argv[2] + " " + action}
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdin = bytes.NewReader(req.encode())
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return out.String(), err
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return "", fmt.Errorf("credential helper timed out: %w", ctx.Err())
	}
}

func parse(out string) Credential {
	var c Credential
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch k {
		case "username":
			c.Username = v
		case "password":
			c.Password = v
		}
	}
	return c
}

func (h Helper) Get(ctx context.Context, req Request, timeout time.Duration) (Credential, error) {
	out, err := h.run(ctx, "get", req, timeout)
	if err != nil {
		return Credential{}, err
	}
	return parse(out), nil
}

func (h Helper) Approve(ctx context.Context, req Request, timeout time.Duration) error {
	_, err := h.run(ctx, "store", req, timeout)
	return err
}

func (h Helper) Reject(ctx context.Context, req Request, timeout time.Duration) error {
	_, err := h.run(ctx, "erase", req, timeout)
	return err
}

// TokenSource is a Go-native, host-scoped token lookup (keychain, gh). The
// product injects github.GetKeychainTokenForHost here so this package never
// imports `github`.
type TokenSource struct {
	Name   string
	Lookup func(host string) (user, token string, ok bool)
}

// Provider tries sources in order, then helpers.
type Provider struct {
	Sources []TokenSource
	Helpers []Helper
	Timeout time.Duration
}

// Result records where a credential came from so a rejected one can be erased.
type Result struct {
	Credential
	From   string
	Helper *Helper
}

func (p *Provider) Fill(ctx context.Context, req Request) (Result, error) {
	for _, s := range p.Sources {
		if u, t, ok := s.Lookup(req.Host); ok && t != "" {
			return Result{Credential: Credential{u, t}, From: s.Name}, nil
		}
	}
	to := p.Timeout
	if to == 0 {
		to = 5 * time.Second
	}
	for i := range p.Helpers {
		c, err := p.Helpers[i].Get(ctx, req, to)
		if err == nil && c.Password != "" {
			return Result{Credential: c, From: "helper", Helper: &p.Helpers[i]}, nil
		}
	}
	return Result{}, errors.New("no credential found")
}

// HostAuth is an http.AuthMethod (go-git plumbing/transport/http) that only
// attaches credentials to requests for the host it was built for. go-git calls
// SetAuth per request it builds; redirects are handled by CheckRedirect.
type HostAuth struct {
	hostPort string
	cred     Credential
}

func NewHostAuth(hostPort string, c Credential) *HostAuth { return &HostAuth{hostPort, c} }

func (a *HostAuth) SetAuth(r *http.Request) {
	if a == nil || r.URL.Host != a.hostPort {
		return
	}
	r.SetBasicAuth(a.cred.Username, a.cred.Password)
}
func (a *HostAuth) Name() string   { return "s5-host-scoped-basic" }
func (a *HostAuth) String() string { return a.Name() + " - " + a.cred.Username + ":*******" }
