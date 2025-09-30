package overlay

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// ExpandPath expands the tilde (~) in a path to the user's home directory
// and returns the absolute path.
func ExpandPath(path string) (string, error) {
	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		usr, err := user.Current()
		if err != nil {
			return path, err
		}
		path = filepath.Join(usr.HomeDir, path[2:])
	} else if path == "~" {
		usr, err := user.Current()
		if err != nil {
			return path, err
		}
		path = usr.HomeDir
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path, err
	}

	return absPath, nil
}

// PathExists checks if a path exists on the filesystem
func PathExists(path string) bool {
	// Expand the path first
	expandedPath, err := ExpandPath(path)
	if err != nil {
		return false
	}

	// Check if path exists
	_, err = os.Stat(expandedPath)
	return err == nil
}

// IsDirectory checks if a path is a directory
func IsDirectory(path string) bool {
	// Expand the path first
	expandedPath, err := ExpandPath(path)
	if err != nil {
		return false
	}

	// Check if path is a directory
	info, err := os.Stat(expandedPath)
	if err != nil {
		return false
	}

	return info.IsDir()
}

// PathValidationResult contains detailed information about path validation
type PathValidationResult struct {
	Valid        bool
	ExpandedPath string
	Error        error
	ErrorMessage string
	Warnings     []string
	IsGitRepo    bool
	Permissions  PathPermissions
}

// PathPermissions contains information about path access permissions
type PathPermissions struct {
	Readable   bool
	Writable   bool
	Executable bool
}

// ValidatePathEnhanced performs comprehensive path validation with detailed error reporting
func ValidatePathEnhanced(path string) PathValidationResult {
	result := PathValidationResult{
		Valid:     false,
		Warnings:  []string{},
	}

	// Early validation for empty paths
	if strings.TrimSpace(path) == "" {
		result.Error = errors.New("empty path")
		result.ErrorMessage = "Path cannot be empty"
		return result
	}

	// Sanitize and validate path characters
	if containsInvalidChars(path) {
		result.Error = errors.New("invalid characters")
		result.ErrorMessage = fmt.Sprintf("Path contains invalid characters: %s", path)
		return result
	}

	// Expand the path
	expandedPath, err := ExpandPath(path)
	if err != nil {
		result.Error = err
		switch {
		case strings.Contains(err.Error(), "user"):
			result.ErrorMessage = "Could not determine home directory. Please use an absolute path."
		default:
			result.ErrorMessage = fmt.Sprintf("Path expansion failed: %s", err.Error())
		}
		return result
	}
	result.ExpandedPath = expandedPath

	// Check if path exists
	info, err := os.Stat(expandedPath)
	if err != nil {
		result.Error = err
		if os.IsNotExist(err) {
			// Check if parent directory exists for better error messages
			parentDir := filepath.Dir(expandedPath)
			if parentDir != expandedPath && PathExists(parentDir) {
				result.ErrorMessage = fmt.Sprintf("Directory '%s' does not exist. Parent directory '%s' exists.",
					filepath.Base(expandedPath), getDisplayPath(parentDir))
			} else {
				result.ErrorMessage = fmt.Sprintf("Path does not exist: %s", getDisplayPath(expandedPath))
			}
		} else if os.IsPermission(err) {
			result.ErrorMessage = fmt.Sprintf("Permission denied accessing: %s", getDisplayPath(expandedPath))
		} else {
			result.ErrorMessage = fmt.Sprintf("Cannot access path: %s (%s)", getDisplayPath(expandedPath), err.Error())
		}
		return result
	}

	// Check if it's a directory
	if !info.IsDir() {
		result.Error = errors.New("not a directory")
		result.ErrorMessage = fmt.Sprintf("Path is not a directory: %s", getDisplayPath(expandedPath))
		return result
	}

	// Check permissions
	result.Permissions = checkPathPermissions(expandedPath)

	// Add warnings for permission issues
	if !result.Permissions.Readable {
		result.Warnings = append(result.Warnings, "Directory is not readable")
	}
	if !result.Permissions.Writable {
		result.Warnings = append(result.Warnings, "Directory is not writable")
	}

	// Check if it's a Git repository
	result.IsGitRepo = isGitRepository(expandedPath)

	// Add informational warnings
	if isSymlink(expandedPath) {
		result.Warnings = append(result.Warnings, "Path is a symbolic link")
	}

	if isNetworkPath(expandedPath) {
		result.Warnings = append(result.Warnings, "Network path detected - performance may be slower")
	}

	// Path is valid
	result.Valid = true
	return result
}

