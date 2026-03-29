# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Build and Run
```bash
# Build the application
go build .

# Run the application (web server mode on localhost:8543)
./stapler-squad

# Web development workflow
make restart-web    # Build web UI and restart server (ALWAYS use this)
                    # NOTE: Do NOT prefix with "pkill -9 -f stapler-squad" - the Makefile already handles stopping gracefully
                    # IMPORTANT: Do NOT pipe or redirect make restart-web output (e.g. | tail -20) as it will block forever.
                    #            Run it plain: make restart-web

# Enable profiling for web server (to diagnose lock-ups)
make restart-web-profile  # Restart with --profile --trace enabled

# Or enable profiling with regular restart:
make restart-web PROFILE_FLAGS="--profile --trace"

# Custom profiling port:
make restart-web-profile PROFILE_PORT=8080

# Auto-rebuild on file changes (recommended for development)
# Install fswatch if not available: brew install fswatch
fswatch -o web-app/src | xargs -n1 -I{} make restart-web

# Auto-rebuild with profiling enabled:
fswatch -o web-app/src | xargs -n1 -I{} make restart-web PROFILE_FLAGS="--profile"

# Debug menu controls (available in web UI)
# - Click 🛠️ button in header to access debug menu
# - Toggle "Terminal Stream Logging" to enable/disable verbose terminal output
# - Or use console: localStorage.setItem('debug-terminal', 'true')

# Terminal Streaming Configuration
# Control mode streaming (tmux -C) is enabled by default for better performance
# - Provides real-time notifications via tmux native protocol
# - Combines full history (capture-pane) + real-time updates (control mode)
# To disable and use legacy capture-pane polling:
STAPLER_SQUAD_USE_CONTROL_MODE=false ./stapler-squad
```

### Bazel Build (Modern Build System)

The project also supports Bazel for building. This is optional - the Makefile-based build is still the primary method.

```bash
# Install Bazelisk (recommended)
brew install bazelisk

# Full build with Bazel (web UI + Go backend)
make bazel-all

# Build just the Go backend (requires web UI built first)
make bazel-build

# Build and run with Bazel
make bazel-run

# Run tests with Bazel
make bazel-test

# Clean Bazel cache
make bazel-clean

# Update Bazel dependencies (run when adding new Go deps)
make bazel-update-deps

# Or manually:
bazel build //:stapler-squad
bazel test //...
```

### Profiling and Debugging Lock-Ups
```bash
# Quick diagnosis for lock-ups/freezes
./stapler-squad --profile --trace

# When lock-up occurs (in another terminal):
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > goroutines.txt
curl http://localhost:6060/debug/pprof/block?debug=1 > block.txt
curl http://localhost:6060/debug/pprof/mutex?debug=1 > mutex.txt

# After exiting, analyze trace:
go tool trace /tmp/stapler-squad-trace-<PID>.out

# Profile specific aspects:
./stapler-squad --profile                    # Enable profiling HTTP server
./stapler-squad --profile --profile-port 8080  # Custom port
./stapler-squad --trace                      # Execution tracing only

# CPU profiling (30 seconds)
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof -http=:8081 cpu.prof

# Memory profiling
curl http://localhost:6060/debug/pprof/heap > heap.prof
go tool pprof -http=:8081 heap.prof

# Race detection (for data races)
go build -race .
./stapler-squad --profile

# See docs/PROFILING.md for comprehensive guide
```

### OpenTelemetry Observability

Stapler Squad supports OpenTelemetry instrumentation for APM integration (Datadog, etc.).

```bash
# Enable telemetry (disabled by default)
OTEL_ENABLED=true ./stapler-squad

# Or use Datadog-specific flag
DD_TRACE_ENABLED=true ./stapler-squad

# Configure OTLP endpoint (default: localhost:4317)
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 OTEL_ENABLED=true ./stapler-squad

# Set environment and version for trace metadata
OTEL_SERVICE_ENVIRONMENT=production OTEL_SERVICE_VERSION=1.0.0 OTEL_ENABLED=true ./stapler-squad
```

**Datadog Agent Configuration** (for OTLP ingestion):
```yaml
# /etc/datadog-agent/datadog.yaml
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
```

