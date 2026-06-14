package featureregistry

// ResetForTest clears the global registry and rpcIndex. Call via t.Cleanup in tests.
// Only for use in test files.
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]Feature{}
	rpcIndex = map[string]string{}
}
