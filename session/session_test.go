package session

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSessionJSONSerialization verifies Session can be serialized and deserialized
func TestSessionJSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second) // Truncate for JSON comparison

	original := &Session{
		ID:        "test-session-1",
		Title:     "Test Session",
		CreatedAt: now,
		UpdatedAt: now,
		Status:    Running,
		Program:   "claude",
		AutoYes:   true,
		Prompt:    "help me code",
		Git: &GitContext{
			Branch:    "feature/test",
			PRNumber:  123,
			PRURL:     "https://github.com/test/repo/pull/123",
			Owner:     "test",
			Repo:      "repo",
			SourceRef: "refs/heads/feature/test",
		},
		Filesystem: &FilesystemContext{
			ProjectPath:    "/home/user/project",
			WorkingDir:     "/home/user/project/src",
			IsWorktree:     true,
			MainRepoPath:   "/home/user/main-repo",
			SessionType:    SessionTypeNewWorktree,
		},
		Terminal: &TerminalContext{
			Height:           24,
			Width:            80,
			TmuxPrefix:       "squad_",
			TmuxSessionName:  "squad_test",
			TmuxServerSocket: "test_socket",
			TerminalType:     "tmux",
		},
		UI: &UIPreferences{
			Category:   "Development",
			IsExpanded: true,
			Tags:       []string{"frontend", "urgent"},
		},
		Activity: &ActivityTracking{
			LastTerminalUpdate:   now,
			LastMeaningfulOutput: now.Add(-5 * time.Minute),
			LastViewed:           now.Add(-10 * time.Minute),
			LastAcknowledged:     now.Add(-15 * time.Minute),
			LastOutputSignature:  "abc123",
			LastAddedToQueue:     now.Add(-20 * time.Minute),
		},
		Cloud: &CloudContext{
			Provider:       "aws",
			Region:         "us-west-2",
			APIEndpoint:    "https://api.example.com",
			CloudSessionID: "cloud-123",
		},
	}

	// Serialize to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal session: %v", err)
	}

	// Deserialize back
	var restored Session
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal session: %v", err)
	}

	// Verify core fields
	if restored.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", restored.ID, original.ID)
	}
	if restored.Title != original.Title {
		t.Errorf("Title mismatch: got %q, want %q", restored.Title, original.Title)
	}
	if restored.Status != original.Status {
		t.Errorf("Status mismatch: got %v, want %v", restored.Status, original.Status)
	}
	if restored.Program != original.Program {
		t.Errorf("Program mismatch: got %q, want %q", restored.Program, original.Program)
	}
	if restored.AutoYes != original.AutoYes {
		t.Errorf("AutoYes mismatch: got %v, want %v", restored.AutoYes, original.AutoYes)
	}

	// Verify Git context
	if restored.Git == nil {
		t.Fatal("Git context is nil after deserialization")
	}
	if restored.Git.Branch != original.Git.Branch {
		t.Errorf("Git.Branch mismatch: got %q, want %q", restored.Git.Branch, original.Git.Branch)
	}
	if restored.Git.PRNumber != original.Git.PRNumber {
		t.Errorf("Git.PRNumber mismatch: got %d, want %d", restored.Git.PRNumber, original.Git.PRNumber)
	}

	// Verify Filesystem context
	if restored.Filesystem == nil {
		t.Fatal("Filesystem context is nil after deserialization")
	}
	if restored.Filesystem.ProjectPath != original.Filesystem.ProjectPath {
		t.Errorf("Filesystem.ProjectPath mismatch: got %q, want %q",
			restored.Filesystem.ProjectPath, original.Filesystem.ProjectPath)
	}
	if restored.Filesystem.IsWorktree != original.Filesystem.IsWorktree {
		t.Errorf("Filesystem.IsWorktree mismatch: got %v, want %v",
			restored.Filesystem.IsWorktree, original.Filesystem.IsWorktree)
	}

	// Verify Terminal context
	if restored.Terminal == nil {
		t.Fatal("Terminal context is nil after deserialization")
	}
	if restored.Terminal.Height != original.Terminal.Height {
		t.Errorf("Terminal.Height mismatch: got %d, want %d",
			restored.Terminal.Height, original.Terminal.Height)
	}

	// Verify UI preferences
	if restored.UI == nil {
		t.Fatal("UI preferences is nil after deserialization")
	}
	if restored.UI.Category != original.UI.Category {
		t.Errorf("UI.Category mismatch: got %q, want %q",
			restored.UI.Category, original.UI.Category)
	}
	if len(restored.UI.Tags) != len(original.UI.Tags) {
		t.Errorf("UI.Tags length mismatch: got %d, want %d",
			len(restored.UI.Tags), len(original.UI.Tags))
	}

	// Verify Activity tracking
	if restored.Activity == nil {
		t.Fatal("Activity tracking is nil after deserialization")
	}
	if restored.Activity.LastOutputSignature != original.Activity.LastOutputSignature {
		t.Errorf("Activity.LastOutputSignature mismatch: got %q, want %q",
			restored.Activity.LastOutputSignature, original.Activity.LastOutputSignature)
	}

	// Verify Cloud context
	if restored.Cloud == nil {
		t.Fatal("Cloud context is nil after deserialization")
	}
	if restored.Cloud.Provider != original.Cloud.Provider {
		t.Errorf("Cloud.Provider mismatch: got %q, want %q",
			restored.Cloud.Provider, original.Cloud.Provider)
	}
}

