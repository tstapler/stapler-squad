// Package cdp provides per-session Chrome DevTools Protocol (CDP) browser
// streaming. Each session owns a dedicated CDP port that Chrome listens on;
// the manager connects to Chrome via its CDP WebSocket endpoint and subscribes
// to Page.screencastFrame events to deliver a live JPEG frame stream.
//
// On Linux, Xvfb still provides the virtual display; on macOS, Chrome runs on
// the real display. CDP streaming works on both platforms, making it the
// cross-platform complement to the Linux-only VNC implementation.
package cdp

// CDPStatus represents the operational state of the CDP subsystem for a session.
type CDPStatus int

const (
	// CDPStatusUnspecified is the zero value; not yet initialized.
	CDPStatusUnspecified CDPStatus = iota
	// CDPStatusWaiting means polling for Chrome on the allocated CDP port.
	CDPStatusWaiting
	// CDPStatusStreaming means connected to Chrome and receiving screencast frames.
	CDPStatusStreaming
	// CDPStatusNoBrowser means the CDP port is allocated but Chrome has not yet
	// been detected (port allocated, Chrome not yet launched or not yet ready).
	CDPStatusNoBrowser
	// CDPStatusUnavailable means CDP is not available for this session — either
	// no Chrome binary was found, or startup failed.
	CDPStatusUnavailable
)

// String returns a human-readable name for the CDPStatus.
func (s CDPStatus) String() string {
	switch s {
	case CDPStatusUnspecified:
		return "Unspecified"
	case CDPStatusWaiting:
		return "Waiting"
	case CDPStatusStreaming:
		return "Streaming"
	case CDPStatusNoBrowser:
		return "NoBrowser"
	case CDPStatusUnavailable:
		return "Unavailable"
	default:
		return "Unknown"
	}
}

// CDPConfig holds configuration for the CDP stream manager.
// Populated from config.BrowserPassthroughConfig and passed to New().
type CDPConfig struct {
	// SessionID is the identifier of the owning session; used to create
	// per-session temporary directories for wrapper scripts.
	SessionID string
	// ChromePath is the resolved real Chrome binary path (not the wrapper script).
	// When empty, New() returns a noopCDPManager.
	ChromePath string
}

// CDPState is the Go-native (non-proto) runtime state of the CDP subsystem.
// It is periodically mapped to the proto CDPState by the session service layer.
type CDPState struct {
	// Status is the current operational status.
	Status CDPStatus
	// Port is the allocated CDP TCP port on localhost, or 0 if unavailable.
	Port int
}
