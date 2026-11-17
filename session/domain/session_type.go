package domain

// SessionType is the session creation mode (directory, new_worktree, existing_worktree, etc.).
type SessionType string

const (
	// SessionTypeDefault uses the default behavior (directory session).
	SessionTypeDefault SessionType = ""
	// SessionTypeDirectory creates a simple directory session without a worktree.
	SessionTypeDirectory SessionType = "directory"
	// SessionTypeNewWorktree creates a new git worktree for the session.
	SessionTypeNewWorktree SessionType = "new_worktree"
	// SessionTypeExistingWorktree reuses an existing git worktree.
	SessionTypeExistingWorktree SessionType = "existing_worktree"
	// SessionTypeNewProject creates a new directory with a git repo and an initial
	// commit; the directory need not exist beforehand.
	SessionTypeNewProject SessionType = "new_project"
	// SessionTypeOneOff creates a temporary directory under OneOffBaseDir with a generated name.
	SessionTypeOneOff SessionType = "one_off"
)

// IsValid reports whether st is a recognized session type.
func (st SessionType) IsValid() bool {
	switch st {
	case SessionTypeDefault:
		return false
	case SessionTypeDirectory, SessionTypeNewWorktree, SessionTypeExistingWorktree,
		SessionTypeNewProject, SessionTypeOneOff:
		return true
	default:
		return false
	}
}