**Instrumented Operations:**
- All HTTP requests (via otelhttp middleware)
- All ConnectRPC endpoints (via otelconnect interceptor)
- History cache operations (cache hit/miss, load duration)
- Search engine operations (sync, search duration, result count)

**Trace Attributes:**
- `session.id`, `session.title`, `session.status` - Session context
- `history.entry_count` - History loading metrics
- `search.query`, `search.result_count`, `search.duration_ms` - Search metrics
- `cache.hit`, `cache.refresh_duration_ms` - Cache performance
- `sync.sessions_added`, `sync.sessions_updated` - Index sync metrics

### Testing
```bash
# Run all tests
go test ./...

# Run tests for specific packages
go test ./ui
go test ./app
go test ./session

# Run with coverage
go test -cover ./...

# Run specific test
go test ./ui -run TestSpecificFunction

# Run benchmarks (performance tests)
# CRITICAL: Benchmarks take 5-30 minutes and MUST be run in background with &
# Do NOT run benchmarks without & as they will block your terminal
go test -bench=. -benchmem ./app -timeout=30m &

# Run specific benchmark categories
go test -bench=BenchmarkNavigation -benchmem ./app -timeout=10m &
go test -bench=BenchmarkInstanceChangedComponents -benchmem ./app -timeout=10m &
go test -bench=BenchmarkListRendering -benchmem ./app -timeout=10m &

# New comprehensive performance benchmarks
go test -bench=BenchmarkLargeSessionNavigation -benchmem ./app -timeout=20m &
go test -bench=BenchmarkAttachDetachPerformance -benchmem ./app -timeout=15m &
go test -bench=BenchmarkFilteringPerformance -benchmem ./app -timeout=10m &
go test -bench=BenchmarkCategoryOrganization -benchmem ./app -timeout=10m &
go test -bench=BenchmarkRenderingPerformance -benchmem ./app -timeout=15m &
go test -bench=BenchmarkMemoryUsage -benchmem ./app -timeout=15m &
go test -bench=BenchmarkStartupPerformance -benchmem ./app -timeout=10m &
go test -bench=BenchmarkRealtimeUpdates -benchmem ./app -timeout=10m &

# Git integration and contextual discovery benchmarks
go test -bench=BenchmarkGitRepositoryDiscovery -benchmem ./ui/overlay -timeout=5m &
go test -bench=BenchmarkContextualDiscovery -benchmem ./ui/overlay -timeout=5m &

# Path validation benchmarks
go test -bench=BenchmarkValidatePath -benchmem ./ui/overlay -timeout=2m &

# Profile benchmarks for detailed performance analysis
go test -bench=BenchmarkLargeSessionNavigation -benchmem -cpuprofile=cpu.prof ./app -timeout=20m
go tool pprof cpu.prof

# Memory profiling for large session counts
go test -bench=BenchmarkMemoryUsage -benchmem -memprofile=mem.prof ./app -timeout=15m
go tool pprof mem.prof

# Trace profiling for detailed execution analysis
go test -bench=BenchmarkAttachDetachPerformance -trace=trace.out ./app -timeout=15m
go tool trace trace.out
```


### Code Quality and Analysis

**Using Makefile (Recommended):**
```bash
# Install all development tools
make install-tools

# Quick development validation
make quick-check

# Comprehensive analysis (all static analysis tools)
make analyze

# Individual analysis tools
make nil-safety     # Comprehensive nil safety analysis
make staticcheck    # Advanced static analysis
make security       # Security vulnerability scanning
make lint          # Comprehensive linting

# Pre-commit validation
make pre-commit

# Dead code detection
make deadcode       # Find unreachable code
deadcode -test ./...  # Include test files in analysis
```

**Manual Commands:**
```bash
# Check for issues
go vet ./...

# Format code
go fmt ./...

# Build and check for compilation errors
go build .
```

### Nil Safety Analysis

Stapler Squad includes comprehensive nil safety analysis tools to prevent panic-causing nil pointer dereferences:

```bash
# Run all nil safety tools
make nil-safety

# Individual tools
make nilaway        # Advanced nil flow analysis (Uber)
go vet -nilness ./... # Built-in Go nilness analyzer
go-nilcheck ./...   # Function pointer validation
```

**Required Tools Installation:**
```bash
make install-tools
# Or manually:
go install go.uber.org/nilaway/cmd/nilaway@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
```

