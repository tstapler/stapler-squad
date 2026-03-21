# Claude Code Hooks Integration

This guide explains how to integrate Claude Squad notifications with Claude Code using hooks. When configured, you'll receive audio chimes and visual notifications when Claude needs your attention.

## Quick Start

### Automatic Installation

Run the installation script:

```bash
# Interactive installation (will ask for scope)
./scripts/ssq-hooks-install

# Install globally (all Claude sessions)
./scripts/ssq-hooks-install --global

# Install for current project only
./scripts/ssq-hooks-install --project
```

### Manual Installation

Add the following to your `~/.claude/settings.json` (global) or `.claude/settings.json` (project):

```json
{
  "hooks": {
    "Notification": [{
      "hooks": [{
        "type": "command",
        "command": "/path/to/claude-squad/scripts/ssq-hook-handler notification"
      }]
    }],
    "Stop": [{
      "hooks": [{
        "type": "command",
        "command": "/path/to/claude-squad/scripts/ssq-hook-handler stop"
      }]
    }],
    "PermissionRequest": [{
      "hooks": [{
        "type": "command",
        "command": "/path/to/claude-squad/scripts/ssq-hook-handler permission"
      }]
    }],
    "PostToolUse": [{
      "matcher": ".*",
      "hooks": [{
        "type": "command",
        "command": "/path/to/claude-squad/scripts/ssq-hook-handler post-tool"
      }]
    }]
  }
}
```

Replace `/path/to/claude-squad` with your actual installation path.

## Hook Events

| Event | Notification Type | Priority | When Triggered |
|-------|------------------|----------|----------------|
| `Notification` | info | medium | Claude sends a notification message |
| `Stop` | task_complete/task_failed | low/high | Claude finishes a task |
| `PermissionRequest` | approval_needed | high | Claude needs your permission |
| `PostToolUse` | error | high | A tool execution fails |

## Configuration Options

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CS_SESSION_ID` | Override session ID detection | Auto-detected |
| `CS_HOOKS_DISABLED` | Set to "true" to disable all notifications | false |
| `CS_HOOKS_QUIET` | Set to "true" to suppress output | false |
| `CS_SERVER_PORT` | Claude Squad server port | 8484 |

### Customizing Individual Hooks

You can selectively enable hooks by only including the ones you want:

**Only task completion notifications:**
```json
{
  "hooks": {
    "Stop": [{
      "hooks": [{
        "type": "command",
        "command": "/path/to/ssq-hook-handler stop"
      }]
    }]
  }
}
```

**Only permission requests:**
```json
{
  "hooks": {
    "PermissionRequest": [{
      "hooks": [{
        "type": "command",
        "command": "/path/to/ssq-hook-handler permission"
      }]
    }]
  }
}
```

### Tool-Specific Hooks

Use the `matcher` field to filter PostToolUse hooks to specific tools:

**Only Bash command errors:**
```json
{
  "hooks": {
    "PostToolUse": [{
      "matcher": "Bash",
      "hooks": [{
        "type": "command",
        "command": "/path/to/ssq-hook-handler post-tool"
      }]
    }]
  }
}
```

**Multiple tool matchers (regex):**
```json
{
  "hooks": {
    "PostToolUse": [{
      "matcher": "Bash|Write|Edit",
      "hooks": [{
        "type": "command",
        "command": "/path/to/ssq-hook-handler post-tool"
      }]
    }]
  }
}
```

## Notification Priorities

The hook handler maps Claude Code events to notification priorities:

| Priority | Audio | Use Case |
|----------|-------|----------|
| `urgent` | 3 rapid beeps | Critical errors, blocking issues |
| `high` | Double ascending tone | Permission requests, tool errors |
| `medium` | Single soft chime | General notifications |
| `low` | Soft low tone | Task completions |

## Troubleshooting

### Notifications not appearing

1. **Check Claude Squad is running:**
   ```bash
   curl -s http://localhost:8484/health
   ```

2. **Verify hook handler is executable:**
   ```bash
   ls -la scripts/ssq-hook-handler
   chmod +x scripts/ssq-hook-handler
   ```

3. **Test hook handler manually:**
   ```bash
   echo '{"session_id": "test"}' | ./scripts/ssq-hook-handler stop
   ```

4. **Check hook is installed:**
   ```bash
   cat ~/.claude/settings.json | jq '.hooks'
   ```

### Hook errors blocking Claude

The hook handler is designed to always exit with code 0 to avoid blocking Claude. If you see issues:

1. **Check ssq-notify exists:**
   ```bash
   ls -la scripts/ssq-notify
   ```

2. **Temporarily disable hooks:**
   ```bash
   export CS_HOOKS_DISABLED=true
   ```

### Session ID not detected

The hook handler tries to detect the session ID from:
1. `CS_SESSION_ID` environment variable
2. Hook input JSON
3. Current tmux session name
4. Current directory name

Set `CS_SESSION_ID` explicitly if auto-detection fails:
```bash
export CS_SESSION_ID="my-session"
```

## Uninstalling

### Using the installer
```bash
./scripts/ssq-hooks-install --uninstall --global
# or
./scripts/ssq-hooks-install --uninstall --project
```

### Manual removal

Remove the `hooks` section from your settings.json, or delete the entire file if it only contains hooks.

## Advanced Usage

### Custom Hook Handler

Create your own hook handler to customize notification behavior:

```bash
#!/usr/bin/env bash
# my-custom-handler.sh

INPUT=$(cat)
HOOK_TYPE="$1"

case "$HOOK_TYPE" in
    stop)
        # Play custom sound on macOS
        afplay /System/Library/Sounds/Glass.aiff &
        # Also send to Claude Squad
        echo "$INPUT" | ssq-hook-handler stop
        ;;
    *)
        echo "$INPUT" | ssq-hook-handler "$HOOK_TYPE"
        ;;
esac
```

### Integration with Desktop Notifications

Combine with native desktop notifications:

```bash
#!/usr/bin/env bash
INPUT=$(cat)
TITLE=$(echo "$INPUT" | jq -r '.notification.title // "Claude"')
MESSAGE=$(echo "$INPUT" | jq -r '.notification.message // ""')

# macOS notification
osascript -e "display notification \"$MESSAGE\" with title \"$TITLE\""

# Also send to Claude Squad
echo "$INPUT" | ssq-hook-handler notification
```

## See Also

- [Claude Code Hooks Documentation](https://code.claude.com/docs/en/hooks)
- [ssq-notify Script](../scripts/ssq-notify) - Send notifications from command line
- [Notification Feature Plan](tasks/notification-chimes.md) - Full feature specification
