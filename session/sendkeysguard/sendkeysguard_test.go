package sendkeysguard

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeReporter is a reporter that records failures instead of actually
// failing the enclosing test — lets these tests assert "this check reports a
// failure" without the check's own (expected) Errorf call permanently
// failing this test file's real *testing.T, which a real subtest can't avoid
// (a subtest that calls Errorf marks its parent failed regardless of what
// code runs afterward).
type fakeReporter struct {
	failed bool
}

func (f *fakeReporter) Helper()                           {}
func (f *fakeReporter) Errorf(format string, args ...any) { f.failed = true }
func (f *fakeReporter) Fatalf(format string, args ...any) { f.failed = true }

// writeScratchFile creates a single-file scratch Go package in a temp dir and
// returns the dir, so CheckNoSingleWriteEnterConcatenation/
// CheckNoAppendCarriageReturnConcatenation can be run against it in
// isolation from this repo's own source.
func writeScratchFile(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "scratch.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write scratch file: %v", err)
	}
	return dir
}

// assertFlagged fails t if running check against src (written as a scratch
// package) does not report a failure via fakeReporter.
func assertFlagged(t *testing.T, src string, check func(reporter, string)) {
	t.Helper()
	dir := writeScratchFile(t, src)
	fr := &fakeReporter{}
	check(fr, dir)
	if !fr.failed {
		t.Error("expected this pattern to be flagged, but it passed")
	}
}

// assertAllowed fails t if running check against src (written as a scratch
// package) reports a failure via fakeReporter.
func assertAllowed(t *testing.T, src string, check func(reporter, string)) {
	t.Helper()
	dir := writeScratchFile(t, src)
	fr := &fakeReporter{}
	check(fr, dir)
	if fr.failed {
		t.Error("expected this pattern to pass, but it was flagged")
	}
}

// TestCheckNoSingleWriteEnterConcatenation_CatchesAllKnownShapes verifies the
// guard actually fails on every shape the BUG-031 single-write pattern has
// taken in this codebase, or could plausibly take next — a prior version of
// this guard silently missed the package-qualified identifier, the raw "\r"
// literal, and one level of variable indirection, all confirmed against a
// scratch package before this test existed.
func TestCheckNoSingleWriteEnterConcatenation_CatchesAllKnownShapes(t *testing.T) {
	cases := map[string]string{
		"bare identifier": `package scratch
const EnterKeySequence = "\r"
func f(inst interface{ SendKeys(string) error }, content string) {
	inst.SendKeys(content + EnterKeySequence)
}
`,
		"package-qualified identifier": `package scratch
import "github.com/tstapler/stapler-squad/session"
func f(inst interface{ SendKeys(string) error }, content string) {
	inst.SendKeys(content + session.EnterKeySequence)
}
`,
		"raw carriage-return literal": `package scratch
func f(inst interface{ SendKeys(string) error }, content string) {
	inst.SendKeys(content + "\r")
}
`,
		"BuildSubmittableInputAndSubmit call": `package scratch
import "github.com/tstapler/stapler-squad/session"
func f(inst interface{ SendKeys(string) error }, content string) {
	inst.SendKeys(session.BuildSubmittableInputAndSubmit(content))
}
`,
		"variable indirection": `package scratch
const EnterKeySequence = "\r"
func f(inst interface{ SendKeys(string) error }, content string) {
	text := content + EnterKeySequence
	inst.SendKeys(text)
}
`,
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			assertFlagged(t, src, func(r reporter, d string) { CheckNoSingleWriteEnterConcatenation(r, d) })
		})
	}
}

// TestCheckNoSingleWriteEnterConcatenation_AllowsTwoWriteShape is the
// negative case: the sanctioned two-write shape must not be flagged.
func TestCheckNoSingleWriteEnterConcatenation_AllowsTwoWriteShape(t *testing.T) {
	assertAllowed(t, `package scratch
const EnterKeySequence = "\r"
func f(inst interface{ SendKeys(string) error }, content string) {
	inst.SendKeys(content)
	inst.SendKeys(EnterKeySequence)
}
`, func(r reporter, d string) { CheckNoSingleWriteEnterConcatenation(r, d) })
}

// TestCheckNoAppendCarriageReturnConcatenation_CatchesAllKnownShapes proves
// the tymux-backend guard isn't defeated by hex case or the char-literal
// spelling of the same byte value — both confirmed missed by a prior version
// of this check.
func TestCheckNoAppendCarriageReturnConcatenation_CatchesAllKnownShapes(t *testing.T) {
	cases := map[string]string{
		"uppercase hex": `package scratch; func f(p []byte) []byte { return append(p, 0x0D) }`,
		"lowercase hex": `package scratch; func f(p []byte) []byte { return append(p, 0x0d) }`,
		"decimal":       `package scratch; func f(p []byte) []byte { return append(p, 13) }`,
		"char literal":  `package scratch; func f(p []byte) []byte { return append(p, '\r') }`,
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			assertFlagged(t, src, func(r reporter, d string) { CheckNoAppendCarriageReturnConcatenation(r, d) })
		})
	}
}

// TestCheckNoAppendCarriageReturnConcatenation_AllowsUnrelatedAppend is the
// negative case: an ordinary append with an unrelated byte must not be flagged.
func TestCheckNoAppendCarriageReturnConcatenation_AllowsUnrelatedAppend(t *testing.T) {
	assertAllowed(t, `package scratch; func f(p []byte) []byte { return append(p, 0x41) }`,
		func(r reporter, d string) { CheckNoAppendCarriageReturnConcatenation(r, d) })
}
