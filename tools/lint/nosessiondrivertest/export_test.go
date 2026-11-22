package nosessiondrivertest

// SetDriverPkgPath points the analyzer at a testdata stub for the test's duration.
func SetDriverPkgPath(p string) (restore func()) {
	old := driverPkgPath
	driverPkgPath = p
	return func() { driverPkgPath = old }
}
