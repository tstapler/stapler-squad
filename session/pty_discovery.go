package session

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"claude-squad/log"
	"claude-squad/session/tmux"
)

// PTYStatus represents the current state of a PTY
type PTYStatus int

const (
	PTYReady PTYStatus = iota // Waiting for input
	PTYBusy                    // Executing command
	PTYIdle                    // No activity
	PTYError                   // Error state
)

func (s PTYStatus) String() string {
	switch s {
	case PTYReady:
		return "Ready"
	case PTYBusy:
		return "Busy"
	case PTYIdle:
		return "Idle"
	case PTYError:
		return "Error"
	default:
		return "Unknown"
	}
}

// PTYConnection represents a discovered PTY
type PTYConnection struct {
	Path         string            // /dev/pts/12
	PID          int               // Process ID
	Command      string            // "claude" or "aider"
	SessionName  string            // Associated squad session (if any)
	Status       PTYStatus         // Current status
	LastActivity time.Time         // Last activity timestamp
	Controller   *ClaudeController // Connected controller (if any)

	// Ownership and management metadata
	IsManaged       bool   // True if this is a squad-managed session
	TmuxSocket      string // Which tmux server socket (empty = default)
	TmuxSessionName string // Full tmux session name
	CanAttach       bool   // Whether attach operations are allowed
	CanDestroy      bool   // Whether destroy operations are allowed
	Owner           string // "squad" for managed, "external" for discovered
}

// PTYCategory represents grouping of PTYs
type PTYCategory int

const (
	PTYCategorySquad PTYCategory = iota // Squad-managed sessions
	PTYCategoryOrphaned                 // Unmanaged Claude instances
	PTYCategoryOther                    // Other tools (aider, etc.)
)

func (c PTYCategory) String() string {
	switch c {
	case PTYCategorySquad:
		return "Squad Sessions"
	case PTYCategoryOrphaned:
		return "Orphaned"
	case PTYCategoryOther:
		return "Other"
	default:
		return "Unknown"
	}
}

// PTYDiscovery manages PTY discovery and monitoring
type PTYDiscovery struct {
	mu            sync.RWMutex
	connections   []*PTYConnection
	sessionMap    map[string]*Instance // Session name -> Instance
	stopCh        chan struct{}
	refreshRate   time.Duration
	config        PTYDiscoveryConfig // Discovery configuration
}

// NewPTYDiscovery creates a new PTY discovery service with default configuration
func NewPTYDiscovery() *PTYDiscovery {
	return &PTYDiscovery{
		connections: make([]*PTYConnection, 0),
		sessionMap:  make(map[string]*Instance),
		stopCh:      make(chan struct{}),
		refreshRate: 5 * time.Second,
		config:      DefaultPTYDiscoveryConfig(),
	}
}

// NewPTYDiscoveryWithConfig creates a new PTY discovery service with custom configuration
func NewPTYDiscoveryWithConfig(config PTYDiscoveryConfig) *PTYDiscovery {
	return &PTYDiscovery{
		connections: make([]*PTYConnection, 0),
		sessionMap:  make(map[string]*Instance),
		stopCh:      make(chan struct{}),
		refreshRate: config.DiscoveryInterval,
		config:      config,
	}
}

// Start begins PTY discovery monitoring
func (pd *PTYDiscovery) Start() {
	go pd.monitorLoop()
}

// Stop halts PTY discovery monitoring
func (pd *PTYDiscovery) Stop() {
	close(pd.stopCh)
}

// SetSessions updates the session map for correlation
func (pd *PTYDiscovery) SetSessions(sessions []*Instance) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	pd.sessionMap = make(map[string]*Instance)
	for _, session := range sessions {
		pd.sessionMap[session.Title] = session
	}
}

// Refresh performs a full PTY discovery scan
func (pd *PTYDiscovery) Refresh() error {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	connections, err := pd.discoverPTYs()
	if err != nil {
		return err
	}

	pd.connections = connections
	return nil
}

