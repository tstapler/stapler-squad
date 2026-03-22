package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/mux"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// ExternalSessionDiscovery discovers and manages external Claude sessions
// from claude-mux multiplexed terminals.
type ExternalSessionDiscovery struct {
	discovery *mux.Discovery

	// External sessions discovered via mux
	sessions   map[string]*Instance
	sessionsMu sync.RWMutex

	// Callbacks for session events (supports multiple callbacks)
	onSessionAddedCallbacks   []func(*Instance)
	onSessionRemovedCallbacks []func(*Instance)

	// Context for lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// NewExternalSessionDiscovery creates a new external session discovery service.
func NewExternalSessionDiscovery() *ExternalSessionDiscovery {
	return &ExternalSessionDiscovery{
		discovery: mux.NewDiscovery(),
		sessions:  make(map[string]*Instance),
	}
}

// OnSessionAdded registers a callback for when a new external session is discovered.
// Multiple callbacks can be registered and will all be invoked.
func (e *ExternalSessionDiscovery) OnSessionAdded(callback func(*Instance)) {
	e.onSessionAddedCallbacks = append(e.onSessionAddedCallbacks, callback)
}

// OnSessionRemoved registers a callback for when an external session is removed.
// Multiple callbacks can be registered and will all be invoked.
func (e *ExternalSessionDiscovery) OnSessionRemoved(callback func(*Instance)) {
	e.onSessionRemovedCallbacks = append(e.onSessionRemovedCallbacks, callback)
}

// Start begins periodic discovery of external sessions.
func (e *ExternalSessionDiscovery) Start(interval time.Duration) {
	e.ctx, e.cancel = context.WithCancel(context.Background())

	// Register for discovery events
	e.discovery.OnSessionChange(func(discovered *mux.DiscoveredSession, isNew bool) {
		if isNew {
			e.handleNewSession(discovered)
		} else {
			e.handleRemovedSession(discovered)
		}
	})

	// Fast initial discovery via tmux user options (single tmux list-sessions call).
	// Run before polling so sessions are available immediately at startup.
	if _, err := e.discovery.ScanFromUserOptions(); err != nil {
		log.WarningLog.Printf("ScanFromUserOptions: %v", err)
	}

	// Start polling
	e.discovery.StartPolling(e.ctx, interval)

	log.InfoLog.Printf("External session discovery started (interval: %v)", interval)
}

// Stop stops the discovery service.
func (e *ExternalSessionDiscovery) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	log.InfoLog.Println("External session discovery stopped")
}

