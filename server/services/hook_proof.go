package services

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// hook_proof.go is the best-effort sender provenance of a permission-request
// hook (ADR-010 decision 2g): proof = HMAC-SHA256(secret, "ssq-hook-v1" || session
// UUID), delivered through a per-session 0600 file the shared hook command reads
// through STAPLER_SESSION_UUID, never in argv. It narrows who can create a
// replyable question; it is NOT the guard against typing into the wrong dialog
// (session.DialogMatch is), and it does not stop the agent or a same-user process.
const (
	hookProofHeader     = "X-CS-Hook-Proof"
	hookProofDomain     = "ssq-hook-v1"
	hookProofSecretFile = "hook-proof-secret"
	hookProofDirName    = "hook-proofs"
	hookProofSecretLen  = 32
	hookProofDirMode    = 0o700
	hookProofFileMode   = 0o600
)

// Causes of a question that is not replyable (the metric and metadata label set).
const (
	replyCauseNoProof  = "no_proof"
	replyCauseBadProof = "bad_proof"
	replyCauseShape    = "shape"
	replyCausePathOnly = "path_only"
)

// HookProofs signs and verifies hook proofs and writes the per-session files.
type HookProofs struct {
	mu     sync.RWMutex
	secret []byte
	dir    string // <config dir>/hook-proofs
}

// NewHookProofs loads (or creates, 0600) the secret under configDir. The
// directory is resolved once here, so a workspace switch cannot move it.
func NewHookProofs(configDir string) (*HookProofs, error) {
	secretPath := filepath.Join(configDir, hookProofSecretFile)
	// #nosec G304 -- configDir comes from config.GetConfigDir().
	b, err := os.ReadFile(secretPath)
	if err == nil && len(b) >= hookProofSecretLen {
		return &HookProofs{secret: b, dir: filepath.Join(configDir, hookProofDirName)}, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read hook proof secret: %w", err)
	}
	p := &HookProofs{dir: filepath.Join(configDir, hookProofDirName)}
	if err := p.rotateTo(secretPath); err != nil {
		return nil, err
	}
	return p, nil
}

// Rotate replaces the secret; every proof file must then be rewritten, which
// revokes running sessions (the hook reads the file on each run).
func (p *HookProofs) Rotate(configDir string) error {
	return p.rotateTo(filepath.Join(configDir, hookProofSecretFile))
}

func (p *HookProofs) rotateTo(secretPath string) error {
	secret := make([]byte, hookProofSecretLen)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("generate hook proof secret: %w", err)
	}
	if err := writeFileAtomic0600(secretPath, secret); err != nil {
		return err
	}
	p.mu.Lock()
	p.secret = secret
	p.mu.Unlock()
	return nil
}

func (p *HookProofs) mac(sessionUUID string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	m := hmac.New(sha256.New, p.secret)
	m.Write([]byte(hookProofDomain))
	m.Write([]byte(sessionUUID))
	return hex.EncodeToString(m.Sum(nil))
}

// HeaderValue is "<uuid>.<proof>".
func (p *HookProofs) HeaderValue(sessionUUID string) string {
	return sessionUUID + "." + p.mac(sessionUUID)
}

// Verify checks a header value in constant time and returns the session UUID it
// vouches for.
func (p *HookProofs) Verify(value string) (sessionUUID string, ok bool) {
	i := strings.LastIndexByte(value, '.')
	if i <= 0 || i == len(value)-1 {
		return "", false
	}
	id, got := value[:i], value[i+1:]
	if !hmac.Equal([]byte(got), []byte(p.mac(id))) {
		return "", false
	}
	return id, true
}

// ProofFilePath is where the hook command looks for the session's proof line.
func (p *HookProofs) ProofFilePath(sessionUUID string) string {
	return filepath.Join(p.dir, sessionUUID)
}

// EnsureFile (re)writes the session's proof file: one `X-CS-Hook-Proof: ...`
// line, 0600 in a 0700 directory, atomically. Idempotent.
func (p *HookProofs) EnsureFile(sessionUUID string) error {
	if sessionUUID == "" || strings.ContainsAny(sessionUUID, `/\`) || strings.Contains(sessionUUID, "..") {
		return errors.New("invalid session uuid for a hook proof file")
	}
	if err := os.MkdirAll(p.dir, hookProofDirMode); err != nil {
		return fmt.Errorf("create hook proof dir: %w", err)
	}
	line := hookProofHeader + ": " + p.HeaderValue(sessionUUID) + "\n"
	return writeFileAtomic0600(p.ProofFilePath(sessionUUID), []byte(line))
}

// RemoveFile deletes the session's proof file (idempotent).
func (p *HookProofs) RemoveFile(sessionUUID string) {
	if sessionUUID == "" || strings.ContainsAny(sessionUUID, `/\`) || strings.Contains(sessionUUID, "..") {
		return
	}
	_ = os.Remove(p.ProofFilePath(sessionUUID))
}

// writeFileAtomic0600 writes data to path via a temporary file created O_EXCL
// 0600 in the same directory, fsync, rename.
func writeFileAtomic0600(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, hookProofDirMode); err != nil {
		return err
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	tmp := filepath.Join(dir, "."+filepath.Base(path)+"."+hex.EncodeToString(nonce[:])+".tmp")
	// #nosec G304 -- tmp is built from a config-dir path and a random suffix.
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, hookProofFileMode)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// defaultHookProofs is the process-wide instance the injectors write proof
// files through; nil (no wiring, unit tests) means no proof is ever written.
var defaultHookProofs atomic.Pointer[HookProofs]

// SetDefaultHookProofs installs the process-wide proofs.
func SetDefaultHookProofs(p *HookProofs) { defaultHookProofs.Store(p) }

// DefaultHookProofs returns the installed proofs or nil.
func DefaultHookProofs() *HookProofs { return defaultHookProofs.Load() }

// ensureHookProofFile writes the proof file of sessionUUID when proofs are wired.
func ensureHookProofFile(sessionUUID string) {
	if p := DefaultHookProofs(); p != nil && sessionUUID != "" {
		if err := p.EnsureFile(sessionUUID); err != nil {
			log.Warn("[HookProof] proof file not written; the session's questions are not replyable", "err", err)
		}
	}
}

// RefreshInstanceHookProof rewrites the session's proof file and replaces a
// stale permission-request hook entry with the current command, so a session
// started before the proof existed gains it without an agent restart (a running
// Claude Code reads the rewritten settings.local.json at its next dialog; Spike
// 1.3g h8). It is a no-op for a remote session, a session with no UUID or an
// unwired process. Idempotent.
func RefreshInstanceHookProof(inst *session.Instance) {
	if DefaultHookProofs() == nil || inst == nil || inst.UUID == "" || inst.IsRemote() {
		return
	}
	root := inst.GetEffectiveRootDir()
	if root == "" {
		return
	}
	if err := InjectHookConfig(root, inst.Snapshot().Title, inst.UUID); err != nil {
		log.Warn("[HookProof] refresh failed", "session", inst.UUID, "err", err)
	}
}

// RefreshHookProofs refreshes every live local session once, at service boot.
// A session whose entry already has the current command is left untouched.
func RefreshHookProofs(instances []*session.Instance) int {
	n := 0
	for _, inst := range instances {
		if inst == nil || !inst.Started() || inst.Snapshot().Status.IsSuspended() {
			continue
		}
		RefreshInstanceHookProof(inst)
		n++
	}
	return n
}