**Nil Safety Best Practices:**
1. Always run `make nil-safety` before committing code
2. Use NilAway for the most comprehensive nil flow analysis
3. Include nil checks before pointer dereferences
4. Use defensive programming in overlay rendering (see app/app.go:1225)

### Static Analysis Tools

**Comprehensive Toolchain:**
- **NilAway** - Advanced nil pointer safety analysis (Uber)
- **Staticcheck** - Production-grade static analyzer  
- **golangci-lint** - Meta-linter with multiple analyzers
- **gosec** - Security-focused static analysis
- **go vet** - Built-in Go static analysis

## Application Data Directory

Stapler Squad stores application data, logs, and git worktrees in the `~/.stapler-squad` directory:

```
~/.stapler-squad/
├── logs/                    # Application logs for debugging
│   ├── stapler-squad.log     # Main application log
│   └── debug.log           # Detailed debug information
├── worktrees/              # Git worktrees for isolated sessions
│   ├── session-name_hash/  # Individual worktree directories
│   └── ...
├── config.json            # Application configuration
└── sessions.json          # Session state persistence
```

**Debugging Session Creation Issues:**
- Check `~/.stapler-squad/logs/stapler-squad.log` for session creation attempts
- Look for tmux command executions, git operations, and timeout messages
- Debug logs show detailed command execution and timing information

**Key Log Patterns to Look For:**
- `Starting tmux session` - Session creation initiation
- `timed out waiting for tmux session` - Session creation hangs
- `which claude` - External command execution that may block
- `git worktree` operations - Git integration issues
- `DoesSessionExist()` polling - Session existence checking loops

## External Session Monitoring (PTY Multiplexing)

Stapler Squad can monitor and interact with Claude sessions running in external terminals (IntelliJ, VS Code, etc.) through the `claude-mux` PTY multiplexer. This enables bidirectional terminal streaming without requiring sessions to be started from stapler-squad.

### How It Works

The `claude-mux` wrapper creates a pseudo-terminal (PTY) that wraps your Claude command and exposes it via a Unix domain socket at `/tmp/claude-mux-<PID>.sock`. Stapler Squad automatically discovers these sockets and connects to them for real-time terminal streaming.

**Architecture:**
```
External Terminal (IntelliJ) → claude-mux → PTY → Claude Process
                                    ↓
                            Unix Socket (/tmp/)
                                    ↓
                          stapler-squad (discovers & connects)
```

### Installation

Run the installation script from the project root:

```bash
cd /path/to/stapler-squad
./scripts/install-mux.sh
```

This will:
- Build `claude-mux` from source
- Install to `~/.local/bin/claude-mux`
- Print setup instructions for shell aliases and IDE configuration
- Show verification and troubleshooting steps

### Setup Methods

**Method 1: Shell Alias (Recommended)**
```bash
# Add to ~/.zshrc or ~/.bashrc
alias claude='claude-mux claude'

# Reload shell config
source ~/.zshrc
```

**Method 2: PATH Override**
```bash
# Create wrapper script at ~/bin/claude
#!/bin/bash
exec claude-mux /usr/local/bin/claude "$@"

# Make executable
chmod +x ~/bin/claude

# Ensure ~/bin is in PATH before /usr/local/bin
export PATH="$HOME/bin:$PATH"
```

**Method 3: IDE Terminal Configuration**

**IntelliJ IDEA / PyCharm / WebStorm:**
1. Open Settings → Tools → Terminal
2. Set 'Shell path' to: `~/.local/bin/claude-mux`
3. Set 'Shell arguments' to: `claude`
4. Restart IDE terminal

**VS Code:**
1. Open Settings (Cmd+, or Ctrl+,)
2. Search for 'terminal.integrated.profiles'
3. Add profile:
   ```json
   "terminal.integrated.profiles.osx": {
     "claude-mux": {
       "path": "~/.local/bin/claude-mux",
       "args": ["claude"]
     }
   }
   ```
4. Set as default terminal profile

### Session Discovery

Stapler Squad uses **auto-discovery with filesystem watching** for immediate session detection:

- **Filesystem Watching**: Uses `fsnotify` to watch `/tmp/` for socket creation/deletion
- **Immediate Connection**: New sessions discovered instantly (no polling delay)
- **Automatic Fallback**: Falls back to polling if filesystem watching fails
- **Zero Configuration**: Works automatically when stapler-squad is running

