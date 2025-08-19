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

// Storage handles saving and loading instances using the state interface
type Storage struct {
	state config.InstanceStorage
}

// NewStorage creates a new storage instance
func NewStorage(state config.InstanceStorage) (*Storage, error) {
	return &Storage{
		state: state,
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
	mergedInstances := make([]*Instance, 0, len(instancesByTitle) + len(existingByTitle))
	
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

	return s.state.SaveInstances(jsonData)
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
