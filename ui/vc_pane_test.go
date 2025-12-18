package ui

import (
	"claude-squad/session/vc"
	"strings"
	"testing"
	"time"
)

func TestNewVCPane(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	if pane == nil {
		t.Fatal("NewVCPane() returned nil")
	}

	if pane.vcRequestCh == nil {
		t.Error("vcRequestCh should be initialized")
	}

	if pane.vcResultCh == nil {
		t.Error("vcResultCh should be initialized")
	}

	if pane.contentCache == nil {
		t.Error("contentCache should be initialized")
	}

	if !pane.showHelp {
		t.Error("showHelp should default to true")
	}
}

func TestVCPaneSetSize(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	pane.SetSize(80, 40)

	if pane.width != 80 {
		t.Errorf("width = %d, want 80", pane.width)
	}

	if pane.height != 40 {
		t.Errorf("height = %d, want 40", pane.height)
	}

	if pane.viewport.Width != 80 {
		t.Errorf("viewport.Width = %d, want 80", pane.viewport.Width)
	}

	// Viewport height should be adjusted for header/help
	expectedViewportHeight := 40 - 10
	if pane.viewport.Height != expectedViewportHeight {
		t.Errorf("viewport.Height = %d, want %d", pane.viewport.Height, expectedViewportHeight)
	}
}

func TestVCPaneGetInstanceID(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	t.Run("nil instance", func(t *testing.T) {
		id := pane.getInstanceID(nil)
		if id != "" {
			t.Errorf("getInstanceID(nil) = %q, want empty string", id)
		}
	})
}

func TestVCPaneNavigation(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	// Create a mock status with some files
	pane.status = &vc.VCSStatus{
		Type:   vc.VCSGit,
		Branch: "main",
		StagedFiles: []vc.FileChange{
			{Path: "staged1.go", Status: vc.FileAdded, IsStaged: true},
			{Path: "staged2.go", Status: vc.FileModified, IsStaged: true},
		},
		UnstagedFiles: []vc.FileChange{
			{Path: "unstaged.go", Status: vc.FileModified, IsStaged: false},
		},
		UntrackedFiles: []vc.FileChange{
			{Path: "untracked.go", Status: vc.FileUntracked, IsStaged: false},
		},
	}

	t.Run("initial selection", func(t *testing.T) {
		if pane.selectedIdx != 0 {
			t.Errorf("Initial selectedIdx = %d, want 0", pane.selectedIdx)
		}
	})

	t.Run("SelectNext wraps around", func(t *testing.T) {
		pane.selectedIdx = 0

		// Move through all 4 files
		pane.SelectNext()
		if pane.selectedIdx != 1 {
			t.Errorf("After first SelectNext, selectedIdx = %d, want 1", pane.selectedIdx)
		}

		pane.SelectNext()
		if pane.selectedIdx != 2 {
			t.Errorf("After second SelectNext, selectedIdx = %d, want 2", pane.selectedIdx)
		}

		pane.SelectNext()
		if pane.selectedIdx != 3 {
			t.Errorf("After third SelectNext, selectedIdx = %d, want 3", pane.selectedIdx)
		}

		// Wrap around
		pane.SelectNext()
		if pane.selectedIdx != 0 {
			t.Errorf("After wrap around SelectNext, selectedIdx = %d, want 0", pane.selectedIdx)
		}
	})

	t.Run("SelectPrev wraps around", func(t *testing.T) {
		pane.selectedIdx = 0

		// Should wrap to last item
		pane.SelectPrev()
		if pane.selectedIdx != 3 {
			t.Errorf("SelectPrev from 0 should wrap to 3, got %d", pane.selectedIdx)
		}

		pane.SelectPrev()
		if pane.selectedIdx != 2 {
			t.Errorf("SelectPrev should go to 2, got %d", pane.selectedIdx)
		}
	})

	t.Run("GetSelectedFile returns correct file", func(t *testing.T) {
		pane.selectedIdx = 0
		file := pane.GetSelectedFile()
		if file == nil {
			t.Fatal("GetSelectedFile returned nil")
		}
		if file.Path != "staged1.go" {
			t.Errorf("GetSelectedFile().Path = %q, want 'staged1.go'", file.Path)
		}

		pane.selectedIdx = 2
		file = pane.GetSelectedFile()
		if file == nil {
			t.Fatal("GetSelectedFile returned nil")
		}
		if file.Path != "unstaged.go" {
			t.Errorf("GetSelectedFile().Path = %q, want 'unstaged.go'", file.Path)
		}
	})

	t.Run("GetSelectedFile with nil status", func(t *testing.T) {
		originalStatus := pane.status
		pane.status = nil

		file := pane.GetSelectedFile()
		if file != nil {
			t.Error("GetSelectedFile with nil status should return nil")
		}

		pane.status = originalStatus
	})

	t.Run("GetSelectedFile with invalid index", func(t *testing.T) {
		pane.selectedIdx = 100

		file := pane.GetSelectedFile()
		if file != nil {
			t.Error("GetSelectedFile with invalid index should return nil")
		}

		pane.selectedIdx = -1
		file = pane.GetSelectedFile()
		if file != nil {
			t.Error("GetSelectedFile with negative index should return nil")
		}
	})
}

