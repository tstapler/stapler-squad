package log

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetLogDir(t *testing.T) {
	// Test with nil config
	dir, err := GetLogDir(nil)
	if err != nil {
		t.Errorf("GetLogDir failed with nil config: %v", err)
	}
	if dir == "" {
		t.Error("GetLogDir returned empty string for nil config")
	}

	// Test with disabled logging
	cfg := &LogConfig{
		LogsEnabled: false,
	}
	dir, err = GetLogDir(cfg)
	if err != nil {
		t.Errorf("GetLogDir failed with disabled logging: %v", err)
	}
	if dir != os.TempDir() {
		t.Errorf("GetLogDir should return temp dir for disabled logging, got %s", dir)
	}

	// Test with custom log dir
	cfg = &LogConfig{
		LogsEnabled: true,
		LogsDir:     "/custom/log/dir",
	}
	dir, err = GetLogDir(cfg)
	if err != nil {
		t.Errorf("GetLogDir failed with custom log dir: %v", err)
	}
	if dir != "/custom/log/dir" {
		t.Errorf("GetLogDir should return custom log dir, got %s", dir)
	}

	// Test with default log dir
	cfg = &LogConfig{
		LogsEnabled: true,
		LogsDir:     "",
	}
	dir, err = GetLogDir(cfg)
	if err != nil {
		t.Errorf("GetLogDir failed with default log dir: %v", err)
	}

	// Should contain .claude-squad/logs
	if !strings.Contains(dir, ".claude-squad"+string(filepath.Separator)+"logs") {
		t.Errorf("GetLogDir should return default log dir, got %s", dir)
	}
}

func TestGetLogFilePath(t *testing.T) {
	// Test with default config
	cfg := &LogConfig{
		LogsEnabled: true,
		LogsDir:     "",
	}
	path, err := GetLogFilePath(cfg)
	if err != nil {
		t.Errorf("GetLogFilePath failed with default config: %v", err)
	}
	if !strings.HasSuffix(path, "claudesquad.log") {
		t.Errorf("GetLogFilePath should end with claudesquad.log, got %s", path)
	}

	// Test with custom log dir
	cfg = &LogConfig{
		LogsEnabled: true,
		LogsDir:     "/custom/log/dir",
	}
	path, err = GetLogFilePath(cfg)
	if err != nil {
		t.Errorf("GetLogFilePath failed with custom log dir: %v", err)
	}
	if path != "/custom/log/dir/claudesquad.log" {
		t.Errorf("GetLogFilePath should return custom log path, got %s", path)
	}
}

func TestGetSessionLogFilePath(t *testing.T) {
	// Test with default config
	cfg := &LogConfig{
		LogsEnabled: true,
		LogsDir:     "",
	}
	path, err := GetSessionLogFilePath(cfg, "test-session")
	if err != nil {
		t.Errorf("GetSessionLogFilePath failed with default config: %v", err)
	}
	if !strings.HasSuffix(path, "session_test-session.log") {
		t.Errorf("GetSessionLogFilePath should end with session_test-session.log, got %s", path)
	}

	// Test with custom log dir
	cfg = &LogConfig{
		LogsEnabled: true,
		LogsDir:     "/custom/log/dir",
	}
	path, err = GetSessionLogFilePath(cfg, "test-session")
	if err != nil {
		t.Errorf("GetSessionLogFilePath failed with custom log dir: %v", err)
	}
	if path != "/custom/log/dir/session_test-session.log" {
		t.Errorf("GetSessionLogFilePath should return custom log path, got %s", path)
	}

	// Test with session ID containing invalid characters
	path, err = GetSessionLogFilePath(cfg, "test/session:with*invalid#chars")
	if err != nil {
		t.Errorf("GetSessionLogFilePath failed with invalid session ID: %v", err)
	}
	if path != "/custom/log/dir/session_test-session-with-invalid-chars.log" {
		t.Errorf("GetSessionLogFilePath should sanitize invalid characters, got %s", path)
	}
}

func TestCreateRotatingWriter(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "claude-squad-log-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test with nil config
	writer := createRotatingWriter(filepath.Join(tempDir, "test.log"), nil)
	if writer == nil {
		t.Error("createRotatingWriter returned nil with nil config")
	}

	// Test with zero max size
	cfg := &LogConfig{
		LogMaxSize: 0,
	}
	writer = createRotatingWriter(filepath.Join(tempDir, "test.log"), cfg)
	if writer == nil {
		t.Error("createRotatingWriter returned nil with zero max size")
	}

	// Test with valid max size (should create lumberjack.Logger)
	// Since we can't easily verify the writer is a lumberjack.Logger directly,
	// we'll just check it's not nil
	cfg = &LogConfig{
		LogMaxSize:  10,
		LogMaxFiles: 5,
		LogMaxAge:   30,
		LogCompress: true,
	}
	writer = createRotatingWriter(filepath.Join(tempDir, "test.log"), cfg)
	if writer == nil {
		t.Error("createRotatingWriter returned nil with valid config")
	}
}

func TestLogForSession(t *testing.T) {
	// Create a temporary log directory
	tempDir, err := os.MkdirTemp("", "claude-squad-log-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set up config with session logs enabled
	cfg := &LogConfig{
		LogsEnabled:    true,
		LogsDir:        tempDir,
		UseSessionLogs: true,
	}

	// Store the global config
	globalConfig = cfg

	// Initialize global loggers
	logPath := filepath.Join(tempDir, "claudesquad.log")
	file, _ := os.Create(logPath)
	InfoLog = NewDummyLogger(file, "INFO: ")
	WarningLog = NewDummyLogger(file, "WARNING: ")
	ErrorLog = NewDummyLogger(file, "ERROR: ")

	// Initialize session loggers map
	sessionLoggers = make(map[string]*SessionLoggers)

	// Test logging for a session
	sessionID := "test-session"
	LogForSession(sessionID, "info", "Test info message")
	LogForSession(sessionID, "warning", "Test warning message")
	LogForSession(sessionID, "error", "Test error message")

	// Verify session log file was created
	sessionLogPath := filepath.Join(tempDir, "session_test-session.log")
	if _, err := os.Stat(sessionLogPath); os.IsNotExist(err) {
		t.Errorf("Session log file not created at %s", sessionLogPath)
	}

	// Test with session logs disabled
	cfg.UseSessionLogs = false
	globalConfig = cfg
	anotherSessionID := "another-session"
	LogForSession(anotherSessionID, "info", "Test info message")

	// Verify session log file was not created
	anotherSessionLogPath := filepath.Join(tempDir, "session_another-session.log")
	if _, err := os.Stat(anotherSessionLogPath); !os.IsNotExist(err) {
		t.Errorf("Session log file should not be created when UseSessionLogs is false")
	}
}

// NewDummyLogger creates a test logger that doesn't panic on write errors
func NewDummyLogger(w io.Writer, prefix string) *log.Logger {
	return log.New(w, prefix, 0)
}
