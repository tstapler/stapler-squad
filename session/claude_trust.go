package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// claudeTrustStore records that claude has been pre-authorized to skip its
// interactive first-run "trust this folder?" dialog for a directory.
// MarkTrusted is idempotent and best-effort: implementations return an error
// only for genuine I/O/parse failures, never for "already trusted".
//
// Two implementations: fileClaudeTrustStore (production default) persists
// into Claude Code's own ~/.claude.json, so a decision made here is honored
// by every claude invocation on the machine, not just stapler-squad's.
// memoryClaudeTrustStore (test default, and available for any test that
// wants to assert what got trusted) never touches disk. See
// Instance.trustStore's doc comment for how the choice between them is made.
type claudeTrustStore interface {
	MarkTrusted(dir string) error
}

// fileClaudeTrustStore is claudeTrustStore's real, disk-backed
// implementation: it pre-seeds ~/.claude.json's
// projects[absDir].hasTrustDialogAccepted, mirroring exactly what Claude Code
// itself writes there after a human answers the dialog once interactively --
// the same trust record, just written proactively by the tool that's about
// to launch claude instead of collected reactively from a person at a
// keyboard that, in an automated/workflow-run session, isn't there.
type fileClaudeTrustStore struct{}

// MarkTrusted implements claudeTrustStore. Preserves every other key in the
// file and in the target project's own entry, touching only
// hasTrustDialogAccepted.
func (fileClaudeTrustStore) MarkTrusted(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for %q: %w", dir, err)
	}

	path, err := claudeGlobalConfigPath()
	if err != nil {
		return fmt.Errorf("failed to resolve claude config path: %w", err)
	}

	raw, err := os.ReadFile(path) //nolint:gosec -- fixed path under $HOME, not user-controlled
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	root := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &root); err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}
	}

	projects, _ := root["projects"].(map[string]any)
	if projects == nil {
		projects = map[string]any{}
	}
	entry, _ := projects[absDir].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	if trusted, ok := entry["hasTrustDialogAccepted"].(bool); ok && trusted {
		return nil // already trusted -- skip the rewrite
	}
	entry["hasTrustDialogAccepted"] = true
	projects[absDir] = entry
	root["projects"] = projects

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", path, err)
	}

	// Write via temp file + rename: ~/.claude.json is read by every claude
	// invocation on the machine (not just stapler-squad's), so a crash
	// mid-write must never leave it truncated or corrupt.
	tmp := path + ".tmp-" + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("failed to write temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to rename temp file into %s: %w", path, err)
	}
	return nil
}

// claudeGlobalConfigPath returns the path to Claude Code's own global config
// file (~/.claude.json), where it persists per-project settings including the
// one-time workspace-trust decision.
func claudeGlobalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", err)
	}
	return filepath.Join(home, ".claude.json"), nil
}

// memoryClaudeTrustStore is claudeTrustStore's in-memory implementation:
// never touches disk, so it's safe as the default for `go test` binaries and
// useful directly in any test that wants to assert which directories got
// trusted.
type memoryClaudeTrustStore struct {
	mu      sync.Mutex
	trusted map[string]bool
}

func newMemoryClaudeTrustStore() *memoryClaudeTrustStore {
	return &memoryClaudeTrustStore{trusted: map[string]bool{}}
}

// MarkTrusted implements claudeTrustStore.
func (s *memoryClaudeTrustStore) MarkTrusted(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for %q: %w", dir, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trusted[absDir] = true
	return nil
}

// IsTrusted reports whether MarkTrusted has recorded dir. Test-only helper —
// production code never needs to read this back.
func (s *memoryClaudeTrustStore) IsTrusted(dir string) bool {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trusted[absDir]
}

// trustStore returns i's claudeTrustStore, lazily defaulting on first use:
// fileClaudeTrustStore in production, memoryClaudeTrustStore under
// config.IsIsolatedInstance() (true for every `go test` binary via
// IsTestMode(), plus named/STAPLER_SQUAD_TEST_DIR instances). This is the
// single place that default lives -- everywhere else just calls
// i.trustStore().MarkTrusted(dir) against whatever was resolved (or
// explicitly injected by a test that wants to assert on it) here.
//
// This exists because ~/.claude.json is real, shared, developer-machine
// state outside every one of this app's own isolation mechanisms (unlike
// ~/.stapler-squad, it has no STAPLER_SQUAD_* override) -- initTmuxSession(),
// which calls markWorkingDirTrusted(), is exercised by plenty of tests that
// construct real claude-program Instances, and those must never write to the
// developer's real Claude Code config.
func (i *Instance) trustStore() claudeTrustStore {
	i.trustStoreMu.Lock()
	defer i.trustStoreMu.Unlock()
	if i.claudeTrustStoreImpl == nil {
		if config.IsIsolatedInstance() {
			i.claudeTrustStoreImpl = newMemoryClaudeTrustStore()
		} else {
			i.claudeTrustStoreImpl = fileClaudeTrustStore{}
		}
	}
	return i.claudeTrustStoreImpl
}

// markWorkingDirTrusted pre-trusts i's effective working directory for
// claude sessions only -- see claudeTrustStore's doc comment. No-op for
// every other agent (aider, pi, ...), none of which read ~/.claude.json.
// Logs and continues on failure rather than propagating it: this is a
// startup-latency optimization (skip a dialog) at worst, never something
// session creation should fail over.
func (i *Instance) markWorkingDirTrusted() {
	if !isClaude(i.Program) {
		return
	}
	dir := i.GetEffectiveRootDir()
	if dir == "" {
		return
	}
	if err := i.trustStore().MarkTrusted(dir); err != nil {
		log.Warn("failed to pre-trust working directory for claude", "session", i.Title, "path", dir, "err", err)
	}
}