// TestSessionMinimalSerialization verifies Session with nil contexts serializes correctly
func TestSessionMinimalSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	minimal := &Session{
		ID:        "minimal-1",
		Title:     "Minimal Session",
		CreatedAt: now,
		UpdatedAt: now,
		Status:    Loading,
		Program:   "aider",
		// All contexts are nil
	}

	data, err := json.Marshal(minimal)
	if err != nil {
		t.Fatalf("Failed to marshal minimal session: %v", err)
	}

	var restored Session
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal minimal session: %v", err)
	}

	// Verify contexts remain nil
	if restored.Git != nil {
		t.Error("Git context should be nil")
	}
	if restored.Filesystem != nil {
		t.Error("Filesystem context should be nil")
	}
	if restored.Terminal != nil {
		t.Error("Terminal context should be nil")
	}
	if restored.UI != nil {
		t.Error("UI preferences should be nil")
	}
	if restored.Activity != nil {
		t.Error("Activity tracking should be nil")
	}
	if restored.Cloud != nil {
		t.Error("Cloud context should be nil")
	}
}

// TestInstanceToSessionConversion verifies Instance -> Session conversion
func TestInstanceToSessionConversion(t *testing.T) {
	now := time.Now()

	instance := &Instance{
		Title:                "Test Instance",
		Path:                 "/home/user/project",
		WorkingDir:           "/home/user/project/src",
		Branch:               "main",
		Status:               Running,
		Program:              "claude",
		Height:               24,
		Width:                80,
		CreatedAt:            now,
		UpdatedAt:            now,
		AutoYes:              true,
		Prompt:               "test prompt",
		Category:             "Work",
		IsExpanded:           true,
		Tags:                 []string{"urgent", "frontend"},
		TmuxPrefix:           "squad_",
		TmuxServerSocket:     "test",
		GitHubPRNumber:       42,
		GitHubPRURL:          "https://github.com/test/repo/pull/42",
		GitHubOwner:          "test",
		GitHubRepo:           "repo",
		GitHubSourceRef:      "refs/pull/42/head",
		IsWorktree:   true,
		MainRepoPath: "/home/user/main",
		ReviewState: ReviewState{
			LastTerminalUpdate:   now,
			LastMeaningfulOutput: now.Add(-5 * time.Minute),
			LastViewed:           now.Add(-10 * time.Minute),
			LastAcknowledged:     now.Add(-15 * time.Minute),
			LastOutputSignature:  "sig123",
		},
	}

	session := InstanceToSession(instance)

	// Verify core fields
	if session.Title != instance.Title {
		t.Errorf("Title mismatch: got %q, want %q", session.Title, instance.Title)
	}
	if session.Status != instance.Status {
		t.Errorf("Status mismatch: got %v, want %v", session.Status, instance.Status)
	}
	if session.Program != instance.Program {
		t.Errorf("Program mismatch: got %q, want %q", session.Program, instance.Program)
	}

	// Verify Git context was populated
	if session.Git == nil {
		t.Fatal("Git context should not be nil")
	}
	if session.Git.Branch != instance.Branch {
		t.Errorf("Git.Branch mismatch: got %q, want %q", session.Git.Branch, instance.Branch)
	}
	if session.Git.PRNumber != instance.GitHubPRNumber {
		t.Errorf("Git.PRNumber mismatch: got %d, want %d", session.Git.PRNumber, instance.GitHubPRNumber)
	}

	// Verify Filesystem context was populated
	if session.Filesystem == nil {
		t.Fatal("Filesystem context should not be nil")
	}
	if session.Filesystem.ProjectPath != instance.Path {
		t.Errorf("Filesystem.ProjectPath mismatch: got %q, want %q",
			session.Filesystem.ProjectPath, instance.Path)
	}

	// Verify Terminal context was populated
	if session.Terminal == nil {
		t.Fatal("Terminal context should not be nil")
	}
	if session.Terminal.Height != instance.Height {
		t.Errorf("Terminal.Height mismatch: got %d, want %d",
			session.Terminal.Height, instance.Height)
	}

	// Verify UI preferences was populated
	if session.UI == nil {
		t.Fatal("UI preferences should not be nil")
	}
	if session.UI.Category != instance.Category {
		t.Errorf("UI.Category mismatch: got %q, want %q",
			session.UI.Category, instance.Category)
	}

	// Verify Activity tracking was populated
	if session.Activity == nil {
		t.Fatal("Activity tracking should not be nil")
	}
	if session.Activity.LastOutputSignature != instance.LastOutputSignature {
		t.Errorf("Activity.LastOutputSignature mismatch: got %q, want %q",
			session.Activity.LastOutputSignature, instance.LastOutputSignature)
	}

	// Cloud should be nil (not in Instance)
	if session.Cloud != nil {
		t.Error("Cloud context should be nil")
	}
}