func TestVCPaneNavigationNilStatus(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	// Navigation with nil status should not panic
	pane.SelectNext()
	pane.SelectPrev()
	pane.ScrollUp()
	pane.ScrollDown()

	// Should remain at initial position
	if pane.selectedIdx != 0 {
		t.Errorf("selectedIdx should remain 0 with nil status, got %d", pane.selectedIdx)
	}
}

func TestVCPaneToggleHelp(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	initialState := pane.showHelp

	pane.ToggleHelp()
	if pane.showHelp == initialState {
		t.Error("ToggleHelp should change showHelp state")
	}

	pane.ToggleHelp()
	if pane.showHelp != initialState {
		t.Error("ToggleHelp again should restore original state")
	}
}

func TestVCPaneCacheOperations(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	instanceID := "test-instance-main"
	status := &vc.VCSStatus{
		Type:   vc.VCSGit,
		Branch: "main",
	}

	t.Run("setCachedContent and getCachedContent", func(t *testing.T) {
		// Initially, cache should be empty
		cached, ok := pane.getCachedContent(instanceID)
		if ok {
			t.Error("getCachedContent should return false for empty cache")
		}
		if cached != nil {
			t.Error("getCachedContent should return nil for empty cache")
		}

		// Set content
		pane.setCachedContent(instanceID, status, nil)

		// Should now be cached
		cached, ok = pane.getCachedContent(instanceID)
		if !ok {
			t.Error("getCachedContent should return true after setting")
		}
		if cached == nil {
			t.Fatal("getCachedContent should return non-nil after setting")
		}
		if cached.status != status {
			t.Error("Cached status should match set status")
		}
	})

	t.Run("InvalidateCache", func(t *testing.T) {
		// Ensure cache is set
		pane.setCachedContent(instanceID, status, nil)

		// Invalidate
		pane.InvalidateCache(instanceID)

		// Should no longer be cached
		cached, ok := pane.getCachedContent(instanceID)
		if ok {
			t.Error("getCachedContent should return false after invalidation")
		}
		if cached != nil {
			t.Error("getCachedContent should return nil after invalidation")
		}
	})
}

func TestVCPaneGetMaxVisibleItems(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	tests := []struct {
		name     string
		height   int
		showHelp bool
		minItems int
	}{
		{"small height with help", 20, true, 1},
		{"small height without help", 20, false, 1},
		{"medium height with help", 30, true, 1},
		{"large height", 50, false, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane.height = tt.height
			pane.showHelp = tt.showHelp
			items := pane.getMaxVisibleItems()

			if items < tt.minItems {
				t.Errorf("getMaxVisibleItems() = %d, want at least %d", items, tt.minItems)
			}
		})
	}
}

func TestVCPaneRenderNoVCS(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	pane.SetSize(80, 40)
	pane.status = nil

	output := pane.String()

	if !strings.Contains(output, "VC Status") {
		t.Error("Output should contain 'VC Status' title")
	}

	if !strings.Contains(output, "No version control system detected") {
		t.Error("Output should indicate no VCS detected")
	}
}

func TestVCPaneRenderWithStatus(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	pane.SetSize(80, 40)
	pane.status = &vc.VCSStatus{
		Type:   vc.VCSGit,
		Branch: "feature/test",
		StagedFiles: []vc.FileChange{
			{Path: "staged.go", Status: vc.FileAdded, IsStaged: true},
		},
		UnstagedFiles: []vc.FileChange{
			{Path: "modified.go", Status: vc.FileModified, IsStaged: false},
		},
		UntrackedFiles: []vc.FileChange{
			{Path: "new.go", Status: vc.FileUntracked, IsStaged: false},
		},
	}

	output := pane.String()

	// Should contain title
	if !strings.Contains(output, "VC Status") {
		t.Error("Output should contain 'VC Status'")
	}

	// Should contain VCS type
	if !strings.Contains(output, "Git") {
		t.Error("Output should contain VCS type 'Git'")
	}

	// Should contain branch
	if !strings.Contains(output, "feature/test") {
		t.Error("Output should contain branch name")
	}

	// Should contain section headers
	if !strings.Contains(output, "Staged") {
		t.Error("Output should contain 'Staged' section")
	}

	if !strings.Contains(output, "Unstaged") {
		t.Error("Output should contain 'Unstaged' section")
	}

	if !strings.Contains(output, "Untracked") {
		t.Error("Output should contain 'Untracked' section")
	}

	// Should contain file paths
	if !strings.Contains(output, "staged.go") {
		t.Error("Output should contain staged file path")
	}

	if !strings.Contains(output, "modified.go") {
		t.Error("Output should contain modified file path")
	}

	if !strings.Contains(output, "new.go") {
		t.Error("Output should contain untracked file path")
	}
}

