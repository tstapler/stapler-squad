// Package log stands in for the real log package at its import path, so analyzers keyed on that path fire on seeds.
package log

type Logger struct{}

func (l *Logger) Printf(format string, args ...interface{}) {}

func WarningLog() *Logger { return &Logger{} }
