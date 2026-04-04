package terminal

// Package-level functions for backward compatibility
// These maintain the existing API while using the new Manager internally

var defaultManager = NewManager()

// GetReliableTerminalSize returns the most reliable terminal dimensions
// This function maintains backward compatibility with existing code
func GetReliableTerminalSize() (width, height int, method string) {
	return defaultManager.GetReliableSize()
}

// DetectTerminalSize provides backward compatibility with the old function
func DetectTerminalSize() *SizeInfo {
	return defaultManager.DetectSize()
}
