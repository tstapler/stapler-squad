package session

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Backends are reached through ProcessManager methods or the capabilities in
// process_capabilities.go. Asserting a concrete backend type outside
// session/procbackend is how tymux was silently skipped (BUG-113). The pattern
// allows a package qualifier because the backends live in procbackend.
var concreteBackendAssertion = regexp.MustCompile(`\.\(\*(\w+\.)?(TmuxBackend|TymuxBackend)\)`)

// findConcreteBackendAssertions scans non-test Go files under root, skipping any
// procbackend directory (a backend may inspect itself), and returns the offending
// lines plus the number of files scanned.
func findConcreteBackendAssertions(t *testing.T, root string) (hits []string, scanned int) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "procbackend" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for n, line := range strings.Split(string(src), "\n") {
			if concreteBackendAssertion.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", path, n+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits, scanned
}

func TestNoConcreteBackendAssertionsOutsideBackendFiles(t *testing.T) {
	hits, scanned := findConcreteBackendAssertions(t, ".")
	if scanned < 50 {
		t.Fatalf("scanned only %d non-test files under session/: the guard is not looking at the tree", scanned)
	}
	for _, h := range hits {
		t.Errorf("asserts a concrete backend; use a ProcessManager method or a capability: %s", h)
	}
}

// Proves the scan sees a qualified assertion and exempts procbackend.
func TestConcreteBackendAssertionScan_FlagsQualifiedAssertionOutsideProcbackend(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("poller.go", "package x\nvar _ = pm.(*procbackend.TmuxBackend)\n")
	write("sub/bare.go", "package x\nvar _ = pm.(*TymuxBackend)\n")
	write("procbackend/own.go", "package x\nvar _ = pm.(*TmuxBackend)\n")
	write("note.go", "package x\n// pm.(*procbackend.TmuxBackend)\n")

	hits, scanned := findConcreteBackendAssertions(t, root)
	if scanned != 3 {
		t.Errorf("scanned = %d, want 3 (procbackend skipped)", scanned)
	}
	if len(hits) != 2 {
		t.Errorf("hits = %v, want the qualified and the bare assertion only", hits)
	}
}
