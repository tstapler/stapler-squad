package log

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	WarningLog *log.Logger
	InfoLog    *log.Logger
	ErrorLog   *log.Logger

	// Global config reference
	globalConfig *LogConfig

	// Session loggers map (sessionID -> loggers)
	sessionLoggers map[string]*SessionLoggers
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
}

// DefaultLogConfig returns the default logging configuration
func DefaultLogConfig() *LogConfig {
	return &LogConfig{
		LogsEnabled:    true,
		LogsDir:        "",
		LogMaxSize:     10,  // 10MB
		LogMaxFiles:    5,   // 5 backups
		LogMaxAge:      30,  // 30 days
		LogCompress:    true,
		UseSessionLogs: true,
	}
}

// Default log directory and filename
var logFileName = filepath.Join(os.TempDir(), "claudesquad.log")

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
	// Check if we already have loggers for this session
	if loggers, exists := sessionLoggers[sessionID]; exists {
		return loggers, nil
	}

	// If session logs are disabled in config, return nil
	if globalConfig != nil && !globalConfig.UseSessionLogs {
		return nil, nil
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
	LogFile    io.Closer
}

func init() {
	sessionLoggers = make(map[string]*SessionLoggers)
	
	// Initialize default loggers for testing environments
	// This ensures that log calls don't panic when tests are run
	if InfoLog == nil {
		InfoLog = log.New(os.Stderr, "INFO: ", log.Ldate|log.Ltime)
	}
	if WarningLog == nil {
		WarningLog = log.New(os.Stderr, "WARNING: ", log.Ldate|log.Ltime)
	}
	if ErrorLog == nil {
		ErrorLog = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime)
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
		MaxSize:    cfg.LogMaxSize,    // megabytes
		MaxBackups: cfg.LogMaxFiles,   // number of backups
		MaxAge:     cfg.LogMaxAge,     // days
		Compress:   cfg.LogCompress,   // compress rotated files
		LocalTime:  true,             // use local time in backup filenames
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
		fmt.Printf("Warning: Using default log file location due to error: %v\n", err)
		logFilePath = logFileName
	}

	// Create rotating writer for logs
	writer := createRotatingWriter(logFilePath, cfg)

	// Set log format to include timestamp and file/line number
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	fmtS := "%s"
	if daemon {
		fmtS = "[DAEMON] %s"
	}
	InfoLog = log.New(writer, fmt.Sprintf(fmtS, "INFO:"), log.Ldate|log.Ltime|log.Lshortfile)
	WarningLog = log.New(writer, fmt.Sprintf(fmtS, "WARNING:"), log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLog = log.New(writer, fmt.Sprintf(fmtS, "ERROR:"), log.Ldate|log.Ltime|log.Lshortfile)

	// Store the closer
	if closer, ok := writer.(io.Closer); ok {
		globalLogFile = closer.(io.WriteCloser)
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

	// TODO: maybe only print if verbose flag is set?
	fmt.Println("wrote logs to " + logFileName)
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
