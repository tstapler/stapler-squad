package session

import "github.com/linkdata/deadlock"

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// paneEntry holds the result of one row from `tmux list-panes -a`.
type paneEntry struct {
	pty string
	pid int
}

// batchPTYInfo fetches pane_tty and pane_pid for all sessions in one tmux call.
// Returns a map keyed by session name; only the first pane per session is kept.
// socket is the tmux server socket name (empty = default server).
func batchPTYInfo(socket string) map[string]paneEntry {
	args := []string{"list-panes", "-a", "-F", "#{session_name} #{pane_tty} #{pane_pid}"}
	if socket != "" {
		args = append([]string{"-L", socket}, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	result := make(map[string]paneEntry)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 3 {
			continue
		}
		sessionName := parts[0]
		if _, exists := result[sessionName]; exists {
			continue // keep first pane per session
		}
		pid, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		result[sessionName] = paneEntry{pty: parts[1], pid: pid}
	}
	return result
}

// batchProcessStates runs a single `ps` invocation for all given PIDs and returns
// their PTYStatus. Missing PIDs (exited processes) map to PTYError.
func batchProcessStates(pids []int) map[int]PTYStatus {
	if len(pids) == 0 {
		return nil
	}
	pidStrs := make([]string, len(pids))
	for i, p := range pids {
		pidStrs[i] = strconv.Itoa(p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-p", strings.Join(pidStrs, ","), "-o", "pid=,state=")
	output, err := cmd.Output()
	result := make(map[int]PTYStatus, len(pids))
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			pid, err := strconv.Atoi(fields[0])
			if err != nil {
				continue
			}
			result[pid] = parseProcessState(fields[1])
		}
	}
	// Fill missing PIDs (ps omits exited processes).
	for _, p := range pids {
		if _, ok := result[p]; !ok {
			result[p] = PTYError
		}
	}
	return result
}

// parseProcessState maps a single-character ps state to PTYStatus.
func parseProcessState(state string) PTYStatus {
	if len(state) == 0 {
		return PTYError
	}
	switch state[0] {
	case 'R':
		return PTYBusy
	case 'S', 'I':
		return PTYReady
	case 'D':
		return PTYBusy
	case 'Z':
		return PTYError
	case 'T':
		return PTYIdle
	default:
		return PTYReady
	}
}

// batchPaneActivity returns the last-activity timestamp for each session (first pane wins)
// by running a single `tmux list-panes -a` call. The tmux format #{pane_last_activity}
// is a Unix timestamp updated whenever the pane produces output. Use it to detect whether
// a session has produced new output since the last capture, without spawning capture-pane.
// Returns nil when tmux is unavailable.
func batchPaneActivity(socket string) map[string]time.Time {
	args := []string{"list-panes", "-a", "-F", "#{session_name} #{pane_last_activity}"}
	if socket != "" {
		args = append([]string{"-L", socket}, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	result := make(map[string]time.Time)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		sessionName := parts[0]
		if _, exists := result[sessionName]; exists {
			continue // keep first pane per session
		}
		ts, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		result[sessionName] = time.Unix(ts, 0)
	}
	return result
}

// batchIsClaudeProcess checks which PIDs are running a Claude process in one ps call.
// Returns a set of PIDs whose command line contains "claude".
func batchIsClaudeProcess(pids []int) map[int]bool {
	if len(pids) == 0 {
		return nil
	}
	pidStrs := make([]string, len(pids))
	for i, p := range pids {
		pidStrs[i] = strconv.Itoa(p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-p", strings.Join(pidStrs, ","), "-o", "pid=,command=")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	result := make(map[int]bool, len(pids))
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		// Format: "  PID command..."  - split on first space boundary after pid
		line = strings.TrimSpace(line)
		spaceIdx := strings.IndexByte(line, ' ')
		if spaceIdx < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:spaceIdx])
		if err != nil {
			continue
		}
		cmdLine := strings.ToLower(line[spaceIdx+1:])
		result[pid] = strings.Contains(cmdLine, "claude")
	}
	return result
}

// PTYStatus represents the current state of a PTY
type PTYStatus int

const (
	PTYReady PTYStatus = iota // Waiting for input
	PTYBusy                   // Executing command
	PTYIdle                   // No activity
	PTYError                  // Error state
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
	PTYCategorySquad    PTYCategory = iota // Squad-managed sessions
	PTYCategoryOrphaned                    // Unmanaged Claude instances
	PTYCategoryOther                       // Other tools (aider, etc.)
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

// PTYDiscoveryOption is a functional option for PTYDiscovery construction.
type PTYDiscoveryOption func(*PTYDiscovery)

// WithSessionLister injects a SessionLister; used in tests to avoid exec.Command forks.
func WithSessionLister(l tmux.SessionLister) PTYDiscoveryOption {
	return func(pd *PTYDiscovery) { pd.sessionLister = l }
}

// PTYDiscovery manages PTY discovery and monitoring
type PTYDiscovery struct {
	mu            deadlock.RWMutex
	connections   []*PTYConnection
	sessionMap    map[string]*Instance // Session name -> Instance
	stopCh        chan struct{}
	refreshRate   time.Duration
	config        PTYDiscoveryConfig // Discovery configuration
	sessionLister tmux.SessionLister // nil = use exec fallback
}

// NewPTYDiscovery creates a new PTY discovery service with default configuration.
// Optional PTYDiscoveryOption values are applied after initialization.
func NewPTYDiscovery(opts ...PTYDiscoveryOption) *PTYDiscovery {
	pd := &PTYDiscovery{
		connections:   make([]*PTYConnection, 0),
		sessionMap:    make(map[string]*Instance),
		stopCh:        make(chan struct{}),
		refreshRate:   5 * time.Second,
		config:        DefaultPTYDiscoveryConfig(),
		sessionLister: tmux.GetServerRegistry(""),
	}
	for _, opt := range opts {
		opt(pd)
	}
	return pd
}

// NewPTYDiscoveryWithConfig creates a new PTY discovery service with custom configuration.
// Optional PTYDiscoveryOption values are applied after initialization.
func NewPTYDiscoveryWithConfig(config PTYDiscoveryConfig, opts ...PTYDiscoveryOption) *PTYDiscovery {
	pd := &PTYDiscovery{
		connections:   make([]*PTYConnection, 0),
		sessionMap:    make(map[string]*Instance),
		stopCh:        make(chan struct{}),
		refreshRate:   config.DiscoveryInterval,
		config:        config,
		sessionLister: tmux.GetServerRegistry(""),
	}
	for _, opt := range opts {
		opt(pd)
	}
	return pd
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

// discoverPTYs performs the actual PTY discovery.
// The default-server pane info is fetched once and shared across all discovery methods
// to avoid redundant tmux subprocess calls.
func (pd *PTYDiscovery) discoverPTYs() ([]*PTYConnection, error) {
	connections := make([]*PTYConnection, 0)

	// Fetch all default-server pane info in one shot; shared by methods 1 and 2.
	defaultPaneInfo := batchPTYInfo("")

	// Method 1: Discover from squad-managed sessions.
	squadPTYs := pd.discoverSquadPTYsWithCache(defaultPaneInfo)
	connections = append(connections, squadPTYs...)

	// Build the set of managed PIDs so method 2 can skip them cheaply.
	managedPIDs := make(map[int]bool, len(squadPTYs))
	for _, c := range squadPTYs {
		managedPIDs[c.PID] = true
	}

	// Method 2: Discover orphaned Claude processes in staplersquad_ prefixed sessions.
	orphanedPTYs := pd.discoverOrphanedPTYsWithCache(defaultPaneInfo, managedPIDs)
	connections = append(connections, orphanedPTYs...)

	// Method 3: Discover external Claude instances if enabled.
	if pd.config.ShouldDiscoverExternal() {
		externalPTYs := pd.discoverExternalClaude("", defaultPaneInfo, managedPIDs)
		connections = append(connections, externalPTYs...)

		for _, socket := range pd.config.ExternalSockets {
			socketPaneInfo := batchPTYInfo(socket)
			morePTYs := pd.discoverExternalClaude(socket, socketPaneInfo, managedPIDs)
			connections = append(connections, morePTYs...)
		}
	}

	return connections, nil
}

func (pd *PTYDiscovery) discoverSquadPTYsWithCache(paneInfoMap map[string]paneEntry) []*PTYConnection {
	// paneInfoMap may be nil (e.g. tmux not running); treat as empty.

	type pendingEntry struct {
		sessionName     string
		tmuxSessionName string
		pty             string
		pid             int
		instance        *Instance
	}

	var pending []pendingEntry
	for sessionName, instance := range pd.sessionMap {
		if instance.Status != Running && instance.Status != Ready {
			continue
		}
		tmuxName := tmux.ToStaplerSquadTmuxName(instance.Title)
		if paneInfoMap == nil {
			continue
		}
		info, ok := paneInfoMap[tmuxName]
		if !ok {
			log.DebugLog.Printf("No pane info found for session %s (tmux name: %s)", sessionName, tmuxName)
			continue
		}
		pending = append(pending, pendingEntry{
			sessionName:     sessionName,
			tmuxSessionName: tmuxName,
			pty:             info.pty,
			pid:             info.pid,
			instance:        instance,
		})
	}

	// Single ps call gets all process states.
	pids := make([]int, len(pending))
	for i, p := range pending {
		pids[i] = p.pid
	}
	states := batchProcessStates(pids)

	connections := make([]*PTYConnection, 0, len(pending))
	for _, p := range pending {
		status := PTYError
		if states != nil {
			status = states[p.pid]
		}
		connections = append(connections, &PTYConnection{
			Path:            p.pty,
			PID:             p.pid,
			Command:         p.instance.Program,
			SessionName:     p.sessionName,
			Status:          status,
			LastActivity:    time.Now(),
			IsManaged:       true,
			TmuxSocket:      p.instance.TmuxServerSocket,
			TmuxSessionName: p.tmuxSessionName,
			CanAttach:       true,
			CanDestroy:      true,
			Owner:           "squad",
		})
	}
	return connections
}

// getPTYInfoFromTmux gets PTY path and PID from tmux for a single session.
// Used as a fallback when no batch pane info is available.
func (pd *PTYDiscovery) getPTYInfoFromTmux(sessionName string) (string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", sessionName,
		"#{pane_tty}:#{pane_pid}")
	output, err := cmd.Output()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get tmux pane info: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(output)), ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("unexpected tmux output format: %s", string(output))
	}
	pid, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid PID '%s': %w", parts[1], err)
	}
	return parts[0], pid, nil
}

