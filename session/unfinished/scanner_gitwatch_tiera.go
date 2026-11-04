package unfinished

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// Tier A linked-gitdir watching: admin dirs, worktree targets and per-worktree gitdir registration.

// watchTierA watches the gitdirs HEAD/index changes land in for linked
// worktrees. It reports false when .git is neither a directory nor a pointer
// file, leaving the caller's plain Add to log the transient condition.
func (s *Scanner) watchTierA(w gitWatcher, repoPath, gitDir string) bool {
	fi, err := os.Lstat(gitDir)
	if err != nil {
		return false
	}
	if !fi.IsDir() {
		// Registered path is itself a linked worktree: watch its resolved
		// admin gitdir, not the pointer file.
		if admin, ok := linkedAdminDir(repoPath); ok {
			s.addWorktreeGitdir(w, admin, worktreeTarget{repoRoot: repoPath, worktree: repoPath})
		} else {
			log.Debug("not watching gitdir outside <commondir>/worktrees", "repo", repoPath)
		}
		return true
	}
	if err := w.Add(gitDir); err != nil {
		log.Debug("could not watch git dir", "dir", gitDir, "err", err)
	} else {
		s.armDirty(repoPath)
	}
	worktreesDir := filepath.Join(gitDir, "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return true
	}
	// Watches are not recursive: once worktrees/ exists, `git worktree add` and
	// `prune` change only its children, which emit nothing on <repo>/.git.
	if err := w.Add(worktreesDir); err != nil {
		log.Debug("could not watch worktrees dir", "dir", worktreesDir, "err", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		admin := filepath.Join(gitDir, "worktrees", e.Name())
		wt, ok := linkedWorktreePath(admin)
		if !ok {
			wt, ok = s.watchPendingAdmin(w, admin)
		}
		if !ok {
			continue
		}
		s.addWorktreeGitdir(w, admin, worktreeTarget{repoRoot: repoPath, worktree: wt})
	}
	return true
}

// linkedAdminDir resolves a linked worktree's admin gitdir from its .git pointer
// file, accepting it only as a direct child of <commondir>/worktrees (after
// Clean). A pointer to anywhere else (absolute elsewhere, or ".." climbing out)
// is never handed to fsnotify or armed.
func linkedAdminDir(repoPath string) (string, bool) {
	admin := worktreeGitDir(repoPath)
	if filepath.Dir(admin) != filepath.Join(gitCommonDir(repoPath), "worktrees") {
		return "", false
	}
	return admin, true
}

func (s *Scanner) addWorktreeGitdir(w gitWatcher, admin string, t worktreeTarget) {
	if err := w.Add(admin); err != nil {
		log.Debug("could not watch linked worktree gitdir", "dir", admin, "err", err)
		return
	}
	s.fsMu.Lock()
	if s.gitwatch.targets == nil {
		s.gitwatch.targets = make(map[string]worktreeTarget)
	}
	s.gitwatch.targets[admin] = t
	s.fsMu.Unlock()
	s.armDirty(t.worktree)
}

// linkedWorktreePath reads <admin>/gitdir, which holds "<worktree>/.git"
// (relative to admin when git wrote it with worktree.useRelativePaths).
func linkedWorktreePath(admin string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(admin, "gitdir")) // #nosec G304 -- admin is a subdirectory of a tracked repo's .git/worktrees
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		return "", false
	}
	return filepath.Dir(resolveGitPath(admin, line)), true
}

func (s *Scanner) worktreeTargetFor(admin string) (worktreeTarget, bool) {
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
	t, ok := s.gitwatch.targets[admin]
	return t, ok
}

// watchPendingAdmin covers `git worktree add`, which mkdirs the admin dir before
// writing <admin>/gitdir: it watches admin so that write re-runs registration,
// then re-reads gitdir in case the write landed before the watch did.
func (s *Scanner) watchPendingAdmin(w gitWatcher, admin string) (string, bool) {
	if err := w.Add(admin); err != nil {
		log.Debug("could not watch pending linked worktree gitdir", "dir", admin, "err", err)
		return "", false
	}
	return linkedWorktreePath(admin)
}
