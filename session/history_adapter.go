package session

// DefaultHistoryAdapters returns the built-in registry. A new agent is a new adapter file
// plus one entry here.
func DefaultHistoryAdapters() *HistoryAdapterRegistry {
	r, err := NewHistoryAdapterRegistry(
		NewClaudeAdapter(),
		NewAgyAdapter(),
		NewOpencodeAdapter(),
		NewGeminiAdapter(),
		NewPiAdapter(),
	)
	if err != nil {
		panic(err) // unreachable while the list above is non-empty
	}
	return r
}

// defaultHistoryRegistry is shared: adapters are stateless and the registry is read-only.
var defaultHistoryRegistry = DefaultHistoryAdapters()

func init() { historyRegistryHook = defaultHistoryRegistry }
