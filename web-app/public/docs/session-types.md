# Session Types

stapler-squad supports four session creation modes. Each mode determines how the working directory is set up for the agent.

## Directory

Creates a session in an **existing directory** on your filesystem. No git worktree is created; the agent operates directly in the specified path.

Use this when:
- The project is not a Git repository
- You want the agent to work in the actual checked-out branch (not an isolated worktree)
- You are running a quick, non-destructive task

**Configuration**: Provide the absolute path to the directory. The agent starts immediately in that directory.

## New Worktree

Creates a **new git worktree** for a new branch, then starts the agent in it. This is the recommended mode for feature development.

Use this when:
- You want to start fresh work on a new branch
- You want the agent to have a fully isolated environment with no staged or uncommitted changes from your main checkout

**Configuration**: Provide the repository path and the new branch name. stapler-squad calls `git worktree add` to create the worktree directory, then launches the agent there.

## Existing Worktree

Attaches the agent to a **git worktree that already exists** on disk. Useful when you have already run `git worktree add` manually and want to start an agent in it.

Use this when:
- You have an existing worktree from a previous session
- You want to resume work without recreating the worktree

**Configuration**: Provide the path to the existing worktree directory.

## One-Off Session

Creates a **temporary, auto-named directory** under the configured one-off base directory (default: `~/.stapler-squad/one-off/`). The directory name is randomly generated (e.g., `swift-falcon-7`). No git repository is initialized.

Use this when:
- You want to test something quickly without setting up a project
- You need a clean scratch space for an ad-hoc task
- You will discard the results after the session ends

**Configuration**: No path required. stapler-squad generates a unique name and creates the directory automatically. The base directory can be changed in `~/.stapler-squad/config.json` via the `oneOffBaseDir` key.

## Choosing the Right Type

| Goal | Recommended type |
|---|---|
| New feature branch, full isolation | New Worktree |
| Fix a bug in an existing branch | Existing Worktree |
| Run agent in your main checkout | Directory |
| Quick scratch task, throwaway work | One-Off |