**Verification:**
```bash
# Start a Claude session with multiplexing
claude-mux claude

# In another terminal, check socket creation
ls /tmp/claude-mux-*.sock

# Stapler Squad will automatically discover and connect to the session
```

### Troubleshooting

**Issue: 'claude-mux: command not found'**
- Ensure `~/.local/bin` is in your PATH
- Run: `export PATH="$HOME/.local/bin:$PATH"`
- Add to shell config for persistence

**Issue: 'stdin is not a terminal'**
- `claude-mux` requires a TTY (interactive terminal)
- Cannot be used in scripts or pipes
- Must run from actual terminal session

**Issue: Sessions not discovered**
1. Check socket exists: `ls /tmp/claude-mux-*.sock`
2. Verify permissions: `ls -l /tmp/claude-mux-*.sock` (should be 0600)
3. Check stapler-squad logs: `~/.stapler-squad/logs/stapler-squad.log`
4. Verify discovery is running (automatic when web UI active)

**Issue: Terminal output garbled**
- Ensure terminal size is set correctly
- `claude-mux` forwards SIGWINCH (resize signals) automatically
- Try resizing the terminal window

**Issue: Socket cleanup**
- Stale sockets removed automatically on next discovery scan
- Manual cleanup: `rm /tmp/claude-mux-*.sock` (when no sessions running)

### Technical Details

**Protocol**: Binary protocol with message framing (type + length + data)

**Message Types**:
- `Output`: Terminal output from Claude → clients
- `Input`: User input from clients → Claude
- `Resize`: Terminal resize events (SIGWINCH)
- `Metadata`: Session info (command, PID, cwd, env)
- `Ping/Pong`: Keepalive for connection health

**Security**:
- Sockets created with 0600 permissions (owner-only access)
- Local Unix domain sockets (no network exposure)
- Process isolation through PTY

**Performance**:
- Zero overhead when no clients connected
- Minimal latency (direct PTY forwarding)
- Automatic cleanup on process exit

## State File Isolation and Multi-Instance Support

Stapler Squad implements hierarchical state file isolation to prevent conflicts between tests, benchmarks, and multiple production instances. This ensures safe concurrent execution and data integrity.

### Isolation Hierarchy

State files are automatically isolated based on a priority hierarchy:

1. **Explicit Instance ID** (Highest Priority)
   ```bash
   # Named instance with dedicated state
   STAPLER_SQUAD_INSTANCE=work ./stapler-squad

   # Backward compatibility - shared global state
   STAPLER_SQUAD_INSTANCE=shared ./stapler-squad
   ```
   - State location: `~/.stapler-squad/instances/{INSTANCE_ID}/`
   - Use case: Named instances for different projects/contexts

2. **Test Mode Auto-Detection**
   - Automatically activated when running `go test` or benchmarks
   - State location: `~/.stapler-squad/test/test-{PID}/`
   - Use case: Prevents tests from corrupting production state
   - No configuration needed - works automatically

3. **Workspace-Based Isolation** (Production Default)
   ```bash
   # Automatic per-directory state (default behavior)
   cd ~/my-project && ./stapler-squad

   # Disable workspace isolation if needed
   STAPLER_SQUAD_WORKSPACE_MODE=false ./stapler-squad
   ```
   - State location: `~/.stapler-squad/workspaces/{WORKSPACE_HASH}/`
   - Use case: Different state per git repository/working directory
   - Each directory gets a stable, unique workspace ID (SHA256 hash)

4. **Global Shared State** (Fallback)
   - State location: `~/.stapler-squad/`
   - Use case: Backward compatibility, explicit sharing
   - Activated when workspace mode is disabled or detection fails

### Instance Identification in Logs

All log messages include instance identifiers for debugging multi-instance scenarios:

```bash
# Named instance logs
[work] INFO: Session created

# PID-based logs (with timestamp to prevent PID reuse confusion)
[pid-12345-1704132000] INFO: Session started

# Daemon logs
[work][DAEMON] INFO: Polling sessions
```

The instance identifier prevents confusion when:
- Multiple instances run simultaneously
- Analyzing historical logs with reused PIDs
- Debugging concurrent test execution