// GetConnections returns all discovered PTY connections
func (pd *PTYDiscovery) GetConnections() []*PTYConnection {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	// Return a deep copy to prevent external modification
	result := make([]*PTYConnection, len(pd.connections))
	for i, conn := range pd.connections {
		connCopy := *conn
		result[i] = &connCopy
	}
	return result
}

// GetConnectionsByCategory returns PTYs grouped by category
func (pd *PTYDiscovery) GetConnectionsByCategory() map[PTYCategory][]*PTYConnection {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	result := make(map[PTYCategory][]*PTYConnection)
	result[PTYCategorySquad] = make([]*PTYConnection, 0)
	result[PTYCategoryOrphaned] = make([]*PTYConnection, 0)
	result[PTYCategoryOther] = make([]*PTYConnection, 0)

	for _, conn := range pd.connections {
		category := pd.categorizeConnection(conn)
		result[category] = append(result[category], conn)
	}

	return result
}

// GetConnection returns a specific PTY connection by path
func (pd *PTYDiscovery) GetConnection(path string) *PTYConnection {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	for _, conn := range pd.connections {
		if conn.Path == path {
			return conn
		}
	}
	return nil
}

// monitorLoop continuously monitors PTYs
func (pd *PTYDiscovery) monitorLoop() {
	ticker := time.NewTicker(pd.refreshRate)
	defer ticker.Stop()

	// Initial scan
	if err := pd.Refresh(); err != nil {
		log.ErrorLog.Printf("Initial PTY discovery failed: %v", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := pd.Refresh(); err != nil {
				log.ErrorLog.Printf("PTY discovery refresh failed: %v", err)
			}
		case <-pd.stopCh:
			return
		}
	}
}

// discoverPTYs performs the actual PTY discovery
func (pd *PTYDiscovery) discoverPTYs() ([]*PTYConnection, error) {
	connections := make([]*PTYConnection, 0)

	// Method 1: Discover from squad-managed sessions
	squadPTYs := pd.discoverSquadPTYs()
	connections = append(connections, squadPTYs...)

	// Method 2: Discover orphaned Claude processes in claudesquad_ prefixed sessions
	orphanedPTYs := pd.discoverOrphanedPTYs()
	connections = append(connections, orphanedPTYs...)

	// Method 3: Discover external Claude instances if enabled
	if pd.config.ShouldDiscoverExternal() {
		// Discover from default tmux server
		externalPTYs := pd.discoverExternalClaude("")
		connections = append(connections, externalPTYs...)

		// Discover from additional specified sockets
		for _, socket := range pd.config.ExternalSockets {
			morePTYs := pd.discoverExternalClaude(socket)
			connections = append(connections, morePTYs...)
		}
	}

	return connections, nil
}

// discoverSquadPTYs finds PTYs from managed sessions
func (pd *PTYDiscovery) discoverSquadPTYs() []*PTYConnection {
	connections := make([]*PTYConnection, 0)

	for sessionName, instance := range pd.sessionMap {
		if instance.Status != Running && instance.Status != Ready {
			continue
		}

		// Get PTY from instance (uses private tmuxSession field via GetTmuxSessionName)
		pty, pid, err := pd.getPTYForInstance(instance)
		if err != nil {
			log.DebugLog.Printf("Failed to get PTY for session %s: %v", sessionName, err)
			continue
		}

		conn := &PTYConnection{
			Path:            pty,
			PID:             pid,
			Command:         instance.Program, // Use actual program name
			SessionName:     sessionName,
			Status:          pd.detectPTYStatus(pty, pid),
			LastActivity:    time.Now(),
			IsManaged:       true,
			TmuxSocket:      instance.TmuxServerSocket,
			TmuxSessionName: tmux.ToClaudeSquadTmuxName(instance.Title),
			CanAttach:       true,
			CanDestroy:      true,
			Owner:           "squad",
		}

		connections = append(connections, conn)
	}

	return connections
}

