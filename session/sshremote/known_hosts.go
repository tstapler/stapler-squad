package sshremote

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// knownHostsFileName is the file KnownHostsStore reads/writes under the
// app's config dir -- distinct from the user's personal ~/.ssh/known_hosts,
// so host-key trust decisions for Stapler Squad remotes are made explicitly
// through this app (via Trust) rather than silently inherited from unrelated
// system state. See Epic 3.3 in
// project_plans/ssh-remote-workspaces/implementation/plan.md.
const knownHostsFileName = "ssh_known_hosts"

// ErrUnknownHostKey is re-exported from session/tmux so callers of Verify
// (and RemoteService, which maps it to a structured RPC response) only ever
// need to errors.As against one type, whether the error came from this
// store's Verify directly or, transitively, from a *tmux.SSHRunner dialed
// with HostKeyCallback(). There's no import-cycle risk re-using it: tmux
// never imports sshremote (Phase 3's wiring layer, not yet written, is
// expected to import both from a third package rather than have tmux depend
// on this one -- see NewSSHRunner's doc comment).
type ErrUnknownHostKey = tmux.ErrUnknownHostKey

// HostKeyFingerprint is re-exported from session/tmux for the same reason:
// one fingerprint-computation implementation (ssh.FingerprintSHA256, OpenSSH's
// default "SHA256:..." display format), not two that could drift.
var HostKeyFingerprint = tmux.HostKeyFingerprint

// ErrHostKeyMismatch is returned by Verify when host was previously trusted
// under a DIFFERENT key than the one presented now -- knownhosts' own
// KeyError doc comment calls this out as a possible MITM signal. This is
// deliberately a distinct type from ErrUnknownHostKey: a caller that only
// checks for ErrUnknownHostKey (e.g. to decide whether to show a "trust and
// connect" prompt) must NOT have a key-mismatch silently fall through the
// same code path -- it needs its own explicit handling, and in particular
// must never be treated as "safe to connect."
type ErrHostKeyMismatch struct {
	// Host is the address (as passed to Verify) whose key changed.
	Host string
	// Fingerprint is the SHA256 fingerprint of the NEW, untrusted key that
	// was presented -- not any of the previously-trusted keys.
	Fingerprint string
	// Err is the underlying *knownhosts.KeyError.
	Err error
}

func (e *ErrHostKeyMismatch) Error() string {
	return fmt.Sprintf("ssh: host key for %s does not match the previously trusted key (fingerprint %s) -- possible MITM: %v", e.Host, e.Fingerprint, e.Err)
}

func (e *ErrHostKeyMismatch) Unwrap() error { return e.Err }

// KnownHostsStore is a file-backed, app-managed known_hosts-equivalent store
// for TOFU (trust-on-first-use) SSH host-key decisions, using
// golang.org/x/crypto/ssh/knownhosts' standard file format
// (knownhosts.New()/knownhosts.Line()) so the backing file stays inspectable
// with ordinary SSH tooling even though it's never read by a real ssh(1)/
// sshd(8) process.
type KnownHostsStore struct {
	mu   sync.Mutex
	path string
}

// KnownHostsStoreOption configures a KnownHostsStore at construction time.
type KnownHostsStoreOption func(*KnownHostsStore)

// withKnownHostsPath overrides the backing file path. Unexported: only
// same-package tests need to point at an isolated temp file; production and
// every other test get isolation for free from config.GetConfigDir()'s own
// test-mode auto-detection (see NewKnownHostsStore).
func withKnownHostsPath(path string) KnownHostsStoreOption {
	return func(s *KnownHostsStore) { s.path = path }
}

// NewKnownHostsStore constructs a KnownHostsStore backed by
// "<config dir>/ssh_known_hosts", where config dir is resolved via
// config.GetConfigDir() -- the same test-mode / named-instance /
// STAPLER_SQUAD_TEST_DIR isolation every other file-backed store in this
// app gets (see config.GetConfigDirForDir's priority hierarchy), so tests
// and named instances never read or write the real
// ~/.stapler-squad/ssh_known_hosts.
func NewKnownHostsStore(opts ...KnownHostsStoreOption) (*KnownHostsStore, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("sshremote: resolve config dir: %w", err)
	}

	s := &KnownHostsStore{path: filepath.Join(configDir, knownHostsFileName)}
	for _, opt := range opts {
		opt(s)
	}

	if err := s.ensureFile(); err != nil {
		return nil, err
	}
	return s, nil
}

