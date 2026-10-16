package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

func (b *Backend) ListBranches(ctx context.Context, loc backend.RepoLocation, req backend.ListBranchesRequest) ([]backend.BranchInfo, error) {
	args := []string{"branch"}
	if req.IncludeRemote {
		args = append(args, "-a")
	}
	if req.Contains != "" {
		if err := optArg("contains", string(req.Contains)); err != nil {
			return nil, err
		}
		args = append(args, "--contains", string(req.Contains))
	}
	args = append(args, "--format=%(refname:lstrip=2)%1f%(objectname:short)%1f%(upstream:lstrip=2)")
	res, err := b.git(ctx, loc, backend.OpListBranches, args...)
	if err != nil {
		return nil, err
	}
	var out []backend.BranchInfo
	for _, ln := range nonEmptyLines(res.text()) {
		f := strings.Split(ln, logSep)
		if len(f) != 3 || strings.HasPrefix(f[0], "(") { // "(HEAD detached at ...)" pseudo entry
			continue
		}
		out = append(out, backend.BranchInfo{Name: backend.BranchName(f[0]), ShortSHA: f[1], Upstream: backend.RefName(f[2])})
	}
	return out, nil
}

func (b *Backend) CreateBranch(ctx context.Context, loc backend.RepoLocation, req backend.CreateBranchRequest) error {
	if err := optArg("name", string(req.Name)); err != nil {
		return err
	}
	args := []string{"branch", string(req.Name)}
	if req.Base != "" {
		if err := optArg("base", string(req.Base)); err != nil {
			return err
		}
		args = append(args, string(req.Base))
	}
	_, err := b.git(ctx, loc, backend.OpCreateBranch, args...)
	return err
}

func (b *Backend) RenameCurrentBranch(ctx context.Context, loc backend.RepoLocation, to backend.BranchName) error {
	if err := optArg("branch", string(to)); err != nil {
		return err
	}
	_, err := b.git(ctx, loc, backend.OpRenameCurrentBranch, "branch", "-m", string(to))
	return err
}

// SwitchBranch uses `git switch`, not `git checkout`: checkout falls back to treating its
// argument as a path, so Branch "." or a file name would silently discard uncommitted changes.
// switch only ever takes a branch.
func (b *Backend) SwitchBranch(ctx context.Context, loc backend.RepoLocation, req backend.SwitchRequest) error {
	if err := optArg("branch", string(req.Branch)); err != nil {
		return err
	}
	args := []string{"switch"}
	if req.Create {
		args = append(args, "-c")
	}
	args = append(args, string(req.Branch))
	if req.Create && req.Base != "" {
		if err := optArg("base", string(req.Base)); err != nil {
			return err
		}
		args = append(args, string(req.Base))
	}
	_, err := b.git(ctx, loc, backend.OpSwitchBranch, args...)
	return err
}

func (b *Backend) CheckoutCommit(ctx context.Context, loc backend.RepoLocation, sha backend.CommitSHA) error {
	if err := optArg("sha", string(sha)); err != nil {
		return err
	}
	_, err := b.git(ctx, loc, backend.OpCheckoutCommit, "switch", "--detach", string(sha))
	return err
}

func (b *Backend) DeleteBranch(ctx context.Context, loc backend.RepoLocation, req backend.DeleteBranchRequest) error {
	if err := optArg("name", string(req.Name)); err != nil {
		return err
	}
	flag := "-d"
	if req.Force {
		flag = "-D"
	}
	_, err := b.git(ctx, loc, backend.OpDeleteBranch, "branch", flag, string(req.Name))
	return err
}

func (b *Backend) SetUpstream(ctx context.Context, loc backend.RepoLocation, req backend.SetUpstreamRequest) error {
	if err := optArg("branch", string(req.Branch)); err != nil {
		return err
	}
	if err := optArg("upstream", string(req.Upstream)); err != nil {
		return err
	}
	_, err := b.git(ctx, loc, backend.OpSetUpstream, "branch", "--set-upstream-to="+string(req.Upstream), string(req.Branch))
	return err
}