// getPTYForInstance gets the PTY from an Instance using tmux's built-in info
func (pd *PTYDiscovery) getPTYForInstance(instance *Instance) (string, int, error) {
	// Get PTY info directly from tmux - this is cross-platform and more reliable
	// than trying to resolve file descriptors
	// Generate tmux session name from instance title using the same sanitization logic
	tmuxSession := tmux.ToClaudeSquadTmuxName(instance.Title)
	return pd.getPTYInfoFromTmux(tmuxSession)
}

// getPTYInfoFromTmux gets both PTY path and PID from tmux in one call
// This is cross-platform and works on Linux, macOS, and BSD systems
func (pd *PTYDiscovery) getPTYInfoFromTmux(sessionName string) (string, int, error) {
	// Use tmux's display-message to get both PTY device and PID
	// #{pane_tty} gives us /dev/pts/X (Linux) or /dev/ttysXXX (macOS)
	// #{pane_pid} gives us the process ID
	cmd := exec.Command("tmux", "display-message", "-p", "-t", sessionName,
		"#{pane_tty}:#{pane_pid}")
	output, err := cmd.Output()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get tmux pane info: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("unexpected tmux output format: %s", string(output))
	}

	ptyPath := parts[0]
	pid, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid PID '%s': %w", parts[1], err)
	}

	return ptyPath, pid, nil
}

// getPIDForPTY gets the PID using a PTY
func (pd *PTYDiscovery) getPIDForPTY(ptyPath string) (int, error) {
	// Use fuser to find which process is using this PTY
	cmd := exec.Command("fuser", ptyPath)
	output, err := cmd.Output()
	if err != nil {
		// fuser returns non-zero if no processes found, but that's okay for us
		// Just return a placeholder PID
		return 0, fmt.Errorf("fuser failed: %w", err)
	}

	pidStr := strings.TrimSpace(string(output))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID %q: %w", pidStr, err)
	}

	return pid, nil
}

// discoverOrphanedPTYs finds unmanaged Claude processes in claude-squad tmux sessions
// This only discovers Claude processes running in tmux sessions with the claude-squad prefix
func (pd *PTYDiscovery) discoverOrphanedPTYs() []*PTYConnection {
	connections := make([]*PTYConnection, 0)

	// Get all tmux sessions with claude-squad prefix
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// No tmux sessions found (normal case)
		return connections
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		sessionName := strings.TrimSpace(scanner.Text())

		// Only check sessions with claude-squad prefix
		if !strings.HasPrefix(sessionName, "claudesquad_") {
			continue
		}

		// Get PTY info for this tmux session
		pty, pid, err := pd.getPTYInfoFromTmux(sessionName)
		if err != nil {
			continue
		}

		// Check if this PID is already managed by a squad session
		if pd.isPIDManaged(pid) {
			continue
		}

		// Check if the process is actually Claude
		if !pd.isClaudeProcess(pid) {
			continue
		}

		conn := &PTYConnection{
			Path:            pty,
			PID:             pid,
			Command:         "claude",
			SessionName:     "",
			Status:          pd.detectPTYStatus(pty, pid),
			LastActivity:    time.Now(),
			IsManaged:       false,
			TmuxSocket:      "", // Default tmux server
			TmuxSessionName: sessionName,
			CanAttach:       pd.config.CanAttachExternal(),
			CanDestroy:      false, // Never allow destroying external instances
			Owner:           "external",
		}

		connections = append(connections, conn)
	}

	return connections
}

// isClaudeProcess checks if a PID is running Claude
func (pd *PTYDiscovery) isClaudeProcess(pid int) bool {
	// Get command line for this PID
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "command=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	cmdLine := strings.ToLower(strings.TrimSpace(string(output)))
	return strings.Contains(cmdLine, "claude")
}

