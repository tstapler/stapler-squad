# Omnibar

The omnibar is the central command interface in stapler-squad. Open it with **⌘K** (macOS) or **Ctrl+K** (Linux/Windows) from anywhere in the app.

## What the Omnibar Does

The omnibar serves two purposes:

1. **Navigate** to an existing session by typing part of its name, branch, or path
2. **Create** a new session from a GitHub URL, a local path, or the creation form

## Auto-Detection Patterns

As you type, the omnibar automatically detects the type of input and shows a preview of what will happen when you press Enter.

### GitHub Pull Request URL

**Pattern**: `https://github.com/<owner>/<repo>/pull/<number>`

**Result**: Creates a new worktree session for the PR's branch. stapler-squad fetches the branch name from the GitHub URL, adds a worktree for it, and starts the agent there.

**Example**: `https://github.com/acme/api/pull/42`

### GitHub Branch URL

**Pattern**: `https://github.com/<owner>/<repo>/tree/<branch>`

**Result**: Creates a session for the specified branch in the corresponding local repository (if it exists).

### GitHub Repository URL

**Pattern**: `https://github.com/<owner>/<repo>`

**Result**: Creates a session at the root of the corresponding local repository.

### Path with Branch

**Pattern**: `/path/to/repo:branch-name`

**Result**: Creates a new worktree for `branch-name` in the repository at `/path/to/repo`. The colon separator is the key indicator — everything before `:` is the repository path, everything after is the branch name.

**Example**: `~/code/myproject:fix/typo`

### Local Path

**Pattern**: `/absolute/path` or `~/path`

**Result**: Creates a directory session at the specified path. If the path is a git repository, a worktree session is offered instead.

### Session Search (fallback)

If the input does not match any of the patterns above, the omnibar switches to **session search mode**: it searches existing session names, branches, and paths using fuzzy matching and displays matching sessions as navigation results.

## Creation Form

Click the **"+"** button or press **Tab** in the omnibar to open the full session creation form. The form lets you choose the session type (Directory / New Worktree / Existing Worktree / One-Off), configure the working directory, select an agent program, and set a custom session name.

The form is also accessible from the **"new:"** shorthand: type `new:/path/to/project` in the omnibar to pre-fill the path and jump directly to the creation form.

## Keyboard Navigation

- **↑ / ↓** — move between suggestions
- **Enter** — confirm the selected action
- **Escape** — close the omnibar without taking action
- **Tab** — switch to creation form
