// Package entfullscan is a deliberate entfullscan violation; never import it.
package entfullscan

import "context"

type fakeQuery struct{}

func (q *fakeQuery) All(ctx context.Context) ([]int, error) { return nil, nil }

// Violation is an unfiltered query .All(ctx).
func Violation(ctx context.Context) { _, _ = (&fakeQuery{}).All(ctx) }
