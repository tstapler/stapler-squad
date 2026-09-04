package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/config/workspacepath"
	"gopkg.in/natefinch/lumberjack.v2"
)

// runtimeLevel is the global minimum log level, adjustable at runtime without restart.
// Both the file and console streams honour this value via levelFilterWriter.
var runtimeLevel atomic.Int32 //nolint:gochecknoglobals

// slogLevel mirrors runtimeLevel for the slog-based logger (the Debug/Info/Warn/Error
// package functions). It exists because slog.HandlerOptions.Level takes a slog.Leveler,
// not our LogLevel type — SetRuntimeLevel keeps the two in sync so both logging paths
// (legacy log.New-based DebugLog/InfoLog/etc. and slog) honour the same runtime level.
var slogLevel slog.LevelVar //nolint:gochecknoglobals

func init() {
	runtimeLevel.Store(int32(INFO))
	slogLevel.Set(slog.LevelInfo)
	slogDefault.Store(slog.Default())
}

// toSlogLevel maps our LogLevel enum to the closest slog.Level.
func toSlogLevel(level LogLevel) slog.Level {
	switch level {
	case DEBUG:
		return slog.LevelDebug
	case INFO:
		return slog.LevelInfo
	case WARNING:
		return slog.LevelWarn
	case ERROR, FATAL:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// SetRuntimeLevel changes the minimum log level for all output streams immediately.
// Safe to call from any goroutine. Takes effect on the next log call.
func SetRuntimeLevel(level LogLevel) {
	// #nosec G115 -- LogLevel is a small internal enum (DEBUG..FATAL, iota-based), nowhere near int32's range
	runtimeLevel.Store(int32(level))
	slogLevel.Set(toSlogLevel(level))
}

// GetRuntimeLevel returns the current minimum log level.
func GetRuntimeLevel() LogLevel {
	return LogLevel(runtimeLevel.Load())
}

// IsDebugEnabled returns true when the runtime level is DEBUG.
// Use this to gate expensive format-string construction before calling DebugLog.Printf.
func IsDebugEnabled() bool {
	return LogLevel(runtimeLevel.Load()) <= DEBUG
}

// Env var names and sentinel values shared by getInstanceIdentifier and GetConfigDir.
// Mirrors the identical literals in config/config.go (GetConfigDirForDir, IsNamedInstance)
// which this package can't import directly (see GetConfigDir's doc comment) — named here
// so the two copies can't silently drift apart on a typo.
const (
	envInstanceID    = "STAPLER_SQUAD_INSTANCE"
	envTestDir       = "STAPLER_SQUAD_TEST_DIR"
	sharedInstanceID = "shared"
)

// getInstanceIdentifier returns a unique identifier for this process instance
// This helps differentiate log messages when multiple instances are running
func getInstanceIdentifier() string {
	// Priority 1: Use explicit instance ID from environment
	if instanceID := os.Getenv(envInstanceID); instanceID != "" {
		return instanceID
	}

	// Priority 2: Generate PID + start time for automatic identification
	// This prevents confusion when PIDs are reused
	pid := os.Getpid()
	startTime := time.Now().Unix()
	return fmt.Sprintf("pid-%d-%d", pid, startTime)
}

// LogLevel represents the severity of a log entry
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARNING
	ERROR
	FATAL
)

// String returns the string representation of a log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARNING:
		return "WARNING"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel parses a string into a LogLevel
func ParseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARNING", "WARN":
		return WARNING
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO // Default to INFO level
	}
}

// atomicLogger holds a *log.Logger behind an atomic.Pointer so it can be swapped
// (by initializeWithConfig, or by tests via the SetXForTest helpers) while other
// goroutines concurrently read it, with no risk of a torn or racy read.
//
// A sync.RWMutex was the other option, but a *log.Logger is always replaced
// wholesale (never mutated in place) — readers just need the current pointer,
// never a lock held across a call. atomic.Pointer gives lock-free reads and
// makes swap-the-whole-value the only possible operation, which matches how
// these loggers are actually used. This mirrors the existing runtimeLevel
// atomic.Int32 pattern above.
type atomicLogger struct {
	ptr atomic.Pointer[log.Logger]
}

func (a *atomicLogger) Load() *log.Logger              { return a.ptr.Load() }
func (a *atomicLogger) Store(l *log.Logger)            { a.ptr.Store(l) }
func (a *atomicLogger) Swap(l *log.Logger) *log.Logger { return a.ptr.Swap(l) }

// atomicSlogLogger holds a *slog.Logger behind an atomic.Pointer, mirroring
// atomicLogger above but for the slog-backed logging path (logAt, ForSession).
// A sibling type rather than a generic atomicLogger[T] to avoid touching the
// already-correct, already-reviewed legacy-*log.Logger swap mechanism.
type atomicSlogLogger struct {
	ptr atomic.Pointer[slog.Logger]
}

func (a *atomicSlogLogger) Load() *slog.Logger               { return a.ptr.Load() }
func (a *atomicSlogLogger) Store(l *slog.Logger)             { a.ptr.Store(l) }
func (a *atomicSlogLogger) Swap(l *slog.Logger) *slog.Logger { return a.ptr.Swap(l) }

