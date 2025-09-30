# Claude Squad [![CI](https://github.com/smtg-ai/claude-squad/actions/workflows/build.yml/badge.svg)](https://github.com/smtg-ai/claude-squad/actions/workflows/build.yml) [![GitHub Release](https://img.shields.io/github/v/release/smtg-ai/claude-squad)](https://github.com/smtg-ai/claude-squad/releases/latest)

[Claude Squad](https://smtg-ai.github.io/claude-squad/) is a terminal app that manages multiple [Claude Code](https://github.com/anthropics/claude-code), [Codex](https://github.com/openai/codex), [Gemini](https://github.com/google-gemini/gemini-cli) (and other local agents including [Aider](https://github.com/Aider-AI/aider)) in separate workspaces, allowing you to work on multiple tasks simultaneously.


![Claude Squad Screenshot](assets/screenshot.png)

### Highlights
- Complete tasks in the background (including yolo / auto-accept mode!)
- Manage instances and tasks in one terminal window
- Review changes before applying them, checkout changes before pushing them
- Each task gets its own isolated git workspace, so no conflicts

<br />

https://github.com/user-attachments/assets/aef18253-e58f-4525-9032-f5a3d66c975a

<br />

### Installation

Both Homebrew and manual installation will install Claude Squad as `cs` on your system.

#### Homebrew

```bash
brew install claude-squad
ln -s "$(brew --prefix)/bin/claude-squad" "$(brew --prefix)/bin/cs"
```

#### Manual

Claude Squad can also be installed by running the following command:

```bash
curl -fsSL https://raw.githubusercontent.com/smtg-ai/claude-squad/main/install.sh | bash
```

This puts the `cs` binary in `~/.local/bin`.

To use a custom name for the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/smtg-ai/claude-squad/main/install.sh | bash -s -- --name <your-binary-name>
```

### Prerequisites

- [tmux](https://github.com/tmux/tmux/wiki/Installing)
- [gh](https://cli.github.com/)

### Configuration

Configuration is stored in `~/.claude-squad/config.json`. You can view the location with `cs debug`.

#### Application Data Directory

Claude Squad stores all application data in `~/.claude-squad/`:

```
~/.claude-squad/
├── logs/                    # Application logs (rotated automatically)
│   ├── claude-squad.log     # Main application log
│   └── debug.log           # Detailed debug information
├── worktrees/              # Git worktrees for isolated sessions
│   ├── session-name_hash/  # Individual worktree directories
│   └── ...
├── config.json            # Application configuration
└── sessions.json          # Session state persistence
```

#### Logging Configuration

Logs are stored in `~/.claude-squad/logs/` by default and include log rotation features. Configure logging with these options:

```json
{
  "logs_enabled": true,
  "logs_dir": "",  // Empty for default location (~/.claude-squad/logs/)
  "log_max_size": 10,  // Max log file size in MB before rotation
  "log_max_files": 5,  // Max number of rotated files to keep
  "log_max_age": 30,  // Max age in days for rotated files
  "log_compress": true,  // Whether to compress rotated files
  "use_session_logs": true,  // Whether to create separate log files for each session
  "tmux_session_prefix": "claudesquad_"  // Custom prefix for tmux session isolation
}
```

#### Performance Configuration

For process isolation when running multiple claude-squad instances, configure a unique tmux session prefix:

```json
{
  "tmux_session_prefix": "myproject_"
}
```

### Usage

```
Usage:
  cs [flags]
  cs [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  debug       Print debug information like config paths
  help        Help about any command
  reset       Reset all stored instances
  version     Print the version number of claude-squad

Flags:
  -y, --autoyes          [experimental] If enabled, all instances will automatically accept prompts for claude code & aider
  -h, --help             help for claude-squad
  -p, --program string   Program to run in new instances (e.g. 'aider --model ollama_chat/gemma3:1b')
```

Run the application with:

```bash
cs
```
NOTE: The default program is `claude` and we recommend using the latest version.

<br />

<b>Using Claude Squad with other AI assistants:</b>
- For [Codex](https://github.com/openai/codex): Set your API key with `export OPENAI_API_KEY=<your_key>`
- Launch with specific assistants:
   - Codex: `cs -p "codex"`
   - Aider: `cs -p "aider ..."`
   - Gemini: `cs -p "gemini"`
- Make this the default, by modifying the config file (locate with `cs debug`)

<br />

#### Menu
The menu at the bottom of the screen shows available commands: 

##### Instance/Session Management
- `n` - Create a new session
- `N` - Create a new session with a prompt
- `D` - Kill (delete) the selected session
- `↑/j`, `↓/k` - Navigate between sessions

##### Actions
- `↵/o` - Attach to the selected session to reprompt
- `ctrl-q` - Detach from session
- `s` - Commit and push branch to github
- `c` - Checkout. Commits changes and pauses the session
- `r` - Resume a paused session
- `?` - Show help menu

##### Navigation
- `tab` - Switch between preview tab and diff tab
- `q` - Quit the application
- `shift-↓/↑` - scroll in diff view

### Development

#### Building from Source

```bash
# Clone the repository
git clone https://github.com/smtg-ai/claude-squad.git
cd claude-squad

# Set up development environment (installs all tools)
make dev-setup

# Build the application
make build

# Quick validation (build + test + lint)
make quick-check
```

#### Using the Makefile

The project includes a comprehensive Makefile for streamlined development:

```bash
# Show all available commands
make help

# Development workflows
make build         # Build the application
make test          # Run tests
make test-coverage # Generate HTML coverage report
make pre-commit    # Full pre-commit validation
make all           # Complete workflow: clean + build + test + analyze

# Code quality and analysis
make analyze       # Run all static analysis tools
make nil-safety    # Comprehensive nil safety analysis
make security      # Security vulnerability scanning
make lint          # Code style and quality checks
make format        # Format code with gofmt

# Performance testing
make benchmark          # Full benchmarks (runs in background)
make benchmark-quick    # Fast subset for development
make benchmark-navigation # Navigation performance tests
make profile-cpu       # CPU profiling analysis

# Tool management
make install-tools # Install all development tools
make validate-env  # Check tool installation status
make clean         # Clean build artifacts
```

#### Manual Testing Commands

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./ui
go test ./app
go test ./session

# Run core integration tests
go test ./session -run "TestComprehensiveSessionCreation|TestSessionRecoveryScenarios" -v

# Run performance benchmarks (WARNING: Long running - use make benchmark instead)
go test -bench=BenchmarkNavigation -benchmem ./app -timeout=10m &
go test -bench=BenchmarkInstanceChangedComponents -benchmem ./app -timeout=10m &
go test -bench=BenchmarkListRendering -benchmem ./app -timeout=10m &
```

**Test Infrastructure:**
- Tests use isolated tmux sockets to prevent conflicts with production sessions
- Mock executors for fast, reliable testing without external dependencies
- Comprehensive session lifecycle testing including git worktree integration
- All tests complete in <30s (core session tests) with proper isolation

#### Code Quality Tools

Claude Squad uses comprehensive static analysis for code quality:

```bash
# Install analysis tools
make install-tools

# Nil safety analysis (prevents panics)
make nil-safety         # All nil safety tools
make nilaway           # Advanced nil flow analysis
go vet -nilness ./...  # Built-in Go nil analyzer

# Comprehensive static analysis
make staticcheck       # Production-grade analyzer
make security          # Security vulnerability scan
make lint             # Multi-tool linting suite
```

**Required Development Tools:**
- [NilAway](https://github.com/uber-go/nilaway) - Advanced nil pointer safety
- [Staticcheck](https://staticcheck.dev/) - Go static analyzer
- [golangci-lint](https://golangci-lint.run/) - Meta-linter suite
- [gosec](https://github.com/securego/gosec) - Security analyzer

Install all tools with: `make install-tools`

### Recent Improvements

**Test Infrastructure Overhaul (2025-01):**
- ✅ Fixed critical tmux integration test timeouts and hangs
- ✅ Implemented isolated tmux sockets for reliable test execution
- ✅ Added mock command executors to bypass external dependencies
- ✅ Comprehensive session creation and lifecycle testing (25+ scenarios)
- ✅ Git worktree integration fully tested and validated
- ✅ Test execution time reduced from indefinite hangs to <30s

**Key Test Improvements:**
- `TestComprehensiveSessionCreation`: Full session lifecycle validation with mocked dependencies
- `TestSessionRecoveryScenarios`: Git worktree restoration and multi-session independence
- All tests now use dedicated tmux servers to prevent conflicts with production
- Builder pattern for clean test setup with proper isolation

**Developer Experience:**
- Enhanced debugging with detailed logs in `~/.claude-squad/logs/`
- Comprehensive Makefile for streamlined development workflows
- Static analysis tools (NilAway, Staticcheck, gosec) for code quality
- Performance benchmarks for navigation and rendering optimization

### FAQs

#### Failed to start new session

If you get an error like `failed to start new session: timed out waiting for tmux session`:

1. **Update the underlying program**: Ensure you're using the latest version of `claude` or your chosen AI assistant
2. **Check logs**: Review `~/.claude-squad/logs/claude-squad.log` for detailed error information
3. **Verify tmux**: Make sure tmux is installed and working (`tmux -V`)
4. **Check for conflicts**: If running multiple claude-squad instances, configure unique `tmux_session_prefix` values in config.json

**Debugging Session Creation:**
- Logs show detailed information about tmux commands, git operations, and timing
- Look for patterns like "timed out waiting for tmux session" or external command hangs
- Check if `which claude` or other external commands are blocking

### How It Works

1. **tmux** to create isolated terminal sessions for each agent
2. **git worktrees** to isolate codebases so each session works on its own branch
3. A simple TUI interface for easy navigation and management

### License

[AGPL-3.0](LICENSE.md)

### Star History

[![Star History Chart](https://api.star-history.com/svg?repos=smtg-ai/claude-squad&type=Date)](https://www.star-history.com/#smtg-ai/claude-squad&Date)