// ensureFile creates the backing file (and its parent dir) if it doesn't
// exist yet -- knownhosts.New() errors on a missing file, so a brand-new
// store must start from an empty-but-present file rather than a missing one.
func (s *KnownHostsStore) ensureFile() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("sshremote: create known_hosts dir: %w", err)
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("sshremote: create known_hosts file: %w", err)
	}
	return f.Close()
}

// normalizeHostPort returns host in explicit "host:port" form, defaulting to
// port 22 when host doesn't already specify one. knownhosts' own
// hostKeyDB.check requires its `remote net.Addr`/`address string` arguments
// to already be splittable via net.SplitHostPort -- unlike knownhosts.Line/
// knownhosts.Normalize, which happily accept a bare hostname -- so this is a
// distinct helper from the storage-side normalization below, not a
// duplicate of it.
func normalizeHostPort(host string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, "22")
}

// fakeAddr adapts a "host:port" string into net.Addr. Verify has no real
// network connection to report a remote.Addr for (unlike a live SSH
// handshake) -- knownhosts' hostKeyDB.check only ever calls remote.String(),
// so a minimal String()-only implementation is sufficient to satisfy it.
type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

// checkHostKey loads the current known_hosts file and evaluates key against
// it for host, returning the RAW, untranslated result: nil (trusted match),
// a *knownhosts.KeyError (unknown host if Want is empty, mismatch
// otherwise), or another error (e.g. a corrupt known_hosts file). Verify and
// HostKeyCallback both funnel through this single implementation so there is
// exactly one place that talks to knownhosts.New()/the backing file for a
// read.
func (s *KnownHostsStore) checkHostKey(host string, key ssh.PublicKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cb, err := knownhosts.New(s.path)
	if err != nil {
		return fmt.Errorf("sshremote: load known_hosts: %w", err)
	}

	hostPort := normalizeHostPort(host)
	return cb(hostPort, fakeAddr(hostPort), key)
}

// IsHostKeyMismatch reports whether err represents knownhosts' key-mismatch
// signal: a *knownhosts.KeyError with a non-empty Want (per KeyError's own
// doc comment, Want non-empty means "there was a mismatch, which can signify
// a MITM attack," as opposed to Want empty, which means the host is simply
// unknown -- see ErrUnknownHostKey). errors.As unwraps err first, so this
// also matches an *ErrHostKeyMismatch (whose Unwrap returns the underlying
// *knownhosts.KeyError) as well as the raw, untranslated error
// HostKeyCallback()'s callback returns.
//
// Verify uses this internally to decide between ErrUnknownHostKey and
// ErrHostKeyMismatch. It's exported so callers on the HostKeyCallback()/
// tmux.SSHRunner dial path -- which, by HostKeyCallback's own design,
// surfaces the raw *knownhosts.KeyError rather than ErrHostKeyMismatch, see
// its doc comment -- can classify a dial failure identically, without a
// second, independently-maintained copy of this check. (A prior version of
// server/services/remote_service.go had exactly that second copy, checking
// for *ErrHostKeyMismatch on an error that could only ever be the raw
// *knownhosts.KeyError -- always false, so the MITM-specific error message
// was dead code. This helper exists so there is exactly one place that
// decides "is this a mismatch.")
func IsHostKeyMismatch(err error) bool {
	var keyErr *knownhosts.KeyError
	return errors.As(err, &keyErr) && len(keyErr.Want) > 0
}

// Verify checks key against the trust decision on file for host (a bare
// hostname or "host:port" address). Returns:
//   - nil if host+key was previously Trust()ed.
//   - *ErrUnknownHostKey if host has never been seen before.
//   - *ErrHostKeyMismatch if host WAS previously trusted, but under a
//     DIFFERENT key than the one presented now -- the actual MITM-relevant
//     case, deliberately a distinct error from ErrUnknownHostKey so a caller
//     can't silently reuse the same "go ahead and trust it" flow for both.
func (s *KnownHostsStore) Verify(host string, key ssh.PublicKey) error {
	err := s.checkHostKey(host, key)
	if err == nil {
		return nil
	}

	var keyErr *knownhosts.KeyError
	if errors.As(err, &keyErr) {
		if !IsHostKeyMismatch(err) {
			return &ErrUnknownHostKey{
				Host:        host,
				Fingerprint: HostKeyFingerprint(key),
				Err:         err,
			}
		}
		return &ErrHostKeyMismatch{
			Host:        host,
			Fingerprint: HostKeyFingerprint(key),
			Err:         err,
		}
	}
	return fmt.Errorf("sshremote: verify host key for %s: %w", host, err)
}

