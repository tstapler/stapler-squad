package gogit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-git/gcfg"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// maxConfigFileSize bounds one config file; real ones are a few KiB.
const maxConfigFileSize = 1 << 20

// maxConfigFiles bounds the files one read may open, so a config that includes itself many times
// over (git's own depth limit still allows an exponential fan-out) cannot spin a caller.
const maxConfigFiles = 64

// maxIncludeDepth mirrors git's limit (MAX_INCLUDE_DEPTH); deeper chains are git errors.
const maxIncludeDepth = 10

// systemConfigCandidates is the fallback used only when the composition root did not declare the
// real system config (WithSystemConfig). The real path is compiled into the git binary, so every
// existing candidate is treated as "might be the one": it is ignored only when it never mentions
// the key being read.
var systemConfigCandidates = []string{ //nolint:gochecknoglobals read-only list
	"/etc/gitconfig",
	"/usr/etc/gitconfig",
	"/usr/local/etc/gitconfig",
	"/opt/homebrew/etc/gitconfig",
	"/opt/local/etc/gitconfig",
	"/run/current-system/sw/etc/gitconfig",
	"/home/linuxbrew/.linuxbrew/etc/gitconfig",
	"/Library/Developer/CommandLineTools/usr/etc/gitconfig",
	"/Applications/Xcode.app/Contents/Developer/usr/etc/gitconfig",
}

// vendorConfigCandidates are the fallback guesses for vendor-added system files, which
// GIT_CONFIG_SYSTEM does not replace.
var vendorConfigCandidates = []string{ //nolint:gochecknoglobals read-only list
	"/Library/Developer/CommandLineTools/usr/share/git-core/gitconfig",
	"/Applications/Xcode.app/Contents/Developer/usr/share/git-core/gitconfig",
}

// systemConfig is the system-scope config source list; known says whether it is authoritative.
type systemConfig struct {
	paths  []string // replaced by GIT_CONFIG_SYSTEM; read after vendor, so it wins a shared key
	vendor []string // not replaced by it; dropped only by GIT_CONFIG_NOSYSTEM
	known  bool
}

// loadAll reads every existing file of list (authoritative) or scans it for the key (guessed).
func (l *configLoader) loadAll(list []string, known bool) {
	for _, c := range list {
		if _, err := os.Stat(c); err != nil {
			continue
		}
		if known {
			l.load(c, 1)
		} else {
			l.scan(c, 1)
		}
	}
}

type cfgEntry struct{ section, sub, name, value string }

// matcher selects the configuration keys a read cares about.
type matcher func(section, sub, name string) bool

func keyMatcher(k cfgKey) matcher {
	return func(section, sub, name string) bool {
		return strings.EqualFold(section, k.section) && sub == k.sub && strings.EqualFold(name, k.name)
	}
}

// cfgKey is a parsed dotted key: section and variable names are case-insensitive, the
// subsection is case-sensitive.
type cfgKey struct{ section, sub, name string }

// parseKey splits "section[.subsection].name". ok is false for anything git would reject or that
// this reader is not sure about (the caller then falls back to the CLI).
func parseKey(key string) (cfgKey, bool) {
	first, last := strings.Index(key, "."), strings.LastIndex(key, ".")
	if first <= 0 || last == len(key)-1 {
		return cfgKey{}, false
	}
	k := cfgKey{section: key[:first], name: key[last+1:]}
	if first != last {
		k.sub = key[first+1 : last]
		if k.sub == "" || strings.ContainsAny(k.sub, "\n\x00") {
			return cfgKey{}, false
		}
	}
	for _, r := range k.section {
		if !isAlnumDash(r) {
			return cfgKey{}, false
		}
	}
	for i, r := range k.name {
		if !isAlnumDash(r) || (i == 0 && !isAlpha(r)) {
			return cfgKey{}, false
		}
	}
	return k, true
}

func isAlpha(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }

func isAlnumDash(r rune) bool { return isAlpha(r) || (r >= '0' && r <= '9') || r == '-' }

