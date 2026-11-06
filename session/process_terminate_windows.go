//go:build windows

package session

import "fmt"

// terminateProcess is not supported on Windows -- SIGTERM has no Windows
// equivalent exposed via syscall.Kill (mirrors process_suspend_windows.go).
func terminateProcess(_ int32) error {
	return fmt.Errorf("terminateProcess: not supported on Windows")
}
