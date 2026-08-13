package vc

import (
	"testing"
)

func TestVCSTypeString(t *testing.T) {
	tests := []struct {
		vcsType  VCSType
		expected string
	}{
		{VCSUnknown, "Unknown"},
		{VCSGit, "Git"},
		{VCSJujutsu, "Jujutsu"},
		{VCSType(999), "Unknown"}, // Unknown type defaults to "Unknown"
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.vcsType.String()
			if result != tt.expected {
				t.Errorf("VCSType(%d).String() = %q, want %q", tt.vcsType, result, tt.expected)
			}
		})
	}
}

func TestFileStatusString(t *testing.T) {
	tests := []struct {
		status   FileStatus
		expected string
	}{
		{FileUnmodified, " "},
		{FileModified, "M"},
		{FileAdded, "A"},
		{FileDeleted, "D"},
		{FileRenamed, "R"},
		{FileCopied, "C"},
		{FileUntracked, "?"},
		{FileIgnored, "!"},
		{FileConflict, "U"},
		{FileStatus(999), " "}, // Unknown status defaults to space
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.status.String()
			if result != tt.expected {
				t.Errorf("FileStatus(%d).String() = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}

func TestVCSStatusAllChangedFiles(t *testing.T) {
	status := &VCSStatus{
		Type:   VCSGit,
		Branch: "main",
		StagedFiles: []FileChange{
			{Path: "staged1.go", Status: FileAdded, IsStaged: true},
			{Path: "staged2.go", Status: FileModified, IsStaged: true},
		},
		UnstagedFiles: []FileChange{
			{Path: "unstaged1.go", Status: FileModified, IsStaged: false},
		},
		UntrackedFiles: []FileChange{
			{Path: "untracked.go", Status: FileUntracked, IsStaged: false},
		},
	}

	all := status.AllChangedFiles()

	if len(all) != 4 {
		t.Errorf("AllChangedFiles() returned %d files, want 4", len(all))
	}

	// Verify order: staged, unstaged, untracked
	expectedOrder := []string{"staged1.go", "staged2.go", "unstaged1.go", "untracked.go"}
	for i, expected := range expectedOrder {
		if i >= len(all) {
			t.Errorf("Missing file at index %d: expected %q", i, expected)
			continue
		}
		if all[i].Path != expected {
			t.Errorf("AllChangedFiles()[%d].Path = %q, want %q", i, all[i].Path, expected)
		}
	}
}

func TestVCSStatusAllChangedFilesEmpty(t *testing.T) {
	status := &VCSStatus{
		Type:   VCSGit,
		Branch: "main",
	}

	all := status.AllChangedFiles()

	if len(all) != 0 {
		t.Errorf("AllChangedFiles() on empty status returned %d files, want 0", len(all))
	}
}

func TestVCSStatusTotalChanges(t *testing.T) {
	tests := []struct {
		name     string
		status   *VCSStatus
		expected int
	}{
		{
			name: "all types",
			status: &VCSStatus{
				StagedFiles:    []FileChange{{Path: "a"}, {Path: "b"}},
				UnstagedFiles:  []FileChange{{Path: "c"}},
				UntrackedFiles: []FileChange{{Path: "d"}, {Path: "e"}, {Path: "f"}},
			},
			expected: 6,
		},
		{
			name: "staged only",
			status: &VCSStatus{
				StagedFiles: []FileChange{{Path: "a"}, {Path: "b"}},
			},
			expected: 2,
		},
		{
			name:     "empty status",
			status:   &VCSStatus{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.TotalChanges()
			if result != tt.expected {
				t.Errorf("TotalChanges() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestVCSStatusAheadBehindString(t *testing.T) {
	tests := []struct {
		name     string
		status   *VCSStatus
		expected string
	}{
		{
			name: "ahead and behind",
			status: &VCSStatus{
				AheadBy:  3,
				BehindBy: 2,
			},
			expected: "+3/-2",
		},
		{
			name: "ahead only",
			status: &VCSStatus{
				AheadBy:  5,
				BehindBy: 0,
			},
			expected: "+5/-0",
		},
		{
			name: "behind only",
			status: &VCSStatus{
				AheadBy:  0,
				BehindBy: 7,
			},
			expected: "+0/-7",
		},
		{
			name: "synced",
			status: &VCSStatus{
				AheadBy:  0,
				BehindBy: 0,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.AheadBehindString()
			if result != tt.expected {
				t.Errorf("AheadBehindString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFileChangeStruct(t *testing.T) {
	change := FileChange{
		Path:     "src/main.go",
		OldPath:  "src/old_main.go",
		Status:   FileRenamed,
		IsStaged: true,
	}

	if change.Path != "src/main.go" {
		t.Errorf("FileChange.Path = %q, want %q", change.Path, "src/main.go")
	}
	if change.OldPath != "src/old_main.go" {
		t.Errorf("FileChange.OldPath = %q, want %q", change.OldPath, "src/old_main.go")
	}
	if change.Status != FileRenamed {
		t.Errorf("FileChange.Status = %v, want %v", change.Status, FileRenamed)
	}
	if !change.IsStaged {
		t.Error("FileChange.IsStaged should be true")
	}
}