// TestSessionToInstanceConversion verifies Session -> Instance conversion
func TestSessionToInstanceConversion(t *testing.T) {
	now := time.Now()

	session := &Session{
		ID:        "session-1",
		Title:     "Test Session",
		CreatedAt: now,
		UpdatedAt: now,
		Status:    Ready,
		Program:   "aider",
		AutoYes:   false,
		Prompt:    "help",
		Git: &GitContext{
			Branch:    "develop",
			PRNumber:  99,
			PRURL:     "https://github.com/org/repo/pull/99",
			Owner:     "org",
			Repo:      "repo",
			SourceRef: "refs/heads/develop",
		},
		Filesystem: &FilesystemContext{
			ProjectPath:  "/projects/myapp",
			WorkingDir:   "/projects/myapp/lib",
			IsWorktree:   false,
			SessionType:  SessionTypeDirectory,
		},
		Terminal: &TerminalContext{
			Height:           30,
			Width:            120,
			TmuxPrefix:       "cs_",
			TmuxServerSocket: "main",
		},
		UI: &UIPreferences{
			Category:   "Personal",
			IsExpanded: false,
			Tags:       []string{"backend", "api"},
		},
		Activity: &ActivityTracking{
			LastTerminalUpdate:   now,
			LastMeaningfulOutput: now.Add(-1 * time.Hour),
			LastViewed:           now.Add(-2 * time.Hour),
			LastAcknowledged:     now.Add(-3 * time.Hour),
			LastOutputSignature:  "xyz789",
			LastAddedToQueue:     now.Add(-4 * time.Hour),
		},
		Cloud: &CloudContext{
			Provider: "gcp",
			Region:   "us-central1",
		},
	}

	instance := SessionToInstance(session)

	// Verify core fields
	if instance.Title != session.Title {
		t.Errorf("Title mismatch: got %q, want %q", instance.Title, session.Title)
	}
	if instance.Status != session.Status {
		t.Errorf("Status mismatch: got %v, want %v", instance.Status, session.Status)
	}

	// Verify Git fields were extracted
	if instance.Branch != session.Git.Branch {
		t.Errorf("Branch mismatch: got %q, want %q", instance.Branch, session.Git.Branch)
	}
	if instance.GitHubPRNumber != session.Git.PRNumber {
		t.Errorf("GitHubPRNumber mismatch: got %d, want %d",
			instance.GitHubPRNumber, session.Git.PRNumber)
	}

	// Verify Filesystem fields were extracted
	if instance.Path != session.Filesystem.ProjectPath {
		t.Errorf("Path mismatch: got %q, want %q",
			instance.Path, session.Filesystem.ProjectPath)
	}

	// Verify Terminal fields were extracted
	if instance.Height != session.Terminal.Height {
		t.Errorf("Height mismatch: got %d, want %d",
			instance.Height, session.Terminal.Height)
	}

	// Verify UI fields were extracted
	if instance.Category != session.UI.Category {
		t.Errorf("Category mismatch: got %q, want %q",
			instance.Category, session.UI.Category)
	}
	if len(instance.Tags) != len(session.UI.Tags) {
		t.Errorf("Tags length mismatch: got %d, want %d",
			len(instance.Tags), len(session.UI.Tags))
	}

	// Verify Activity fields were extracted
	if instance.LastOutputSignature != session.Activity.LastOutputSignature {
		t.Errorf("LastOutputSignature mismatch: got %q, want %q",
			instance.LastOutputSignature, session.Activity.LastOutputSignature)
	}

	// Cloud data is lost (expected - no Instance equivalent)
	// No assertion needed
}

