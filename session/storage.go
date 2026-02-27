package session

import (
	"claude-squad/config"
	"claude-squad/log"
	"context"
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
	Tags       []string `json:"tags,omitempty"` // Multi-valued tags for flexible organization

	// Session type determines the workflow (directory, new_worktree, existing_worktree)
	SessionType SessionType `json:"session_type,omitempty"`

	// GitHub integration fields for PR/URL-based session creation
	GitHubPRNumber  int    `json:"github_pr_number,omitempty"`
	GitHubPRURL     string `json:"github_pr_url,omitempty"`
	GitHubOwner     string `json:"github_owner,omitempty"`
	GitHubRepo      string `json:"github_repo,omitempty"`
	GitHubSourceRef string `json:"github_source_ref,omitempty"`
	ClonedRepoPath  string `json:"cloned_repo_path,omitempty"`
	// Worktree detection fields
	MainRepoPath string `json:"main_repo_path,omitempty"` // Path to main repo when this is a worktree
	IsWorktree   bool   `json:"is_worktree,omitempty"`    // True if path is a git worktree

	// Claude Code session persistence
	ClaudeSession ClaudeSessionData `json:"claude_session,omitempty"`
	// Tmux session prefix for isolation
	TmuxPrefix string `json:"tmux_prefix,omitempty"`

	// Terminal update timestamps for activity tracking
	LastTerminalUpdate   time.Time `json:"last_terminal_update,omitempty"`
	LastMeaningfulOutput time.Time `json:"last_meaningful_output,omitempty"`

	// Content signature for detecting actual terminal changes vs restarts
	// This is a SHA256 hash of the terminal content used to prevent false "new activity"
	// notifications when app restarts but terminal content hasn't changed
	LastOutputSignature string `json:"last_output_signature,omitempty"`

	// Review queue spam prevention
	LastAddedToQueue time.Time `json:"last_added_to_queue,omitempty"`

	// User interaction tracking
	// LastViewed tracks when the user last viewed this session (terminal, session details, etc.)
	// Used for smarter review queue notifications (don't notify if just viewed)
	LastViewed time.Time `json:"last_viewed,omitempty"`

	// Review queue snooze tracking
	// LastAcknowledged tracks when the user last dismissed this session from review queue
	// Sessions acknowledged after their last update won't appear in the queue until they update again
	LastAcknowledged time.Time `json:"last_acknowledged,omitempty"`

	// Prompt detection and interaction tracking for smart review queue behavior
	LastPromptDetected   time.Time `json:"last_prompt_detected,omitempty"`
	LastPromptSignature  string    `json:"last_prompt_signature,omitempty"`
	LastUserResponse     time.Time `json:"last_user_response,omitempty"`
	ProcessingGraceUntil time.Time `json:"processing_grace_until,omitempty"`
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
// Note: Content is excluded from JSON serialization to reduce state file size.
// Diffs are generated on-demand via GetSessionDiff RPC when needed.
type DiffStatsData struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Content string `json:"-"` // Excluded from serialization - generated on-demand
}

