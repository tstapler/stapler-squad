# Claude Squad Logging System

This package implements a configurable logging system for Claude Squad with the following features:

## Key Features

- **Configurable Log Location**: Logs are stored in `~/.claude-squad/logs/` by default, but this can be changed in the config.
- **Global and Session-Specific Logs**: Separate log files are created for each session.
- **Log Rotation**: Logs are automatically rotated based on size and age.
- **Configuration Options**: Several options can be configured in `~/.claude-squad/config.json`

## Configuration

The following logging options can be configured in `config.json`:

```json
{
  "logs_enabled": true,
  "logs_dir": "",  // Empty for default location (~/.claude-squad/logs/)
  "log_max_size": 10,  // Max log file size in MB before rotation
  "log_max_files": 5,  // Max number of rotated files to keep
  "log_max_age": 30,  // Max age in days for rotated files
  "log_compress": true,  // Whether to compress rotated files
  "use_session_logs": true  // Whether to create separate log files for each session
}
```

## Usage

### Global Logging

The global loggers (`InfoLog`, `WarningLog`, and `ErrorLog`) can be used directly:

```go
log.InfoLog.Printf("This is an info message")
log.WarningLog.Printf("This is a warning message")
log.ErrorLog.Printf("This is an error message")
```

### Session-Specific Logging

For session-specific logging, use the `LogForSession` function:

```go
// Log to session-specific file and global log
log.LogForSession("session-id", "info", "This is an info message for session %s", "session-id")
log.LogForSession("session-id", "warning", "This is a warning message for session %s", "session-id")
log.LogForSession("session-id", "error", "This is an error message for session %s", "session-id")
```

## Implementation Details

- Log files are stored in `~/.claude-squad/logs/` by default
- Global log file is named `claudesquad.log`
- Session log files are named `session_<session-id>.log`
- Log rotation is implemented using the lumberjack package
- Logs are rotated when they reach the configured size
- Old log files are compressed if the `log_compress` option is enabled