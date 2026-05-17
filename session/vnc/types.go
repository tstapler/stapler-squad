// Package vnc provides per-session virtual display and VNC server lifecycle management.
// On Linux, each session can own a dedicated Xvfb virtual framebuffer and a paired
// x11vnc server. On non-Linux platforms or when required binaries are absent,
// a no-op manager is returned and all VNC functionality is gracefully disabled.
package vnc

// VNCStatus represents the operational state of the VNC subsystem for a session.
type VNCStatus int

const (
	// VNCStatusUnspecified is the zero value; not yet initialized.
	VNCStatusUnspecified VNCStatus = iota
	// VNCStatusStarting means Xvfb/x11vnc processes are launching.
	VNCStatusStarting
	// VNCStatusReady means a browser window has been detected and x11vnc is
	// focused on it via -id <windowID>.
	VNCStatusReady
	// VNCStatusNoBrowser means VNC is running (full display mode) but no browser
	// window has been detected yet on the virtual display.
	VNCStatusNoBrowser
	// VNCStatusUnavailable means VNC is not available for this session — either
	// required binaries are missing, the platform is not Linux, or startup failed
	// after all retry attempts.
	VNCStatusUnavailable
)

// String returns a human-readable name for the VNCStatus.
func (s VNCStatus) String() string {
	switch s {
	case VNCStatusUnspecified:
		return "Unspecified"
	case VNCStatusStarting:
		return "Starting"
	case VNCStatusReady:
		return "Ready"
	case VNCStatusNoBrowser:
		return "NoBrowser"
	case VNCStatusUnavailable:
		return "Unavailable"
	default:
		return "Unknown"
	}
}

// VNCConfig holds configuration for the VNC process manager.
// Populated from config.BrowserPassthroughConfig and passed to New().
type VNCConfig struct {
	// DisplayBase is the first X11 display number to try allocating (e.g. 100).
	DisplayBase int
	// DisplayRangeMax is the number of display numbers to search above DisplayBase.
	// Displays DisplayBase through DisplayBase+DisplayRangeMax-1 are candidates.
	DisplayRangeMax int
	// Resolution is the Xvfb screen resolution string, e.g. "1280x800x24".
	Resolution string
	// MaxRestarts is the maximum number of x11vnc crash-restart attempts before
	// the manager sets VNCStatusUnavailable. Default 3.
	MaxRestarts int
	// SessionID is the identifier of the owning session; used by DisplayAllocator
	// to associate the display lock with a specific session.
	SessionID string
}

// DefaultVNCConfig returns a VNCConfig with sensible defaults.
func DefaultVNCConfig() VNCConfig {
	return VNCConfig{
		DisplayBase:     100,
		DisplayRangeMax: 100,
		Resolution:      "1280x800x24",
		MaxRestarts:     3,
	}
}

// VNCState is the Go-native (non-proto) runtime state of the VNC subsystem.
// It is periodically mapped to the proto VNCState by the session service layer.
type VNCState struct {
	// Status is the current operational status.
	Status VNCStatus
	// DisplayNumber is the allocated X11 display number (e.g. 100 for :100).
	// Zero if no display has been allocated.
	DisplayNumber int
	// Port is the localhost TCP port that x11vnc is listening on.
	// Zero if x11vnc is not running.
	Port int
	// BrowserWindowDetected is true when a browser window has been found on the
	// virtual display and x11vnc has been restarted in -id mode targeting it.
	BrowserWindowDetected bool
}