// containsInvalidChars checks for characters that are invalid in file paths
func containsInvalidChars(path string) bool {
	// Allow most characters, but reject null bytes and other problematic chars
	invalidChars := []string{"\x00"} // Null byte

	for _, char := range invalidChars {
		if strings.Contains(path, char) {
			return true
		}
	}

	// Check for extremely long paths (most filesystems have limits)
	if len(path) > 4096 {
		return true
	}

	return false
}

// checkPathPermissions checks read/write/execute permissions for a path with graceful degradation
func checkPathPermissions(path string) PathPermissions {
	perms := PathPermissions{}

	// Check read permission with timeout for network paths
	if file, err := os.Open(path); err == nil {
		perms.Readable = true
		file.Close()
	} else if os.IsPermission(err) {
		// Explicitly denied - readable is false but we know why
		perms.Readable = false
	} else {
		// Other error (network timeout, path doesn't exist, etc.)
		perms.Readable = false
	}

	// Check write permission by attempting to create a temporary file
	// Use a more unique temporary file name to avoid conflicts
	tempFile := filepath.Join(path, fmt.Sprintf(".claude-squad-test-write-%d", os.Getpid()))
	if file, err := os.Create(tempFile); err == nil {
		perms.Writable = true
		file.Close()
		os.Remove(tempFile) // Clean up
	} else if os.IsPermission(err) {
		// Permission explicitly denied
		perms.Writable = false
	} else {
		// Other errors (read-only filesystem, network issues, etc.)
		perms.Writable = false
	}

	// Check execute permission (ability to traverse directory)
	// This is crucial for Git operations and directory browsing
	if _, err := os.ReadDir(path); err == nil {
		perms.Executable = true
	} else if os.IsPermission(err) {
		// Permission explicitly denied
		perms.Executable = false
	} else {
		// Other errors (network timeout, path issues, etc.)
		perms.Executable = false
	}

	return perms
}

// isSymlink checks if a path is a symbolic link
func isSymlink(path string) bool {
	if info, err := os.Lstat(path); err == nil {
		return info.Mode()&os.ModeSymlink != 0
	}
	return false
}

// isNetworkPath detects if a path might be on a network filesystem
func isNetworkPath(path string) bool {
	// Enhanced network path detection with more patterns
	networkPrefixes := []string{
		"//",           // UNC paths
		"/mnt/",        // Common mount point
		"/net/",        // Network mount point
		"/media/",      // Media mount point (often network)
		"/run/media/",  // Systemd media mount
		"/Volumes/",    // macOS network volumes
	}

	networkIndicators := []string{
		"nfs", "smb", "cifs", "ftp", "sftp", "sshfs",
		"afs", "ncp", "davfs", "9p", "gvfs",
	}

	// Check for network prefixes
	for _, prefix := range networkPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	// Check for network indicators in path components
	pathLower := strings.ToLower(path)
	for _, indicator := range networkIndicators {
		if strings.Contains(pathLower, indicator) {
			return true
		}
	}

	return false
}

// isGitRepository checks if a directory contains a .git directory
func isGitRepository(path string) bool {
	gitPath := filepath.Join(path, ".git")
	if info, err := os.Stat(gitPath); err == nil {
		return info.IsDir()
	}
	return false
}

// getDisplayPath converts a path to a user-friendly display format
func getDisplayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}

	return path
}

// ValidatePathQuick provides a quick path validation for performance-sensitive operations
func ValidatePathQuick(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path cannot be empty")
	}

	expandedPath, err := ExpandPath(path)
	if err != nil {
		return fmt.Errorf("path expansion failed: %w", err)
	}

	if !PathExists(expandedPath) {
		return fmt.Errorf("path does not exist: %s", getDisplayPath(expandedPath))
	}

	if !IsDirectory(expandedPath) {
		return fmt.Errorf("path is not a directory: %s", getDisplayPath(expandedPath))
	}

	return nil
}
