package cli

import (
	"context"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

var _ backend.Backend = (*Backend)(nil)

func (b *Backend) ListWorktrees(ctx context.Context, loc backend.RepoLocation) ([]backend.WorktreeInfo, error) {
	res, err := b.git(ctx, loc, backend.OpListWorktrees, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreeList(string(res.out)), nil
}

// parseWorktreeList parses `worktree list --porcelain`: blank-line separated blocks of
// "key value" lines (bare, detached, locked and prunable carry no value or a reason).
func parseWorktreeList(text string) []backend.WorktreeInfo {
	var out []backend.WorktreeInfo
	var cur *backend.WorktreeInfo
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimRight(ln, "\r")
		if ln == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(ln, " ")
		if key == "worktree" {
			flush()
			cur = &backend.WorktreeInfo{Path: backend.WorktreePath(val)}
			continue
		}
		if cur == nil {
			continue
		}
		switch key {
		case "HEAD":
			cur.HeadSHA = backend.CommitSHA(val)
		case "branch":
			cur.Branch = backend.RefName(val)
		case "bare":
			cur.Bare = true
		case "detached":
			cur.Detached = true
		case "locked":
			cur.Locked = true
		case "prunable":
			cur.Prunable = true
		}
	}
	flush()
	return out
}

func (b *Backend) AddWorktree(ctx context.Context, loc backend.RepoLocation, req backend.AddWorktreeRequest) error {
	if err := optArg("path", string(req.Path)); err != nil {
		return err
	}
	if err := optArg("branch", string(req.Branch)); err != nil {
		return err
	}
	args := []string{"worktree", "add", "-b", string(req.Branch), "--", string(req.Path)}
	if req.Base != "" {
		if err := optArg("base", string(req.Base)); err != nil {
			return err
		}
		args = append(args, string(req.Base))
	}
	_, err := b.git(ctx, loc, backend.OpAddWorktree, args...)
	return err
}

func (b *Backend) AddWorktreeForExistingBranch(ctx context.Context, loc backend.RepoLocation, path backend.WorktreePath, br backend.BranchName) error {
	if err := optArg("path", string(path)); err != nil {
		return err
	}
	if err := optArg("branch", string(br)); err != nil {
		return err
	}
	_, err := b.git(ctx, loc, backend.OpAddWorktreeForExistingBranch, "worktree", "add", "--", string(path), string(br))
	return err
}

func (b *Backend) RemoveWorktree(ctx context.Context, loc backend.RepoLocation, req backend.RemoveWorktreeRequest) error {
	if err := optArg("path", string(req.Path)); err != nil {
		return err
	}
	args := []string{"worktree", "remove"}
	if req.Force {
		args = append(args, "--force")
	}
	args = append(args, "--", string(req.Path))
	_, err := b.git(ctx, loc, backend.OpRemoveWorktree, args...)
	return err
}

func (b *Backend) PruneWorktrees(ctx context.Context, loc backend.RepoLocation) error {
	_, err := b.git(ctx, loc, backend.OpPruneWorktrees, "worktree", "prune")
	return err
}
