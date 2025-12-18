package vc

// VCSType represents the type of version control system
type VCSType int

const (
	VCSUnknown VCSType = iota
	VCSGit
	VCSJujutsu
)

// String returns the string representation of the VCS type
func (v VCSType) String() string {
	switch v {
	case VCSGit:
		return "Git"
	case VCSJujutsu:
		return "Jujutsu"
	default:
		return "Unknown"
	}
}

// FileStatus represents the status of a file in version control
type FileStatus int

const (
	FileUnmodified FileStatus = iota
	FileModified
	FileAdded
	FileDeleted
	FileRenamed
	FileCopied
	FileUntracked
	FileIgnored
	FileConflict
)

// String returns a single character representing the file status
func (s FileStatus) String() string {
	switch s {
	case FileModified:
		return "M"
	case FileAdded:
		return "A"
	case FileDeleted:
		return "D"
	case FileRenamed:
		return "R"
	case FileCopied:
		return "C"
	case FileUntracked:
		return "?"
	case FileIgnored:
		return "!"
	case FileConflict:
		return "U"
	default:
		return " "
	}
}

// FileChange represents a changed file in the working directory
type FileChange struct {
	Path     string     // File path relative to repository root
	Status   FileStatus // Type of change
	IsStaged bool       // Whether the change is staged for commit
	OldPath  string     // Original path for renames/copies
}

// VCSStatus represents the current status of the version control system
type VCSStatus struct {
	// VCS identification
	Type VCSType

	// Branch/change information
	Branch      string // Current branch name (Git) or bookmark (Jujutsu)
	HeadCommit  string // Short SHA (Git) or change ID (Jujutsu)
	Description string // Commit message or change description

	// Remote tracking
	AheadBy  int    // Commits ahead of upstream
	BehindBy int    // Commits behind upstream
	Upstream string // Name of upstream branch/remote

	// Working directory state
	HasStaged    bool // Has staged changes
	HasUnstaged  bool // Has unstaged changes
	HasUntracked bool // Has untracked files
	HasConflicts bool // Has merge/rebase conflicts
	IsClean      bool // Working directory is clean

	// File lists
	StagedFiles    []FileChange
	UnstagedFiles  []FileChange
	UntrackedFiles []FileChange
	ConflictFiles  []FileChange
}

// AllChangedFiles returns all files with changes, in display order
func (s *VCSStatus) AllChangedFiles() []FileChange {
	var files []FileChange
	files = append(files, s.StagedFiles...)
	files = append(files, s.UnstagedFiles...)
	files = append(files, s.UntrackedFiles...)
	files = append(files, s.ConflictFiles...)
	return files
}

// TotalChanges returns the total number of changed files
func (s *VCSStatus) TotalChanges() int {
	return len(s.StagedFiles) + len(s.UnstagedFiles) + len(s.UntrackedFiles) + len(s.ConflictFiles)
}

// AheadBehindString returns a formatted string like "+2/-1"
func (s *VCSStatus) AheadBehindString() string {
	if s.AheadBy == 0 && s.BehindBy == 0 {
		return ""
	}
	return "+" + itoa(s.AheadBy) + "/-" + itoa(s.BehindBy)
}

// itoa converts an int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
