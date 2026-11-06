package services

import (
	"crypto/hmac"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// mcp_proof.go is the per-session MCP credential of ADR-007 rule 0:
// proof = "ssqmcp1." + hex HMAC-SHA256(secret, "ssq-mcp-v1" || session UUID),
// built on the HookProofs secret (so HookProofs.Rotate revokes MCP proofs too)
// with its own domain string, which makes it non-interchangeable with a hook
// proof. It is delivered only through 0600 per-session files, never argv or
// the tmux environment. It defeats header forgery by UUID knowledge; it does
// not stop a same-user process or an agent that makes another session print a
// proof file (accepted risk, see docs/reference/wake-triggers-and-agent-messaging.md).
const mcpProofDomain = "ssq-mcp-v1"

// McpProofs mints, verifies and stores per-session MCP proofs.
type McpProofs struct {
	keys      *HookProofs
	configDir string
}

// NewMcpProofs builds McpProofs over keys' secret, with proof files under
// <configDir>/mcp-proofs and per-session configs under <configDir>/mcp.
func NewMcpProofs(keys *HookProofs, configDir string) *McpProofs {
	return &McpProofs{keys: keys, configDir: configDir}
}

// HeaderValue is the X-Stapler-Session-Proof value for sessionUUID.
func (m *McpProofs) HeaderValue(sessionUUID string) string {
	return session.MCPProofValuePrefix + m.keys.macForDomain(mcpProofDomain, sessionUUID)
}

// Verify reports whether value is sessionUUID's proof, in constant time. A
// proof minted for another session, or a hook proof, never verifies.
func (m *McpProofs) Verify(sessionUUID, value string) bool {
	if !session.ValidMCPSessionUUID(sessionUUID) || !strings.HasPrefix(value, session.MCPProofValuePrefix) {
		return false
	}
	return hmac.Equal([]byte(value), []byte(m.HeaderValue(sessionUUID)))
}

// ProofFilePath is the session's proof file (read by the stdio proxy and by
// OpenCode's {file:} substitution).
func (m *McpProofs) ProofFilePath(sessionUUID string) string {
	return session.MCPProofFilePath(m.configDir, sessionUUID)
}

// EnsureFile (re)writes the session's proof file: the bare header value, no
// newline (OpenCode substitutes the file's content verbatim), 0600 in a 0700
// directory, atomically. Idempotent.
func (m *McpProofs) EnsureFile(sessionUUID string) error {
	if !session.ValidMCPSessionUUID(sessionUUID) {
		return errors.New("invalid session uuid for an mcp proof file")
	}
	return writeFileAtomic0600(m.ProofFilePath(sessionUUID), []byte(m.HeaderValue(sessionUUID)))
}

// Provider is the session.SetMCPProofProvider callback: it writes the proof
// file and returns the header value. When the file cannot be written the
// header is still returned (the Claude config carries it itself); only the
// stdio-proxy and OpenCode paths, which read the file, are then unverified.
func (m *McpProofs) Provider() func(sessionUUID string) (string, bool) {
	return func(sessionUUID string) (string, bool) {
		if !session.ValidMCPSessionUUID(sessionUUID) {
			return "", false
		}
		if err := m.EnsureFile(sessionUUID); err != nil {
			log.Warn("[McpProof] proof file not written; proxy and OpenCode paths stay unverified", "session", sessionUUID, "err", err)
		}
		return m.HeaderValue(sessionUUID), true
	}
}

// sessionFiles lists every file that holds or references sessionUUID's proof.
func (m *McpProofs) sessionFiles(sessionUUID string) []string {
	mcpDir := filepath.Join(m.configDir, session.MCPConfigDirName)
	return []string{
		m.ProofFilePath(sessionUUID),
		filepath.Join(mcpDir, sessionUUID+".json"),
		filepath.Join(mcpDir, sessionUUID+".opencode.json"),
	}
}

// RemoveFiles deletes the session's proof file and per-session configs
// (idempotent). Called when the session is deleted.
func (m *McpProofs) RemoveFiles(sessionUUID string) {
	if !session.ValidMCPSessionUUID(sessionUUID) {
		return
	}
	for _, path := range m.sessionFiles(sessionUUID) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Warn("[McpProof] could not remove proof file", "path", path, "err", err)
		}
	}
}

// SweepStale removes proof files and per-session configs whose session is not
// known, returning how many files it removed. Startup runs it once: nothing
// else removes mcp/*.json, so a file for a dead UUID would still verify.
func (m *McpProofs) SweepStale(known func(sessionUUID string) bool) int {
	removed := 0
	for _, dir := range []string{
		filepath.Join(m.configDir, session.MCPProofsDirName),
		filepath.Join(m.configDir, session.MCPConfigDirName),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			uuid := sessionUUIDFromProofFile(e.Name())
			if e.IsDir() || uuid == "" || known(uuid) {
				continue
			}
			if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed
}

// sessionUUIDFromProofFile maps "<uuid>", "<uuid>.json" and
// "<uuid>.opencode.json" to the uuid; other names (temp files) map to "".
func sessionUUIDFromProofFile(name string) string {
	if strings.HasPrefix(name, ".") {
		return ""
	}
	for _, suffix := range []string{".opencode.json", ".json"} {
		if base, ok := strings.CutSuffix(name, suffix); ok {
			return base
		}
	}
	return name
}

// SelfCheck mints, writes and verifies a proof for a synthetic session and
// removes it again, so a broken credential path is visible in the startup log
// rather than as "the tools aren't working" (pre-mortem #13).
func (m *McpProofs) SelfCheck() error {
	const synthetic = "self-check-00000000-0000-0000-0000-000000000000"
	defer m.RemoveFiles(synthetic)
	if err := m.EnsureFile(synthetic); err != nil {
		return fmt.Errorf("write proof file: %w", err)
	}
	onDisk, err := os.ReadFile(m.ProofFilePath(synthetic))
	if err != nil {
		return fmt.Errorf("read proof file back: %w", err)
	}
	if !m.Verify(synthetic, string(onDisk)) {
		return errors.New("proof read back from disk does not verify")
	}
	if m.Verify(synthetic+"x", string(onDisk)) {
		return errors.New("proof verifies for a different session")
	}
	return nil
}

// defaultMcpProofs is the process-wide instance the MCP middleware and the
// tools consult; nil (unwired, unit tests) means nothing verifies.
var defaultMcpProofs atomic.Pointer[McpProofs]

// SetDefaultMcpProofs installs the process-wide proofs.
func SetDefaultMcpProofs(p *McpProofs) { defaultMcpProofs.Store(p) }

// DefaultMcpProofs returns the installed proofs or nil.
func DefaultMcpProofs() *McpProofs { return defaultMcpProofs.Load() }
