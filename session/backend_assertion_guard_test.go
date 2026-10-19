package session

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Instance code reaches backends through ProcessManager methods or the capabilities in
// process_capabilities.go. Asserting a concrete backend type there is how tymux was
// silently skipped (BUG-113); each backend's own file may inspect itself.
func TestNoConcreteBackendAssertionsOutsideBackendFiles(t *testing.T) {
	assertion := regexp.MustCompile(`\.\(\*(TmuxBackend|TymuxBackend)\)`)
	own := map[string]bool{"tmux_backend.go": true, "backend_tymux.go": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || own[f] {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(src), "\n") {
			if assertion.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				t.Errorf("%s:%d asserts a concrete backend; use a ProcessManager method or a capability: %s", f, n+1, strings.TrimSpace(line))
			}
		}
	}
}
