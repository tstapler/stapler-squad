package log

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// withCleanEnv unsets STAPLER_SQUAD_TEST_DIR and STAPLER_SQUAD_INSTANCE, restoring
// their prior values via t.Cleanup. Do not run tests using this helper with
// t.Parallel() — they mutate process-global env state.
func withCleanEnv(t *testing.T) {
	t.Helper()
	originalTestDir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	originalInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
	os.Unsetenv("STAPLER_SQUAD_INSTANCE")
	t.Cleanup(func() {
		if originalTestDir == "" {
			os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
		} else {
			os.Setenv("STAPLER_SQUAD_TEST_DIR", originalTestDir)
		}
		if originalInstance == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", originalInstance)
		}
	})
}

func TestGetConfigDir(t *testing.T) {
	tests := []struct {
		name            string
		instance        string // "" = leave STAPLER_SQUAD_INSTANCE unset
		wantSuffix      string
		wantNoInstances bool // dir must not contain "instances"
	}{
		{
			name:            "unset instance returns unchanged base dir",
			wantSuffix:      ".stapler-squad",
			wantNoInstances: true,
		},
		{
			name:            "shared instance returns unchanged base dir",
			instance:        "shared",
			wantSuffix:      ".stapler-squad",
			wantNoInstances: true,
		},
		{
			name:       "named instance returns instance-scoped path",
			instance:   "alpha",
			wantSuffix: filepath.Join(".stapler-squad", "instances", "alpha"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCleanEnv(t)
			t.Setenv("HOME", t.TempDir()) // keep the real-homedir fallback off the developer's actual ~/.stapler-squad
			if tt.instance != "" {
				os.Setenv("STAPLER_SQUAD_INSTANCE", tt.instance)
			}

			dir, err := GetConfigDir()
			if err != nil {
				t.Fatalf("GetConfigDir failed: %v", err)
			}
			if !strings.HasSuffix(dir, tt.wantSuffix) {
				t.Errorf("expected path ending in %s, got %s", tt.wantSuffix, dir)
			}
			if tt.wantNoInstances && strings.Contains(dir, "instances") {
				t.Errorf("expected no instances/ segment, got %s", dir)
			}
		})
	}

	t.Run("STAPLER_SQUAD_TEST_DIR wins outright over STAPLER_SQUAD_INSTANCE", func(t *testing.T) {
		withCleanEnv(t)
		testDir := filepath.Join(t.TempDir(), "custom-test-dir")
		os.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
		os.Setenv("STAPLER_SQUAD_INSTANCE", "alpha")

		dir, err := GetConfigDir()
		if err != nil {
			t.Fatalf("GetConfigDir failed: %v", err)
		}
		if dir != testDir {
			t.Errorf("expected STAPLER_SQUAD_TEST_DIR to win, got %s want %s", dir, testDir)
		}
		if info, statErr := os.Stat(testDir); statErr != nil || !info.IsDir() {
			t.Errorf("expected GetConfigDir to create %s via MkdirAll", testDir)
		}
	})

	for _, bad := range []string{"../escape", "a/../../etc", "nested/segment", `back\slash`} {
		t.Run("rejects instance ID "+bad, func(t *testing.T) {
			withCleanEnv(t)
			t.Setenv("HOME", t.TempDir())
			os.Setenv("STAPLER_SQUAD_INSTANCE", bad)

			if _, err := GetConfigDir(); err == nil {
				t.Errorf("expected error for STAPLER_SQUAD_INSTANCE=%q, got nil", bad)
			}
		})
	}
}

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

	// Should contain .stapler-squad/logs
	if !strings.Contains(dir, ".stapler-squad"+string(filepath.Separator)+"logs") {
		t.Errorf("GetLogDir should return default log dir, got %s", dir)
	}
}

func TestGetLogDir_NamedInstance_CreatesNestedInstanceLogsDir(t *testing.T) {
	withCleanEnv(t)
	t.Setenv("HOME", t.TempDir()) // GetLogDir's MkdirAll must not land in the real ~/.stapler-squad
	os.Setenv("STAPLER_SQUAD_INSTANCE", "alpha")

	dir, err := GetLogDir(&LogConfig{LogsEnabled: true})
	if err != nil {
		t.Fatalf("GetLogDir failed: %v", err)
	}

	wantSuffix := filepath.Join("instances", "alpha", "logs")
	if !strings.HasSuffix(dir, wantSuffix) {
		t.Errorf("expected path ending in %s, got %s", wantSuffix, dir)
	}
	if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
		t.Errorf("expected GetLogDir to create %s via MkdirAll", dir)
	}
}