// TestRoundTripConversion verifies Instance -> Session -> Instance preserves data
func TestRoundTripConversion(t *testing.T) {
	now := time.Now()

	original := &Instance{
		Title:                "Round Trip Test",
		Path:                 "/test/path",
		WorkingDir:           "/test/path/subdir",
		Branch:               "feature",
		Status:               Paused,
		Program:              "claude",
		Height:               40,
		Width:                160,
		CreatedAt:            now,
		UpdatedAt:            now,
		AutoYes:              true,
		Prompt:               "round trip",
		Category:             "Testing",
		IsExpanded:           true,
		Tags:                 []string{"test", "important"},
		TmuxPrefix:           "rt_",
		GitHubPRNumber:       7,
		GitHubPRURL:          "https://github.com/a/b/pull/7",
		GitHubOwner:          "a",
		GitHubRepo:           "b",
		IsWorktree:   true,
		MainRepoPath: "/main/repo",
		ReviewState: ReviewState{
			LastTerminalUpdate:   now,
			LastMeaningfulOutput: now,
			LastOutputSignature:  "roundtrip",
		},
	}

	// Convert to Session and back
	session := InstanceToSession(original)
	restored := SessionToInstance(session)

	// Verify key fields match
	if restored.Title != original.Title {
		t.Errorf("Title mismatch after round trip")
	}
	if restored.Path != original.Path {
		t.Errorf("Path mismatch after round trip")
	}
	if restored.Branch != original.Branch {
		t.Errorf("Branch mismatch after round trip")
	}
	if restored.Status != original.Status {
		t.Errorf("Status mismatch after round trip")
	}
	if restored.Category != original.Category {
		t.Errorf("Category mismatch after round trip")
	}
	if len(restored.Tags) != len(original.Tags) {
		t.Errorf("Tags length mismatch after round trip")
	}
	if restored.GitHubPRNumber != original.GitHubPRNumber {
		t.Errorf("GitHubPRNumber mismatch after round trip")
	}
}

// TestNilInstanceConversion verifies nil handling
func TestNilInstanceConversion(t *testing.T) {
	if session := InstanceToSession(nil); session != nil {
		t.Error("InstanceToSession(nil) should return nil")
	}

	if instance := SessionToInstance(nil); instance != nil {
		t.Error("SessionToInstance(nil) should return nil")
	}
}

