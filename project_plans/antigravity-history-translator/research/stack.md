# Technology Stack: antigravity-history-translator

## Languages & Frameworks
- **Go**: Version 1.26.3 is the primary language used for this repository.
- **CLI Framework**: `github.com/spf13/cobra` (v1.10.1) is used for building the CLI commands, providing a standard structure for the history translator integration.

## Key Libraries & Dependencies
- **Shell Parser**: `mvdan.cc/sh/v3` (v3.13.0) - This is the standard, robust way in Go to parse shell scripts and commands into an Abstract Syntax Tree (AST). This will be essential for correctly parsing shell history without relying on fragile regex.
- **Storage**: `github.com/mattn/go-sqlite3` (v1.14.40) - Used for local storage of history data, allowing structured querying, curation, and retrieval of past commands across sessions.
- **Terminal & Session Management**: `golang.org/x/term` (v0.43.0) and `github.com/creack/pty` (v1.1.24) for interacting with terminal sessions and handling raw input/output when switching between programs.
- **Observability**: `go.opentelemetry.io/otel` (v1.44.0) for telemetry, which can be useful to instrument the history translation and insertion process.

## Design Patterns
- **CLI Subcommand Pattern**: The translator should be implemented as a subcommand or integrated into the existing Cobra command structure.
- **AST-based Translation**: Instead of simple text replacement, use `mvdan.cc/sh` to parse history lines into an AST, sanitize or translate them, and format them back.
- **Embedded Database Pattern**: Using SQLite for durable, structured history storage, enabling robust transfers and querying for agent context.
