package jules

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// revealCallPattern matches an actual invocation of JulesAPIKey's unexported
// reveal() method -- e.g. "key.reveal()" -- the only sanctioned path from a
// JulesAPIKey to its plaintext value (types.go's doc comment on reveal()).
// It requires a literal "." immediately before "reveal(" so it matches only
// call sites, not the method's own declaration ("func (k JulesAPIKey)
// reveal() string {", no preceding dot) or doc-comment prose that mentions
// "reveal()" without calling it (types.go's comment above the type and
// above the method itself).
var revealCallPattern = regexp.MustCompile(`\.reveal\(\)`)

// funcSignaturePattern matches a top-level function/method declaration line,
// used to find which function encloses a reveal() call site.
var funcSignaturePattern = regexp.MustCompile(`^func\s`)

type revealSite struct {
	file string
	line int
	text string
}

// TestJulesPackage_should_NotLogSecrets_When_SourceScanned is a whole-package
// static scan enforcing Story 1.2.2's no-secret-logging guard: it greps
// every non-test .go file under jules/ for reveal() call sites and asserts
// there is exactly one, that it lives inside Client.newRequest in
// client.go, and that its line never also touches a logging call --
// compensating for the repo's missing outbound-HTTP redaction (pitfalls §2)
// by making it structurally impossible for a second call site to leak the
// key without this test catching it.
func TestJulesPackage_should_NotLogSecrets_When_SourceScanned(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob jules/*.go: %v", err)
	}

	var sites []revealSite
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}

		enclosingFunc := ""
		lineNum := 0
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			if funcSignaturePattern.MatchString(line) {
				enclosingFunc = line
			}

			if !revealCallPattern.MatchString(line) {
				continue
			}
			sites = append(sites, revealSite{file: path, line: lineNum, text: line})

			if !strings.Contains(enclosingFunc, "func (c *Client) newRequest(") {
				t.Errorf("%s:%d: reveal() called outside Client.newRequest (enclosing func: %q) -- want the sole reveal() call site to stay confined to newRequest", path, lineNum, enclosingFunc)
			}
			for _, forbidden := range []string{"slog.", "fmt.Print", "log."} {
				if strings.Contains(line, forbidden) {
					t.Errorf("%s:%d: reveal() call site also references %q -- the resolved key must never reach a logging call: %s", path, lineNum, forbidden, line)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("scan %s: %v", path, err)
		}
		f.Close()
	}

	if len(sites) != 1 {
		t.Fatalf("found %d reveal() call site(s) in jules/*.go (excluding _test.go), want exactly 1: %+v", len(sites), sites)
	}
	if sites[0].file != "client.go" {
		t.Errorf("reveal() called in %s, want client.go", sites[0].file)
	}
}
