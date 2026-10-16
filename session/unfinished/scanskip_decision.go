package unfinished

// scanSkipDecision tells one scan whether cached state may be trusted. force
// bypasses the parsed-index and untracked-dir caches (they are refreshed from
// disk); done must be called once the forced walk completed.
func (g *GoGitVCSReader) scanSkipDecision(worktreePath string) (force bool, done func()) {
	return false, func() {}
}
