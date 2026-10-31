package cli

// SetDirectGitForTest pins the local git binary name; call the returned func to restore.
func SetDirectGitForTest(name string) (restore func()) {
	prev := directGit
	directGit = func() string { return name }
	return func() { directGit = prev }
}