### Common Usage Patterns

**Development Workflow:**
```bash
# Default: Workspace isolation (safest, per-project state)
cd ~/my-feature && ./stapler-squad

# Different project, different state automatically
cd ~/other-project && ./stapler-squad
```

**Testing and Benchmarks:**
```bash
# Automatic isolation - no configuration needed
go test ./...
go test -bench=. ./app

# Tests never interfere with production state
# Each test/benchmark gets isolated directories
```

**Multiple Named Instances:**
```bash
# Work sessions
STAPLER_SQUAD_INSTANCE=work ./stapler-squad

# Personal projects
STAPLER_SQUAD_INSTANCE=personal ./stapler-squad

# Completely isolated state files
```

**Legacy Shared State:**
```bash
# Use pre-isolation behavior if needed
STAPLER_SQUAD_INSTANCE=shared ./stapler-squad

# Or disable workspace mode
STAPLER_SQUAD_WORKSPACE_MODE=false ./stapler-squad
```

### Migration Notes

**Existing Users:**
- Workspace isolation is now the default (per-directory state)
- To use old shared behavior: `STAPLER_SQUAD_INSTANCE=shared`
- Or disable workspace mode: `STAPLER_SQUAD_WORKSPACE_MODE=false`
- Your existing `~/.stapler-squad/` state is preserved

**Test Authors:**
- Tests automatically get isolated state - no code changes needed
- Benchmarks no longer corrupt production state files
- Test mode is auto-detected from command-line arguments

**Multi-Instance Users:**
- Safe concurrent execution is now supported by default
- Each workspace/instance gets its own state file
- File locking prevents corruption within each instance
- Log messages include instance IDs for debugging

### Troubleshooting

**Issue: "Can't find my sessions after restart"**
- Check if you're in a different directory (workspace isolation active)
- Use `STAPLER_SQUAD_WORKSPACE_MODE=false` for directory-independent state
- Or set `STAPLER_SQUAD_INSTANCE=shared` for global shared state

**Issue: "Tests are modifying production state"**
- Should not happen with auto-detection
- Verify tests are run with `go test` command
- Check test binary names contain `.test` suffix

**Issue: "Multiple instances conflicting"**
- Each instance should have its own workspace automatically
- Check instance identifiers in logs to verify isolation
- Use explicit `STAPLER_SQUAD_INSTANCE` for named instances

**Issue: "Want to share state across multiple directories"**
- Use named instance: `STAPLER_SQUAD_INSTANCE=shared`
- Or disable workspace mode: `STAPLER_SQUAD_WORKSPACE_MODE=false`
- Both approaches give you global shared state

## Architecture Overview

Stapler Squad is a web-based session management application built with Go. It manages multiple AI agent sessions (Claude Code, Aider, etc.) in isolated tmux sessions with git worktrees. The application runs as a web server on localhost:8543 and provides a React-based web UI.

### Core Architecture Layers

**1. Web Server (`server/`)**
- `server.go` - HTTP server with ConnectRPC handlers
- `services/` - Session management and streaming services
- `middleware/` - HTTP middleware (CORS, logging, etc.)
- `web/` - Static file serving for web UI

**2. Session Management (`session/`)**
- `instance.go` - Session lifecycle management (create, start, pause, resume)
- `storage.go` - Session persistence and state management
- `tmux/` - tmux session integration
- `git/` - Git worktree management for isolated branches
- `scrollback/` - Terminal output buffering and streaming

**3. Configuration (`config/`)**
- JSON-based configuration with logging settings
- State persistence for session data

**4. Web UI (`web-app/`)**
- React-based single-page application
- Real-time terminal streaming via ConnectRPC
- Session organization with tags and grouping

### Key Features

**Filtering**: Sessions can be filtered by status, tags, and category via the web UI.

**Search**: Full-text search across session titles, paths, branches, and tags.

**Categorization**: Sessions are organized by category, tags, or other grouping strategies.

**Real-time Terminal**: Streaming terminal output with full ANSI support and scrollback.

## Tag-Based Session Organization

Stapler Squad supports flexible session organization through tags and dynamic grouping strategies, enabling multi-dimensional organization beyond traditional single-category hierarchies.

### Grouping Modes (Web UI)

Use the "Group by" dropdown to switch between organizational views:

