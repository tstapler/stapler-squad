package headless

import "context"

// PoolClient is the narrow interface BacklogService uses for headless triage calls.
// Satisfied by *Pool; allows test injection without needing FakeRunner WorkDir support.
type PoolClient interface {
	CallBlockingWithOptions(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions) (string, error)
}

// compile-time check that *Pool satisfies PoolClient.
var _ PoolClient = (*Pool)(nil)