// parseConfigFile reads one config file in order. exists is false when it is absent (which git
// treats as empty). A syntax error comes back as err: git would fail where this reader might
// not, so the caller must not guess.
func parseConfigFile(path string) (entries []cfgEntry, exists bool, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !fi.Mode().IsRegular() && path != os.DevNull {
		return nil, true, fmt.Errorf("%s is not a regular file", path) // a FIFO would block forever
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxConfigFileSize+1))
	if err != nil {
		return nil, true, err
	}
	if len(data) > maxConfigFileSize {
		return nil, true, fmt.Errorf("%s is larger than %d bytes", path, maxConfigFileSize)
	}
	if !headersAreStrict(data) {
		return nil, true, errors.New("config file has a section header git would reject")
	}
	if hasLineContinuation(data) {
		// gcfg and git disagree on how a backslash-newline joins lines, so the CLI must read it.
		return nil, true, errors.New("config file uses line continuation")
	}
	err = gcfg.ReadWithCallback(bytes.NewReader(data), func(section, sub, name, value string, _ bool) error {
		if name == "" {
			return nil // a section header
		}
		entries = append(entries, cfgEntry{section: section, sub: sub, name: name, value: value})
		return nil
	})
	if err != nil {
		return nil, true, err
	}
	return entries, true, nil
}

var strictHeader = regexp.MustCompile(`^[ \t]*\[[A-Za-z0-9-]+( "(?:[^"\\\n]|\\.)*")?\]`)

// headersAreStrict reports whether every line that opens a section does so in the form git
// accepts, "[name]" or "[name \"sub\"]" with no padding inside the brackets; gcfg also accepts
// "[ name ]", where git fails every command.
func headersAreStrict(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		if trimmed := bytes.TrimLeft(line, " \t"); len(trimmed) > 0 && trimmed[0] == '[' && !strictHeader.Match(line) {
			return false
		}
	}
	return true
}

// hasLineContinuation reports whether any line ends in an odd run of backslashes, which git reads
// as "continue on the next line".
func hasLineContinuation(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		n := 0
		for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
			n++
		}
		if n%2 == 1 {
			return true
		}
	}
	return false
}

// configReadResult is what one pass over a config scope chain found.
type configReadResult struct {
	entries []cfgEntry // matching entries in the order git evaluates them; the last one wins
	unsure  bool       // some source that cannot be evaluated here mentions a matching key
}

func (r configReadResult) last() (cfgEntry, bool) {
	if len(r.entries) == 0 {
		return cfgEntry{}, false
	}
	return r.entries[len(r.entries)-1], true
}

// configLoader evaluates config files the way git does, except includeIf conditions: an
// unconditional include is spliced in at its position; a conditional one is scanned only to see
// whether it could matter. If it never mentions a matching key its condition is irrelevant and
// it is skipped, otherwise the read is marked unsure (go-git cannot evaluate gitdir:, onbranch:
// or hasconfig: conditions, and guessing would return a wrong value).
type configLoader struct {
	ctx    context.Context
	match  matcher
	home   string // "" when unknown, which makes "~/" includes unsure
	res    configReadResult
	budget int // files this read may still open
}

func newConfigLoader(ctx context.Context, match matcher, home string) *configLoader {
	return &configLoader{ctx: ctx, match: match, home: home, budget: maxConfigFiles}
}

// admit spends one file of the budget; false means the read is abandoned as unsure.
func (l *configLoader) admit(depth int) bool {
	l.budget--
	if depth > maxIncludeDepth || l.budget < 0 || l.ctx.Err() != nil {
		l.res.unsure = true
		return false
	}
	return true
}

func (l *configLoader) load(path string, depth int) {
	if !l.admit(depth) {
		return
	}
	entries, _, err := parseConfigFile(path)
	if err != nil {
		l.res.unsure = true
		return
	}
	for _, e := range entries {
		switch {
		case isIncludeEntry(e):
			if inc, ok := l.includePath(path, e.value); ok {
				l.load(inc, depth+1)
			} else {
				l.res.unsure = true
			}
		case isConditionalInclude(e):
			if inc, ok := l.includePath(path, e.value); !ok {
				l.res.unsure = true
			} else {
				l.scan(inc, depth+1)
			}
		case l.match(e.section, e.sub, e.name):
			l.res.entries = append(l.res.entries, e)
		}
	}
}

// scan marks the read unsure if path, or anything it includes, mentions a matching key.
func (l *configLoader) scan(path string, depth int) {
	if !l.admit(depth) {
		return
	}
	entries, _, err := parseConfigFile(path)
	if err != nil {
		l.res.unsure = true
		return
	}
	for _, e := range entries {
		switch {
		case isIncludeEntry(e) || isConditionalInclude(e):
			if inc, ok := l.includePath(path, e.value); ok {
				l.scan(inc, depth+1)
			} else {
				l.res.unsure = true
			}
		case l.match(e.section, e.sub, e.name):
			l.res.unsure = true
		}
	}
}

