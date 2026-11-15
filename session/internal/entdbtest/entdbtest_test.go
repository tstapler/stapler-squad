package entdbtest

import (
	"context"
	"testing"

	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/ent/tag"
)

func TestMain(m *testing.M) {
	if err := Warm(); err != nil {
		panic(err)
	}
	// The template's pinned connection and pool goroutines live for the whole process.
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

func TestNewClient_should_ReturnMigratedEmptyDatabase(t *testing.T) {
	t.Parallel()
	c := NewClient(t)
	n, err := c.Tag.Query().Count(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("Tag count = %d, err = %v; want 0, nil", n, err)
	}
}

func TestNewClient_should_IsolateWritesBetweenClients(t *testing.T) {
	t.Parallel()
	a, b := NewClient(t), NewClient(t)
	if _, err := a.Tag.Create().SetName("only-in-a").Save(context.Background()); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	n, err := b.Tag.Query().Where(tag.Name("only-in-a")).Count(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("second client saw %d rows, err = %v; want 0, nil", n, err)
	}
}
