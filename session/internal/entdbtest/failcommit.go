package entdbtest

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"

	"github.com/tstapler/stapler-squad/session/ent"
)

// NewClientFailingCommit returns two clients over one private in-memory
// database: failing, whose transactions execute their writes and then fail
// Commit with commitErr (rolling the writes back), and plain, a normal client
// for seeding and reading back. Ent has no other way to inject a commit failure.
func NewClientFailingCommit(t testing.TB, commitErr error) (failing, plain *ent.Client) {
	t.Helper()
	drv := newDriver(t)
	return ent.NewClient(ent.Driver(failCommitDriver{Driver: drv, err: commitErr})), ent.NewClient(ent.Driver(drv))
}

type failCommitDriver struct {
	dialect.Driver
	err error
}

func (d failCommitDriver) Tx(ctx context.Context) (dialect.Tx, error) {
	tx, err := d.Driver.Tx(ctx)
	if err != nil {
		return nil, err
	}
	return failCommitTx{Tx: tx, err: d.err}, nil
}

type failCommitTx struct {
	dialect.Tx
	err error
}

func (x failCommitTx) Commit() error {
	if rbErr := x.Rollback(); rbErr != nil {
		return rbErr
	}
	return x.err
}
