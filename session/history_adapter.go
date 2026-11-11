package session

import (
	"context"
	"errors"
)

type HistoryAdapter interface {
	Name() string
	CanHandle(program string) bool

	// Import reads this CLI's native format and returns canonical turns.
	Import(ctx context.Context, inst *Instance) ([]CanonicalTurn, error)

	// Export writes canonical turns into this CLI's native format so it can resume.
	Export(ctx context.Context, turns []CanonicalTurn, inst *Instance) error
}

// HistoryAdapterRegistry resolves a program string to the first adapter whose CanHandle
// claims it, in registration order. Order is the priority for programs more than one
// adapter matches (e.g. "antigravity-claude" resolves to claude).
type HistoryAdapterRegistry struct {
	adapters []HistoryAdapter
}

// NewHistoryAdapterRegistry builds a registry; an empty list is a wiring bug, so it errors.
func NewHistoryAdapterRegistry(adapters ...HistoryAdapter) (*HistoryAdapterRegistry, error) {
	if len(adapters) == 0 {
		return nil, errors.New("history adapter registry needs at least one adapter")
	}
	return &HistoryAdapterRegistry{adapters: append([]HistoryAdapter(nil), adapters...)}, nil
}

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

// Resolve returns the first adapter that claims program, or nil if none does. It is the
// single resolution point shared by PortSessionHistory and the checkpoint create/fork paths.
func (r *HistoryAdapterRegistry) Resolve(program string) HistoryAdapter {
	for _, a := range r.adapters {
		if a.CanHandle(program) {
			return a
		}
	}
	return nil
}

// Handles reports whether any adapter claims program.
func (r *HistoryAdapterRegistry) Handles(program string) bool {
	return r.Resolve(program) != nil
}

// CrossesAdapters reports whether the two programs resolve to two distinct adapters.
func (r *HistoryAdapterRegistry) CrossesAdapters(oldProgram, newProgram string) bool {
	src, dst := r.Resolve(oldProgram), r.Resolve(newProgram)
	return src != nil && dst != nil && src.Name() != dst.Name()
}

func (i *Instance) historyRegistry() *HistoryAdapterRegistry {
	return defaultHistoryRegistry
}

// defaultHistoryRegistry is shared: adapters are stateless and the registry is read-only.
var defaultHistoryRegistry = DefaultHistoryAdapters()
