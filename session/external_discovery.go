package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"claude-squad/log"
	"claude-squad/session/mux"
)

// ExternalSessionDiscovery discovers and manages external Claude sessions
// from claude-mux multiplexed terminals.
type ExternalSessionDiscovery struct {
	discovery *mux.Discovery

	// External sessions discovered via mux
	sessions   map[string]*Instance
	sessionsMu sync.RWMutex

	// Callbacks for session events
	onSessionAdded   func(*Instance)
	onSessionRemoved func(*Instance)

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
func (e *ExternalSessionDiscovery) OnSessionAdded(callback func(*Instance)) {
	e.onSessionAdded = callback
}

// OnSessionRemoved registers a callback for when an external session is removed.
func (e *ExternalSessionDiscovery) OnSessionRemoved(callback func(*Instance)) {
	e.onSessionRemoved = callback
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

// GetSession returns a specific external session by socket path.
func (e *ExternalSessionDiscovery) GetSession(socketPath string) *Instance {
	e.sessionsMu.RLock()
	defer e.sessionsMu.RUnlock()
	return e.sessions[socketPath]
}

// handleNewSession creates an Instance wrapper for a newly discovered mux session.
func (e *ExternalSessionDiscovery) handleNewSession(discovered *mux.DiscoveredSession) {
	if discovered.Metadata == nil {
		log.WarningLog.Printf("Discovered session without metadata: %s", discovered.SocketPath)
		return
	}

	// Create a unique title for this external session
	title := generateExternalTitle(discovered.Metadata)

	// Create Instance wrapper
	instance := &Instance{
		Title:        title,
		Path:         discovered.Metadata.Cwd,
		Program:      discovered.Metadata.Command,
		Status:       Running,
		InstanceType: InstanceTypeExternal,
		Category:     "External",
		Tags:         []string{"external", "mux"},
		ExternalMetadata: &ExternalInstanceMetadata{
			MuxSocketPath:  discovered.SocketPath,
			MuxEnabled:     true,
			SourceTerminal: guessSourceTerminal(discovered.Metadata),
			DiscoveredAt:   time.Now(),
			LastSeen:       time.Now(),
			OriginalPID:    discovered.Metadata.PID,
		},
		// Permissions are computed via GetPermissions() method
	}

	// Register the session
	e.sessionsMu.Lock()
	e.sessions[discovered.SocketPath] = instance
	e.sessionsMu.Unlock()

	log.InfoLog.Printf("Discovered external Claude session: %s (socket: %s, cwd: %s)",
		title, discovered.SocketPath, discovered.Metadata.Cwd)

	// Notify callback
	if e.onSessionAdded != nil {
		e.onSessionAdded(instance)
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

		// Notify callback
		if e.onSessionRemoved != nil {
			e.onSessionRemoved(instance)
		}
	}
}

// generateExternalTitle creates a display title for an external session.
func generateExternalTitle(meta *mux.SessionMetadata) string {
	// Use the directory name as the primary identifier
	dirName := filepath.Base(meta.Cwd)
	if dirName == "" || dirName == "." || dirName == "/" {
		dirName = "External"
	}

	// Add command info if not claude
	if meta.Command != "claude" && !isClaudeCommand(meta.Command) {
		return fmt.Sprintf("%s (%s)", dirName, filepath.Base(meta.Command))
	}

	return fmt.Sprintf("%s (External)", dirName)
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
