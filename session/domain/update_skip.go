package domain

// UpdateSkip names the child-table sections Update must leave alone because the
// row already holds exactly this data.
type UpdateSkip struct {
	DiffStats, Tags, Worktree, Claude bool
}