// discoverOrphanedPTYs finds unmanaged Claude processes in stapler-squad tmux sessions.
// Uses batched subprocess calls; accepts a precomputed managedPIDs set to avoid O(n²) work.
// paneInfoMap is the result of a prior batchPTYInfo("") call (may be nil).
// managedPIDs is the set of PIDs already accounted for by discoverSquadPTYs (may be nil).
func (pd *PTYDiscovery) discoverOrphanedPTYs() []*PTYConnection {
	return pd.discoverOrphanedPTYsWithCache(nil, nil)
}

func (pd *PTYDiscovery) discoverOrphanedPTYsWithCache(paneInfoMap map[string]paneEntry, managedPIDs map[int]bool) []*PTYConnection {
	connections := make([]*PTYConnection, 0)

	// Collect session names: prefer the injected SessionLister to avoid exec forks.
	var sessionNames []string
	if pd.sessionLister != nil && pd.sessionLister.IsHealthy() {
		m := pd.sessionLister.ListSessions()
		for name := range m {
			sessionNames = append(sessionNames, name)
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "tmux", "list-sessions", "-F", "#{session_name}")
		output, err := cmd.Output()
		if err != nil {
			return connections
		}
		scanner := bufio.NewScanner(strings.NewReader(string(output)))
		for scanner.Scan() {
			sessionNames = append(sessionNames, strings.TrimSpace(scanner.Text()))
		}
	}

	// Filter to prefixed sessions, then collect PIDs using the pre-fetched pane map.
	type candidate struct {
		sessionName string
		pty         string
		pid         int
	}
	var candidates []candidate
	for _, sessionName := range sessionNames {
		if !strings.HasPrefix(sessionName, "staplersquad_") && !strings.HasPrefix(sessionName, "claudesquad_") {
			continue
		}
		var info paneEntry
		if paneInfoMap != nil {
			var ok bool
			info, ok = paneInfoMap[sessionName]
			if !ok {
				continue
			}
		} else {
			// Fallback: individual tmux call (only when no batch map provided).
			pty, pid, err := pd.getPTYInfoFromTmux(sessionName)
			if err != nil {
				continue
			}
			info = paneEntry{pty: pty, pid: pid}
		}
		if managedPIDs != nil && managedPIDs[info.pid] {
			continue
		}
		candidates = append(candidates, candidate{sessionName, info.pty, info.pid})
	}

	if len(candidates) == 0 {
		return connections
	}

	// Single ps call to check which PIDs are Claude processes.
	pids := make([]int, len(candidates))
	for i, c := range candidates {
		pids[i] = c.pid
	}
	isClaudeMap := batchIsClaudeProcess(pids)
	states := batchProcessStates(pids)

	for _, c := range candidates {
		if isClaudeMap != nil && !isClaudeMap[c.pid] {
			continue
		}
		status := PTYError
		if states != nil {
			status = states[c.pid]
		}
		connections = append(connections, &PTYConnection{
			Path:            c.pty,
			PID:             c.pid,
			Command:         "claude",
			SessionName:     "",
			Status:          status,
			LastActivity:    time.Now(),
			IsManaged:       false,
			TmuxSocket:      "",
			TmuxSessionName: c.sessionName,
			CanAttach:       pd.config.CanAttachExternal(),
			CanDestroy:      false,
			Owner:           "external",
		})
	}
	return connections
}

