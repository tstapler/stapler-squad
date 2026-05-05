package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// helperBin is the path to the compiled testdata/helper binary.
// Set in TestMain; used by all test functions in this package.
var helperBin string

func TestMain(m *testing.M) {
	exe, err := buildHelper()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build testdata/helper: %v\n", err)
		os.Exit(1)
	}
	helperBin = exe
	os.Exit(m.Run())
}

// buildHelper compiles the testdata/helper binary and returns its path.
// The binary is placed in a temporary directory that persists for the test run.
func buildHelper() (string, error) {
	tmpDir, err := os.MkdirTemp("", "safeexec-helper-*")
	if err != nil {
		return "", err
	}
	exe := filepath.Join(tmpDir, "helper")
	// Use the current module path to find testdata/helper.
	cmd := exec.Command("go", "build", "-o", exe, "./testdata/helper")
	cmd.Dir = filepath.Join(findModuleRoot(), "executor")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build helper: %w\n%s", err, out)
	}
	return exe, nil
}

// findModuleRoot walks up from the current file's directory to find go.mod.
func findModuleRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
