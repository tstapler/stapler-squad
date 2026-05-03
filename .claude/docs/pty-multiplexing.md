# External Session Monitoring (PTY Multiplexing)

Monitor and interact with Claude sessions running in external terminals (IntelliJ, VS Code, etc.) through the `claude-mux` PTY multiplexer. Enables bidirectional terminal streaming without requiring sessions to be started from stapler-squad.

## Architecture

```
External Terminal (IntelliJ) → claude-mux → PTY → Claude Process
                                    ↓
                            Unix Socket (/tmp/claude-mux-<PID>.sock)
                                    ↓
                          stapler-squad (auto-discovers & connects)
```

## Installation

```bash
cd /path/to/stapler-squad
./scripts/install-mux.sh
# Installs to ~/.local/bin/claude-mux
```

## Setup Methods

### Method 1: Shell Alias (Recommended)

```bash
# Add to ~/.zshrc or ~/.bashrc
alias claude='claude-mux claude'
source ~/.zshrc
```

### Method 2: PATH Override

```bash
# Create wrapper script at ~/bin/claude
#!/bin/bash
exec claude-mux /usr/local/bin/claude "$@"
chmod +x ~/bin/claude
export PATH="$HOME/bin:$PATH"
```

### Method 3: IDE Terminal Configuration

**IntelliJ IDEA / PyCharm / WebStorm:**
1. Settings → Tools → Terminal
2. Shell path: `~/.local/bin/claude-mux`
3. Shell arguments: `claude`

**VS Code:**
```json
"terminal.integrated.profiles.osx": {
  "claude-mux": {
    "path": "~/.local/bin/claude-mux",
    "args": ["claude"]
  }
}
```

## Session Discovery

Uses `fsnotify` to watch `/tmp/` for socket creation/deletion — immediate detection, no polling. Falls back to polling automatically.

```bash
# Verify
ls /tmp/claude-mux-*.sock
```

## Troubleshooting

| Issue | Fix |
|---|---|
| `claude-mux: command not found` | `export PATH="$HOME/.local/bin:$PATH"` |
| `stdin is not a terminal` | Must run from real terminal, not script/pipe |
| Sessions not discovered | Check socket exists, verify permissions (0600), check logs at `~/.stapler-squad/logs/stapler-squad.log` |
| Terminal output garbled | Resize the terminal window (SIGWINCH forwarded automatically) |
| Stale sockets | `rm /tmp/claude-mux-*.sock` (when no sessions running) |

## Protocol Details

**Message Types:** Output, Input, Resize (SIGWINCH), Metadata (command/PID/cwd/env), Ping/Pong

**Security:** Sockets created with 0600 permissions, local Unix domain sockets only (no network exposure).

**Performance:** Zero overhead when no clients connected; direct PTY forwarding; automatic cleanup on process exit.
