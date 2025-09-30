package terminal

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"claude-squad/log"

	tea "github.com/charmbracelet/bubbletea"
)

// ResizeMsg is sent when the terminal is resized
type ResizeMsg struct {
	Width  int
	Height int
}

// SignalManager handles terminal-related signal management
type SignalManager struct {
	sizeManager *Manager
}

// NewSignalManager creates a new signal manager with terminal size detection
func NewSignalManager(sizeManager *Manager) *SignalManager {
	return &SignalManager{
		sizeManager: sizeManager,
	}
}

// SetupResizeHandler sets up proper SIGWINCH signal handling for terminal resize events
func (sm *SignalManager) SetupResizeHandler() tea.Cmd {
	return func() tea.Msg {
		// Create a channel to receive SIGWINCH signals
		sigwinch := make(chan os.Signal, 1)
		signal.Notify(sigwinch, syscall.SIGWINCH)

		// Wait for SIGWINCH signal
		<-sigwinch

		// Get the new terminal size
		width, height, method := sm.sizeManager.GetReliableSize()
		log.InfoLog.Printf("SIGWINCH received - terminal resized to %dx%d (method: %s)", width, height, method)

		// For tiling window managers, detect if PTY size seems much larger than typical visible area
		if height > 80 || width > 200 {
			log.WarningLog.Printf("SIGWINCH: PTY reports large size (%dx%d) - may indicate tiling window manager with PTY/visible area mismatch", width, height)
		}

		return ResizeMsg{Width: width, Height: height}
	}
}

// CreateSizeCheckCmd creates a ticker for checking terminal size changes (for IntelliJ compatibility)
func (sm *SignalManager) CreateSizeCheckCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return SizeCheckMsg{}
	})
}

// SizeCheckMsg is sent periodically to check terminal size for IntelliJ compatibility
type SizeCheckMsg struct{}