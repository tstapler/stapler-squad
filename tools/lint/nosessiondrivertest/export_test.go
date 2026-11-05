package nosessiondrivertest

// SetSessionPkgPath points the analyzer at a testdata stub for the test's duration.
func SetSessionPkgPath(p string) (restore func()) {
	old := sessionPkgPath
	sessionPkgPath = p
	return func() { sessionPkgPath = old }
}