//nolint:gochecknoglobals
var (
	warningLog atomicLogger
	infoLog    atomicLogger
	errorLog   atomicLogger
	debugLog   atomicLogger

	// slogDefault holds the *slog.Logger read by logAt/ForSession. It is kept in
	// sync with the real slog.Default() by initializeWithConfig, but tests swap
	// it via SetSlogDefaultForTest instead of calling slog.SetDefault() directly
	// — slog.SetDefault also redirects stdlib log.Print process-wide, which is
	// what let an unrelated httptest.Server's hang-detector log line land in a
	// concurrent test's capture buffer under -race (see log_test.go and
	// server/services's captureLogs helpers).
	slogDefault atomicSlogLogger

	// Global config reference
	globalConfig *LogConfig

	// Session loggers map (sessionID -> loggers)
	sessionLoggers map[string]*SessionLoggers
	sessionMutex   sync.RWMutex

	// Structured logger
	structuredLogger *StructuredLogger

	// defaultManager is the LogManager that wraps the package-level globals.
	// It is populated by initializeWithConfig and used by GetSessionLoggers and Close
	// so callers can inject it for zero-migration compatibility.
	defaultManager *LogManager

	// asyncFileWriter and asyncConsoleWriter are the async queued writers that back the
	// stdlib log.Logger shims (InfoLog, WarningLog, etc.). They are populated by
	// initializeWithConfig and must be drained (via Close) before the underlying file
	// is closed. Declared here so Close() can reach them even without a LogManager.
	asyncLogFileWriter    *asyncWriter
	asyncLogConsoleWriter *asyncWriter
)

// WarningLog returns the current warning-level logger. Safe to call concurrently
// with SetWarningLogForTest or initializeWithConfig replacing it.
func WarningLog() *log.Logger { return warningLog.Load() }

// InfoLog returns the current info-level logger. Safe to call concurrently
// with SetInfoLogForTest or initializeWithConfig replacing it.
func InfoLog() *log.Logger { return infoLog.Load() }

// ErrorLog returns the current error-level logger. Safe to call concurrently
// with SetErrorLogForTest or initializeWithConfig replacing it.
func ErrorLog() *log.Logger { return errorLog.Load() }

// DebugLog returns the current debug-level logger. Safe to call concurrently
// with SetDebugLogForTest or initializeWithConfig replacing it.
func DebugLog() *log.Logger { return debugLog.Load() }

// SetWarningLogForTest atomically replaces the warning logger and returns the
// previous value, so callers can restore it via t.Cleanup instead of racing a
// bare package-var assignment against concurrent t.Parallel() reads.
func SetWarningLogForTest(l *log.Logger) *log.Logger { return warningLog.Swap(l) }

// SetSlogDefaultForTest atomically replaces the slog-backed default logger
// (read by logAt/ForSession) and returns the previous value, so tests can
// restore it via t.Cleanup instead of calling slog.SetDefault() — which
// would also rewire stdlib log.Print process-wide and is the root cause of
// the server/services capture-buffer race under -race this seam removes
// tests from touching at all.
func SetSlogDefaultForTest(l *slog.Logger) *slog.Logger { return slogDefault.Swap(l) }

// SetInfoLogForTest atomically replaces the info logger and returns the previous
// value, so callers can restore it via t.Cleanup.
func SetInfoLogForTest(l *log.Logger) *log.Logger { return infoLog.Swap(l) }

// SetErrorLogForTest atomically replaces the error logger and returns the
// previous value, so callers can restore it via t.Cleanup.
func SetErrorLogForTest(l *log.Logger) *log.Logger { return errorLog.Swap(l) }

// SetDebugLogForTest atomically replaces the debug logger and returns the
// previous value, so callers can restore it via t.Cleanup.
func SetDebugLogForTest(l *log.Logger) *log.Logger { return debugLog.Swap(l) }

// LogConfig holds logging configuration
type LogConfig struct {
	LogsEnabled    bool
	LogsDir        string
	LogMaxSize     int
	LogMaxFiles    int
	LogMaxAge      int
	LogCompress    bool
	UseSessionLogs bool
	LogLevel       LogLevel // Deprecated: Use FileLevel and ConsoleLevel instead
	StructuredLogs bool
	PrettyLogs     bool // For development - formats JSON logs for readability

	// Dual-stream logging configuration (file + console)
	ConsoleEnabled bool     // Enable/disable console output (default: true)
	ConsoleLevel   LogLevel // Minimum level for console (default: ERROR for tests, INFO for production)
	FileEnabled    bool     // Enable/disable file output (default: true)
	FileLevel      LogLevel // Minimum level for file (default: DEBUG)
}

// DefaultLogConfig returns the default logging configuration
func DefaultLogConfig() *LogConfig {
	return &LogConfig{
		LogsEnabled:    true,
		LogsDir:        "",
		LogMaxSize:     10, // 10MB
		LogMaxFiles:    5,  // 5 backups
		LogMaxAge:      30, // 30 days
		LogCompress:    true,
		UseSessionLogs: true,
		LogLevel:       INFO,  // Deprecated: kept for backward compatibility
		StructuredLogs: false, // Default to traditional logging
		PrettyLogs:     false, // Default to compact JSON

		// Dual-stream defaults (production settings).
		// Debug logging is disabled by default; enable via SetRuntimeLevel(DEBUG)
		// or the debug menu in the web UI.
		ConsoleEnabled: true,
		ConsoleLevel:   INFO,
		FileEnabled:    true,
		FileLevel:      INFO,
	}
}

