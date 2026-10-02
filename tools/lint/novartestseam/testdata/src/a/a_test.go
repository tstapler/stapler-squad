package a

import "testing"

// baseURL and requestTimeoutMs are declared and only ever reassigned right here, in a
// _test.go file — the flagged pattern. Real instances of this pattern (see
// cmd/ssq-hooks/main.go's original remoteClassifyBaseURL) usually span two files — declared
// in the main package, reassigned only from tests — but collapsing both into one file here
// keeps this test independent of analysistest's Tests:true multi-variant package loading:
// that loader also runs the analyzer against a package variant excluding this file, which
// would otherwise have to satisfy the same expectation-comment despite never seeing the
// reassignment. The analyzer's file-suffix check for the assignment site is what's under
// test, not which file the declaration lives in.
//
// The blank line above matters: it keeps this explanation from attaching as either var's own
// Go doc comment, which would exempt them under the doc-comment carve-out below — these two
// are deliberately undocumented, matching remoteClassifyBaseURL's original state.

var baseURL = "http://localhost:8543" // want `var baseURL is only reassigned in test files`

const defaultTimeoutMs = 75

var requestTimeoutMs = defaultTimeoutMs // want `var requestTimeoutMs is only reassigned in test files`

// suppressed is only reassigned in tests but carries an explicit nolint — must not be flagged.
var suppressed = "http://localhost:7777" //nolint:novartestseam -- deliberately a var, see test

// documentedTunable carries its own doc comment explaining the tradeoff, mirroring the dozen
// legitimate real-world instances (e.g. session/headless/pool.go's maxQueueWait) found when
// this analyzer was first run repo-wide: a var, not a const, so tests can shrink it — must not
// be flagged even though it's only ever reassigned in a test file.
var documentedTunable = 5000

func TestOverridesBaseURL(t *testing.T) {
	orig := baseURL
	baseURL = "http://127.0.0.1:0"
	defer func() { baseURL = orig }()
}

func TestOverridesTimeout(t *testing.T) {
	orig := requestTimeoutMs
	requestTimeoutMs = 1
	defer func() { requestTimeoutMs = orig }()
}

func TestOverridesSuppressed(t *testing.T) {
	orig := suppressed
	suppressed = "http://127.0.0.1:1"
	defer func() { suppressed = orig }()
}

func TestOverridesDocumentedTunable(t *testing.T) {
	orig := documentedTunable
	documentedTunable = 1
	defer func() { documentedTunable = orig }()
}

func TestOverridesLookupFn(t *testing.T) {
	orig := lookupFn
	lookupFn = func() string { return "fake" }
	defer func() { lookupFn = orig }()
	_ = useLookupFn()
}

func TestAssignsRuntimeMutated(t *testing.T) {
	bumped = 0
	addrTaken = "y"
}

// Test-only compound/incdec/address-of mutations are caught too. The blank line before each
// var keeps this prose from becoming its Doc comment.

var tickCount = 0 // want `var tickCount is only reassigned in test files`

func TestIncrements(t *testing.T) { tickCount++ }

var accumulated = 0 // want `var accumulated is only reassigned in test files`

func TestCompound(t *testing.T) { accumulated += 3 }

var viaPointer = "a" // want `var viaPointer is only reassigned in test files`

func TestAddressOf(t *testing.T) {
	p := &viaPointer
	*p = "b"
}