- **Category** (Default): Organize by category field, supports nested categories (e.g., "Work/Frontend")
- **Tag**: Multi-dimensional organization - sessions appear in multiple tag groups simultaneously
- **Branch**: Group by git branch name
- **Path**: Group by repository path (abbreviated for readability)
- **Program**: Group by program (claude, aider, etc.)
- **Status**: Group by session status (Running, Paused, Ready, etc.)
- **Session Type**: Group by session type (directory, worktree, etc.)
- **None**: Flat list with no grouping

Session counts are shown for each group.

### Tag-Based Search (Web UI)

Tags are automatically indexed for instant search:
- Search queries match against session tags with high-priority ranking
- Tag matches provide instant results via optimized index (O(1) lookup)
- Fuzzy search includes tags in the searchable content
- Tag matches boost relevance scores in hybrid search results

### Tag Management (Web UI)

The web UI provides a visual tag management interface:

1. **View Tags**: Tags appear as blue pills on session cards
2. **Edit Tags**: Click "Add Tags" or "Edit Tags" button on any session card
3. **Tag Editor Modal**:
   - Add new tags with input field (Enter to add)
   - Remove tags by clicking the × button
   - Real-time validation prevents duplicates and empty tags
   - Dark/light mode support with smooth animations

### Grouping Strategies (Web UI)

Use the "Group by" dropdown to switch between organizational views:
- Dropdown selector in the filters section
- Instant reorganization when strategy changes
- All 8 grouping strategies available (matching TUI)
- Session counts shown for each group

### Tag Filtering (Web UI)

Filter sessions by specific tags:
- **Tag Filter Dropdown**: Select from all available tags
- **Multi-Filter Support**: Combine with status and category filters
- **Search Integration**: Tags included in search bar queries
- **Clear Filters**: One-click filter reset button

### Best Practices

**Multi-Dimensional Organization**:
```
Example session tags: ["Frontend", "Urgent", "Client-Work", "React"]
- Appears in all 4 tag groups when grouped by tag
- Searchable by any tag keyword
- Can filter to specific tags in web UI
- Enables flexible project organization
```

**Tag Naming Conventions**:
- Use **PascalCase** or **kebab-case** for consistency
- Keep tags concise (1-2 words maximum)
- Avoid redundant tags (e.g., "Work" + "Work-Project")
- Common tag categories:
  - **Priority**: `Urgent`, `Low-Priority`, `Backlog`
  - **Type**: `Frontend`, `Backend`, `Infrastructure`, `DevOps`
  - **Client**: `Client-A`, `Client-B`, `Internal`
  - **Technology**: `React`, `Go`, `Python`, `Docker`, `Kubernetes`
  - **Phase**: `Planning`, `Development`, `Review`, `Maintenance`

**Multi-Membership Benefits**:
- A session tagged `["Frontend", "Urgent", "React"]` appears in 3 groups when grouped by tag
- Switch grouping strategies to view different organizational perspectives
- Filter by single tag to focus on specific work type
- Search matches any tag for quick discovery

### Backward Compatibility

**Category Field Preserved**:
- Existing `Category` field continues to work
- Categories auto-migrate to tags on first load
- Nested categories (e.g., "Work/Frontend") split into individual tags `["Work", "Frontend"]`
- No data loss during migration
- `GroupByCategory` remains the default grouping strategy

**Migration Behavior**:
- Happens automatically when loading existing sessions
- Idempotent - safe to run multiple times
- Empty categories generate `["Uncategorized"]` tag
- Category field kept for backward compatibility

### Technical Implementation

**Search Index Optimization**:
- Tags stored in dedicated `tagIndex` map for O(1) lookup
- Exact tag matches return instant results
- Prefix matching supports partial tag queries
- Multi-membership: sessions indexed under all their tags

**Grouping Engine**:
- Strategy pattern with pluggable grouping strategies
- Performance optimization: only reorganizes when needed
- Multi-membership support for GroupByTag strategy
- Expansion state preserved across strategy changes

**Data Model**:
- `Tags []string` field in `session.Instance` struct
- Thread-safe tag management methods (`AddTag`, `RemoveTag`, `HasTag`, `SetTags`)
- Tags serialized in session persistence (JSON)
- Protobuf schema includes tags for web UI integration

### Use Case Examples