// discoverExternalClaude discovers Claude instances from non-prefixed tmux sessions.
// paneInfoMap is a pre-fetched batch result for socket (nil = fetch individually as fallback).
// managedPIDs is the set of PIDs already tracked; used to skip duplicates.
func (pd *PTYDiscovery) discoverExternalClaude(socket string, paneInfoMap map[string]paneEntry, managedPIDs map[int]bool) []*PTYConnection {
	connections := make([]*PTYConnection, 0)

	// Collect session names via the registry (no exec) when possible.
	var sessionNames []string
	if socket == "" && pd.sessionLister != nil && pd.sessionLister.IsHealthy() {
		m := pd.sessionLister.ListSessions()
		for name := range m {
			sessionNames = append(sessionNames, name)
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var cmd *exec.Cmd
		if socket != "" {
			cmd = exec.CommandContext(ctx, "tmux", "-L", socket, "list-sessions", "-F", "#{session_name}")
		} else {
			cmd = exec.CommandContext(ctx, "tmux", "list-sessions", "-F", "#{session_name}")
		}
		output, err := cmd.Output()
		if err != nil {
			return connections
		}
		scanner := bufio.NewScanner(strings.NewReader(string(output)))
		for scanner.Scan() {
			sessionNames = append(sessionNames, strings.TrimSpace(scanner.Text()))
		}
	}

	type candidate struct {
		sessionName string
		pty         string
		pid         int
	}
	var candidates []candidate
	for _, sessionName := range sessionNames {
		if strings.HasPrefix(sessionName, pd.config.ManagedPrefix) {
			continue
		}
		var info paneEntry
		if paneInfoMap != nil {
			var ok bool
			info, ok = paneInfoMap[sessionName]
			if !ok {
				continue
			}
		} else {
			pty, pid, err := pd.getPTYInfoFromTmuxWithSocket(sessionName, socket)
			if err != nil {
				log.DebugLog.Printf("Failed to get PTY info for external session %s (socket: %s): %v", sessionName, socket, err)
				continue
			}
			info = paneEntry{pty: pty, pid: pid}
		}
		if managedPIDs != nil && managedPIDs[info.pid] {
			continue
		}
		candidates = append(candidates, candidate{sessionName, info.pty, info.pid})
	}

	if len(candidates) == 0 {
		return connections
	}

	pids := make([]int, len(candidates))
	for i, c := range candidates {
		pids[i] = c.pid
	}
	isClaudeMap := batchIsClaudeProcess(pids)
	states := batchProcessStates(pids)

	for _, c := range candidates {
		if isClaudeMap != nil && !isClaudeMap[c.pid] {
			continue
		}
		status := PTYError
		if states != nil {
			status = states[c.pid]
		}
		connections = append(connections, &PTYConnection{
			Path:            c.pty,
			PID:             c.pid,
			Command:         "claude",
			SessionName:     "",
			Status:          status,
			LastActivity:    time.Now(),
			IsManaged:       false,
			TmuxSocket:      socket,
			TmuxSessionName: c.sessionName,
			CanAttach:       pd.config.CanAttachExternal(),
			CanDestroy:      false,
			Owner:           "external",
		})
	}
	return connections
}

// getPTYInfoFromTmuxWithSocket gets PTY path and PID from tmux with socket support
// This is similar to getPTYInfoFromTmux but supports specifying a tmux server socket
func (pd *PTYDiscovery) getPTYInfoFromTmuxWithSocket(sessionName string, socket string) (string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if socket != "" {
		cmd = exec.CommandContext(ctx, "tmux", "-L", socket, "display-message", "-p", "-t", sessionName,
			"#{pane_tty}:#{pane_pid}")
	} else {
		cmd = exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", sessionName,
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