func TestVCPaneRenderCleanRepo(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	pane.SetSize(80, 40)
	pane.status = &vc.VCSStatus{
		Type:    vc.VCSGit,
		Branch:  "main",
		IsClean: true,
	}

	output := pane.String()

	if !strings.Contains(output, "Working directory clean") {
		t.Error("Output should indicate working directory is clean")
	}
}

func TestVCPaneRenderHelp(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	pane.SetSize(80, 40)
	pane.status = &vc.VCSStatus{
		Type:    vc.VCSGit,
		Branch:  "main",
		IsClean: true,
	}

	t.Run("help shown", func(t *testing.T) {
		pane.showHelp = true
		output := pane.String()

		helpKeywords := []string{"navigate", "stage", "unstage", "terminal"}
		for _, keyword := range helpKeywords {
			if !strings.Contains(output, keyword) {
				t.Errorf("Output with help should contain '%s'", keyword)
			}
		}
	})

	t.Run("help hidden", func(t *testing.T) {
		pane.showHelp = false
		output := pane.String()

		// Help section should not appear (no help separator)
		if strings.Contains(output, "───") {
			t.Error("Output without help should not contain help separator")
		}
	})
}

func TestVCPaneRenderEmptySize(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	// Don't set size - should return empty string
	output := pane.String()
	if output != "" {
		t.Errorf("String() with zero size should return empty string, got %q", output)
	}
}

func TestVCPaneAheadBehind(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	pane.SetSize(80, 40)
	pane.status = &vc.VCSStatus{
		Type:     vc.VCSGit,
		Branch:   "main",
		AheadBy:  3,
		BehindBy: 2,
		IsClean:  true,
	}

	output := pane.String()

	if !strings.Contains(output, "+3/-2") {
		t.Error("Output should contain ahead/behind counts")
	}
}

func TestVCPaneConflictFiles(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	pane.SetSize(80, 40)
	pane.status = &vc.VCSStatus{
		Type:   vc.VCSGit,
		Branch: "main",
		ConflictFiles: []vc.FileChange{
			{Path: "conflict.go", Status: vc.FileConflict, IsStaged: false},
		},
	}

	output := pane.String()

	if !strings.Contains(output, "Conflicts") {
		t.Error("Output should contain 'Conflicts' section")
	}

	if !strings.Contains(output, "conflict.go") {
		t.Error("Output should contain conflict file path")
	}
}

func TestVCPaneCommandPalette(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	pane.SetSize(80, 40)

	t.Run("initially not visible", func(t *testing.T) {
		if pane.IsCommandPaletteVisible() {
			t.Error("Command palette should not be visible initially")
		}
	})

	t.Run("show command palette", func(t *testing.T) {
		pane.ShowCommandPalette()

		if !pane.IsCommandPaletteVisible() {
			t.Error("Command palette should be visible after ShowCommandPalette")
		}

		if pane.GetCommandPalette() == nil {
			t.Error("GetCommandPalette should return non-nil after ShowCommandPalette")
		}
	})

	t.Run("hide command palette", func(t *testing.T) {
		pane.HideCommandPalette()

		if pane.IsCommandPaletteVisible() {
			t.Error("Command palette should not be visible after HideCommandPalette")
		}
	})

	t.Run("set command palette callbacks", func(t *testing.T) {
		var selectedCmd VCCommand
		var cancelled bool

		pane.SetCommandPaletteCallbacks(
			func(cmd VCCommand) { selectedCmd = cmd },
			func() { cancelled = true },
		)

		// Verify callbacks are set by checking GetCommandPalette
		cp := pane.GetCommandPalette()
		if cp == nil {
			t.Fatal("GetCommandPalette should not be nil after SetCommandPaletteCallbacks")
		}

		// Execute callbacks to verify they were set
		if cp.OnSelect != nil {
			cp.OnSelect(VCCommand{Name: "test"})
			if selectedCmd.Name != "test" {
				t.Error("OnSelect callback not properly set")
			}
		}

		if cp.OnCancel != nil {
			cp.OnCancel()
			if !cancelled {
				t.Error("OnCancel callback not properly set")
			}
		}
	})
}

func TestVCPaneGetProvider(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	// Initially nil
	if pane.GetProvider() != nil {
		t.Error("GetProvider should return nil initially")
	}
}

