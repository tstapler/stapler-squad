package unfinished

// statPassReader is implemented by readers that can hand the tracked-file
// lstat results of HasUncommitted to DiffShortstat within one scan (Story
// 4.2.4); GoGitVCSReader does so under scan_skip.
type statPassReader interface {
	HasUncommittedPass(worktreePath string) (bool, *statPass, error)
	DiffShortstatPass(worktreePath string, pass *statPass) (DiffStat, error)
}

// hasUncommitted is the scan's HasUncommitted call; the returned pass (nil for
// readers without the capability) goes to diffShortstat in the same scan.
func (s *Scanner) hasUncommitted(worktreePath string) (bool, *statPass, error) {
	if r, ok := s.reader.(statPassReader); ok {
		return r.HasUncommittedPass(worktreePath)
	}
	dirty, err := s.reader.HasUncommitted(worktreePath)
	return dirty, nil, err
}

func (s *Scanner) diffShortstat(worktreePath string, pass *statPass) (DiffStat, error) {
	if r, ok := s.reader.(statPassReader); ok {
		return r.DiffShortstatPass(worktreePath, pass)
	}
	return s.reader.DiffShortstat(worktreePath)
}
