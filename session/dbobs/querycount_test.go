package dbobs

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryCounter_should_CountOnlyStatementsUnderItsContext(t *testing.T) {
	drv, _ := newDriver(t, Config{Name: "qc"})
	ctx, counter := WithQueryCounter(context.Background())

	var res sql.Result
	for range 3 {
		require.NoError(t, drv.Exec(ctx, "INSERT INTO t (v) VALUES (?)", []any{"x"}, &res))
	}
	require.NoError(t, drv.Exec(context.Background(), "INSERT INTO t (v) VALUES (?)", []any{"y"}, &res))

	assert.EqualValues(t, 3, counter.Count())
}
