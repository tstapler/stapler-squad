# tmux Integration

stapler-squad uses **tmux** as its session backend. Every agent session runs inside a tmux window, which means sessions persist independently of your browser connection and can be managed from the terminal as well as the web UI.

## How Sessions Map to tmux

Each stapler-squad session corresponds to a **tmux window** inside a single tmux server. The server name is `stapler-squad` (or `stapler-squad-<instance>` for named instances).

You can inspect the tmux sessions directly:

```bash
tmux ls -t stapler-squad
tmux attach -t stapler-squad
```

Inside the tmux server, each window is named after the stapler-squad session ID. The agent process runs in that window.

## Control Mode vs. Polling Mode

stapler-squad supports two methods for monitoring tmux output:

### Control Mode (default)

tmux's **control mode** (`tmux -C`) provides a real-time event stream: stapler-squad receives structured events whenever output is written to a window, instead of polling. This reduces latency and CPU usage significantly.

Control mode is enabled by default. To disable it:

```bash
STAPLER_SQUAD_USE_CONTROL_MODE=false ./stapler-squad
```

Use polling mode if you encounter tmux compatibility issues or are running an older version of tmux (control mode requires tmux 2.6+).

### Polling Mode

In polling mode, stapler-squad reads the tmux pane content on a fixed interval. This is less efficient but works with any tmux version.

## `--tmux-keep-server`

By default, when the last session is deleted, stapler-squad kills the tmux server to clean up. Pass `--tmux-keep-server` to prevent this:

```bash
./stapler-squad --tmux-keep-server
```

This is useful when:
- You are running e2e tests that need the tmux server to persist between test cases
- You want to inspect tmux state after all sessions have been removed

## Session Isolation

Each session is isolated at two levels:

1. **tmux level**: separate windows, separate process trees
2. **filesystem level**: separate git worktrees (for worktree-based sessions)

Sessions cannot see each other's environment variables, terminal state, or working directory contents unless they are explicitly sharing a directory.

## Bundled tmux

For single-binary deployment without requiring a system tmux install, stapler-squad can bundle a pinned version of tmux (3.4):

```bash
git submodule update --init third_party/tmux
make build-tmux
make build-embedded
```

The embedded binary is self-contained — no system tmux required. See the `Makefile` for the full build process.

## Scrollback

stapler-squad captures tmux scrollback and stores it per-session. The terminal view in the browser shows the full history, not just the visible pane. Scrollback is truncated at a configurable line limit to prevent unbounded memory growth.
