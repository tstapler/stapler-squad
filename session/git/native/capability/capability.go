package capability

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/native/gitconfig"
)

// Config is what the Detector needs from its environment. It spawns nothing: system config
// locations are learned once by the composition root and passed in.
type Config struct {
	// Environ is the environment the git CLI would run with. Nil means os.Environ at call time.
	Environ func() []string
	// SystemDefault and SystemVendor are git's system config files (see gitconfig.Env).
	SystemDefault, SystemVendor []string
	// SystemAttributes are system-wide gitattributes files. Nil derives them from the system
	// config directories plus /etc/gitattributes.
	SystemAttributes []string
	// SSHConfigs are the ssh client config files to scan for ProxyCommand, ProxyJump and Match.
	// Nil means ~/.ssh/config and /etc/ssh/ssh_config.
	SSHConfigs []string
}

// Detector implements backend.CapabilityProbe.
type Detector struct {
	cfg     Config
	mu      sync.Mutex
	index   map[string]indexFacts // keyed by index path; valid while the trailing checksum matches
	decodes int                   // index decodes performed, for tests
}

var _ backend.CapabilityProbe = (*Detector)(nil)

// New returns a Detector.
func New(cfg Config) *Detector { return &Detector{cfg: cfg, index: map[string]indexFacts{}} }

func (d *Detector) environ() []string {
	if d.cfg.Environ != nil {
		return d.cfg.Environ()
	}
	return os.Environ()
}

// facts is everything one Capabilities call may need, computed lazily so an operation pays only
// for the kinds backend.RelevantCapabilities names.
type facts struct {
	d      *Detector
	ctx    context.Context
	op     backend.OperationName
	l      layout
	env    []string
	cfg    *gitconfig.Result
	idx    *indexFacts
	idxErr error
}

func (f *facts) config() (*gitconfig.Result, error) {
	if f.cfg != nil {
		return f.cfg, nil
	}
	res, err := gitconfig.Resolve(f.ctx, gitconfig.Env{
		Environ: f.env, SystemDefault: f.d.cfg.SystemDefault, SystemVendor: f.d.cfg.SystemVendor,
	}, gitconfig.Repo{GitDir: f.l.gitDir, CommonDir: f.l.commonDir})
	if err != nil {
		return nil, toDetectError(err)
	}
	f.cfg = res
	return res, nil
}

func (f *facts) indexFacts() (*indexFacts, error) {
	if f.idx == nil && f.idxErr == nil {
		f.idx, f.idxErr = f.d.readIndex(f.l)
	}
	return f.idx, f.idxErr
}

// toDetectError maps a gitconfig failure onto the closed backend label set.
func toDetectError(err error) error {
	var ge *gitconfig.Error
	if !errors.As(err, &ge) {
		return detectErr(backend.DetectUnclassified, err)
	}
	for _, s := range backend.AllDetectSubs() {
		if string(s) == string(ge.Sub) {
			return detectErr(s, err)
		}
	}
	return detectErr(backend.DetectUnclassified, err)
}

// Capabilities implements backend.CapabilityProbe: it computes exactly the capabilities
// backend.RelevantCapabilities(op) names for the repository at loc, from fresh state. It fails
// closed with a *backend.DetectError.
func (d *Detector) Capabilities(ctx context.Context, loc backend.Local, op backend.OperationName) ([]backend.Capability, error) {
	kinds := backend.RelevantCapabilities(op)
	if len(kinds) == 0 {
		return nil, nil
	}
	l, err := resolveLayout(string(loc.Root))
	if err != nil {
		return nil, err
	}
	f := &facts{d: d, ctx: ctx, op: op, l: l, env: d.environ()}
	var caps []backend.Capability
	for _, k := range kinds {
		found, err := f.detect(k)
		if err != nil {
			return nil, err
		}
		caps = append(caps, found...)
		if err := ctx.Err(); err != nil {
			return nil, detectErr(backend.DetectCanceled, err)
		}
	}
	return caps, nil
}

func (f *facts) detect(k backend.CapabilityKind) ([]backend.Capability, error) {
	switch k {
	case backend.CapHooks:
		return f.hooks()
	case backend.CapGPGSign:
		return f.signing()
	case backend.CapLFS:
		return f.filters()
	case backend.CapUnsupportedIndex:
		return f.unsupportedIndex()
	case backend.CapSparse:
		return f.sparse()
	case backend.CapSubmodule:
		return f.submodules()
	case backend.CapShallow:
		return f.shallow(), nil
	case backend.CapLocalTransport, backend.CapSSHProxy:
		return f.transport(k)
	}
	return nil, nil
}

func one(k backend.CapabilityKind, detail string) []backend.Capability {
	return []backend.Capability{{Kind: k, Detail: detail}}
}

// expandPath applies git's pathname expansion to a config value: a leading ~/ is the home directory.
func (f *facts) expandPath(value string) string {
	if rest, ok := strings.CutPrefix(value, "~/"); ok {
		if home := envGet(f.env, "HOME"); home != "" {
			return filepath.Join(home, rest)
		}
	}
	return value
}

func envGet(environ []string, key string) string {
	for i := len(environ) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(environ[i], key+"="); ok {
			return v
		}
	}
	return ""
}

// shallow reports a shallow clone: git records the grafted commits in <CommonDir>/shallow.
func (f *facts) shallow() []backend.Capability {
	fi, err := os.Stat(filepath.Join(f.l.commonDir, "shallow"))
	if err == nil && fi.Mode().IsRegular() && fi.Size() > 0 {
		return one(backend.CapShallow, "")
	}
	return nil
}
