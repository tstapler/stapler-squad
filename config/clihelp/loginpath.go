package clihelp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	pathStartMarker = "__CLIHELP_PATH_START__"
	pathEndMarker   = "__CLIHELP_PATH_END__"

	loginPathTTL     = 10 * time.Minute
	loginPathRetry   = 30 * time.Second
	loginPathTimeout = 2 * time.Second
	loginPathMaxOut  = 64 << 10
)

var errNoLoginShell = errors.New("no login shell to derive PATH from")

// shellRunner runs `shell -c script` and returns its stdout. Injectable so
// tests can fake timeouts; production uses runShellScript.
type shellRunner func(ctx context.Context, shell, script string) (string, error)

// loginPathScript returns the constant script that prints $PATH between
// sentinels, mirroring the rc sourcing in config/config.go. No user text is
// ever part of it. derive is false when there is no shell worth asking
// (unset or plain sh).
func loginPathScript(shell string) (script string, derive bool) {
	base := filepath.Base(shell)
	if shell == "" || base == "sh" {
		return "", false
	}
	printPath := `printf '` + pathStartMarker + `%s` + pathEndMarker + `' "$PATH"`
	switch {
	case strings.Contains(base, "zsh"):
		return "source ~/.zshrc &>/dev/null || true; " + printPath, true
	case strings.Contains(base, "bash"):
		return "source ~/.bashrc &>/dev/null || true; " + printPath, true
	default:
		return printPath, true
	}
}

// parseSentinelPath extracts the directories between the markers, so rc-file
// banner noise cannot inject entries. Relative entries are dropped.
func parseSentinelPath(out string) ([]string, bool) {
	start := strings.Index(out, pathStartMarker)
	if start < 0 {
		return nil, false
	}
	span := out[start+len(pathStartMarker):]
	end := strings.Index(span, pathEndMarker)
	if end < 0 {
		return nil, false
	}
	var dirs []string
	for _, d := range strings.Split(span[:end], ":") {
		if filepath.IsAbs(d) {
			dirs = append(dirs, d)
		}
	}
	return dirs, len(dirs) > 0
}

// deriveLoginPath asks the user's login shell for its PATH.
func deriveLoginPath(ctx context.Context, shell string, run shellRunner) ([]string, error) {
	script, ok := loginPathScript(shell)
	if !ok {
		return nil, errNoLoginShell
	}
	out, err := run(ctx, shell, script)
	if err != nil {
		return nil, err
	}
	dirs, ok := parseSentinelPath(out)
	if !ok {
		return nil, errors.New("login shell printed no PATH between sentinels")
	}
	return dirs, nil
}

// mergeDirs concatenates lists in order, dropping empties and duplicates.
func mergeDirs(lists ...[]string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, l := range lists {
		for _, d := range l {
			if _, dup := seen[d]; d == "" || dup {
				continue
			}
			seen[d] = struct{}{}
			out = append(out, d)
		}
	}
	return out
}

func fallbackDirs(home string) []string {
	var dirs []string
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	return append(dirs, "/usr/local/bin")
}

// loginPathSource is the LoginPath: shell dirs first, then server PATH, then
// fallback dirs. Only a successful derivation is cached (with a TTL); a
// failure is retried at most once per loginPathRetry. Nothing runs until Start.
type loginPathSource struct {
	shell      func() string
	serverPath func() string
	home       func() string
	run        shellRunner
	now        func() time.Time

	mu        sync.Mutex
	started   bool
	inflight  bool
	loginDirs []string
	expires   time.Time
	failedAt  time.Time
}

func newLoginPathSource(shell, serverPath, home func() string, run shellRunner, now func() time.Time) *loginPathSource {
	return &loginPathSource{shell: shell, serverPath: serverPath, home: home, run: run, now: now}
}

// Dirs returns the merged lookup dirs. Before a derivation has succeeded it
// returns server PATH plus fallback dirs, for that call only.
func (s *loginPathSource) Dirs() []string {
	s.mu.Lock()
	var shellDirs []string
	if s.now().Before(s.expires) {
		shellDirs = s.loginDirs
	}
	kick := s.started && shellDirs == nil && s.retryDueLocked()
	s.mu.Unlock()
	if kick {
		go s.Refresh(context.Background())
	}
	server := filepath.SplitList(s.serverPath())
	return mergeDirs(shellDirs, server, fallbackDirs(s.home()))
}

// Start begins derivation in the background; later Dirs calls retry failures.
func (s *loginPathSource) Start() {
	s.mu.Lock()
	s.started = true
	s.mu.Unlock()
	go s.Refresh(context.Background())
}

func (s *loginPathSource) retryDueLocked() bool {
	return !s.inflight && (s.failedAt.IsZero() || s.now().Sub(s.failedAt) >= loginPathRetry)
}

// Refresh derives the login PATH unless a fresh result is cached, a
// derivation is in flight, or a recent failure is still inside the retry window.
func (s *loginPathSource) Refresh(ctx context.Context) {
	s.mu.Lock()
	if s.now().Before(s.expires) || !s.retryDueLocked() {
		s.mu.Unlock()
		return
	}
	s.inflight = true
	s.mu.Unlock()

	dirs, err := deriveLoginPath(ctx, s.shell(), s.run)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.inflight = false
	if err != nil {
		s.failedAt = s.now()
		return
	}
	s.loginDirs, s.expires, s.failedAt = dirs, s.now().Add(loginPathTTL), time.Time{}
}

func serverPathFromEnv() string { return os.Getenv("PATH") }
