// Package log contains a minimal stand-in for the real
// github.com/tstapler/stapler-squad/log package (log/log.go), resolved via
// analysistest's GOPATH-style testdata overlay at exactly the real import
// path so nolegacylog's type-based package check activates during the test.
package log

// Logger stands in for the real *log.Logger the legacy accessors return.
type Logger struct{}

// Printf stands in for the real (*log.Logger).Printf.
func (l *Logger) Printf(format string, args ...interface{}) {}

func InfoLog() *Logger    { return &Logger{} }
func WarningLog() *Logger { return &Logger{} }
func ErrorLog() *Logger   { return &Logger{} }
func DebugLog() *Logger   { return &Logger{} }

// Info/Warn/Error/Debug stand in for the real modern, structured API
// (log/log.go:681-692) — never flagged by nolegacylog.
func Info(msg string, args ...any)  {}
func Warn(msg string, args ...any)  {}
func Error(msg string, args ...any) {}
func Debug(msg string, args ...any) {}