func TestVCPaneGetStatus(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	// Initially nil
	if pane.GetStatus() != nil {
		t.Error("GetStatus should return nil initially")
	}

	// Set status
	status := &vc.VCSStatus{
		Type:   vc.VCSGit,
		Branch: "main",
	}
	pane.status = status

	if pane.GetStatus() != status {
		t.Error("GetStatus should return the set status")
	}
}

func TestVCPaneProcessResultsResetSelection(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	// Set up a status with 2 files and selected index at 3
	pane.selectedIdx = 3

	// Simulate receiving a result with fewer files
	result := vcResult{
		status: &vc.VCSStatus{
			Type:   vc.VCSGit,
			Branch: "main",
			StagedFiles: []vc.FileChange{
				{Path: "file1.go", Status: vc.FileAdded, IsStaged: true},
			},
		},
		provider:   nil,
		err:        nil,
		instanceID: "test-id",
	}

	// Send result to channel
	pane.vcResultCh <- result

	// Process results
	_ = pane.ProcessResults()

	// Selection should be reset to valid index
	if pane.selectedIdx >= 1 { // Only 1 file, so valid indices are 0
		// ProcessResults should reset selectedIdx to 0
		if pane.selectedIdx != 0 {
			t.Errorf("selectedIdx should be reset to 0 when out of bounds, got %d", pane.selectedIdx)
		}
	}
}

func TestVCPaneCleanup(t *testing.T) {
	pane := NewVCPane()

	// Should not panic
	pane.Cleanup()

	// Double cleanup should not panic
	pane.Cleanup()
}

func TestVCPaneScrollUpDown(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	pane.status = &vc.VCSStatus{
		Type:   vc.VCSGit,
		Branch: "main",
		StagedFiles: []vc.FileChange{
			{Path: "file1.go", Status: vc.FileAdded, IsStaged: true},
			{Path: "file2.go", Status: vc.FileAdded, IsStaged: true},
			{Path: "file3.go", Status: vc.FileAdded, IsStaged: true},
		},
	}

	// ScrollUp should call SelectPrev
	pane.selectedIdx = 1
	pane.ScrollUp()
	if pane.selectedIdx != 0 {
		t.Errorf("ScrollUp should decrement selection, got %d", pane.selectedIdx)
	}

	// ScrollDown should call SelectNext
	pane.selectedIdx = 1
	pane.ScrollDown()
	if pane.selectedIdx != 2 {
		t.Errorf("ScrollDown should increment selection, got %d", pane.selectedIdx)
	}
}

func TestVCPaneCacheTTL(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	instanceID := "test-ttl"
	status := &vc.VCSStatus{
		Type:   vc.VCSGit,
		Branch: "main",
	}

	// Set content with old timestamp
	pane.mu.Lock()
	pane.contentCache[instanceID] = cachedVCContent{
		status:    status,
		provider:  nil,
		timestamp: time.Now().Add(-3 * time.Second), // Older than TTL (2s)
		isValid:   true,
	}
	pane.mu.Unlock()

	// Should not return expired cache
	cached, ok := pane.getCachedContent(instanceID)
	if ok {
		t.Error("getCachedContent should return false for expired cache")
	}
	if cached != nil {
		t.Error("getCachedContent should return nil for expired cache")
	}
}

func TestVCPaneEnsureSelectedVisible(t *testing.T) {
	pane := NewVCPane()
	defer pane.Cleanup()

	pane.height = 20
	pane.showHelp = false

	// Create many files
	files := make([]vc.FileChange, 50)
	for i := 0; i < 50; i++ {
		files[i] = vc.FileChange{
			Path:     "file" + string(rune('a'+i)) + ".go",
			Status:   vc.FileModified,
			IsStaged: false,
		}
	}
	pane.status = &vc.VCSStatus{
		Type:          vc.VCSGit,
		Branch:        "main",
		UnstagedFiles: files,
	}

	t.Run("selection below viewport adjusts offset", func(t *testing.T) {
		pane.scrollOffset = 0
		pane.selectedIdx = 30

		pane.ensureSelectedVisible()

		// Scroll offset should have been adjusted
		maxVisible := pane.getMaxVisibleItems()
		if pane.scrollOffset < pane.selectedIdx-maxVisible+1 {
			t.Errorf("scrollOffset should be adjusted to show selected item")
		}
	})

	t.Run("selection above viewport adjusts offset", func(t *testing.T) {
		pane.scrollOffset = 20
		pane.selectedIdx = 5

		pane.ensureSelectedVisible()

		if pane.scrollOffset > pane.selectedIdx {
			t.Errorf("scrollOffset should be adjusted to show selected item above viewport")
		}
	})
}
