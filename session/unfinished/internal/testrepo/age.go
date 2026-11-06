package testrepo

import (
	"os"
	"testing"
	"time"
)

// Age backdates the atime and mtime of a file or directory by d via
// os.Chtimes, leaving content untouched. ctime cannot be set, so tests that
// exercise the 2 s racy-timestamp guards must also inject a clock read at
// least 10 s ahead of the real file-system now.
func Age(t testing.TB, path string, d time.Duration) {
	t.Helper()
	when := time.Now().Add(-d)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("testrepo: Age %s: %v", path, err)
	}
}