func TestGetLogDir_LogsDirOverride_WinsOverSTAPLER_SQUAD_INSTANCE(t *testing.T) {
	withCleanEnv(t)
	os.Setenv("STAPLER_SQUAD_INSTANCE", "alpha")

	dir, err := GetLogDir(&LogConfig{LogsEnabled: true, LogsDir: "/custom/log/dir"})
	if err != nil {
		t.Fatalf("GetLogDir failed: %v", err)
	}
	if dir != "/custom/log/dir" {
		t.Errorf("expected LogsDir override to win regardless of instance, got %s", dir)
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
	if !strings.HasSuffix(path, "staplersquad.log") {
		t.Errorf("GetLogFilePath should end with staplersquad.log, got %s", path)
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
	if path != "/custom/log/dir/staplersquad.log" {
		t.Errorf("GetLogFilePath should return custom log path, got %s", path)
	}
}

func TestGetLogFilePath_NamedInstance_DivergesFromDefaultInstance(t *testing.T) {
	withCleanEnv(t)
	t.Setenv("HOME", t.TempDir()) // GetLogFilePath's underlying MkdirAll must not land in the real ~/.stapler-squad

	cfg := &LogConfig{LogsEnabled: true}

	os.Setenv("STAPLER_SQUAD_INSTANCE", "alpha")
	alphaPath, err := GetLogFilePath(cfg)
	if err != nil {
		t.Fatalf("GetLogFilePath failed for alpha: %v", err)
	}

	os.Setenv("STAPLER_SQUAD_INSTANCE", "beta")
	betaPath, err := GetLogFilePath(cfg)
	if err != nil {
		t.Fatalf("GetLogFilePath failed for beta: %v", err)
	}

	if alphaPath == betaPath {
		t.Errorf("expected distinct log paths per instance, both got %s", alphaPath)
	}
	for name, path := range map[string]string{"alpha": alphaPath, "beta": betaPath} {
		wantSuffix := filepath.Join("instances", name, "logs", "staplersquad.log")
		if !strings.HasSuffix(path, wantSuffix) {
			t.Errorf("expected %s path ending in %s, got %s", name, wantSuffix, path)
		}
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
	tempDir, err := os.MkdirTemp("", "stapler-squad-log-test")
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
	tempDir, err := os.MkdirTemp("", "stapler-squad-log-test")
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
	logPath := filepath.Join(tempDir, "staplersquad.log")
	file, _ := os.Create(logPath)
	SetInfoLogForTest(NewDummyLogger(file, "INFO: "))
	SetWarningLogForTest(NewDummyLogger(file, "WARNING: "))
	SetErrorLogForTest(NewDummyLogger(file, "ERROR: "))

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

// TestAtomicLoggerConcurrentAccess exercises the atomicLogger accessor under
// -race: one set of goroutines repeatedly swaps the warning logger while
// another set concurrently loads and uses it. Before atomicLogger, the
// equivalent bare package-var reassignment raced with these reads; this test
// guards against that regression reappearing.
// TestSetRuntimeLevel_UpdatesSlogLevel guards against the bug this fixes: the
// slog-based Debug/Info/Warn/Error logger was wired to a hardcoded
// slog.LevelDebug and never consulted SetRuntimeLevel, so Debug() always
// emitted regardless of the configured runtime level.
func TestSetRuntimeLevel_UpdatesSlogLevel(t *testing.T) {
	t.Cleanup(func() { SetRuntimeLevel(INFO) })

	SetRuntimeLevel(WARNING)
	if got := slogLevel.Level(); got != slog.LevelWarn {
		t.Errorf("slogLevel = %v, want %v", got, slog.LevelWarn)
	}

	SetRuntimeLevel(DEBUG)
	if got := slogLevel.Level(); got != slog.LevelDebug {
		t.Errorf("slogLevel = %v, want %v", got, slog.LevelDebug)
	}
}

func TestAtomicLoggerConcurrentAccess(t *testing.T) {
	orig := SetWarningLogForTest(NewDummyLogger(io.Discard, "WARNING: "))
	defer SetWarningLogForTest(orig)

	const writers = 4
	const readers = 8
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				SetWarningLogForTest(NewDummyLogger(io.Discard, "WARNING: "))
			}
		}()
	}

	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				l := WarningLog()
				if l == nil {
					t.Error("WarningLog() returned nil during concurrent access")
					return
				}
				l.Printf("concurrent read %d", j)
			}
		}()
	}

	wg.Wait()
}