**Scenario 1: Organize by Project Phase**
```bash
# Tag sessions by development phase
tags: Planning, Development, Review, Done

# Group by Tag to see all sessions in each phase (Web UI)
Group By dropdown → select "Tag"

# Filter to focus on "Development" phase only
Tag Filter dropdown → select "Development"
```

**Scenario 2: Multi-Project Development**
```bash
# Tag sessions with client and type
tags: Client-A, Client-B, Internal, Frontend, Backend

# View all Client-A work across tech stacks (Web UI)
Group By → "Tag" → see all Client-A sessions

# Switch to see tool distribution
Group By → "Program" → see claude vs aider usage

# Search for specific combination in search bar
Search → "Client-A Frontend" → instant results
```

**Scenario 3: Priority Management**
```bash
# Tag sessions by priority
tags: Urgent, High-Priority, Low-Priority, Backlog

# Daily standup: view urgent items (Web UI)
Group By → "Tag" → focus on "Urgent" group

# Check what's actively running
Group By → "Status" → see Running vs Paused

# Filter to urgent only
Tag Filter → "Urgent" → clear view of critical work
```

### Git Integration

Each session gets its own git worktree, allowing multiple concurrent branches without conflicts. The system handles:
- Worktree creation and cleanup
- Branch management and switching  
- Diff generation for preview pane
- Commit and push operations

### tmux Integration

Sessions run in isolated tmux sessions for:
- Process isolation and persistence
- Terminal multiplexing within each session
- Background execution capabilities
- Session attachment/detachment

### State Management

**Application State**: Managed through BubbleTea's state machine with states like `stateDefault`, `stateNew`, `statePrompt`.

**Session State**: Persisted in JSON format with status tracking (Running, Paused, Stopped).

**UI State**: Navigation indices, filter settings, and view preferences are maintained across operations.

## Adding New Features

### New Web UI Features
1. Create React components in `web-app/src/components/`
2. Add ConnectRPC endpoints in `server/services/`
3. Update protobuf definitions in `proto/session/v1/` if needed
4. Run `make generate-proto` to regenerate code
5. Test with `make restart-web`

### New Session Filters
1. Add filter parameters to ConnectRPC service definitions
2. Implement filter logic in `session/storage.go` or service layer
3. Update web UI filter components
4. Test filtering behavior

### New API Endpoints
1. Define new RPC methods in `proto/session/v1/session.proto`
2. Implement handler in `server/services/`
3. Register handler in `server/server.go`
4. Generate client code with `make generate-proto`
5. Call from web UI

## Performance Optimization

### Web UI Performance
The web application implements several performance optimizations:

**Terminal Streaming**: Real-time terminal output via ConnectRPC streaming with efficient delta compression.

**Scrollback Buffering**: Circular buffer implementation for efficient terminal history management.

**Search Indexing**: BM25-based search with inverted index for instant results.

**Repository Name Caching**: Git repository names are cached to avoid repeated expensive git operations.

### Tmux Session Isolation
Configure tmux session prefixes for process isolation:

```json
{
  "tmux_session_prefix": "myapp_"
}
```

This allows multiple stapler-squad processes to run simultaneously without session conflicts.

## Makefile Usage

The project includes a comprehensive Makefile for development workflows:

### Quick Start
```bash
make help          # Show all available commands
make dev-setup     # Set up development environment
make validate-env  # Check if tools are installed
```

### Development Workflows
```bash
make build         # Build the application
make test         # Run tests
make quick-check   # Build + test + lint (fast validation)
make pre-commit    # Full pre-commit validation
make all          # Complete workflow: clean + build + test + analyze
```

### Analysis and Quality
```bash
make analyze       # Run all static analysis tools
make nil-safety    # Comprehensive nil safety analysis
make security      # Security vulnerability scanning
make lint         # Code style and quality checks
```

### Performance Testing
```bash
make benchmark          # Full benchmarks (runs in background)
make benchmark-quick    # Fast subset for validation
make benchmark-navigation # Navigation performance only
make profile-cpu       # CPU profiling
```

### Tool Management
```bash
make install-tools # Install all development tools
make clean        # Clean build artifacts  
make clean-tools  # Remove installed tools (caution)
```

The Makefile handles long-running benchmarks automatically and provides comprehensive development automation.