// TestContextHelperMethods verifies context checker and accessor methods
func TestContextHelperMethods(t *testing.T) {
	// Session with no contexts
	empty := &Session{
		ID:      "empty",
		Title:   "Empty",
		Status:  Loading,
		Program: "test",
	}

	if empty.HasGitContext() {
		t.Error("Empty session should not have Git context")
	}
	if empty.HasFilesystemContext() {
		t.Error("Empty session should not have Filesystem context")
	}
	if empty.GetBranch() != "" {
		t.Error("GetBranch should return empty string for nil Git context")
	}
	if empty.GetPath() != "" {
		t.Error("GetPath should return empty string for nil Filesystem context")
	}
	if empty.GetCategory() != "" {
		t.Error("GetCategory should return empty string for nil UI context")
	}

	// Session with contexts
	full := &Session{
		ID:      "full",
		Title:   "Full",
		Status:  Running,
		Program: "claude",
		Git: &GitContext{
			Branch: "main",
		},
		Filesystem: &FilesystemContext{
			ProjectPath: "/path",
		},
		UI: &UIPreferences{
			Category: "Work",
			Tags:     []string{"tag1"},
		},
	}

	if !full.HasGitContext() {
		t.Error("Full session should have Git context")
	}
	if full.GetBranch() != "main" {
		t.Errorf("GetBranch should return 'main', got %q", full.GetBranch())
	}
	if full.GetPath() != "/path" {
		t.Errorf("GetPath should return '/path', got %q", full.GetPath())
	}
	if full.GetCategory() != "Work" {
		t.Errorf("GetCategory should return 'Work', got %q", full.GetCategory())
	}
	tags := full.GetTags()
	if len(tags) != 1 || tags[0] != "tag1" {
		t.Errorf("GetTags mismatch: got %v", tags)
	}
}

// TestContextOptionsPresets verifies the ContextOptions presets exist and have expected values
func TestContextOptionsPresets(t *testing.T) {
	// ContextMinimal should have nothing loaded
	if ContextMinimal.AnyContextLoaded() {
		t.Error("ContextMinimal should not have any contexts loaded")
	}

	// ContextFull should have everything loaded
	if !ContextFull.LoadGit || !ContextFull.LoadFilesystem || !ContextFull.LoadTerminal ||
		!ContextFull.LoadUI || !ContextFull.LoadActivity || !ContextFull.LoadCloud {
		t.Error("ContextFull should have all contexts loaded")
	}
	if !ContextFull.LoadDiffContent || !ContextFull.LoadTags || !ContextFull.LoadClaudeSession {
		t.Error("ContextFull should have all child data loaded")
	}

	// ContextUIView should have UI and Activity
	if !ContextUIView.LoadUI || !ContextUIView.LoadActivity {
		t.Error("ContextUIView should load UI and Activity")
	}
	if ContextUIView.LoadTerminal {
		t.Error("ContextUIView should not load Terminal")
	}

	// ContextCloudSession should have Cloud context
	if !ContextCloudSession.LoadCloud {
		t.Error("ContextCloudSession should load Cloud context")
	}
	if ContextCloudSession.LoadFilesystem {
		t.Error("ContextCloudSession should not load Filesystem")
	}
}

// TestContextOptionsMerge verifies merging ContextOptions
func TestContextOptionsMerge(t *testing.T) {
	opts1 := ContextOptions{
		LoadGit:      true,
		LoadActivity: true,
	}
	opts2 := ContextOptions{
		LoadFilesystem: true,
		LoadCloud:      true,
	}

	merged := opts1.Merge(opts2)

	if !merged.LoadGit || !merged.LoadActivity || !merged.LoadFilesystem || !merged.LoadCloud {
		t.Error("Merged options should have all flags from both sources")
	}
	if merged.LoadTerminal || merged.LoadUI {
		t.Error("Merged options should not have flags not in either source")
	}
}
