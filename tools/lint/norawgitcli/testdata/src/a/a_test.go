package a

import (
	"os/exec"
	"testing"
)

// Test files are exempt: they build git fixtures.
func TestFixture(t *testing.T) { _ = exec.Command("git", "init") }
