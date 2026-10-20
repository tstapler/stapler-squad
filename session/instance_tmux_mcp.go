package session

// instance_tmux_mcp.go holds the per-session MCP credential plumbing of
// ADR-007 rule 0: the proof is delivered only through 0600 files, never argv
// and never the tmux environment. The minting side lives in server/services
// (McpProofs); session reaches it through the injected provider because
// server/services imports session.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/localtoken"
)

const (
	// MCPConfigDirName holds the per-session MCP config files (0700 directory).
	MCPConfigDirName = "mcp"
	// MCPProofsDirName holds one proof file per session (0700 directory); the
	// stdio proxy and OpenCode's {file:} substitution read it.
	MCPProofsDirName = "mcp-proofs"
	// MCPProofHeader is the request header that carries the proof.
	MCPProofHeader = "X-Stapler-Session-Proof"
	// MCPProofValuePrefix starts every proof value, so the best-effort output
	// redaction matches proofs and not arbitrary 64-hex strings.
	MCPProofValuePrefix = "ssqmcp1."

	mcpServerName      = "stapler-squad"
	mcpDirMode         = 0o700
	mcpFileMode        = 0o600
	mcpProofRedaction  = "[redacted-proof]"
	opencodeConfigEnv  = "OPENCODE_CONFIG"
	opencodeConfigName = ".opencode.json"
)

var mcpProofValuePattern = regexp.MustCompile(regexp.QuoteMeta(MCPProofValuePrefix) + `[0-9a-f]{64}`)

// RedactMCPProofs replaces every proof value in s. Best effort, not a boundary
// (ADR-007 rule 0): it only covers the tools that call it.
func RedactMCPProofs(s string) string {
	return mcpProofValuePattern.ReplaceAllString(s, mcpProofRedaction)
}

// ValidMCPSessionUUID reports whether uuid is safe to use as a file name.
func ValidMCPSessionUUID(uuid string) bool {
	return uuid != "" && !strings.ContainsAny(uuid, `/\`) && !strings.Contains(uuid, "..")
}

// MCPProofFilePath is where a session's proof is kept for the stdio proxy.
func MCPProofFilePath(configDir, sessionUUID string) string {
	return filepath.Join(configDir, MCPProofsDirName, sessionUUID)
}

// MCPProofFilePresent reports whether the session has a proof file.
func MCPProofFilePresent(configDir, sessionUUID string) bool {
	if !ValidMCPSessionUUID(sessionUUID) {
		return false
	}
	info, err := os.Stat(MCPProofFilePath(configDir, sessionUUID))
	return err == nil && info.Mode().IsRegular()
}

// mcpProofProvider mints a session's proof header value; nil (boot window,
// unit tests) or ok=false means no proof is emitted.
var mcpProofProvider atomic.Pointer[func(sessionUUID string) (string, bool)]

// mcpProofHeaderValue returns this session's proof, or "" when none is available.
func (i *Instance) mcpProofHeaderValue() string {
	p := mcpProofProvider.Load()
	if p == nil || !ValidMCPSessionUUID(i.UUID) {
		return ""
	}
	value, ok := (*p)(i.UUID)
	if !ok {
		return ""
	}
	return value
}

// BuildLaunchCommandForTest exposes buildLaunchCommand to cross-package
// spawn-path tests (server/mcp).
func (i *Instance) BuildLaunchCommandForTest(resumeSessionID string) string {
	return i.buildLaunchCommand(resumeSessionID)
}

func marshalMCPServerConfig(url string, headers map[string]string) string {
	cfg, err := json.Marshal(map[string]any{"mcpServers": map[string]any{mcpServerName: map[string]any{
		"type": "http", "url": url, "headers": headers,
	}}})
	if err != nil { // unreachable: only strings
		return "{}"
	}
	return string(cfg)
}

// writeMCPConfigFile writes data to <configDir>/mcp/<name>, 0600 in a 0700
// directory (permissions are re-asserted: WriteFile keeps an old file's mode).
func writeMCPConfigFile(configDir, name string, data []byte) (string, error) {
	dir := filepath.Join(configDir, MCPConfigDirName)
	if err := os.MkdirAll(dir, mcpDirMode); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, mcpDirMode); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, mcpFileMode); err != nil {
		return "", err
	}
	if err := os.Chmod(path, mcpFileMode); err != nil {
		return "", err
	}
	return path, nil
}

// mcpCredentials are the secrets a session's MCP config may carry.
type mcpCredentials struct {
	Token string // local API token (require_local_auth); "" when none
	Proof string // per-session proof header value; "" when none
}

// claudeMCPConfigArgsWithCredentials builds the --mcp-config flag. Secrets
// (the local API token, the session proof) are written only to a 0600 file
// whose path is the argument. The inline fallback never carries a secret: when
// the file cannot be written the session launches without them (its identity
// is then unverified and the five identity-gated tools refuse it, fail closed).
func (i *Instance) claudeMCPConfigArgsWithCredentials(mcpURL, configDir string, creds mcpCredentials) (string, string) {
	public := map[string]string{"X-Stapler-Session-UUID": i.UUID}
	secret := map[string]string{"X-Stapler-Session-UUID": i.UUID}
	if creds.Token != "" {
		secret["Authorization"] = "Bearer " + creds.Token
	}
	if creds.Proof != "" {
		secret[MCPProofHeader] = creds.Proof
	}
	inline := func() (string, string) {
		return "--mcp-config", shellQuote(marshalMCPServerConfig(mcpURL, public))
	}
	if len(secret) == len(public) {
		return inline()
	}
	if configDir == "" || !ValidMCPSessionUUID(i.UUID) {
		log.Warn("mcp config: no usable config dir for a credential-bearing config; launching without credentials")
		return inline()
	}
	path, err := writeMCPConfigFile(configDir, i.UUID+".json", []byte(marshalMCPServerConfig(mcpURL, secret)))
	if err != nil {
		log.Warn("mcp config: could not write credential-bearing config file; launching without credentials", "err", err)
		return inline()
	}
	return "--mcp-config", shellQuote(path)
}

// opencodeMCPEnvPrefix returns "OPENCODE_CONFIG='<file>' " for an OpenCode
// launch, or "" when no verified credential can be delivered. The proof stays
// in the 0600 proof file; the per-session config references it with
// OpenCode's {file:} substitution and only the config's path is in the
// command (spike S5, Task 0.5e).
func (i *Instance) opencodeMCPEnvPrefix() string {
	configDir, err := config.GetConfigDir()
	if err != nil || i.mcpProofHeaderValue() == "" || !MCPProofFilePresent(configDir, i.UUID) {
		return ""
	}
	mcpURL := i.resolveMCPServerURLFrom(i.mcpServerURLProvider.Load())
	if mcpURL == "" {
		return ""
	}
	headers := map[string]string{
		"X-Stapler-Session-UUID": i.UUID,
		MCPProofHeader:           "{file:" + MCPProofFilePath(configDir, i.UUID) + "}",
	}
	if token := localtoken.FromConfigDir(configDir); token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	cfg, err := json.Marshal(map[string]any{"mcp": map[string]any{mcpServerName: map[string]any{
		"type": "remote", "url": mcpURL, "enabled": true, "headers": headers,
	}}})
	if err != nil { // unreachable: only strings
		return ""
	}
	path, err := writeMCPConfigFile(configDir, i.UUID+opencodeConfigName, cfg)
	if err != nil {
		log.Warn("mcp config: could not write OpenCode config; launching without a verified credential", "err", err)
		return ""
	}
	return opencodeConfigEnv + "=" + shellQuote(path) + " "
}