// Default log directory and filename
var logFileName = filepath.Join(os.TempDir(), "staplersquad.log") //nolint:gochecknoglobals

// StructuredLogEntry represents a structured log entry
type StructuredLogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	SessionID string                 `json:"session_id,omitempty"`
	Component string                 `json:"component,omitempty"`
	Function  string                 `json:"function,omitempty"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// StructuredLogger provides structured logging functionality
type StructuredLogger struct {
	writer    io.Writer
	level     LogLevel
	prettyLog bool
	mutex     sync.Mutex
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(writer io.Writer, level LogLevel, prettyLog bool) *StructuredLogger {
	return &StructuredLogger{
		writer:    writer,
		level:     level,
		prettyLog: prettyLog,
	}
}

// Log writes a structured log entry
func (sl *StructuredLogger) Log(level LogLevel, message string, fields map[string]interface{}) {
	// Check if we should log this level
	if level < sl.level {
		return
	}

	sl.mutex.Lock()
	defer sl.mutex.Unlock()

	entry := StructuredLogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   message,
		Fields:    fields,
	}

	// Add error field if present
	if err, exists := fields["error"]; exists {
		if e, ok := err.(error); ok {
			entry.Error = e.Error()
			// Remove from fields to avoid duplication
			if entry.Fields == nil {
				entry.Fields = make(map[string]interface{})
			}
			delete(entry.Fields, "error")
		}
	}

	var output []byte
	var err error

	if sl.prettyLog {
		output, err = json.MarshalIndent(entry, "", "  ")
	} else {
		output, err = json.Marshal(entry)
	}

	if err != nil {
		// Fallback to simple text logging if JSON marshaling fails
		_, _ = fmt.Fprintf(sl.writer, "%s [%s] %s\n", entry.Timestamp.Format(time.RFC3339), entry.Level, entry.Message)
		return
	}

	_, _ = sl.writer.Write(output)
	_, _ = sl.writer.Write([]byte("\n"))
}

// LogWithFields logs a message with additional fields
func (sl *StructuredLogger) LogWithFields(level LogLevel, message string, fields map[string]interface{}) {
	sl.Log(level, message, fields)
}

// Debug logs a debug message
func (sl *StructuredLogger) Debug(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	sl.Log(DEBUG, message, f)
}

// Info logs an info message
func (sl *StructuredLogger) Info(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	sl.Log(INFO, message, f)
}

// Warning logs a warning message
func (sl *StructuredLogger) Warning(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	sl.Log(WARNING, message, f)
}

// Error logs an error message
func (sl *StructuredLogger) Error(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	sl.Log(ERROR, message, f)
}

// Fatal logs a fatal message
func (sl *StructuredLogger) Fatal(message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	sl.Log(FATAL, message, f)
}

// GetConfigDir returns the path to the application's configuration directory,
// mirroring config.GetConfigDirForDir("")'s full 6-priority precedence so log
// paths never drift from where DB/session state lands. Duplicated (via the
// shared config/workspacepath leaf package) rather than imported directly
// because config already imports log, and importing back would create a
// cycle.
func GetConfigDir() (string, error) {
	return GetConfigDirForDir("")
}

// GetConfigDirForDir mirrors config.GetConfigDirForDir(dir), see that
// function's doc comment for the full 6-priority list. Kept here so any
// future caller with an explicit dir (e.g. a hooks binary) needs no extra
// plumbing.
func GetConfigDirForDir(dir string) (string, error) {
	// Priority 1: Test directory override (from --test-mode flag) wins outright.
	if testDir := os.Getenv(envTestDir); testDir != "" {
		if err := os.MkdirAll(testDir, 0750); err != nil {
			return "", fmt.Errorf("failed to create test directory: %w", err)
		}
		return testDir, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	baseDir := filepath.Join(homeDir, ".stapler-squad")

	// Priority 2: Explicit instance ID (tests, named instances, backward
	// compat). "shared" short-circuits straight to baseDir — same as
	// config.GetConfigDirForDir — rather than falling through to Priority 3-6;
	// it must NOT be treated as "unset", since that would silently land
	// test/workspace-mode/preferred-workspace logic on a directory the caller
	// explicitly opted out of.
	if instanceID := os.Getenv(envInstanceID); instanceID != "" {
		if instanceID == sharedInstanceID {
			return baseDir, nil
		}
		// Reject path separators/".." so a stray or malicious instance ID can't
		// escape baseDir via filepath.Join's lexical Clean() — same gap as
		// config.GetConfigDirForDir, closed here first since this is the one
		// touching this precedence logic today.
		if strings.ContainsAny(instanceID, `/\`) || strings.Contains(instanceID, "..") {
			return "", fmt.Errorf("invalid STAPLER_SQUAD_INSTANCE %q: must not contain path separators or \"..\"", instanceID)
		}
		return filepath.Join(baseDir, "instances", instanceID), nil
	}

	// Priority 3: Test mode auto-detection — must be checked before the
	// preferred workspace file, same reasoning as config.GetConfigDirForDir.
	if workspacepath.IsTestMode() {
		pid := os.Getpid()
		return filepath.Join(baseDir, "test", fmt.Sprintf("test-%d", pid)), nil
	}

	return resolveDefaultConfigDir(dir, baseDir)
}

// resolveDefaultConfigDir implements Priority 4-6 of GetConfigDirForDir,
// mirroring config.resolveDefaultConfigDir exactly (see its doc comment).
// Split out so it can be tested directly — Priority 3 (test mode
// auto-detection) is always true inside a `go test` binary, which would
// otherwise make this logic unreachable in tests.
func resolveDefaultConfigDir(dir, baseDir string) (string, error) {
	result := workspacepath.ResolveDefaultDir(dir, baseDir)
	if result.WithinStateDir {
		Warn("cwd is inside stapler-squad state directory; this process will use a different workspace than usual and may appear to have no sessions",
			"cwd", result.WorkDir, "state_dir", baseDir)
	}
	if result.GetwdErr != nil {
		Warn("failed to get working directory for workspace isolation", "err", result.GetwdErr)
	}
	return result.Dir, nil
}

// GetLogDir returns the directory where logs should be stored
func GetLogDir(cfg *LogConfig) (string, error) {
	// If logging is disabled, return temp directory
	if cfg != nil && !cfg.LogsEnabled {
		return os.TempDir(), nil
	}

	// If a custom log directory is specified in config, use it
	if cfg != nil && cfg.LogsDir != "" {
		return cfg.LogsDir, nil
	}

	// Otherwise use ~/.stapler-squad/logs/
	configDir, err := GetConfigDir()
	if err != nil {
		return os.TempDir(), fmt.Errorf("failed to get config directory: %w", err)
	}

	logDir := filepath.Join(configDir, "logs")
	// Create the log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return os.TempDir(), fmt.Errorf("failed to create log directory: %w", err)
	}

	return logDir, nil
}

// GetTestLogDir returns the directory where test logs should be stored
// Test logs are isolated in a dedicated subdirectory for easy cleanup
func GetTestLogDir() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return os.TempDir(), fmt.Errorf("failed to get config directory: %w", err)
	}

	testLogDir := filepath.Join(configDir, "logs", "test")
	// Create the test log directory if it doesn't exist
	if err := os.MkdirAll(testLogDir, 0750); err != nil {
		return os.TempDir(), fmt.Errorf("failed to create test log directory: %w", err)
	}

	return testLogDir, nil
}

// GetLogFilePath returns the full path to the log file
func GetLogFilePath(cfg *LogConfig) (string, error) {
	// Get log directory
	logDir, err := GetLogDir(cfg)
	if err != nil {
		return logFileName, err
	}

	return filepath.Join(logDir, "staplersquad.log"), nil
}

// GetSessionLogFilePath returns the full path to a session-specific log file
func GetSessionLogFilePath(cfg *LogConfig, sessionID string) (string, error) {
	// Get log directory
	logDir, err := GetLogDir(cfg)
	if err != nil {
		return "", err
	}

	// Sanitize sessionID to be safe as a filename
	safeSessionID := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, sessionID)

	return filepath.Join(logDir, fmt.Sprintf("session_%s.log", safeSessionID)), nil
}

var ( //nolint:gochecknoglobals
	// ErrSessionLogsDisabled is returned when session logs are disabled in config
	ErrSessionLogsDisabled = fmt.Errorf("session logs disabled in config")
)

// GetSessionLoggers creates or retrieves loggers for a specific session
func GetSessionLoggers(sessionID string) (*SessionLoggers, error) {
	if defaultManager != nil {
		return defaultManager.ForSession(sessionID)
	}

	sessionMutex.RLock()
	// Check if we already have loggers for this session
	if loggers, exists := sessionLoggers[sessionID]; exists {
		sessionMutex.RUnlock()
		return loggers, nil
	}
	sessionMutex.RUnlock()

	// If session logs are disabled in config, return an error
	if globalConfig != nil && !globalConfig.UseSessionLogs {
		return nil, ErrSessionLogsDisabled
	}

	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	// Double-check after acquiring write lock
	if loggers, exists := sessionLoggers[sessionID]; exists {
		return loggers, nil
	}

	// Create new session loggers
	logFilePath, err := GetSessionLogFilePath(globalConfig, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session log file path: %w", err)
	}

	// Create rotating writer for logs
	writer := createRotatingWriter(logFilePath, globalConfig)

	// Create loggers
	loggers := &SessionLoggers{
		InfoLog:    log.New(writer, fmt.Sprintf("[%s] INFO: ", sessionID), log.Ldate|log.Ltime|log.Lshortfile),
		WarningLog: log.New(writer, fmt.Sprintf("[%s] WARNING: ", sessionID), log.Ldate|log.Ltime|log.Lshortfile),
		ErrorLog:   log.New(writer, fmt.Sprintf("[%s] ERROR: ", sessionID), log.Ldate|log.Ltime|log.Lshortfile),
		DebugLog:   log.New(writer, fmt.Sprintf("[%s] DEBUG: ", sessionID), log.Ldate|log.Ltime|log.Lshortfile),
	}

	// Store the closer if available
	if closer, ok := writer.(io.Closer); ok {
		loggers.LogFile = closer
	}

	// Store in map
	sessionLoggers[sessionID] = loggers

	return loggers, nil
}

// LogForSession logs a message to the session-specific log file
func LogForSession(sessionID, level, format string, v ...interface{}) {
	if globalConfig == nil || !globalConfig.UseSessionLogs {
		// If session logs are disabled, log to the global logger
		switch level {
		case "info":
			InfoLog().Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		case "warning":
			WarningLog().Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		case "error":
			ErrorLog().Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		}
		return
	}

	// Get session loggers
	loggers, err := GetSessionLoggers(sessionID)
	if err != nil {
		// If we can't get session loggers, fall back to global
		ErrorLog().Printf("Failed to get session loggers for %s: %v", sessionID, err)
		switch level {
		case "info":
			InfoLog().Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		case "warning":
			WarningLog().Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		case "error":
			ErrorLog().Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		}
		return
	}

	// Log to session file
	switch level {
	case "info":
		loggers.InfoLog.Printf(format, v...)
		// Also log to global file with session prefix
		InfoLog().Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
	case "warning":
		loggers.WarningLog.Printf(format, v...)
		// Also log to global file with session prefix
		WarningLog().Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
	case "error":
		loggers.ErrorLog.Printf(format, v...)
		// Also log to global file with session prefix
		ErrorLog().Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
	}
}

var globalLogFile io.WriteCloser //nolint:gochecknoglobals

// SessionLoggers holds the loggers for a specific session
type SessionLoggers struct {
	WarningLog *log.Logger
	InfoLog    *log.Logger
	ErrorLog   *log.Logger
	DebugLog   *log.Logger
	LogFile    io.Closer
}

// SessionLogger is a session-scoped logger that automatically injects the session ID
// into every log call, eliminating the need to pass the session ID manually.
//
// Usage:
//
//	logger := log.ForSession(i.Title)
//	logger.Error("Failed to setup git worktree: %v", err)
type SessionLogger struct {
	sessionID string
}

// ForSession returns a *slog.Logger pre-populated with "session" = sessionID.
// All calls route through the async slog handler — no stdlib mutex serialization.
// Session-specific log files still receive the entry via LogForSession when needed.
func ForSession(sessionID string) *slog.Logger {
	return slogDefault.Load().With("session", sessionID)
}

// ForSessionLegacy returns the old SessionLogger for callers that write to
// per-session log files. New code should use ForSession instead.
//
// Deprecated: use ForSession.
func ForSessionLegacy(sessionID string) *SessionLogger {
	return &SessionLogger{sessionID: sessionID}
}

func (sl *SessionLogger) Debug(format string, v ...interface{}) {
	LogForSession(sl.sessionID, "debug", format, v...)
}

func (sl *SessionLogger) Info(format string, v ...interface{}) {
	LogForSession(sl.sessionID, "info", format, v...)
}

func (sl *SessionLogger) Warning(format string, v ...interface{}) {
	LogForSession(sl.sessionID, "warning", format, v...)
}

func (sl *SessionLogger) Error(format string, v ...interface{}) {
	LogForSession(sl.sessionID, "error", format, v...)
}

// logAt builds and emits a slog.Record with the PC of Info/Warn/Error/Debug's
// caller (not this function, and not Info/Warn/etc. themselves) so
// PackageLevelHandler can resolve per-package overrides correctly (skip=3:
// Callers, logAt, Info/Warn/Error/Debug, caller).
// See also: https://pkg.go.dev/log/slog#hdr-Wrapping_output_methods.
func logAt(level slog.Level, msg string, args ...any) {
	logger := slogDefault.Load()
	ctx := context.Background()
	if !logger.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = logger.Handler().Handle(ctx, r)
}

// Info logs an info-level message through the default slog handler (async, no mutex hold).
// args are alternating key-value pairs: log.Info("msg", "key", val, "key2", val2)
func Info(msg string, args ...any) { logAt(slog.LevelInfo, msg, args...) }

// Warn logs a warning-level message through the default slog handler.
func Warn(msg string, args ...any) { logAt(slog.LevelWarn, msg, args...) }

// Error logs an error-level message through the default slog handler.
func Error(msg string, args ...any) { logAt(slog.LevelError, msg, args...) }

// Debug logs a debug-level message through the default slog handler.
// The handler drops debug records when the runtime level is above DEBUG, so
// this is safe to call without an IsDebugEnabled() guard.
func Debug(msg string, args ...any) { logAt(slog.LevelDebug, msg, args...) }

// Global convenience functions for structured logging (legacy — prefer Info/Warn/Error/Debug)

// DebugS logs a structured debug message
func DebugS(message string, fields ...map[string]interface{}) {
	if structuredLogger != nil {
		structuredLogger.Debug(message, fields...)
	}
}

// InfoS logs a structured info message
func InfoS(message string, fields ...map[string]interface{}) {
	if structuredLogger != nil {
		structuredLogger.Info(message, fields...)
	}
}

// WarningS logs a structured warning message
func WarningS(message string, fields ...map[string]interface{}) {
	if structuredLogger != nil {
		structuredLogger.Warning(message, fields...)
	}
}

// ErrorS logs a structured error message
func ErrorS(message string, fields ...map[string]interface{}) {
	if structuredLogger != nil {
		structuredLogger.Error(message, fields...)
	}
}

// FatalS logs a structured fatal message
func FatalS(message string, fields ...map[string]interface{}) {
	if structuredLogger != nil {
		structuredLogger.Fatal(message, fields...)
	}
}

// levelFilterWriter wraps an io.Writer and filters out logs below the runtime level.
// The static initialLevel is the level of the log messages that flow through this
// writer (e.g. DEBUG for DebugLog, INFO for InfoLog). On each Write it checks whether
// the global runtimeLevel allows that level through.
type levelFilterWriter struct {
	writer       io.Writer
	messageLevel LogLevel // the level of the messages routed through this writer (static)
}

// Write passes the log line through only when the global runtime level allows it.
func (w *levelFilterWriter) Write(p []byte) (n int, err error) {
	if w.messageLevel >= LogLevel(runtimeLevel.Load()) {
		return w.writer.Write(p)
	}
	return len(p), nil
}

// newLevelFilterWriter creates a writer that only passes through messages at or above
// the global runtime level. minLevel is ignored — kept for API compatibility during
// the transition; the runtime atomic is the single source of truth.
func newLevelFilterWriter(writer io.Writer, _ LogLevel, logLevel LogLevel) io.Writer {
	return &levelFilterWriter{
		writer:       writer,
		messageLevel: logLevel,
	}
}

func init() {
	sessionLoggers = make(map[string]*SessionLoggers)

	// Initialize default loggers for safety - will be replaced by Initialize/InitializeWithConfig
	// Use a null writer temporarily to avoid premature output
	nullWriter := io.Discard
	infoLog.Store(log.New(nullWriter, "INFO: ", log.Ldate|log.Ltime))
	warningLog.Store(log.New(nullWriter, "WARNING: ", log.Ldate|log.Ltime))
	errorLog.Store(log.New(nullWriter, "ERROR: ", log.Ldate|log.Ltime))
	debugLog.Store(log.New(nullWriter, "DEBUG: ", log.Ldate|log.Ltime))
}

// Initialize should be called once at the beginning of the program to set up logging.
// defer Close() after calling this function. It sets the go log output to the file in
// the configured log directory (default: ~/.stapler-squad/logs/).
//
// Must run after config.LoadConfig() in any real entry point: GetConfigDir (used
// internally here) doesn't perform config.GetConfigDirForDir's legacy ~/.claude-squad
// migration, so calling this first would create ~/.stapler-squad ahead of migration
// and cause config's migration guard to skip it.
func Initialize(daemon bool) {
	// Use default config
	cfg := DefaultLogConfig()
	initializeWithConfig(daemon, cfg)
}

// InitializeForTests sets up logging specifically for test environments with dual-stream configuration.
// This allows DEBUG logs to go to file while ERROR logs appear in console for immediate visibility.
//
// Parameters:
//   - fileLevel: Minimum level for file logging (typically DEBUG to capture everything)
//   - consoleLevel: Minimum level for console logging (typically ERROR to avoid noise)
//
// Example:
//
//	log.InitializeForTests(log.DEBUG, log.ERROR)  // DEBUG→file, ERROR→console
func InitializeForTests(fileLevel LogLevel, consoleLevel LogLevel) {
	cfg := DefaultLogConfig()
	cfg.FileLevel = fileLevel
	cfg.ConsoleLevel = consoleLevel
	cfg.FileEnabled = true
	cfg.ConsoleEnabled = true

	// Use dedicated test log directory with timestamp
	testLogDir, err := GetTestLogDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to get test log directory: %v\n", err)
		testLogDir = filepath.Join(os.TempDir(), "stapler-squad-test")
	}
	cfg.LogsDir = testLogDir

	initializeWithConfig(false, cfg)

	// Print prominent log location message
	logPath := GetGlobalLogPath()
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "═══════════════════════════════════════════════════════════════\n")
	fmt.Fprintf(os.Stderr, " Test Logs Configuration:\n")
	fmt.Fprintf(os.Stderr, "   File:    %s (level: %s)\n", logPath, fileLevel.String())
	fmt.Fprintf(os.Stderr, "   Console: stderr (level: %s)\n", consoleLevel.String())
	fmt.Fprintf(os.Stderr, "═══════════════════════════════════════════════════════════════\n")
	fmt.Fprintf(os.Stderr, "\n")
}

// ConfigToLogConfig converts an external config to our internal LogConfig
func ConfigToLogConfig(externalConfig interface{}) *LogConfig {
	// If nil, use default
	if externalConfig == nil {
		return DefaultLogConfig()
	}

	// If it's already a LogConfig, return it directly
	if logCfg, ok := externalConfig.(*LogConfig); ok {
		return logCfg
	}

	// Fallback to default if type doesn't match
	return DefaultLogConfig()
}

// InitializeWithConfig sets up logging with the provided configuration.
func InitializeWithConfig(daemon bool, externalConfig interface{}) {
	// Convert external config to internal LogConfig
	cfg := ConfigToLogConfig(externalConfig)
	initializeWithConfig(daemon, cfg)
}

// createRotatingWriter creates a writer that handles log rotation based on config
func createRotatingWriter(logFilePath string, cfg *LogConfig) io.Writer {
	// Check if log rotation is needed (file size > 0)
	if cfg == nil || cfg.LogMaxSize <= 0 {
		// Create log directory if it doesn't exist
		logDir := filepath.Dir(logFilePath)
		if err := os.MkdirAll(logDir, 0750); err != nil {
			panic(fmt.Sprintf("could not create log directory: %s", err))
		}

		// No rotation, use standard file
		// #nosec G304 -- logFilePath comes from GetLogFilePath/GetLogDir, which resolve
		// to config.GetConfigDir() (or an isolated test dir) plus fixed filenames, not
		// caller/user-controlled input.
		f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			panic(fmt.Sprintf("could not open log file: %s", err))
		}
		return f
	}

	// Use lumberjack for log rotation
	return &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    cfg.LogMaxSize,  // megabytes
		MaxBackups: cfg.LogMaxFiles, // number of backups
		MaxAge:     cfg.LogMaxAge,   // days
		Compress:   cfg.LogCompress, // compress rotated files
		LocalTime:  true,            // use local time in backup filenames
	}
}

// initializeWithConfig is the internal implementation of Initialize with config
func initializeWithConfig(daemon bool, cfg *LogConfig) {
	// Store config reference for later use
	globalConfig = cfg
	// Get log file path from config
	logFilePath, err := GetLogFilePath(cfg)
	if err != nil {
		// Fall back to default log file in temp dir
		// Only print warning if console is enabled (don't interfere with TUI)
		if cfg.ConsoleEnabled {
			fmt.Fprintf(os.Stderr, "Warning: Using default log file location due to error: %v\n", err)
		}
		logFilePath = logFileName
	}

	// Seed the runtime level from config (takes the lower/more-verbose of file and console).
	configLevel := cfg.FileLevel
	if cfg.ConsoleLevel < configLevel {
		configLevel = cfg.ConsoleLevel
	}
	// #nosec G115 -- configLevel is a LogLevel enum (DEBUG..FATAL, iota-based), nowhere near int32's range
	runtimeLevel.Store(int32(configLevel))

	// Set log format to include timestamp and file/line number
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Get instance identifier for this process
	instanceID := getInstanceIdentifier()

	// Build log prefix with instance ID and optional daemon marker
	var prefix string
	if daemon {
		prefix = fmt.Sprintf("[%s][DAEMON] ", instanceID)
	} else {
		prefix = fmt.Sprintf("[%s] ", instanceID)
	}

	// Create dual-stream logging setup
	var writers []io.Writer

	// File writer (if enabled)
	if cfg.FileEnabled {
		fileWriter := createRotatingWriter(logFilePath, cfg)
		writers = append(writers, fileWriter)

		// Store the closer for file writer
		if closer, ok := fileWriter.(io.Closer); ok {
			globalLogFile = closer.(io.WriteCloser)
		}
	}

	// Console writer (if enabled)
	if cfg.ConsoleEnabled {
		writers = append(writers, os.Stderr)
	}

	// Combine writers - if no writers enabled, use discard
	var combinedWriter io.Writer
	if len(writers) == 0 {
		combinedWriter = io.Discard
	} else if len(writers) == 1 {
		combinedWriter = writers[0]
	} else {
		combinedWriter = io.MultiWriter(writers...)
	}

	// Initialize traditional loggers with level filtering for each stream.
	// Both the file and console destinations are wrapped in asyncWriter so that
	// log.Logger's internal mutex is held only for message formatting and a
	// non-blocking channel send — not for the underlying I/O.
	if cfg.FileEnabled && cfg.ConsoleEnabled {
		// Dual-stream: reuse globalLogFile (already created above) so we have
		// exactly one lumberjack instance writing to the log file.
		asyncFile := newAsyncWriter(globalLogFile, asyncWriterBufSize)
		asyncCons := newAsyncWriter(os.Stderr, asyncWriterBufSize)
		asyncLogFileWriter = asyncFile
		asyncLogConsoleWriter = asyncCons

		fileFiltered := newLevelFilterWriter(asyncFile, cfg.FileLevel, DEBUG)
		consoleFiltered := newLevelFilterWriter(asyncCons, cfg.ConsoleLevel, DEBUG)

		infoLog.Store(log.New(io.MultiWriter(
			newLevelFilterWriter(asyncFile, cfg.FileLevel, INFO),
			newLevelFilterWriter(asyncCons, cfg.ConsoleLevel, INFO),
		), prefix+"INFO:", log.Ldate|log.Ltime|log.Lshortfile))

		warningLog.Store(log.New(io.MultiWriter(
			newLevelFilterWriter(asyncFile, cfg.FileLevel, WARNING),
			newLevelFilterWriter(asyncCons, cfg.ConsoleLevel, WARNING),
		), prefix+"WARNING:", log.Ldate|log.Ltime|log.Lshortfile))

		errorLog.Store(log.New(io.MultiWriter(
			newLevelFilterWriter(asyncFile, cfg.FileLevel, ERROR),
			newLevelFilterWriter(asyncCons, cfg.ConsoleLevel, ERROR),
		), prefix+"ERROR:", log.Ldate|log.Ltime|log.Lshortfile))

		debugLog.Store(log.New(io.MultiWriter(
			newLevelFilterWriter(asyncFile, cfg.FileLevel, DEBUG),
			newLevelFilterWriter(asyncCons, cfg.ConsoleLevel, DEBUG),
		), prefix+"DEBUG:", log.Ldate|log.Ltime|log.Lshortfile))

		if cfg.StructuredLogs {
			structuredLogger = NewStructuredLogger(io.MultiWriter(fileFiltered, consoleFiltered), cfg.FileLevel, cfg.PrettyLogs)
		}
	} else {
		// Single-stream: wrap the combined writer so the path is also async.
		asyncCombined := newAsyncWriter(combinedWriter, asyncWriterBufSize)
		asyncLogFileWriter = asyncCombined

		minLevel := cfg.LogLevel // Use deprecated field for backward compatibility
		if cfg.FileEnabled {
			minLevel = cfg.FileLevel
		} else if cfg.ConsoleEnabled {
			minLevel = cfg.ConsoleLevel
		}

		infoLog.Store(log.New(newLevelFilterWriter(asyncCombined, minLevel, INFO), prefix+"INFO:", log.Ldate|log.Ltime|log.Lshortfile))
		warningLog.Store(log.New(newLevelFilterWriter(asyncCombined, minLevel, WARNING), prefix+"WARNING:", log.Ldate|log.Ltime|log.Lshortfile))
		errorLog.Store(log.New(newLevelFilterWriter(asyncCombined, minLevel, ERROR), prefix+"ERROR:", log.Ldate|log.Ltime|log.Lshortfile))
		debugLog.Store(log.New(newLevelFilterWriter(asyncCombined, minLevel, DEBUG), prefix+"DEBUG:", log.Ldate|log.Ltime|log.Lshortfile))

		if cfg.StructuredLogs {
			structuredLogger = NewStructuredLogger(asyncCombined, minLevel, cfg.PrettyLogs)
		}
	}

	// Store the log file path for Close() to report
	logFileName = logFilePath

	// Install async slog bridge so log.Printf calls route through slog.
	// Handler ordering: TraceIDHandler (outermost, captures trace IDs at call time)
	// → PackageLevelHandler (per-package level overrides, see log/package_level.go)
	// → AsyncHandler → JSONHandler (innermost, writes to combinedWriter).
	// TraceIDHandler is a no-op identity handler until E2-S2 adds the real implementation.
	jsonHandler := slog.NewJSONHandler(combinedWriter, &slog.HandlerOptions{Level: slog.LevelDebug})
	asyncHandler := NewAsyncHandler(jsonHandler, defaultAsyncBufSize)
	asyncHandler.StartDrain()
	packageLevelHandler := NewPackageLevelHandler(asyncHandler)
	prodLogger := slog.New(NewTraceIDHandler(packageLevelHandler))
	slog.SetDefault(prodLogger)
	slogDefault.Store(prodLogger)
	LoadPackageLevelsFromEnv()

	// Populate the default LogManager so package consumers can use it via dependency injection.
	defaultManager = newLogManager(cfg, InfoLog(), WarningLog(), ErrorLog(), DebugLog(), globalLogFile, structuredLogger, asyncHandler, asyncLogFileWriter, asyncLogConsoleWriter)
}

func Close() {
	if defaultManager != nil {
		defaultManager.Close()
		return
	}

	// Close global log file
	if globalLogFile != nil {
		_ = globalLogFile.Close()
	}

	// Close all session log files
	for _, loggers := range sessionLoggers {
		if loggers.LogFile != nil {
			_ = loggers.LogFile.Close()
		}
	}

	// Removed global log file message since we use per-session logs
	// Individual session logs are written to their respective directories
}

// GetActiveSessionLogPaths returns the paths to all active session log files
func GetActiveSessionLogPaths() map[string]string {
	sessionPaths := make(map[string]string)

	if globalConfig == nil || !globalConfig.UseSessionLogs {
		return sessionPaths
	}

	for sessionID := range sessionLoggers {
		if logPath, err := GetSessionLogFilePath(globalConfig, sessionID); err == nil {
			sessionPaths[sessionID] = logPath
		}
	}

	return sessionPaths
}

// GetGlobalLogPath returns the path to the global log file
func GetGlobalLogPath() string {
	if globalConfig == nil {
		return logFileName
	}

	if logPath, err := GetLogFilePath(globalConfig); err == nil {
		return logPath
	}

	return logFileName
}

// LogSessionPathsToStderr outputs session log file paths to stderr on exit
func LogSessionPathsToStderr() {
	sessionPaths := GetActiveSessionLogPaths()
	globalPath := GetGlobalLogPath()

	if len(sessionPaths) > 0 {
		fmt.Fprintf(os.Stderr, "Session logs:\n")
		for sessionID, logPath := range sessionPaths {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", sessionID, logPath)
		}
	}

	fmt.Fprintf(os.Stderr, "Global log: %s\n", globalPath)
}

// Every is used to log at most once every timeout duration.
type Every struct {
	timeout time.Duration
	timer   *time.Timer
}

func NewEvery(timeout time.Duration) *Every {
	return &Every{timeout: timeout}
}

// ShouldLog returns true if the timeout has passed since the last log.
func (e *Every) ShouldLog() bool {
	if e.timer == nil {
		e.timer = time.NewTimer(e.timeout)
		e.timer.Reset(e.timeout)
		return true
	}

	select {
	case <-e.timer.C:
		e.timer.Reset(e.timeout)
		return true
	default:
		return false
	}
}
