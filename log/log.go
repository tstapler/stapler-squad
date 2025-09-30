package log

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// getInstanceIdentifier returns a unique identifier for this process instance
// This helps differentiate log messages when multiple instances are running
func getInstanceIdentifier() string {
	// Priority 1: Use explicit instance ID from environment
	if instanceID := os.Getenv("CLAUDE_SQUAD_INSTANCE"); instanceID != "" {
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

var (
	WarningLog *log.Logger
	InfoLog    *log.Logger
	ErrorLog   *log.Logger
	DebugLog   *log.Logger

	// Global config reference
	globalConfig *LogConfig

	// Session loggers map (sessionID -> loggers)
	sessionLoggers map[string]*SessionLoggers
	sessionMutex   sync.RWMutex

	// Structured logger
	structuredLogger *StructuredLogger
)

// LogConfig holds logging configuration
type LogConfig struct {
	LogsEnabled    bool
	LogsDir        string
	LogMaxSize     int
	LogMaxFiles    int
	LogMaxAge      int
	LogCompress    bool
	UseSessionLogs bool
	LogLevel       LogLevel    // Deprecated: Use FileLevel and ConsoleLevel instead
	StructuredLogs bool
	PrettyLogs     bool        // For development - formats JSON logs for readability

	// Dual-stream logging configuration (file + console)
	ConsoleEnabled bool        // Enable/disable console output (default: true)
	ConsoleLevel   LogLevel    // Minimum level for console (default: ERROR for tests, INFO for production)
	FileEnabled    bool        // Enable/disable file output (default: true)
	FileLevel      LogLevel    // Minimum level for file (default: DEBUG)
}

// DefaultLogConfig returns the default logging configuration
func DefaultLogConfig() *LogConfig {
	return &LogConfig{
		LogsEnabled:    true,
		LogsDir:        "",
		LogMaxSize:     10,    // 10MB
		LogMaxFiles:    5,     // 5 backups
		LogMaxAge:      30,    // 30 days
		LogCompress:    true,
		UseSessionLogs: true,
		LogLevel:       INFO,  // Deprecated: kept for backward compatibility
		StructuredLogs: false, // Default to traditional logging
		PrettyLogs:     false, // Default to compact JSON

		// Dual-stream defaults (production settings)
		ConsoleEnabled: true,
		ConsoleLevel:   INFO,  // Show INFO and above on console
		FileEnabled:    true,
		FileLevel:      DEBUG, // Log everything to file
	}
}

// Default log directory and filename
var logFileName = filepath.Join(os.TempDir(), "claudesquad.log")

// StructuredLogEntry represents a structured log entry
type StructuredLogEntry struct {
	Timestamp  time.Time         `json:"timestamp"`
	Level      string            `json:"level"`
	Message    string            `json:"message"`
	SessionID  string            `json:"session_id,omitempty"`
	Component  string            `json:"component,omitempty"`
	Function   string            `json:"function,omitempty"`
	File       string            `json:"file,omitempty"`
	Line       int               `json:"line,omitempty"`
	Fields     map[string]interface{} `json:"fields,omitempty"`
	Error      string            `json:"error,omitempty"`
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
		fmt.Fprintf(sl.writer, "%s [%s] %s\n", entry.Timestamp.Format(time.RFC3339), entry.Level, entry.Message)
		return
	}

	sl.writer.Write(output)
	sl.writer.Write([]byte("\n"))
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

// GetConfigDir returns the path to the application's configuration directory
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".claude-squad"), nil
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

	// Otherwise use ~/.claude-squad/logs/
	configDir, err := GetConfigDir()
	if err != nil {
		return os.TempDir(), fmt.Errorf("failed to get config directory: %w", err)
	}

	logDir := filepath.Join(configDir, "logs")
	// Create the log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
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
	if err := os.MkdirAll(testLogDir, 0755); err != nil {
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

	return filepath.Join(logDir, "claudesquad.log"), nil
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

// GetSessionLoggers creates or retrieves loggers for a specific session
func GetSessionLoggers(sessionID string) (*SessionLoggers, error) {
	sessionMutex.RLock()
	// Check if we already have loggers for this session
	if loggers, exists := sessionLoggers[sessionID]; exists {
		sessionMutex.RUnlock()
		return loggers, nil
	}
	sessionMutex.RUnlock()

	// If session logs are disabled in config, return nil
	if globalConfig != nil && !globalConfig.UseSessionLogs {
		return nil, nil
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
			InfoLog.Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		case "warning":
			WarningLog.Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		case "error":
			ErrorLog.Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		}
		return
	}

	// Get session loggers
	loggers, err := GetSessionLoggers(sessionID)
	if err != nil {
		// If we can't get session loggers, fall back to global
		ErrorLog.Printf("Failed to get session loggers for %s: %v", sessionID, err)
		switch level {
		case "info":
			InfoLog.Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		case "warning":
			WarningLog.Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		case "error":
			ErrorLog.Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
		}
		return
	}

	// Log to session file
	switch level {
	case "info":
		loggers.InfoLog.Printf(format, v...)
		// Also log to global file with session prefix
		InfoLog.Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
	case "warning":
		loggers.WarningLog.Printf(format, v...)
		// Also log to global file with session prefix
		WarningLog.Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
	case "error":
		loggers.ErrorLog.Printf(format, v...)
		// Also log to global file with session prefix
		ErrorLog.Printf(fmt.Sprintf("[%s] %s", sessionID, format), v...)
	}
}

var globalLogFile io.WriteCloser

// SessionLoggers holds the loggers for a specific session
type SessionLoggers struct {
	WarningLog *log.Logger
	InfoLog    *log.Logger
	ErrorLog   *log.Logger
	DebugLog   *log.Logger
	LogFile    io.Closer
}

// Global convenience functions for structured logging

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

// levelFilterWriter wraps an io.Writer and filters out logs below a certain level
type levelFilterWriter struct {
	writer    io.Writer
	level     LogLevel
	logLevel  LogLevel // The level of logs this writer should handle
}

// Write implements io.Writer interface with level filtering
func (w *levelFilterWriter) Write(p []byte) (n int, err error) {
	// Only write if the log level is at or above the configured level
	if w.logLevel >= w.level {
		return w.writer.Write(p)
	}
	// Pretend we wrote the data but discard it
	return len(p), nil
}

// newLevelFilterWriter creates a writer that only passes through logs at or above the specified level
func newLevelFilterWriter(writer io.Writer, minLevel LogLevel, logLevel LogLevel) io.Writer {
	return &levelFilterWriter{
		writer:   writer,
		level:    minLevel,
		logLevel: logLevel,
	}
}

func init() {
	sessionLoggers = make(map[string]*SessionLoggers)

	// Initialize default loggers for safety - will be replaced by Initialize/InitializeWithConfig
	// Use a null writer temporarily to avoid premature output
	nullWriter := io.Discard
	if InfoLog == nil {
		InfoLog = log.New(nullWriter, "INFO: ", log.Ldate|log.Ltime)
	}
	if WarningLog == nil {
		WarningLog = log.New(nullWriter, "WARNING: ", log.Ldate|log.Ltime)
	}
	if ErrorLog == nil {
		ErrorLog = log.New(nullWriter, "ERROR: ", log.Ldate|log.Ltime)
	}
	if DebugLog == nil {
		DebugLog = log.New(nullWriter, "DEBUG: ", log.Ldate|log.Ltime)
	}
}

// Initialize should be called once at the beginning of the program to set up logging.
// defer Close() after calling this function. It sets the go log output to the file in
// the configured log directory (default: ~/.claude-squad/logs/).

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
//   log.InitializeForTests(log.DEBUG, log.ERROR)  // DEBUG→file, ERROR→console
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
		testLogDir = filepath.Join(os.TempDir(), "claude-squad-test")
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

	// Try to access the log-related fields using reflection
	return DefaultLogConfig() // Fallback
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
		if err := os.MkdirAll(logDir, 0755); err != nil {
			panic(fmt.Sprintf("could not create log directory: %s", err))
		}

		// No rotation, use standard file
		f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
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
		fmt.Fprintf(os.Stderr, "[LOG INIT] Console output ENABLED - adding stderr to writers\n")
	} else {
		fmt.Fprintf(os.Stderr, "[LOG INIT] Console output DISABLED - stderr will NOT be used\n")
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

	fmt.Fprintf(os.Stderr, "[LOG INIT] Number of writers: %d\n", len(writers))

	// Initialize traditional loggers with level filtering for each stream
	// For dual-stream, we need separate filtering per stream
	if cfg.FileEnabled && cfg.ConsoleEnabled {
		// Dual-stream: create separate filtered writers
		fileWriter := createRotatingWriter(logFilePath, cfg)
		fileFiltered := newLevelFilterWriter(fileWriter, cfg.FileLevel, DEBUG)
		consoleFiltered := newLevelFilterWriter(os.Stderr, cfg.ConsoleLevel, DEBUG)

		// Create multi-writers for each log level
		InfoLog = log.New(io.MultiWriter(
			newLevelFilterWriter(fileWriter, cfg.FileLevel, INFO),
			newLevelFilterWriter(os.Stderr, cfg.ConsoleLevel, INFO),
		), prefix+"INFO:", log.Ldate|log.Ltime|log.Lshortfile)

		WarningLog = log.New(io.MultiWriter(
			newLevelFilterWriter(fileWriter, cfg.FileLevel, WARNING),
			newLevelFilterWriter(os.Stderr, cfg.ConsoleLevel, WARNING),
		), prefix+"WARNING:", log.Ldate|log.Ltime|log.Lshortfile)

		ErrorLog = log.New(io.MultiWriter(
			newLevelFilterWriter(fileWriter, cfg.FileLevel, ERROR),
			newLevelFilterWriter(os.Stderr, cfg.ConsoleLevel, ERROR),
		), prefix+"ERROR:", log.Ldate|log.Ltime|log.Lshortfile)

		DebugLog = log.New(io.MultiWriter(
			newLevelFilterWriter(fileWriter, cfg.FileLevel, DEBUG),
			newLevelFilterWriter(os.Stderr, cfg.ConsoleLevel, DEBUG),
		), prefix+"DEBUG:", log.Ldate|log.Ltime|log.Lshortfile)

		// Initialize structured logger with multi-writer
		if cfg.StructuredLogs {
			structuredLogger = NewStructuredLogger(io.MultiWriter(fileFiltered, consoleFiltered), cfg.FileLevel, cfg.PrettyLogs)
		}
	} else {
		// Single-stream: use existing logic with combined writer
		minLevel := cfg.LogLevel // Use deprecated field for backward compatibility
		if cfg.FileEnabled {
			minLevel = cfg.FileLevel
		} else if cfg.ConsoleEnabled {
			minLevel = cfg.ConsoleLevel
		}

		InfoLog = log.New(newLevelFilterWriter(combinedWriter, minLevel, INFO), prefix+"INFO:", log.Ldate|log.Ltime|log.Lshortfile)
		WarningLog = log.New(newLevelFilterWriter(combinedWriter, minLevel, WARNING), prefix+"WARNING:", log.Ldate|log.Ltime|log.Lshortfile)
		ErrorLog = log.New(newLevelFilterWriter(combinedWriter, minLevel, ERROR), prefix+"ERROR:", log.Ldate|log.Ltime|log.Lshortfile)
		DebugLog = log.New(newLevelFilterWriter(combinedWriter, minLevel, DEBUG), prefix+"DEBUG:", log.Ldate|log.Ltime|log.Lshortfile)

		// Initialize structured logger if enabled
		if cfg.StructuredLogs {
			structuredLogger = NewStructuredLogger(combinedWriter, minLevel, cfg.PrettyLogs)
		}
	}

	// Store the log file path for Close() to report
	logFileName = logFilePath
}

func Close() {
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