// DiscardChanges mirrors vcs.GitClient.AbandonChanges: unstage (error ignored, an unborn HEAD
// cannot be reset), restore tracked files, remove untracked files and directories.
func (b *Backend) DiscardChanges(ctx context.Context, loc backend.RepoLocation) error {
	t, err := b.resolve(loc)
	if err != nil {
		return err
	}
	_, _ = t.git(ctx, backend.OpDiscardChanges, "reset", "HEAD")
	if _, err := t.git(ctx, backend.OpDiscardChanges, "checkout", "--", "."); err != nil {
		return err
	}
	_, err = t.git(ctx, backend.OpDiscardChanges, "clean", "-fd")
	return err
}

func (b *Backend) StashPush(ctx context.Context, loc backend.RepoLocation, req backend.StashRequest) error {
	args := []string{"stash", "push"}
	if req.Message != "" {
		args = append(args, "-m", req.Message)
	}
	_, err := b.git(ctx, loc, backend.OpStashPush, args...)
	return err
}

func (b *Backend) StashPop(ctx context.Context, loc backend.RepoLocation) error {
	_, err := b.git(ctx, loc, backend.OpStashPop, "stash", "pop")
	return err
}

func (b *Backend) Add(ctx context.Context, loc backend.RepoLocation, req backend.AddRequest) error {
	if !req.All && len(req.Paths) == 0 {
		return fmt.Errorf("%w: add needs All or at least one path", backend.ErrInvalidArgument)
	}
	args := []string{"add"}
	if req.All {
		args = append(args, "-A")
	}
	if len(req.Paths) > 0 {
		args = append(args, "--")
		args = append(args, paths(req.Paths)...)
	}
	_, err := b.git(ctx, loc, backend.OpAdd, args...)
	return err
}

func (b *Backend) Restore(ctx context.Context, loc backend.RepoLocation, req backend.RestoreRequest) error {
	if !req.Staged && !req.Worktree {
		return fmt.Errorf("%w: restore needs Staged and/or Worktree", backend.ErrInvalidArgument)
	}
	if len(req.Paths) == 0 {
		return fmt.Errorf("%w: restore needs at least one path", backend.ErrInvalidArgument)
	}
	args := []string{"restore"}
	if req.Staged {
		args = append(args, "--staged")
	}
	if req.Worktree {
		args = append(args, "--worktree")
	}
	args = append(args, "--")
	args = append(args, paths(req.Paths)...)
	_, err := b.git(ctx, loc, backend.OpRestore, args...)
	return err
}

func (b *Backend) Reset(ctx context.Context, loc backend.RepoLocation, req backend.ResetRequest) error {
	if req.Mode > backend.ResetHard {
		return fmt.Errorf("%w: unknown reset mode %d", backend.ErrInvalidArgument, req.Mode)
	}
	target := "HEAD"
	if req.Target != "" {
		if err := optArg("target", string(req.Target)); err != nil {
			return err
		}
		target = string(req.Target)
	}
	modeFlag := [...]string{"--mixed", "--soft", "--hard"}[req.Mode]
	_, err := b.git(ctx, loc, backend.OpReset, "reset", modeFlag, target)
	return err
}

func (b *Backend) RemoveFiles(ctx context.Context, loc backend.RepoLocation, req backend.RemoveFilesRequest) error {
	if len(req.Paths) == 0 {
		return fmt.Errorf("%w: rm needs at least one path", backend.ErrInvalidArgument)
	}
	args := []string{"rm"}
	if req.Cached {
		args = append(args, "--cached")
	}
	if req.Recursive {
		args = append(args, "-r")
	}
	if req.Force {
		args = append(args, "-f")
	}
	args = append(args, "--")
	args = append(args, paths(req.Paths)...)
	_, err := b.git(ctx, loc, backend.OpRemoveFiles, args...)
	return err
}

func (b *Backend) MoveFile(ctx context.Context, loc backend.RepoLocation, req backend.MoveFileRequest) error {
	if req.From == "" || req.To == "" {
		return fmt.Errorf("%w: mv needs a source and a destination", backend.ErrInvalidArgument)
	}
	_, err := b.git(ctx, loc, backend.OpMoveFile, "mv", "--", string(req.From), string(req.To))
	return err
}

func (b *Backend) Commit(ctx context.Context, loc backend.RepoLocation, req backend.CommitRequest) error {
	args := []string{"commit"}
	switch {
	case req.Amend && req.Message == "":
		args = append(args, "--amend", "--no-edit")
	case req.Amend:
		args = append(args, "--amend", "-m", req.Message)
	case req.Message == "":
		return fmt.Errorf("%w: commit message is empty", backend.ErrInvalidArgument)
	default:
		args = append(args, "-m", req.Message)
	}
	_, err := b.git(ctx, loc, backend.OpCommit, args...)
	return err
}

