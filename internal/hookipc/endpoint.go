package hookipc

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/internal/unixsocket"
)

const (
	SocketEnvironment      = "SSQ_HOOK_SOCKET"
	FingerprintEnvironment = "SSQ_HOOK_INSTANCE_FINGERPRINT"
	ProtocolEnvironment    = "SSQ_HOOK_PROTOCOL"
)

type HookEndpoint struct {
	SocketPath          string
	InstanceFingerprint InstanceFingerprint
	ProtocolVersion     ProtocolVersion
}

// ResolveEndpoint resolves exactly one endpoint from the same config-directory
// hierarchy used by the caller's cwd. It never scans for listeners and never
// falls back from an isolated namespace to the shared instance.
func ResolveEndpoint(cwd string) (HookEndpoint, error) {
	configDir, err := config.GetConfigDirForDir(cwd)
	if err != nil {
		return HookEndpoint{}, fmt.Errorf("hookipc: resolve config directory: %w", err)
	}
	canonicalDir, err := filepath.Abs(filepath.Clean(configDir))
	if err != nil {
		return HookEndpoint{}, fmt.Errorf("hookipc: canonicalize config directory: %w", err)
	}

	endpoint := HookEndpoint{
		InstanceFingerprint: Fingerprint(canonicalDir),
		ProtocolVersion:     CurrentProtocolVersion,
	}
	if value := os.Getenv(FingerprintEnvironment); value != "" && InstanceFingerprint(value) != endpoint.InstanceFingerprint {
		return HookEndpoint{}, fmt.Errorf("%w: injected fingerprint does not match resolved state", ErrInstanceMismatch)
	}
	if value := os.Getenv(ProtocolEnvironment); value != "" {
		version, err := strconv.ParseUint(value, 10, 16)
		if err != nil {
			return HookEndpoint{}, fmt.Errorf("hookipc: parse injected protocol: %w", err)
		}
		if ProtocolVersion(version) != CurrentProtocolVersion {
			return HookEndpoint{}, fmt.Errorf("%w: injected version %d", ErrProtocolMismatch, version)
		}
	}
	if value := os.Getenv(SocketEnvironment); value != "" {
		endpoint.SocketPath = value
		return endpoint, nil
	}
	endpoint.SocketPath, err = unixsocket.Path("ssq-hook", "classifier.sock", canonicalDir)
	if err != nil {
		return HookEndpoint{}, fmt.Errorf("hookipc: resolve socket path: %w", err)
	}
	return endpoint, nil
}
