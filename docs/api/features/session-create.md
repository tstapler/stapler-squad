# Create Session

**ID**: `session-create`  
**Status**: stable  
**Since**: v1.0.0

Creates a new AI agent session in a directory, worktree, or one-off mode.

## RPCs

- `session:create`

## Components

- `components/sessions/Omnibar.tsx`
- `components/sessions/OmnibarCreationPanel.tsx`

## Tests

- Session Lifecycle > e2e:session-create - Session create UI is accessible
- Session Create Directory > creates a session in a directory
- One-Off Session > creates a one-off session