func (b *Backend) Fetch(ctx context.Context, loc backend.RepoLocation, req backend.FetchRequest) error {
	args := []string{"fetch"}
	if req.All {
		args = append(args, "--all")
	}
	if req.Prune {
		args = append(args, "--prune")
	}
	if req.Remote != "" {
		if err := optArg("remote", string(req.Remote)); err != nil {
			return err
		}
		args = append(args, string(req.Remote))
	}
	if req.Branch != "" {
		if req.Remote == "" {
			return fmt.Errorf("%w: fetch of a branch needs a remote", backend.ErrInvalidArgument)
		}
		if err := refspecArg("branch", string(req.Branch)); err != nil {
			return err
		}
		args = append(args, "--", string(req.Branch))
	}
	_, err := b.git(ctx, loc, backend.OpFetch, args...)
	return err
}

// remoteBranchArgs appends the optional `<remote> [<branch>]` tail shared by push and pull.
func remoteBranchArgs(args []string, remote backend.RemoteName, branch backend.BranchName) ([]string, error) {
	if remote == "" {
		if branch != "" {
			return nil, fmt.Errorf("%w: a branch needs a remote", backend.ErrInvalidArgument)
		}
		return args, nil
	}
	if err := optArg("remote", string(remote)); err != nil {
		return nil, err
	}
	args = append(args, string(remote))
	if branch != "" {
		if err := refspecArg("branch", string(branch)); err != nil {
			return nil, err
		}
		args = append(args, string(branch))
	}
	return args, nil
}

func (b *Backend) Pull(ctx context.Context, loc backend.RepoLocation, req backend.PullRequest) error {
	args, err := remoteBranchArgs([]string{"pull"}, req.Remote, req.Branch)
	if err != nil {
		return err
	}
	_, err = b.git(ctx, loc, backend.OpPull, args...)
	return err
}

func (b *Backend) Push(ctx context.Context, loc backend.RepoLocation, req backend.PushRequest) error {
	args := []string{"push"}
	if req.Force {
		args = append(args, "--force")
	}
	if req.SetUpstream {
		args = append(args, "--set-upstream")
	}
	args, err := remoteBranchArgs(args, req.Remote, req.Branch)
	if err != nil {
		return err
	}
	_, err = b.git(ctx, loc, backend.OpPush, args...)
	return err
}

// Clone runs outside any repository, so it executes in the runner's default directory and
// passes the destination (the location's own directory) as an argument.
func (b *Backend) Clone(ctx context.Context, loc backend.RepoLocation, req backend.CloneRequest) error {
	if err := optArg("url", string(req.URL)); err != nil {
		return err
	}
	t, err := b.resolve(loc)
	if err != nil {
		return err
	}
	dest := t.dir
	t.dir = ""
	_, err = t.git(ctx, backend.OpClone, "clone", "--", string(req.URL), dest)
	return err
}

func (b *Backend) ListRemote(ctx context.Context, loc backend.RepoLocation, req backend.ListRemoteRequest) ([]backend.RemoteRef, error) {
	if err := optArg("remote", string(req.Remote)); err != nil {
		return nil, err
	}
	args := []string{"ls-remote"}
	if req.HeadsOnly {
		args = append(args, "--heads")
	}
	if req.TagsOnly {
		args = append(args, "--tags")
	}
	args = append(args, string(req.Remote))
	if req.Pattern != "" {
		if err := optArg("pattern", string(req.Pattern)); err != nil {
			return nil, err
		}
		args = append(args, string(req.Pattern))
	}
	res, err := b.git(ctx, loc, backend.OpListRemote, args...)
	if err != nil {
		return nil, err
	}
	var out []backend.RemoteRef
	for _, ln := range nonEmptyLines(res.text()) {
		sha, name, ok := strings.Cut(ln, "\t")
		if !ok {
			return nil, fmt.Errorf("git ls-remote: malformed line %q", ln)
		}
		out = append(out, backend.RemoteRef{SHA: backend.CommitSHA(sha), Name: backend.RefName(name)})
	}
	return out, nil
}
