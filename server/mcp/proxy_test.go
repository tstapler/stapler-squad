package mcp

import (
	"context"
	"fmt"
	"testing"
)

func TestRunHandshakeUsesFreshContextForEveryStep(t *testing.T) {
	t.Parallel()

	var contexts []context.Context
	var sequence int
	newContext := func(parent context.Context) (context.Context, context.CancelFunc) {
		sequence++
		return context.WithValue(parent, handshakeTestContextKey{}, sequence), func() {}
	}
	step := func(ctx context.Context) error {
		contexts = append(contexts, ctx)
		return nil
	}

	if err := runHandshake(context.Background(), newContext, step, step, step); err != nil {
		t.Fatalf("run handshake: %v", err)
	}
	if len(contexts) != 3 {
		t.Fatalf("step contexts = %d, want 3", len(contexts))
	}
	for index, ctx := range contexts {
		if got := ctx.Value(handshakeTestContextKey{}); got != index+1 {
			t.Errorf("step %d context marker = %v, want %d", index, got, index+1)
		}
	}
}

func TestRunHandshakeStopsAtFirstFailedStep(t *testing.T) {
	t.Parallel()

	var calls int
	err := runHandshake(context.Background(), context.WithCancel, func(context.Context) error {
		calls++
		return fmt.Errorf("unavailable")
	}, func(context.Context) error {
		calls++
		return nil
	})
	if err == nil {
		t.Fatal("run handshake error = nil, want error")
	}
	if calls != 1 {
		t.Errorf("steps called = %d, want 1", calls)
	}
}

type handshakeTestContextKey struct{}
