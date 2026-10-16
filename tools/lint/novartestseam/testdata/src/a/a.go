package a

// runtimeCounter is reassigned outside any test file too, so it's a real mutable var, not a
// test-only seam — must not be flagged.
var runtimeCounter = 0

func bump() {
	runtimeCounter = runtimeCounter + 1
}

// neverReassigned looks the same shape but is never reassigned anywhere — a different smell
// (should just be a const), not this analyzer's concern. Must not be flagged.
var neverReassigned = "http://localhost:9999"

func useNeverReassigned() string {
	return neverReassigned
}

// resolver/lookupFn mirror server/services/webhook_ssrf.go's real lookupIPAddr pattern: a
// method value off a composite literal, only ever reassigned in a _test.go file to swap in a
// fake. Must not be flagged — a method value can never be a const, so this is a legitimate
// var-as-DI-seam, not the anti-pattern this analyzer targets.
type resolver struct{}

func (resolver) Lookup() string { return "real" }

var lookupFn = (&resolver{}).Lookup

func useLookupFn() string {
	return lookupFn()
}

// bumped is incremented outside tests (`+=`), so it is a real mutable var, not a test-only
// seam — must not be flagged even though a test also assigns it.
var bumped = 0

func bumpMore() { bumped += 2 }

// addrTaken has its address taken outside tests (e.g. handed to a flag/setter), so it is
// mutable by real code — must not be flagged.
var addrTaken = "x"

func takeAddr() *string { return &addrTaken }
