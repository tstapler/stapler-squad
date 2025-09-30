package app

import (
	"claude-squad/terminal"
)

// Compatibility wrapper for existing code that uses app-level terminal size functions
// These functions delegate to the new terminal module

// TerminalSizeInfo is deprecated - use terminal.SizeInfo instead
type TerminalSizeInfo = terminal.SizeInfo

// DetectTerminalSize is deprecated - use terminal.Manager.DetectSize() instead
func DetectTerminalSize() *TerminalSizeInfo {
	return terminal.DetectTerminalSize()
}

// GetReliableTerminalSize is deprecated - use terminal.Manager.GetReliableSize() instead
func GetReliableTerminalSize() (width, height int, method string) {
	return terminal.GetReliableTerminalSize()
}