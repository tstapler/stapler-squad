package session

import (
	"claude-squad/config"
	"claude-squad/log"
	"encoding/json"
	"fmt"
	"time"
)

// InstanceData represents the serializable data of an Instance
type InstanceData struct {
	Title      string    `json:"title"`
	Path       string    `json:"path"`
	WorkingDir string    `json:"working_dir"`
	Branch     string    `json:"branch"`
	Status     Status    `json:"status"`
	Height     int       `json:"height"`
	Width      int       `json:"width"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	AutoYes    bool      `json:"auto_yes"`
	Prompt     string    `json:"prompt"`

	Program          string          `json:"program"`
	ExistingWorktree string          `json:"existing_worktree,omitempty"`
	Worktree         GitWorktreeData `json:"worktree"`
	DiffStats        DiffStatsData   `json:"diff_stats"`

	// New fields for session organization and grouping
	Category   string   `json:"category,omitempty"`
	IsExpanded bool     `json:"is_expanded,omitempty"`

	// Session type determines the workflow (directory, new_worktree, existing_worktree)
	SessionType SessionType `json:"session_type,omitempty"`

	// Claude Code session persistence
	ClaudeSession ClaudeSessionData `json:"claude_session,omitempty"`
	// Tmux session prefix for isolation
	TmuxPrefix string `json:"tmux_prefix,omitempty"`
}

// GitWorktreeData represents the serializable data of a GitWorktree
type GitWorktreeData struct {
	RepoPath      string `json:"repo_path"`
	WorktreePath  string `json:"worktree_path"`
	SessionName   string `json:"session_name"`
	BranchName    string `json:"branch_name"`
	BaseCommitSHA string `json:"base_commit_sha"`
}

// DiffStatsData represents the serializable data of a DiffStats
type DiffStatsData struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Content string `json:"content"`
}

// ClaudeSessionData represents Claude Code session information
type ClaudeSessionData struct {
	SessionID      string            `json:"session_id,omitempty"`       // Claude Code session identifier
	ConversationID string            `json:"conversation_id,omitempty"`  // Conversation thread ID
	ProjectName    string            `json:"project_name,omitempty"`     // Project name in Claude Code
	LastAttached   time.Time         `json:"last_attached,omitempty"`    // When this session was last used
	Settings       ClaudeSettings    `json:"settings,omitempty"`         // User preferences for Claude Code
	Metadata       map[string]string `json:"metadata,omitempty"`         // Additional session metadata
}

// ClaudeSettings contains user preferences for Claude Code integration
type ClaudeSettings struct {
	AutoReattach          bool   `json:"auto_reattach"`           // Automatically reattach to last session on resume
	PreferredSessionName  string `json:"preferred_session_name"`  // Preferred session naming pattern
	CreateNewOnMissing    bool   `json:"create_new_on_missing"`   // Create new session if previous one is missing
	ShowSessionSelector   bool   `json:"show_session_selector"`   // Show session selection menu on resume
	SessionTimeoutMinutes int    `json:"session_timeout_minutes"` // Consider sessions stale after this time
}

// Storage handles saving and loading instances using the state interface
// It manages async saves through a StateService to prevent UI blocking
type Storage struct {
	state        config.InstanceStorage
	stateService *config.StateService
}

// NewStorage creates a new storage instance with async save capabilities
func NewStorage(state config.InstanceStorage) (*Storage, error) {
	// Create and start the state service for async saves
	// Only if the state is actually a *config.State (not a mock in tests)
	var stateService *config.StateService
	if concreteState, ok := state.(*config.State); ok {
		stateService = config.NewStateService(concreteState)
		stateService.Start()
	}

	return &Storage{
		state:        state,
		stateService: stateService,
	}, nil
}

// SaveInstances saves the list of instances to disk
// with built-in merging of any existing instances from other windows
func (s *Storage) SaveInstances(instances []*Instance) error {
	// First load existing instances from disk
	existingInstances, err := s.LoadInstances()
	if err != nil {
		// If we can't load existing instances, just use what we have
		log.WarningLog.Printf("failed to load existing instances for merging: %v", err)
		existingInstances = []*Instance{}
	}

	// Create a map of our instances by title for quick lookup
	instancesByTitle := make(map[string]*Instance)
	for _, instance := range instances {
		if instance.Started() {
			instancesByTitle[instance.Title] = instance
		}
	}

	// Create a map of existing instances by title for quick lookup
	existingByTitle := make(map[string]*Instance)
	for _, instance := range existingInstances {
		// Skip any instances we're already tracking (our version is newer)
		if _, exists := instancesByTitle[instance.Title]; !exists {
			existingByTitle[instance.Title] = instance
		}
	}

	// Create a merged list with our instances taking precedence
	mergedInstances := make([]*Instance, 0, len(instancesByTitle)+len(existingByTitle))

	// Add our instances first (we know they're valid)
	for _, instance := range instances {
		if instance.Started() {
			mergedInstances = append(mergedInstances, instance)
		}
	}

	// Then add instances from disk that we're not tracking
	for title, instance := range existingByTitle {
		if instance.Started() {
			log.InfoLog.Printf("merging instance from disk: %s", title)
			mergedInstances = append(mergedInstances, instance)
		}
	}

	// Convert merged instances to InstanceData
	data := make([]InstanceData, 0, len(mergedInstances))
	for _, instance := range mergedInstances {
		data = append(data, instance.ToInstanceData())
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	// Use async save if available, otherwise fall back to synchronous
	if s.stateService != nil {
		s.stateService.SaveAsync(jsonData)
		return nil
	}

	// Fallback for tests or when state service not available
	return s.state.SaveInstances(jsonData)
}

// SaveInstancesSync saves instances synchronously (blocks until complete)
// Use this for critical operations like shutdown or CLI commands
func (s *Storage) SaveInstancesSync(instances []*Instance) error {
	// Perform the same merging logic as SaveInstances
	existingInstances, err := s.LoadInstances()
	if err != nil {
		log.WarningLog.Printf("failed to load existing instances for merging: %v", err)
		existingInstances = []*Instance{}
	}

	instancesByTitle := make(map[string]*Instance)
	for _, instance := range instances {
		if instance.Started() {
			instancesByTitle[instance.Title] = instance
		}
	}

	existingByTitle := make(map[string]*Instance)
	for _, instance := range existingInstances {
		if _, exists := instancesByTitle[instance.Title]; !exists {
			existingByTitle[instance.Title] = instance
		}
	}

	mergedInstances := make([]*Instance, 0, len(instancesByTitle)+len(existingByTitle))
	for _, instance := range instances {
		if instance.Started() {
			mergedInstances = append(mergedInstances, instance)
		}
	}

	for title, instance := range existingByTitle {
		if instance.Started() {
			log.InfoLog.Printf("merging instance from disk: %s", title)
			mergedInstances = append(mergedInstances, instance)
		}
	}

	data := make([]InstanceData, 0, len(mergedInstances))
	for _, instance := range mergedInstances {
		data = append(data, instance.ToInstanceData())
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	// Use sync save if available, otherwise fall back to direct save
	if s.stateService != nil {
		return s.stateService.SaveSync(jsonData)
	}

	return s.state.SaveInstances(jsonData)
}

// Close performs graceful shutdown of storage
// This flushes any pending async saves and releases locks
func (s *Storage) Close() error {
	// Shutdown state service if available
	if s.stateService != nil {
		if err := s.stateService.Shutdown(); err != nil {
			log.WarningLog.Printf("Failed to shutdown state service: %v", err)
			// Continue with cleanup anyway
		}
	}

	// Close the underlying state manager if it supports it
	if stateManager, ok := s.state.(config.StateManager); ok {
		return stateManager.Close()
	}

	return nil
}

// LoadInstances loads the list of instances from disk
func (s *Storage) LoadInstances() ([]*Instance, error) {
	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	instances := make([]*Instance, len(instancesData))
	for i, data := range instancesData {
		instance, err := FromInstanceData(data)
		if err != nil {
			return nil, fmt.Errorf("failed to create instance %s: %w", data.Title, err)
		}
		instances[i] = instance
	}

	return instances, nil
}

// DeleteInstance removes an instance from storage
func (s *Storage) DeleteInstance(title string) error {
	instances, err := s.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	found := false
	newInstances := make([]*Instance, 0)
	for _, instance := range instances {
		data := instance.ToInstanceData()
		if data.Title != title {
			newInstances = append(newInstances, instance)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	return s.SaveInstances(newInstances)
}

// UpdateInstance updates an existing instance in storage
func (s *Storage) UpdateInstance(instance *Instance) error {
	instances, err := s.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	data := instance.ToInstanceData()
	found := false
	for i, existing := range instances {
		existingData := existing.ToInstanceData()
		if existingData.Title == data.Title {
			instances[i] = instance
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", data.Title)
	}

	return s.SaveInstances(instances)
}

// DeleteAllInstances removes all stored instances
func (s *Storage) DeleteAllInstances() error {
	return s.state.DeleteAllInstances()
}

// GetStateManager returns the underlying state manager
func (s *Storage) GetStateManager() interface{} {
	return s.state
}
