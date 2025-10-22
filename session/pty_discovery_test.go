package session

import (
	"testing"
	"time"
)

func TestNewPTYDiscovery(t *testing.T) {
	pd := NewPTYDiscovery()

	if pd == nil {
		t.Fatal("NewPTYDiscovery returned nil")
	}

	if pd.connections == nil {
		t.Error("connections slice not initialized")
	}

	if pd.sessionMap == nil {
		t.Error("sessionMap not initialized")
	}

	if pd.stopCh == nil {
		t.Error("stopCh not initialized")
	}

	if pd.refreshRate != 5*time.Second {
		t.Errorf("refreshRate = %v, want %v", pd.refreshRate, 5*time.Second)
	}
}

func TestPTYDiscovery_SetSessions(t *testing.T) {
	pd := NewPTYDiscovery()

	sessions := []*Instance{
		{Title: "session1", Status: Running, Program: "claude"},
		{Title: "session2", Status: Paused, Program: "claude"},
		{Title: "session3", Status: Running, Program: "aider"},
	}

	pd.SetSessions(sessions)

	if len(pd.sessionMap) != 3 {
		t.Errorf("sessionMap length = %d, want 3", len(pd.sessionMap))
	}

	if pd.sessionMap["session1"] == nil {
		t.Error("session1 not found in sessionMap")
	}

	if pd.sessionMap["session1"].Program != "claude" {
		t.Errorf("session1 Program = %s, want claude", pd.sessionMap["session1"].Program)
	}
}

func TestPTYDiscovery_GetConnections(t *testing.T) {
	pd := NewPTYDiscovery()

	// Initially empty
	conns := pd.GetConnections()
	if len(conns) != 0 {
		t.Errorf("initial connections length = %d, want 0", len(conns))
	}

	// Add connections directly for testing
	pd.mu.Lock()
	pd.connections = []*PTYConnection{
		{Path: "/dev/pts/1", PID: 1234, Command: "claude"},
		{Path: "/dev/pts/2", PID: 5678, Command: "aider"},
	}
	pd.mu.Unlock()

	conns = pd.GetConnections()
	if len(conns) != 2 {
		t.Errorf("connections length = %d, want 2", len(conns))
	}

	// Verify it's a copy (modifying returned slice shouldn't affect internal state)
	conns[0].Path = "/modified"
	internalConns := pd.GetConnections()
	if internalConns[0].Path == "/modified" {
		t.Error("GetConnections did not return a copy")
	}
}

func TestPTYDiscovery_GetConnectionsByCategory(t *testing.T) {
	pd := NewPTYDiscovery()

	// Set up test data
	pd.mu.Lock()
	pd.connections = []*PTYConnection{
		{Path: "/dev/pts/1", PID: 1234, Command: "claude", SessionName: "session1"},
		{Path: "/dev/pts/2", PID: 5678, Command: "claude", SessionName: ""},
		{Path: "/dev/pts/3", PID: 9012, Command: "aider", SessionName: ""},
	}
	pd.mu.Unlock()

	categorized := pd.GetConnectionsByCategory()

	// Check squad category (has SessionName)
	if len(categorized[PTYCategorySquad]) != 1 {
		t.Errorf("Squad category count = %d, want 1", len(categorized[PTYCategorySquad]))
	}

	// Check orphaned category (Claude without SessionName)
	if len(categorized[PTYCategoryOrphaned]) != 1 {
		t.Errorf("Orphaned category count = %d, want 1", len(categorized[PTYCategoryOrphaned]))
	}

	// Check other category (non-Claude)
	if len(categorized[PTYCategoryOther]) != 1 {
		t.Errorf("Other category count = %d, want 1", len(categorized[PTYCategoryOther]))
	}
}

func TestPTYDiscovery_GetConnection(t *testing.T) {
	pd := NewPTYDiscovery()

	pd.mu.Lock()
	pd.connections = []*PTYConnection{
		{Path: "/dev/pts/1", PID: 1234, Command: "claude"},
		{Path: "/dev/pts/2", PID: 5678, Command: "aider"},
	}
	pd.mu.Unlock()

	// Test finding existing connection
	conn := pd.GetConnection("/dev/pts/1")
	if conn == nil {
		t.Fatal("GetConnection returned nil for existing path")
	}
	if conn.PID != 1234 {
		t.Errorf("conn.PID = %d, want 1234", conn.PID)
	}

	// Test non-existent connection
	conn = pd.GetConnection("/dev/pts/99")
	if conn != nil {
		t.Error("GetConnection should return nil for non-existent path")
	}
}

