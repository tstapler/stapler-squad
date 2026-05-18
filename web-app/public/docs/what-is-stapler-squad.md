# What is stapler-squad

stapler-squad is a session manager for AI coding agents. It runs each agent (Claude Code, Aider, and others) in an isolated environment so multiple agents can work on the same codebase simultaneously without interfering with each other.

## Core Concepts

### Sessions

A **session** is a running instance of an AI coding agent. Each session has:

- A **name** — displayed in the session list and used for quick navigation
- A **working directory** — the file system path where the agent operates
- A **status** — running, paused, waiting for approval, or idle
- An **agent type** — Claude Code, Aider, or a custom program

Sessions are managed in isolated tmux windows so they persist independently of your browser or terminal connection. You can disconnect and reconnect at any time; the agent keeps running.

### Git Worktrees

By default, stapler-squad creates sessions in **git worktrees** — a Git feature that lets you check out multiple branches simultaneously in separate directories. Each worktree-based session gets its own directory under `~/.stapler-squad/worktrees/`, with the target branch checked out.

This means:
- Agent A can work on `feature/login` while Agent B works on `feature/dashboard`
- Neither agent sees the other's uncommitted changes
- You can run tests in one worktree while code is being generated in another

### The Agent Model

stapler-squad acts as an orchestration layer, not an agent itself. It:

1. Creates the isolated tmux session and git worktree
2. Launches the selected agent in that environment
3. Streams the terminal output back to your browser in real time
4. Monitors the session for approval requests, errors, and completion
5. Shows all sessions in a unified dashboard so you can switch between agents with a single keystroke

## Why Use stapler-squad?

Without stapler-squad, running multiple AI agents requires manually managing tmux sessions, git worktrees, and terminal windows. stapler-squad automates all of that so you can focus on reviewing the agents' output and approving changes.

See [Session Types](session-types) for details on the different kinds of sessions you can create.
