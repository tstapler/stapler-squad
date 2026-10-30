package dbobs

import (
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type busyTx struct {
	dialect.Tx
	commitErr, rollbackErr error
}

func (b busyTx) Commit() error   { return b.commitErr }
func (b busyTx) Rollback() error { return b.rollbackErr }

func busyStatsFor(t *testing.T, name string) Stats {
	t.Helper()
	for _, s := range Snapshot() {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("%s pool missing from Snapshot", name)
	return Stats{}
}

// A deferred write transaction usually fails at COMMIT, not at its statements.
func TestBusyOnCommitAndRollbackIsCounted(t *testing.T) {
	_, db := newDriver(t, Config{Name: "commitbusy-pool", Warn: func(string, ...any) {}})
	c := &collector{cfg: Config{Name: "commitbusy", Warn: func(string, ...any) {}}, db: db}
	register(c)
	t.Cleanup(func() { unregister(c) })

	busy := errors.New("database is locked (5) (SQLITE_BUSY)")
	tx := &txWrapper{Tx: busyTx{commitErr: busy, rollbackErr: busy}, c: c}

	require.ErrorIs(t, tx.Commit(), busy)
	require.ErrorIs(t, tx.Rollback(), busy)

	s := busyStatsFor(t, "commitbusy")
	assert.EqualValues(t, 2, s.BusyErrors)
	assert.EqualValues(t, 2, s.Errors)
}