// GetSessions returns all currently discovered external sessions.
func (e *ExternalSessionDiscovery) GetSessions() []*Instance {
	e.sessionsMu.RLock()
	defer e.sessionsMu.RUnlock()

	sessions := make([]*Instance, 0, len(e.sessions))
	for _, session := range e.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// GetSession returns a specific external session by socket path (deprecated - use GetSessionByTmux).
func (e *ExternalSessionDiscovery) GetSession(socketPath string) *Instance {
	e.sessionsMu.RLock()
	defer e.sessionsMu.RUnlock()
	return e.sessions[socketPath]
}

// GetSessionByTmux returns a specific external session by tmux session name.
func (e *ExternalSessionDiscovery) GetSessionByTmux(tmuxSessionName string) *Instance {
	e.sessionsMu.RLock()
	defer e.sessionsMu.RUnlock()

	for _, instance := range e.sessions {
		if instance.ExternalMetadata != nil && instance.ExternalMetadata.TmuxSessionName == tmuxSessionName {
			return instance
		}
	}
	return nil
}

// handleNewSession creates an Instance wrapper for a newly discovered mux session.
func (e *ExternalSessionDiscovery) handleNewSession(discovered *mux.DiscoveredSession) {
	if discovered.Metadata == nil {
		log.WarningLog.Printf("Discovered session without metadata: %s", discovered.SocketPath)
		return
	}

	// Skip sessions without tmux integration - we need this for unified streaming
	if discovered.Metadata.TmuxSession == "" {
		log.WarningLog.Printf("Discovered session without tmux session name: %s (cannot attach for unified streaming)",
			discovered.SocketPath)
		return
	}

	// Create a unique title for this external session
	title := generateExternalTitle(discovered.Metadata)

	// Create Instance wrapper
	now := time.Now()
	instance := &Instance{
		Title:                title,
		Path:                 discovered.Metadata.Cwd,
		Program:              discovered.Metadata.Command,
		Status:               Running,
		InstanceType:         InstanceTypeExternal,
		Category:             "External",
		Tags:                 []string{"external", "mux"},
		CreatedAt: now, // Initialize timestamps to avoid stale notifications
		UpdatedAt: now,
		ReviewState: ReviewState{
			LastTerminalUpdate:   now,
			LastMeaningfulOutput: now, // Initialize to now - external sessions have output when discovered
		},
		ExternalMetadata: &ExternalInstanceMetadata{
			MuxSocketPath:   discovered.SocketPath,
			MuxEnabled:      true,
			SourceTerminal:  guessSourceTerminal(discovered.Metadata),
			DiscoveredAt:    now,
			LastSeen:        now,
			OriginalPID:     discovered.Metadata.PID,
			TmuxSessionName: discovered.Metadata.TmuxSession, // For unified tmux control
		},
		// Use mux permissions which enable destroy (unified architecture)
		Permissions: GetMuxExternalPermissions(),
	}

	// UNIFIED ARCHITECTURE: Attach to the existing tmux session so external sessions
	// use the same streaming/resize infrastructure as regular sessions.
	// This enables GetPTYReader() to work, which is required for WebSocket streaming.
	tmuxSession := tmux.NewTmuxSessionFromExisting(discovered.Metadata.TmuxSession)
	if err := tmuxSession.AttachToExisting(); err != nil {
		log.ErrorLog.Printf("Failed to attach to tmux session '%s' for external session '%s': %v",
			discovered.Metadata.TmuxSession, title, err)
		// Continue without PTY attachment - session will still be visible but streaming won't work
		// The streamExternalTerminal fallback can still handle it via capture-pane polling
	} else {
		// Successfully attached - set the tmux session on the instance
		// This also sets instance.started = true, enabling GetPTYReader()
		instance.SetTmuxSession(tmuxSession)
		log.InfoLog.Printf("Attached to tmux session '%s' for unified streaming of external session '%s'",
			discovered.Metadata.TmuxSession, title)
	}

	// Register the session
	e.sessionsMu.Lock()
	e.sessions[discovered.SocketPath] = instance
	e.sessionsMu.Unlock()

	log.InfoLog.Printf("Discovered external Claude session: %s (socket: %s, cwd: %s, tmux: %s)",
		title, discovered.SocketPath, discovered.Metadata.Cwd, discovered.Metadata.TmuxSession)

	// Notify all registered callbacks
	for _, callback := range e.onSessionAddedCallbacks {
		callback(instance)
	}
}

// handleRemovedSession removes an Instance when the mux session disconnects.
func (e *ExternalSessionDiscovery) handleRemovedSession(discovered *mux.DiscoveredSession) {
	e.sessionsMu.Lock()
	instance, exists := e.sessions[discovered.SocketPath]
	if exists {
		delete(e.sessions, discovered.SocketPath)
	}
	e.sessionsMu.Unlock()

	if exists {
		log.InfoLog.Printf("External session disconnected: %s", instance.Title)

		// Notify all registered callbacks
		for _, callback := range e.onSessionRemovedCallbacks {
			callback(instance)
		}
	}
}

// generateExternalTitle creates a display title for an external session.
// Includes PID to ensure uniqueness when multiple sessions run in the same directory.
func generateExternalTitle(meta *mux.SessionMetadata) string {
	// Use the directory name as the primary identifier
	dirName := filepath.Base(meta.Cwd)
	if dirName == "" || dirName == "." || dirName == "/" {
		dirName = "External"
	}

	// Include PID to differentiate multiple sessions in the same directory
	pid := meta.PID

	// Add command info if not claude
	if meta.Command != "claude" && !isClaudeCommand(meta.Command) {
		return fmt.Sprintf("%s (%s #%d)", dirName, filepath.Base(meta.Command), pid)
	}

	return fmt.Sprintf("%s (External #%d)", dirName, pid)
}

// guessSourceTerminal attempts to identify the source terminal from environment.
func guessSourceTerminal(meta *mux.SessionMetadata) string {
	// Check for common terminal indicators in environment
	if termProgram, ok := meta.Env["TERM_PROGRAM"]; ok {
		switch termProgram {
		case "iTerm.app":
			return "iTerm"
		case "vscode":
			return "VS Code"
		case "Apple_Terminal":
			return "Terminal.app"
		}
	}

	// Check for IDE-specific environment variables
	if _, ok := meta.Env["IDEA_INITIAL_DIRECTORY"]; ok {
		return "IntelliJ"
	}
	if _, ok := meta.Env["VSCODE_INJECTION"]; ok {
		return "VS Code"
	}

	// Check TERM variable
	if term, ok := meta.Env["TERM"]; ok {
		if term == "xterm-256color" {
			return "Terminal"
		}
	}

	return "Unknown"
}

// isClaudeCommand checks if a command is Claude-related.
func isClaudeCommand(cmd string) bool {
	base := filepath.Base(cmd)
	return base == "claude" || base == "claude-code"
}
