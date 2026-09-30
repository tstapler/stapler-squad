package backend

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

// scriptServicePatternRegex extracts SERVICE_PATTERN='...' from
// tools/scanner/list-backend-protos.sh, which hand-copies servicePattern (above) into a
// POSIX ERE for bash's `grep -qE` since bash can't share a compiled Go regexp. Extracting
// the live pattern from the script file (rather than hardcoding a second copy here) means
// this test fails the moment either copy is edited without the other, instead of a
// hardcoded duplicate silently drifting the same way the original per-file hardcoded
// proto lists did (see TestMethodToIDCompleteness's doc comment).
func scriptServicePatternRegex(t *testing.T) *regexp.Regexp {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	scriptPath := filepath.Join(repoRoot, "tools", "scanner", "list-backend-protos.sh")

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", scriptPath, err)
	}
	m := regexp.MustCompile(`SERVICE_PATTERN='([^']*)'`).FindSubmatch(content)
	if m == nil {
		t.Fatalf("could not find SERVICE_PATTERN=... in %s", scriptPath)
	}
	return regexp.MustCompile(string(m[1]))
}

// TestServicePatternParity_BashScriptMatchesGoRegex feeds edge-case proto lines through
// both proto_scanner.go's servicePattern (Go regexp, drives ScanProto/methodToID
// completeness) and list-backend-protos.sh's SERVICE_PATTERN (POSIX ERE, drives which
// files Makefile/prune-stale-backend.sh/validate-registry.sh scan), asserting identical
// match results. Without this, the two hand-maintained copies could silently diverge --
// e.g. one accepting a tab before "service" or a brace with no preceding space and the
// other not -- so a proto's RPCs would be scanned by one path and pruned/rejected by
// another.
func TestServicePatternParity_BashScriptMatchesGoRegex(t *testing.T) {
	if _, err := exec.LookPath("grep"); err != nil {
		t.Skip("grep not available")
	}
	bashPattern := scriptServicePatternRegex(t)

	cases := []struct {
		name string
		line string
		want bool
	}{
		{"vanilla", "service FooService {", true},
		{"leading tab", "\tservice FooService {", true},
		{"leading spaces", "   service FooService {", true},
		{"no space before brace", "service FooService{", true},
		{"extra internal spaces", "service   FooService   {", true},
		{"trailing spaces before brace", "service FooService   {", true},
		{"underscore in name", "service Foo_Service {", true},
		{"digits in name", "service FooService2 {", true},
		{"message, not service", "message Foo {", false},
		{"commented out", "// service FooService {", false},
		{"missing brace", "service FooService", false},
		{"missing space after service", "serviceFooService {", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goMatch := servicePattern.MatchString(tc.line)
			bashMatch := bashPattern.MatchString(tc.line)
			if goMatch != tc.want {
				t.Errorf("Go servicePattern on %q: got %v, want %v", tc.line, goMatch, tc.want)
			}
			if bashMatch != tc.want {
				t.Errorf("bash SERVICE_PATTERN on %q: got %v, want %v", tc.line, bashMatch, tc.want)
			}
			if goMatch != bashMatch {
				t.Errorf("parity mismatch on %q: Go=%v bash=%v", tc.line, goMatch, bashMatch)
			}
		})
	}
}
