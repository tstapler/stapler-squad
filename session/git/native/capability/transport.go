package capability

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/native/gitconfig"
)

// remoteURLs are every URL any remote could use for fetch or push, before and after the
// url.<base>.insteadOf / pushInsteadOf rewrites, taken from the effective config (not go-git's
// per-repository view, which misses global and included rewrites). The operation's remote is
// not known at preflight, so this is a superset: a local path on any remote routes the call.
func remoteURLs(cfg *gitconfig.Result) []string {
	var raw []string
	for _, e := range cfg.Entries {
		if e.Section == "remote" && e.HasSubsection && e.HasValue && (e.Key == "url" || e.Key == "pushurl") {
			raw = append(raw, e.Value)
		}
	}
	type rewrite struct{ base, prefix string }
	var rewrites []rewrite
	for _, e := range cfg.Entries {
		if e.Section == "url" && e.HasSubsection && e.HasValue && (e.Key == "insteadof" || e.Key == "pushinsteadof") {
			rewrites = append(rewrites, rewrite{base: e.Subsection, prefix: e.Value})
		}
	}
	urls := append([]string(nil), raw...)
	for _, u := range raw {
		best := rewrite{}
		for _, r := range rewrites {
			if strings.HasPrefix(u, r.prefix) && len(r.prefix) >= len(best.prefix) && r.prefix != "" {
				best = r
			}
		}
		if best.prefix != "" {
			urls = append(urls, best.base+strings.TrimPrefix(u, best.prefix))
		}
	}
	return urls
}

var (
	remoteHelperURL = regexp.MustCompile(`^[A-Za-z0-9+.-]+::`)
	windowsDrive    = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
)

// protocolOf classifies a remote URL the way go-git would use it, with two forms go-git's
// parser misreads: remote helpers ("ext::...", "codecommit::...") that no in-process transport
// can run, and Windows drive paths. Both are reported as "file" so they route to the CLI.
func protocolOf(url string) string {
	if remoteHelperURL.MatchString(url) || windowsDrive.MatchString(url) {
		return "file"
	}
	ep, err := transport.NewEndpoint(url)
	if err != nil {
		return "file" // cannot tell: treat as local, which routes to the CLI
	}
	return ep.Protocol
}

// transport reports local-path remotes (go-git would exec git-upload-pack for the file
// protocol) and ssh remotes whose client setup go-git cannot reproduce (ProxyCommand,
// ProxyJump, Match, core.sshCommand, GIT_SSH*).
func (f *facts) transport(kind backend.CapabilityKind) ([]backend.Capability, error) {
	cfg, err := f.config()
	if err != nil {
		return nil, err
	}
	var ssh, git bool
	for _, u := range remoteURLs(cfg) {
		switch protocolOf(u) {
		case "file":
			if kind == backend.CapLocalTransport {
				return one(backend.CapLocalTransport, ""), nil
			}
		case "ssh":
			ssh = true
		case "git":
			git = true
		}
	}
	if kind != backend.CapSSHProxy {
		return nil, nil
	}
	if git {
		if e, ok := cfg.Lookup("core.gitProxy"); (ok && e.HasValue && e.Value != "") || envGet(f.env, "GIT_PROXY_COMMAND") != "" {
			return one(backend.CapSSHProxy, "core.gitProxy"), nil
		}
	}
	if !ssh {
		return nil, nil
	}
	if e, ok := cfg.Lookup("core.sshCommand"); ok && e.HasValue && e.Value != "" {
		return one(backend.CapSSHProxy, "core.sshCommand"), nil
	}
	for _, v := range []string{"GIT_SSH_COMMAND", "GIT_SSH"} {
		if envGet(f.env, v) != "" {
			return one(backend.CapSSHProxy, v), nil
		}
	}
	return f.sshConfigDirectives()
}

var proxyDirectives = map[string]bool{"proxycommand": true, "proxyjump": true, "match": true}

func (f *facts) sshConfigDirectives() ([]backend.Capability, error) {
	paths := f.d.cfg.SSHConfigs
	if paths == nil {
		paths = []string{"/etc/ssh/ssh_config"}
		if home := envGet(f.env, "HOME"); home != "" {
			paths = append(paths, filepath.Join(home, ".ssh", "config"))
		}
	}
	for _, p := range paths {
		d, err := scanSSHConfig(p, filepath.Dir(p), envGet(f.env, "HOME"), 0)
		if err != nil {
			return nil, err
		}
		if d != "" {
			return one(backend.CapSSHProxy, d), nil
		}
	}
	return nil, nil
}

const (
	maxSSHIncludeDepth = 4
	maxSSHConfigSize   = 4 << 20
)

// scanSSHConfig returns the first proxy directive in an ssh_config file or anything it includes
// (relative includes resolve against baseDir). It does not match Host patterns: any such
// directive anywhere routes, which errs toward the CLI.
func scanSSHConfig(path, baseDir, home string, depth int) (string, error) {
	data, err := gitconfig.ReadRegularFile(path, maxSSHConfigSize)
	if errors.Is(err, fs.ErrNotExist) || isNotDir(err) {
		return "", nil
	}
	if err != nil {
		return "", detectErr(backend.DetectSSHConfigUnreadab, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), maxSSHConfigSize) // a long comment line must not end the scan early
	for sc.Scan() {
		fields := sshFields(sc.Text())
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		key := strings.ToLower(fields[0])
		if proxyDirectives[key] {
			return fields[0], nil
		}
		if key == "include" && depth < maxSSHIncludeDepth {
			for _, pat := range fields[1:] {
				if rest, ok := strings.CutPrefix(pat, "~/"); ok {
					pat = filepath.Join(home, rest)
				}
				if !filepath.IsAbs(pat) {
					pat = filepath.Join(baseDir, pat)
				}
				matches, _ := filepath.Glob(pat)
				for _, m := range matches {
					if d, err := scanSSHConfig(m, baseDir, home, depth+1); d != "" || err != nil {
						return d, err
					}
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", detectErr(backend.DetectSSHConfigUnreadab, err)
	}
	return "", nil
}

// sshFields splits an ssh_config line into words: whitespace or '=' separate keyword and
// arguments, and double quotes group (and are removed), so `"ProxyCommand" nc` is ProxyCommand.
func sshFields(line string) []string {
	var fields []string
	var cur strings.Builder
	inQuote, started := false, false
	flush := func() {
		if started {
			fields = append(fields, cur.String())
			cur.Reset()
			started = false
		}
	}
	for _, r := range strings.TrimSpace(line) {
		switch {
		case r == '"':
			inQuote, started = !inQuote, true
		case !inQuote && (r == ' ' || r == '\t' || r == '='):
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	flush()
	return fields
}