// HostKeyCallback returns an ssh.HostKeyCallback suitable for
// ssh.ClientConfig.HostKeyCallback (and, in particular, for
// tmux.NewSSHRunner's config argument). It returns the RAW knownhosts error
// on failure -- deliberately NOT pre-translated into ErrUnknownHostKey/
// ErrHostKeyMismatch -- so tmux.NewSSHRunner's own wrapHostKeyCallback (which
// every SSHRunner already wraps its configured callback in) is the single
// place that performs that translation for the real-dial path, exactly
// mirroring what Verify does for the direct-call path above.
func (s *KnownHostsStore) HostKeyCallback() ssh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		return s.checkHostKey(hostname, key)
	}
}

// IsHostTrusted reports whether ANY key is currently trusted for host,
// independent of which key it is. Exists for callers that need to enforce
// "TOFU already happened for this host" as a precondition WITHOUT holding a
// candidate key to Verify against -- server/services.RemoteService.
// CreateRemote is the motivating case: its contract deliberately never
// dials the remote itself (see its own doc comment), so it never has a real
// key on hand, yet review found it was saving a RemoteConfig with no
// server-side check that TestRemoteConnection/TrustRemoteHostKey ever ran,
// relying entirely on frontend flow discipline.
//
// Implemented by probing checkHostKey with a freshly generated, never-
// persisted-anywhere throwaway key: IsHostKeyMismatch(err) on the result
// means an entry exists for host (just not matching the probe key, which is
// guaranteed since nothing else could ever hold it) -- this is the only way
// to ask "does the knownhosts.New callback have ANY entry for host" without
// already knowing the real key, since golang.org/x/crypto/ssh/knownhosts
// exposes no direct "has host" query and checkHostKey/Verify are both
// inherently key-comparison operations.
func (s *KnownHostsStore) IsHostTrusted(host string) (bool, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return false, fmt.Errorf("sshremote: generate probe key: %w", err)
	}
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		return false, fmt.Errorf("sshremote: build probe signer: %w", err)
	}

	err = s.checkHostKey(host, signer.PublicKey())
	if err == nil {
		// Astronomically unlikely (the probe key happened to match a stored
		// key) but still correctly "trusted" if it somehow occurred.
		return true, nil
	}
	return IsHostKeyMismatch(err), nil
}

// Trust records key as the trusted host key for host, persisting it via an
// atomic write (temp file + rename) to the backing file. Any existing
// entries for host are replaced, not merely appended to: this store models
// "the app's current trust decision per host" (one trusted key at a time),
// not OpenSSH's own known_hosts semantics of accumulating every key ever
// seen -- replacing stale entries closes the rollback-attack window a purely
// additive store would otherwise leave open after a legitimate key rotation
// (an old, now-revoked key would otherwise keep verifying successfully
// forever).
func (s *KnownHostsStore) Trust(host string, key ssh.PublicKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	storageKey := knownhosts.Normalize(host)

	existing, err := s.readLinesLocked()
	if err != nil {
		return fmt.Errorf("sshremote: read known_hosts: %w", err)
	}

	kept := existing[:0]
	for _, line := range existing {
		if !lineMatchesHost(line, storageKey) {
			kept = append(kept, line)
		}
	}
	kept = append(kept, knownhosts.Line([]string{host}, key))

	if err := s.writeLinesLocked(kept); err != nil {
		return fmt.Errorf("sshremote: write known_hosts: %w", err)
	}
	return nil
}

// readLinesLocked returns every non-empty line in the backing file. Caller
// must hold s.mu.
func (s *KnownHostsStore) readLinesLocked() ([]string, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// writeLinesLocked atomically rewrites the backing file's contents to
// lines. Caller must hold s.mu. Mirrors
// server/services/mcp_injector.go's writeSettingsAtomic: a unique temp file
// (not path+".tmp") avoids two concurrent writers clobbering each other's
// temp file mid-write, then os.Rename makes the swap atomic.
func (s *KnownHostsStore) writeLinesLocked(lines []string) error {
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup if rename fails

	var out strings.Builder
	for _, line := range lines {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	if _, err := tmp.WriteString(out.String()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod temp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("rename %s: %w", tmpPath, err)
	}
	return nil
}

// lineMatchesHost reports whether line's comma-separated host-pattern field
// contains storageKey exactly. This store only ever writes lines it
// generated itself via knownhosts.Line([]string{host}, key) -- a single,
// already-normalized host per line, never a hashed hostname or a
// multi-host pattern -- so an exact string match against the first
// whitespace-delimited field is sufficient; it doesn't need knownhosts'
// own wildcard/hash matching logic.
func lineMatchesHost(line, storageKey string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return false
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	for _, h := range strings.Split(fields[0], ",") {
		if h == storageKey {
			return true
		}
	}
	return false
}
