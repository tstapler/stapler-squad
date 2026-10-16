// Package socket provides test helpers for AF_UNIX socket paths. It is
// intentionally dependency-free (no import of the session package tree) so
// it can be imported from test files anywhere, including packages that
// session itself depends on, without creating an import cycle.
package socket

import (
	"os"
	"testing"
)

// ShortTempSocketDir returns a short-lived temp directory suitable for
// unix-domain-socket paths, which are limited to 104 bytes on macOS
// (sockaddr_un.sun_path). Unlike t.TempDir(), which embeds the full test
// function name in the path, this uses a short fixed prefix so basePath +
// a socket filename stays well under that limit.
func ShortTempSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ssq-appr-")
	if err != nil {
		t.Fatalf("os.MkdirTemp() error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}