// discoverExternalClaude discovers Claude instances from non-prefixed tmux sessions
// This discovers Claude instances NOT managed by claude-squad on the specified tmux server
// socket: tmux server socket name (empty string = default server)
func (pd *PTYDiscovery) discoverExternalClaude(socket string) []*PTYConnection {
	connections := make([]*PTYConnection, 0)

	// Build tmux command based on socket
	var cmd *exec.Cmd
	if socket != "" {
		cmd = exec.Command("tmux", "-L", socket, "list-sessions", "-F", "#{session_name}")
	} else {
		cmd = exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	}

	output, err := cmd.Output()
	if err != nil {
		// No tmux sessions found on this server (normal case)
		return connections
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		sessionName := strings.TrimSpace(scanner.Text())

		// Skip squad-managed sessions (they're handled by discoverSquadPTYs and discoverOrphanedPTYs)
		if strings.HasPrefix(sessionName, pd.config.ManagedPrefix) {
			continue
		}

		// Get PTY info for this tmux session
		pty, pid, err := pd.getPTYInfoFromTmuxWithSocket(sessionName, socket)
		if err != nil {
			log.DebugLog.Printf("Failed to get PTY info for external session %s (socket: %s): %v", sessionName, socket, err)
			continue
		}

		// Check if this PID is already managed by a squad session
		if pd.isPIDManaged(pid) {
			continue
		}

		// Check if the process is actually Claude
		if !pd.isClaudeProcess(pid) {
			continue
		}

		conn := &PTYConnection{
			Path:            pty,
			PID:             pid,
			Command:         "claude",
			SessionName:     "", // External instances don't have squad session names
			Status:          pd.detectPTYStatus(pty, pid),
			LastActivity:    time.Now(),
			IsManaged:       false,
			TmuxSocket:      socket,
			TmuxSessionName: sessionName,
			CanAttach:       pd.config.CanAttachExternal(),
			CanDestroy:      false, // Never allow destroying external instances
			Owner:           "external",
		}

		connections = append(connections, conn)
	}

	return connections
}

// getPTYInfoFromTmuxWithSocket gets PTY path and PID from tmux with socket support
// This is similar to getPTYInfoFromTmux but supports specifying a tmux server socket
func (pd *PTYDiscovery) getPTYInfoFromTmuxWithSocket(sessionName string, socket string) (string, int, error) {
	// Build command based on socket
	var cmd *exec.Cmd
	if socket != "" {
		cmd = exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", sessionName,
			"#{pane_tty}:#{pane_pid}")
	} else {
		cmd = exec.Command("tmux", "display-message", "-p", "-t", sessionName,
			"#{pane_tty}:#{pane_pid}")
	}

	output, err := cmd.Output()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get tmux pane info: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("unexpected tmux output format: %s", string(output))
	}

	ptyPath := parts[0]
	pid, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid PID '%s': %w", parts[1], err)
	}

	return ptyPath, pid, nil
}

// getTmuxPTY gets the PTY path and PID for a tmux session
func (pd *PTYDiscovery) getTmuxPTY(sessionName string) (string, int, error) {
	// Get the pane PID
	cmd := exec.Command("tmux", "display-message", "-p", "-t", sessionName, "#{pane_pid}")
	output, err := cmd.Output()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get pane PID: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return "", 0, fmt.Errorf("invalid PID: %w", err)
	}

	// Get the PTY path
	pty, err := pd.getPTYForPID(pid)
	if err != nil {
		return "", 0, err
	}

	return pty, pid, nil
}

// getPTYForPID gets the PTY path for a given PID
// This uses platform-specific methods: lsof on macOS/BSD, /proc on Linux
func (pd *PTYDiscovery) getPTYForPID(pid int) (string, error) {
	// Method 1: Try lsof (works on macOS, BSD, and Linux)
	// lsof -a -p <pid> -d 0,1,2 -F n | grep /dev/tty
	cmd := exec.Command("lsof", "-a", "-p", fmt.Sprintf("%d", pid), "-d", "0,1,2", "-F", "n")
	output, err := cmd.Output()
	if err == nil {
		// Parse lsof output (format: nFILENAME)
		scanner := bufio.NewScanner(strings.NewReader(string(output)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "n/dev/tty") || strings.HasPrefix(line, "n/dev/pts/") {
				pty := strings.TrimPrefix(line, "n")
				return pty, nil
			}
		}
	}

	// Method 2: Try /proc filesystem (Linux only)
	if _, err := os.Stat("/proc"); err == nil {
		// Read /proc/{pid}/fd/0 symlink (stdin)
		fdPath := fmt.Sprintf("/proc/%d/fd/0", pid)
		pty, err := os.Readlink(fdPath)
		if err != nil {
			// Fallback: check fd/1 (stdout)
			fdPath = fmt.Sprintf("/proc/%d/fd/1", pid)
			pty, err = os.Readlink(fdPath)
			if err != nil {
				return "", fmt.Errorf("failed to read PTY via /proc: %w", err)
			}
		}

		// Validate it's a PTY
		if strings.HasPrefix(pty, "/dev/pts/") || strings.HasPrefix(pty, "/dev/tty") {
			return pty, nil
		}
		return "", fmt.Errorf("not a PTY: %s", pty)
	}

	return "", fmt.Errorf("failed to get PTY for PID %d: no supported method worked", pid)
}

