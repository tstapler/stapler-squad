package overlay

import (
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
