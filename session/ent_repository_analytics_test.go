package session

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validAnalyticsData(id string) AnalyticsData {
	return AnalyticsData{
		ID:        id,
		ToolName:  "Bash",
		Decision:  "auto_allow",
		RiskLevel: "low",
		CreatedAt: time.Now(),
	}
}

func TestEntRepositoryUsesWALForAnalyticsBatches(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "analytics.db")
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo.Close()
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	var mode string
	require.NoError(t, db.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&mode))
	require.True(t, strings.EqualFold(mode, "wal"), "journal mode = %q", mode)
}

func TestEntRepositoryRecordAnalyticsBatchIsAtomicAndIdempotent(t *testing.T) {
	t.Parallel()
	repo, _ := createTestEntRepository(t)
	ctx := context.Background()

	batch := make([]AnalyticsData, 0, 128)
	for i := range 127 {
		batch = append(batch, validAnalyticsData(fmt.Sprintf("event-%03d", i)))
	}
	batch = append(batch, validAnalyticsData("event-000"))
	require.NoError(t, repo.RecordAnalyticsBatch(ctx, batch))

	rows, err := repo.ListAnalytics(ctx, 0)
	require.NoError(t, err)
	require.Len(t, rows, 127)

	invalid := []AnalyticsData{
		validAnalyticsData("atomic-good"),
		{ID: "atomic-invalid", ToolName: "", Decision: "auto_allow", RiskLevel: "low", CreatedAt: time.Now()},
	}
	require.Error(t, repo.RecordAnalyticsBatch(ctx, invalid))
	rows, err = repo.ListAnalytics(ctx, 0)
	require.NoError(t, err)
	require.Len(t, rows, 127, "a rejected bulk statement must not persist its valid prefix")
}