// ClaudeSessionData represents Claude Code session information
type ClaudeSessionData struct {
	SessionID      string            `json:"session_id,omitempty"`      // Claude Code session identifier
	ConversationID string            `json:"conversation_id,omitempty"` // Conversation thread ID
	ProjectName    string            `json:"project_name,omitempty"`    // Project name in Claude Code
	LastAttached   time.Time         `json:"last_attached,omitempty"`   // When this session was last used
	Settings       ClaudeSettings    `json:"settings,omitempty"`        // User preferences for Claude Code
	Metadata       map[string]string `json:"metadata,omitempty"`        // Additional session metadata
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
// It manages async saves through a StateService to prevent UI blocking.
//
// Two backends are supported:
//   - JSON-based (default): wraps config.InstanceStorage, which is implemented by SQLiteState
//   - Repository-based (CLAUDE_SQUAD_USE_ENT=true): uses Repository (EntRepository) directly,
//     bypassing the JSON round-trip for significantly better performance.
type Storage struct {
	state        config.InstanceStorage // JSON-based state (nil when repo is set)
	stateService *config.StateService   // Async JSON save (nil when repo is set)
	repo         Repository             // Optional direct repository (set when CLAUDE_SQUAD_USE_ENT=true)
}

// NewStorage creates a new storage instance backed by config.InstanceStorage (JSON path).
// Note: You must call Start() after wiring dependencies (statusManager, reviewQueue)
// to load and start instances. This prevents initialization timing issues where
// instances try to start controllers before statusManager is available.
func NewStorage(state config.InstanceStorage) (*Storage, error) {
	// Create the state service but DON'T start it yet
	// Starting happens explicitly via Start() after dependencies are wired
	var stateService *config.StateService
	if concreteState, ok := state.(*config.State); ok {
		stateService = config.NewStateService(concreteState)
	}

	return &Storage{
		state:        state,
		stateService: stateService,
	}, nil
}

// NewStorageWithRepository creates a Storage backed directly by a Repository.
// This bypasses the JSON round-trip of the default config.InstanceStorage path.
// Use when CLAUDE_SQUAD_USE_ENT=true is set.
func NewStorageWithRepository(repo Repository) (*Storage, error) {
	return &Storage{repo: repo}, nil
}

// UseRepositoryBackend returns true if this Storage uses a Repository instead of
// the config.InstanceStorage (JSON) path.
func (s *Storage) UseRepositoryBackend() bool {
	return s.repo != nil
}

// Start initializes the storage by starting the StateService, which loads
// and starts all persisted instances. This should be called AFTER wiring
// dependencies like statusManager and reviewQueue to instances.
func (s *Storage) Start() error {
	if s.stateService != nil {
		s.stateService.Start()
	}
	return nil
}

// SaveInstances saves the list of instances to disk
// with built-in merging of any existing instances from other windows.
func (s *Storage) SaveInstances(instances []*Instance) error {
	if s.repo != nil {
		return s.saveInstancesToRepo(instances)
	}
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
		// DEBUG: Log worktree info being persisted
		log.InfoLog.Printf("[SaveInstances] Converting instance '%s': IsWorktree=%v, MainRepoPath=%s, GitHubOwner=%s, GitHubRepo=%s",
			instance.Title, instance.IsWorktree, instance.MainRepoPath, instance.GitHubOwner, instance.GitHubRepo)
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

// saveInstancesToRepo upserts each started instance into the repository.
// Unlike the JSON path, no merging is required — the DB handles concurrent writers.
func (s *Storage) saveInstancesToRepo(instances []*Instance) error {
	ctx := context.Background()
	for _, inst := range instances {
		if !inst.Started() {
			continue
		}
		data := inst.ToInstanceData()
		if err := s.repo.Update(ctx, data); err != nil {
			// Not found → create it
			if createErr := s.repo.Create(ctx, data); createErr != nil {
				log.ErrorLog.Printf("[SaveInstances] Failed to upsert instance '%s': update=%v, create=%v",
					data.Title, err, createErr)
			}
		}
	}
	return nil
}

// SaveInstancesSync saves instances synchronously (blocks until complete)
// Use this for critical operations like shutdown or CLI commands
func (s *Storage) SaveInstancesSync(instances []*Instance) error {
	if s.repo != nil {
		return s.saveInstancesToRepo(instances)
	}
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

// LoadInstances loads the list of instances from disk.
func (s *Storage) LoadInstances() ([]*Instance, error) {
	if s.repo != nil {
		return s.loadInstancesFromRepo()
	}

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

func (s *Storage) loadInstancesFromRepo() ([]*Instance, error) {
	dataSlice, err := s.repo.List(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to list instances from repository: %w", err)
	}
	instances := make([]*Instance, 0, len(dataSlice))
	for _, data := range dataSlice {
		inst, err := FromInstanceData(data)
		if err != nil {
			log.WarningLog.Printf("Skipping instance '%s' from repository: %v", data.Title, err)
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

// DeleteInstance removes an instance from storage.
func (s *Storage) DeleteInstance(title string) error {
	if s.repo != nil {
		return s.repo.Delete(context.Background(), title)
	}

	// JSON path: directly manipulate to avoid merge logic restoring deleted instances
	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	// Filter out the instance to delete
	found := false
	newData := make([]InstanceData, 0, len(instancesData))
	for _, data := range instancesData {
		if data.Title != title {
			newData = append(newData, data)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	// Marshal the filtered list back to JSON
	newJsonData, err := json.Marshal(newData)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	// Save synchronously to ensure deletion is persisted before returning.
	// This is critical because the server reloads instances immediately after
	// deletion to update the ReviewQueuePoller. If we used SaveAsync, the poller
	// might reload the old state that still contains the deleted session.
	return s.state.SaveInstances(newJsonData)
}

// AddInstance adds a new instance to storage.
// Unlike SaveInstances, this does not require instance.Started() to be true.
func (s *Storage) AddInstance(instance *Instance) error {
	if s.repo != nil {
		data := instance.ToInstanceData()
		ctx := context.Background()
		if err := s.repo.Create(ctx, data); err != nil {
			// Already exists → update instead
			return s.repo.Update(ctx, data)
		}
		return nil
	}

	// JSON path
	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		// If unmarshal fails, start with empty slice
		instancesData = []InstanceData{}
	}

	// Check if instance already exists
	data := instance.ToInstanceData()
	for _, existing := range instancesData {
		if existing.Title == data.Title {
			// Already exists, update instead
			return s.UpdateInstance(instance)
		}
	}

	// Add the new instance
	instancesData = append(instancesData, data)

	// Marshal back to JSON
	newJsonData, err := json.Marshal(instancesData)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	// Save synchronously to ensure persistence
	return s.state.SaveInstances(newJsonData)
}

// UpdateInstance updates an existing instance in storage.
func (s *Storage) UpdateInstance(instance *Instance) error {
	if s.repo != nil {
		return s.repo.Update(context.Background(), instance.ToInstanceData())
	}

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

// DeleteAllInstances removes all stored instances.
func (s *Storage) DeleteAllInstances() error {
	if s.repo != nil {
		ctx := context.Background()
		dataSlice, err := s.repo.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list instances for deletion: %w", err)
		}
		for _, data := range dataSlice {
			if err := s.repo.Delete(ctx, data.Title); err != nil {
				log.WarningLog.Printf("Failed to delete instance '%s': %v", data.Title, err)
			}
		}
		return nil
	}
	return s.state.DeleteAllInstances()
}

// updateFieldInRepo loads the InstanceData for title, applies fn to it, then saves.
// Used for partial-field updates when using the repository backend.
func (s *Storage) updateFieldInRepo(title string, fn func(*InstanceData)) error {
	ctx := context.Background()
	data, err := s.repo.Get(ctx, title)
	if err != nil {
		return fmt.Errorf("failed to get instance '%s': %w", title, err)
	}
	fn(data)
	return s.repo.Update(ctx, *data)
}

// UpdateInstanceTimestampsOnly updates ONLY the timestamp fields in storage without
// creating Instance objects. This preserves in-memory state like controllers.
// This is critical for WebSocket terminal streaming which updates timestamps frequently.
func (s *Storage) UpdateInstanceTimestampsOnly(title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string, lastViewed time.Time) error {
	if s.repo != nil {
		// Use the optimized UpdateTimestamps for the three core fields, then patch lastViewed
		if err := s.repo.UpdateTimestamps(context.Background(), title, lastTerminalUpdate, lastMeaningfulOutput, lastOutputSignature); err != nil {
			return err
		}
		if !lastViewed.IsZero() {
			return s.updateFieldInRepo(title, func(d *InstanceData) { d.LastViewed = lastViewed })
		}
		return nil
	}

	// JSON path: load raw data directly
	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	// Find and update the matching instance data
	found := false
	for i, data := range instancesData {
		if data.Title == title {
			instancesData[i].LastTerminalUpdate = lastTerminalUpdate
			instancesData[i].LastMeaningfulOutput = lastMeaningfulOutput
			instancesData[i].LastOutputSignature = lastOutputSignature
			instancesData[i].LastViewed = lastViewed
			found = true
			log.DebugLog.Printf("Updating timestamps in storage for session %s (no instance objects created)", title)
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	// Marshal back to JSON
	updatedJSON, err := json.Marshal(instancesData)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	// Use async save if available
	if s.stateService != nil {
		s.stateService.SaveAsync(updatedJSON)
		return nil
	}

	// Fallback to synchronous save
	return s.state.SaveInstances(updatedJSON)
}

// UpdateInstanceLastAddedToQueue updates ONLY the LastAddedToQueue field for a specific instance.
func (s *Storage) UpdateInstanceLastAddedToQueue(title string, lastAddedToQueue time.Time) error {
	if s.repo != nil {
		return s.updateFieldInRepo(title, func(d *InstanceData) { d.LastAddedToQueue = lastAddedToQueue })
	}

	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	// Find and update the matching instance data
	found := false
	for i, data := range instancesData {
		if data.Title == title {
			instancesData[i].LastAddedToQueue = lastAddedToQueue
			found = true
			log.DebugLog.Printf("Updating LastAddedToQueue in storage for session %s (no instance objects created)", title)
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	// Marshal back to JSON
	updatedJSON, err := json.Marshal(instancesData)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	// Use async save if available
	if s.stateService != nil {
		s.stateService.SaveAsync(updatedJSON)
		return nil
	}

	// Fallback to synchronous save
	return s.state.SaveInstances(updatedJSON)
}

// UpdateInstanceLastUserResponse updates just the LastUserResponse timestamp for a specific instance.
func (s *Storage) UpdateInstanceLastUserResponse(title string, lastUserResponse time.Time) error {
	if s.repo != nil {
		return s.updateFieldInRepo(title, func(d *InstanceData) { d.LastUserResponse = lastUserResponse })
	}

	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	// Find and update the matching instance data
	found := false
	for i, data := range instancesData {
		if data.Title == title {
			instancesData[i].LastUserResponse = lastUserResponse
			found = true
			log.DebugLog.Printf("Updating LastUserResponse in storage for session %s (no instance objects created)", title)
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	// Marshal back to JSON
	updatedJSON, err := json.Marshal(instancesData)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	// Use async save if available
	if s.stateService != nil {
		s.stateService.SaveAsync(updatedJSON)
		return nil
	}

	// Fallback to synchronous save
	return s.state.SaveInstances(updatedJSON)
}

// UpdateInstanceProcessingGrace updates just the ProcessingGraceUntil timestamp for a specific instance.
func (s *Storage) UpdateInstanceProcessingGrace(title string, processingGraceUntil time.Time) error {
	if s.repo != nil {
		return s.updateFieldInRepo(title, func(d *InstanceData) { d.ProcessingGraceUntil = processingGraceUntil })
	}

	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	// Find and update the matching instance data
	found := false
	for i, data := range instancesData {
		if data.Title == title {
			instancesData[i].ProcessingGraceUntil = processingGraceUntil
			found = true
			log.DebugLog.Printf("Updating ProcessingGraceUntil in storage for session %s (no instance objects created)", title)
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	// Marshal back to JSON
	updatedJSON, err := json.Marshal(instancesData)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	// Use async save if available
	if s.stateService != nil {
		s.stateService.SaveAsync(updatedJSON)
		return nil
	}

	// Fallback to synchronous save
	return s.state.SaveInstances(updatedJSON)
}

// GetStateManager returns the underlying state manager
func (s *Storage) GetStateManager() interface{} {
	return s.state
}
