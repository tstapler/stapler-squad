# gh pr merge — Always Use --repo

Always pass `--repo owner/repo` when running `gh pr merge`. Never run it without this flag.

**Wrong:**
```bash
gh pr merge 45 --squash --delete-branch --admin
```

**Right:**
```bash
gh pr merge 45 --squash --delete-branch --admin --repo fanatics-gaming/engineering-score-cards
```

## Why

Stapler Squad manages git worktrees. The `main` branch is typically checked out in a worktree at `~/.stapler-squad/repos/<org>/<repo>`, which causes `gh pr merge` (without `--repo`) to fail with:

```
fatal: 'main' is already used by worktree at '/Users/.../.stapler-squad/repos/...'
```

The `--repo` flag forces `gh` to target the remote directly, bypassing the local worktree conflict.
