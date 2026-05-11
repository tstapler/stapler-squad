package log

import (
	"context"
	"io"
	stdlog "log"
	"sync"
)

// LogManager encapsulates all log state that was previously in package-level globals.
// Use NewLogManager to create one; use the package-level functions (InfoLog, etc.) via
// the defaultManager for zero-migration compatibility.
type LogManager struct {
	config       *LogConfig
	infoLog      *stdlog.Logger
	warnLog      *stdlog.Logger
	errorLog     *stdlog.Logger
	debugLog     *stdlog.Logger
	logFile      io.WriteCloser
	structured   *StructuredLogger
	asyncHandler *AsyncHandler
	// asyncFileWriter and asyncConsoleWriter are drained before logFile is closed.
	asyncFileWriter    *asyncWriter
	asyncConsoleWriter *asyncWriter

	sessionsMu sync.RWMutex
	sessions   map[string]*SessionLoggers
}

// newLogManager constructs a LogManager from an already-initialised set of loggers.
// It is called at the end of initializeWithConfig to capture the configured state.
func newLogManager(
	cfg *LogConfig,
	info, warn, errL, debug *stdlog.Logger,
	logFile io.WriteCloser,
	structured *StructuredLogger,
	async *AsyncHandler,
	asyncFileW, asyncConsoleW *asyncWriter,
) *LogManager {
	return &LogManager{
		config:             cfg,
		infoLog:            info,
		warnLog:            warn,
		errorLog:           errL,
		debugLog:           debug,
		logFile:            logFile,
		structured:         structured,
		asyncHandler:       async,
		asyncFileWriter:    asyncFileW,
		asyncConsoleWriter: asyncConsoleW,
		sessions:           make(map[string]*SessionLoggers),
	}
}

// ForSession returns or creates session-scoped loggers.
func (m *LogManager) ForSession(id string) (*SessionLoggers, error) {
	m.sessionsMu.RLock()
	if l, ok := m.sessions[id]; ok {
		m.sessionsMu.RUnlock()
		return l, nil
	}
	m.sessionsMu.RUnlock()

	if m.config != nil && !m.config.UseSessionLogs {
		return nil, ErrSessionLogsDisabled
	}

	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()
	if l, ok := m.sessions[id]; ok {
		return l, nil
	}

	logFilePath, err := GetSessionLogFilePath(m.config, id)
	if err != nil {
		return nil, err
	}
	writer := createRotatingWriter(logFilePath, m.config)
	loggers := &SessionLoggers{
		InfoLog:    stdlog.New(writer, "["+id+"] INFO: ", stdlog.Ldate|stdlog.Ltime|stdlog.Lshortfile),
		WarningLog: stdlog.New(writer, "["+id+"] WARNING: ", stdlog.Ldate|stdlog.Ltime|stdlog.Lshortfile),
		ErrorLog:   stdlog.New(writer, "["+id+"] ERROR: ", stdlog.Ldate|stdlog.Ltime|stdlog.Lshortfile),
		DebugLog:   stdlog.New(writer, "["+id+"] DEBUG: ", stdlog.Ldate|stdlog.Ltime|stdlog.Lshortfile),
	}
	if closer, ok := writer.(io.Closer); ok {
		loggers.LogFile = closer
	}

	// Evict oldest sessions when map exceeds 500 entries.
	if len(m.sessions) >= 500 {
		for k := range m.sessions {
			delete(m.sessions, k)
			break // delete one arbitrary entry
		}
	}

	m.sessions[id] = loggers
	return loggers, nil
}

// CloseSession removes session-scoped loggers and closes their file handle.
func (m *LogManager) CloseSession(id string) {
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()
	if l, ok := m.sessions[id]; ok {
		if l.LogFile != nil {
			_ = l.LogFile.Close()
		}
		delete(m.sessions, id)
	}
}

// Close drains async writers, flushes the slog handler, and closes all log files.
// Drain order matters: async writers must be drained before the underlying file is
// closed, otherwise buffered entries are lost.
func (m *LogManager) Close() {
	// Drain stdlib log async writers first — they write to the underlying file.
	if m.asyncConsoleWriter != nil {
		_ = m.asyncConsoleWriter.Close()
	}
	if m.asyncFileWriter != nil {
		_ = m.asyncFileWriter.Close()
	}
	// Flush the slog async handler after the stdlib writers are drained.
	if m.asyncHandler != nil {
		_ = m.asyncHandler.Flush(context.Background())
	}
	if m.logFile != nil {
		_ = m.logFile.Close()
	}
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()
	for _, l := range m.sessions {
		if l.LogFile != nil {
			_ = l.LogFile.Close()
		}
	}
}