// isPIDManaged checks if a PID is already managed by squad
func (pd *PTYDiscovery) isPIDManaged(pid int) bool {
	for _, instance := range pd.sessionMap {
		if instance.Status == Running || instance.Status == Ready {
			// Get PTY for this instance and check PID
			_, instancePID, err := pd.getPTYForInstance(instance)
			if err == nil && instancePID == pid {
				return true
			}
		}
	}
	return false
}

// detectPTYStatus detects the current status of a PTY
// This is cross-platform and works on Linux, macOS, and BSD
func (pd *PTYDiscovery) detectPTYStatus(ptyPath string, pid int) PTYStatus {
	// Use ps command for cross-platform process state detection
	// ps -p PID -o state= returns just the state character
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "state=")
	output, err := cmd.Output()
	if err != nil {
		// Process doesn't exist or ps failed
		return PTYError
	}

	state := strings.TrimSpace(string(output))
	if len(state) == 0 {
		return PTYError
	}

	// Parse first character of state (same across platforms)
	// R = Running, S = Sleeping, I = Idle, T = Stopped, Z = Zombie
	switch state[0] {
	case 'R': // Running
		return PTYBusy
	case 'S': // Sleeping (interruptible) - waiting for input
		return PTYReady
	case 'I': // Idle (BSD/macOS specific)
		return PTYReady
	case 'D': // Disk sleep (uninterruptible)
		return PTYBusy
	case 'Z': // Zombie
		return PTYError
	case 'T': // Stopped
		return PTYIdle
	default:
		return PTYReady
	}
}

// categorizeConnection determines the category for a PTY connection
func (pd *PTYDiscovery) categorizeConnection(conn *PTYConnection) PTYCategory {
	if conn.SessionName != "" {
		return PTYCategorySquad
	}

	if strings.Contains(strings.ToLower(conn.Command), "claude") {
		return PTYCategoryOrphaned
	}

	return PTYCategoryOther
}

// GetStatusIcon returns a visual indicator for PTY status
func (conn *PTYConnection) GetStatusIcon() string {
	switch conn.Status {
	case PTYReady:
		return "●" // Green dot
	case PTYBusy:
		return "◐" // Half-filled circle
	case PTYIdle:
		return "◯" // Empty circle
	case PTYError:
		return "✗" // X mark
	default:
		return "?"
	}
}

// GetStatusColor returns a color code for PTY status
func (conn *PTYConnection) GetStatusColor() string {
	switch conn.Status {
	case PTYReady:
		return "82" // Green
	case PTYBusy:
		return "214" // Orange
	case PTYIdle:
		return "240" // Gray
	case PTYError:
		return "196" // Red
	default:
		return "255" // White
	}
}

// GetDisplayName returns a human-readable name for the PTY
func (conn *PTYConnection) GetDisplayName() string {
	if conn.SessionName != "" {
		return conn.SessionName
	}
	return fmt.Sprintf("(%s)", conn.Command)
}

// GetPTYBasename returns just the PTY number (e.g., "12" from "/dev/pts/12")
func (conn *PTYConnection) GetPTYBasename() string {
	return filepath.Base(conn.Path)
}