func TestPTYStatus_String(t *testing.T) {
	tests := []struct {
		status PTYStatus
		want   string
	}{
		{PTYReady, "Ready"},
		{PTYBusy, "Busy"},
		{PTYIdle, "Idle"},
		{PTYError, "Error"},
		{PTYStatus(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPTYCategory_String(t *testing.T) {
	tests := []struct {
		category PTYCategory
		want     string
	}{
		{PTYCategorySquad, "Squad Sessions"},
		{PTYCategoryOrphaned, "Orphaned"},
		{PTYCategoryOther, "Other"},
		{PTYCategory(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.category.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPTYConnection_GetStatusIcon(t *testing.T) {
	tests := []struct {
		status PTYStatus
		want   string
	}{
		{PTYReady, "●"},
		{PTYBusy, "◐"},
		{PTYIdle, "◯"},
		{PTYError, "✗"},
		{PTYStatus(99), "?"},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			conn := &PTYConnection{Status: tt.status}
			got := conn.GetStatusIcon()
			if got != tt.want {
				t.Errorf("GetStatusIcon() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPTYConnection_GetStatusColor(t *testing.T) {
	tests := []struct {
		status PTYStatus
		want   string
	}{
		{PTYReady, "82"},
		{PTYBusy, "214"},
		{PTYIdle, "240"},
		{PTYError, "196"},
		{PTYStatus(99), "255"},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			conn := &PTYConnection{Status: tt.status}
			got := conn.GetStatusColor()
			if got != tt.want {
				t.Errorf("GetStatusColor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPTYConnection_GetDisplayName(t *testing.T) {
	tests := []struct {
		name string
		conn PTYConnection
		want string
	}{
		{
			name: "with session name",
			conn: PTYConnection{SessionName: "my-session", Command: "claude"},
			want: "my-session",
		},
		{
			name: "without session name",
			conn: PTYConnection{SessionName: "", Command: "claude"},
			want: "(claude)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.conn.GetDisplayName()
			if got != tt.want {
				t.Errorf("GetDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPTYConnection_GetPTYBasename(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/dev/pts/12", "12"},
		{"/dev/pts/0", "0"},
		{"/dev/tty1", "tty1"},
		{"/dev/pts/", "pts"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			conn := &PTYConnection{Path: tt.path}
			got := conn.GetPTYBasename()
			if got != tt.want {
				t.Errorf("GetPTYBasename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPTYDiscovery_CategorizeConnection(t *testing.T) {
	pd := NewPTYDiscovery()

	tests := []struct {
		name string
		conn *PTYConnection
		want PTYCategory
	}{
		{
			name: "squad session",
			conn: &PTYConnection{SessionName: "my-session", Command: "claude"},
			want: PTYCategorySquad,
		},
		{
			name: "orphaned claude",
			conn: &PTYConnection{SessionName: "", Command: "claude"},
			want: PTYCategoryOrphaned,
		},
		{
			name: "orphaned claude mixed case",
			conn: &PTYConnection{SessionName: "", Command: "Claude"},
			want: PTYCategoryOrphaned,
		},
		{
			name: "other tool",
			conn: &PTYConnection{SessionName: "", Command: "aider"},
			want: PTYCategoryOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pd.categorizeConnection(tt.conn)
			if got != tt.want {
				t.Errorf("categorizeConnection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPTYDiscovery_StartStop(t *testing.T) {
	pd := NewPTYDiscovery()

	// Start monitoring
	pd.Start()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop monitoring
	pd.Stop()

	// Verify stop channel is closed
	select {
	case <-pd.stopCh:
		// Good, channel is closed
	case <-time.After(100 * time.Millisecond):
		t.Error("Stop did not close stopCh")
	}
}

func TestPTYDiscovery_OrganizeByCategory(t *testing.T) {
	pd := NewPTYDiscovery()

	pd.mu.Lock()
	pd.connections = []*PTYConnection{
		{Path: "/dev/pts/1", SessionName: "session1", Command: "claude"},
		{Path: "/dev/pts/2", SessionName: "session2", Command: "claude"},
		{Path: "/dev/pts/3", SessionName: "", Command: "claude"},
		{Path: "/dev/pts/4", SessionName: "", Command: "aider"},
	}
	pd.mu.Unlock()

	categorized := pd.GetConnectionsByCategory()

	if len(categorized[PTYCategorySquad]) != 2 {
		t.Errorf("Squad category = %d, want 2", len(categorized[PTYCategorySquad]))
	}

	if len(categorized[PTYCategoryOrphaned]) != 1 {
		t.Errorf("Orphaned category = %d, want 1", len(categorized[PTYCategoryOrphaned]))
	}

	if len(categorized[PTYCategoryOther]) != 1 {
		t.Errorf("Other category = %d, want 1", len(categorized[PTYCategoryOther]))
	}
}
