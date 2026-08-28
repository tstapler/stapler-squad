---
name: prefer-go-git-over-subshells
description: Use when writing or reviewing Go code in stapler-squad that shells out to git — prefer github.com/go-git/go-git/v5 over safeexec.CommandContext("git", ...) wherever go-git can do the job, to avoid subprocess overhead, zombie-process risk, and text-parsed output.
---

# Prefer go-git Over Shelling Out

When a git operation can be done with `github.com/go-git/go-git/v5`, use it instead of shelling out via `safeexec.CommandContext("git", ...)`. Reduce subshells wherever a native Go integration exists.

## When to Use This Skill

- Writing new Go code that needs to inspect or mutate a git repository
- Reviewing a diff that adds a `safeexec.CommandContext(ctx, "git", ...)` call
- Deciding whether an existing `git` subshell can be replaced with a native go-git call

**Wrong:**
```go
func IsCommitOnMain(repoPath, mainBranch, sha string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "-C", repoPath, "merge-base", "--is-ancestor", sha, mainBranch)
	err := cmd.Run()
	// ...parse exit code...
}
```

**Right:**
```go
func IsCommitOnMain(repoPath, mainBranch, sha string) (bool, error) {
	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return false, fmt.Errorf("failed to open git repo at %s: %w", repoPath, err)
	}
	shaCommit, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return false, fmt.Errorf("failed to resolve commit %s: %w", sha, err)
	}
	mainCommit, err := repo.CommitObject(mainRef.Hash())
	if err != nil {
		return false, err
	}
	return shaCommit.IsAncestor(mainCommit)
}
```

See `session/git/ops.go`'s `IsCommitOnMain` for the full version (with an `origin/main` fallback for remotely-merged commits).

## When a subshell is still fine

`safeexec.CommandContext` is still the right tool when go-git genuinely can't do the job — e.g. `MergeMainIntoWorktree` (`session/git/ops.go`) shells out for the actual `git merge`/`git fetch` because go-git's merge support is limited, and any operation needing a credential helper for push/fetch against a real remote. The rule is "prefer go-git when it can do the job," not "never shell out."

`session/git/util.go`'s `getHeadCommitSHA` shows the hybrid pattern when go-git has a known limitation: try go-git first, and only fall back to the CLI (`getHeadCommitSHAViaCLI`) for a specific, documented failure mode (a torn-read race on ref files that the git CLI's atomic-rename ref updates don't hit). Don't fall back "just in case" — name the specific failure the fallback exists for.

## Why

Every `safeexec.CommandContext` call is a subprocess: fork/exec overhead, a zombie-process risk if not reaped correctly, and output that has to be parsed as text (exit codes, stdout/stderr) instead of returned as typed Go values. go-git is already a project dependency (`github.com/go-git/go-git/v5`, used throughout `session/git/`), so reaching for a subshell for something go-git already exposes (opening a repo, resolving a ref, checking commit ancestry, reading HEAD) adds a process boundary and a parsing step for no benefit.