func isIncludeEntry(e cfgEntry) bool {
	return strings.EqualFold(e.section, "include") && e.sub == "" && strings.EqualFold(e.name, "path")
}

func isConditionalInclude(e cfgEntry) bool {
	return strings.EqualFold(e.section, "includeIf") && e.sub != "" && strings.EqualFold(e.name, "path")
}

// includePath resolves an include path value. ok is false for forms this reader does not
// evaluate (~user/, %(prefix)/, empty).
func (l *configLoader) includePath(from, value string) (string, bool) {
	switch {
	case value == "" || strings.HasPrefix(value, "%(") || (strings.HasPrefix(value, "~") && !strings.HasPrefix(value, "~/")):
		return "", false
	case strings.HasPrefix(value, "~/"):
		if l.home == "" {
			return "", false
		}
		return filepath.Join(l.home, value[2:]), true
	case filepath.IsAbs(value):
		return value, true
	}
	return filepath.Join(filepath.Dir(from), value), true
}

// envValue returns the last value of key in an environ slice.
func envValue(environ []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(environ) - 1; i >= 0; i-- {
		if strings.HasPrefix(environ[i], prefix) {
			return environ[i][len(prefix):], true
		}
	}
	return "", false
}

// envBool reports a boolean variable the way git_env_bool does. ok is false for a value that is
// neither a recognised word nor an integer.
func envBool(environ []string, key string) (value, ok bool) {
	v, set := envValue(environ, key)
	if !set {
		return false, true
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off", "":
		return false, true
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return false, false
	}
	return n != 0, true
}

// readScopes evaluates system, global and (when localConfig is non-empty) repository config for
// the keys match selects, in git's precedence order.
func readScopes(ctx context.Context, environ []string, system systemConfig, localConfig string, match matcher) configReadResult {
	home, _ := envValue(environ, "HOME")
	l := newConfigLoader(ctx, match, home)

	noSystem, ok := envBool(environ, "GIT_CONFIG_NOSYSTEM")
	switch sysPath, set := envValue(environ, "GIT_CONFIG_SYSTEM"); {
	case !ok:
		l.res.unsure = true
	case noSystem:
	// git reads vendor-added files first, so the system file wins a key set in both
	// (Apple git: `git config --show-origin --list` lists share/git-core/gitconfig first).
	case set:
		l.loadAll(system.vendor, system.known)
		l.load(sysPath, 1)
	default:
		l.loadAll(system.vendor, system.known)
		l.loadAll(system.paths, system.known)
	}

	switch globalPath, set := envValue(environ, "GIT_CONFIG_GLOBAL"); {
	case set:
		l.load(globalPath, 1)
	case home == "":
		l.res.unsure = true // git would ask the password database for the home directory
	default:
		xdg, _ := envValue(environ, "XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		l.load(filepath.Join(xdg, "git", "config"), 1)
		l.load(filepath.Join(home, ".gitconfig"), 1)
	}

	if localConfig != "" {
		l.load(localConfig, 1)
	}
	return l.res
}

// GetConfig returns the effective value of key, as `git config --get` prints it (the last
// value across system, global and local scope, includes followed).
func (b *Backend) GetConfig(ctx context.Context, loc backend.RepoLocation, key backend.ConfigKey) (string, error) {
	if err := optArg("key", string(key)); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	k, ok := parseKey(string(key))
	if !ok {
		return "", fallbackEligible(fmt.Errorf("config key %q is not a plain section.name key", key))
	}
	dir, err := localDir(loc)
	if err != nil {
		return "", err
	}
	environ := b.environ()
	localConfig := ""
	lay, err := b.locate(ctx, dir, environ)
	switch {
	case err == nil:
		localConfig = filepath.Join(lay.commonDir, "config")
	case errors.Is(err, backend.ErrNotARepo) && !errors.Is(err, errDanglingGitfile):
		// `git config` works outside a repository: global and system scope only. A dangling
		// .git file is not "outside": git fails the command.
	default:
		return "", err
	}
	res := readScopes(ctx, environ, systemConfig{paths: b.systemCfgs, vendor: b.vendorCfgs, known: b.systemKnown}, localConfig, keyMatcher(k))
	if res.unsure {
		return "", fallbackEligible(errors.New("config source cannot be evaluated in-process (include condition or unreadable file)"))
	}
	e, found := res.last()
	if !found {
		return "", fmt.Errorf("%w: %s", backend.ErrConfigUnset, key)
	}
	return lastLine(e.value), nil
}

// lastLine mirrors the CLI backend's output handling: the last non-empty line, trimmed.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
