package session_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlogstore"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent/acauditevent"
)

// seedAuditRow inserts one audit row with an explicit created_at, bypassing the
// recorder so retention boundaries can be pinned exactly.
func seedAuditRow(t *testing.T, repo *session.EntRepository, itemID uuid.UUID, outcome string, createdAt time.Time) {
	t.Helper()
	_, err := repo.Client.AcAuditEvent.Create().
		SetItemID(itemID).SetItemPublicID("pub").SetCriterionIndex(0).
		SetPreviousStatus("pending").SetNewStatus("waived").SetReason("r").SetOutcome(outcome).
		SetActorClass("system").SetActorSessionUUID("").SetSessionVerified(false).
		SetSource("system").SetRemoteAddr("").SetUserAgent("").
		SetCreatedAt(createdAt).Save(context.Background())
	require.NoError(t, err)
}

func liveItemUUID(t *testing.T, repo *session.EntRepository) uuid.UUID {
	t.Helper()
	item, err := repo.GetBacklogItem(context.Background(), newWaiverItem(t, repo))
	require.NoError(t, err)
	id, err := uuid.Parse(item.ID)
	require.NoError(t, err)
	return id
}

func countAuditRows(t *testing.T, repo *session.EntRepository, itemID uuid.UUID) int {
	t.Helper()
	n, err := repo.Client.AcAuditEvent.Query().Where(acauditevent.ItemID(itemID)).Count(context.Background())
	require.NoError(t, err)
	return n
}

func TestPruneAcAuditOlderThan_should_TreatCutoffAsExclusive(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := liveItemUUID(t, repo)
	cutoff := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	seedAuditRow(t, repo, id, "denied:at-cutoff", cutoff)
	seedAuditRow(t, repo, id, "denied:before-cutoff", cutoff.Add(-time.Second))
	gone := uuid.New()
	seedAuditRow(t, repo, gone, "applied", cutoff)
	seedAuditRow(t, repo, gone, "applied", cutoff.Add(-time.Second))

	n, err := repo.PruneAcAuditOlderThan(context.Background(), cutoff)

	require.NoError(t, err)
	assert.Equal(t, 2, n, "only rows strictly older than the cutoff go, for live and deleted items alike")
	assert.Equal(t, 1, countAuditRows(t, repo, id))
	assert.Equal(t, 1, countAuditRows(t, repo, gone))
}

func TestPruneAcAuditOlderThan_should_ClassifyOutcomes_When_LiveItemHasOldRows(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := liveItemUUID(t, repo)
	old := time.Now().Add(-48 * time.Hour)
	for _, outcome := range []string{"applied", "waiver_cleared", "waiver_cleared:edited"} {
		seedAuditRow(t, repo, id, outcome, old)
	}
	for _, outcome := range []string{"denied:x", "rejected:y", "applied2", "xwaiver_cleared", "waiverish"} {
		seedAuditRow(t, repo, id, outcome, old)
	}

	n, err := repo.PruneAcAuditOlderThan(context.Background(), time.Now())

	require.NoError(t, err)
	assert.Equal(t, 5, n, "refusals (including look-alike outcomes) are pruned")
	kept, err := repo.Client.AcAuditEvent.Query().Where(acauditevent.ItemID(id)).All(context.Background())
	require.NoError(t, err)
	outcomes := make([]string, 0, len(kept))
	for _, k := range kept {
		outcomes = append(outcomes, k.Outcome)
	}
	assert.ElementsMatch(t, []string{"applied", "waiver_cleared", "waiver_cleared:edited"}, outcomes)
}

func TestPruneAcAuditOlderThan_should_SumRefusedAndOrphanedCounts(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	live := liveItemUUID(t, repo)
	gone := uuid.New()
	old := time.Now().Add(-48 * time.Hour)
	seedAuditRow(t, repo, live, "denied:a", old)
	seedAuditRow(t, repo, gone, "applied", old)
	seedAuditRow(t, repo, gone, "waiver_cleared:z", old)

	n, err := repo.PruneAcAuditOlderThan(context.Background(), time.Now())

	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, 0, countAuditRows(t, repo, gone))
}

func TestRecordAcAudit_should_NeverEvictChangeOutcomes_When_FloodExceedsRefusalCap(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := liveItemUUID(t, repo)
	extra := backlogstore.MaxAcAuditRefusalsPerItem + 5
	for _, outcome := range []string{"applied", "waiver_cleared:x"} {
		for i := 0; i < extra; i++ {
			require.NoError(t, repo.RecordAcAudit(ownerCtx(), backlogdto.AcAuditEventData{ItemID: id.String(), Outcome: outcome}))
		}
	}
	assert.Equal(t, 2*extra, countAuditRows(t, repo, id), "real-change rows are never pruned")

	for i := 0; i < extra; i++ {
		require.NoError(t, repo.RecordAcAudit(ownerCtx(), backlogdto.AcAuditEventData{ItemID: id.String(), Outcome: "applied-ish"}))
	}
	assert.Equal(t, 2*extra+backlogstore.MaxAcAuditRefusalsPerItem, countAuditRows(t, repo, id), "look-alike outcome is a refusal, capped")
}

func TestRecordAcAudit_should_CapFieldLengthsAndDropSplitRunes(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := liveItemUUID(t, repo)
	ctx := domain.WithAcActor(context.Background(), domain.AcActor{
		Class: domain.AcActorMCPSession, SessionUUID: strings.Repeat("s", 100), SessionVerified: true,
		Source: "mcp", RemoteAddr: strings.Repeat("r", 150), UserAgent: strings.Repeat("u", 300),
	})
	reason := strings.Repeat("a", 1999) + "é" // the 2-byte rune straddles the 2000-byte cap
	require.NoError(t, repo.RecordAcAudit(ctx, backlogdto.AcAuditEventData{
		ItemID: id.String(), Reason: reason, Outcome: "denied:" + strings.Repeat("o", 400),
	}))

	row, err := repo.Client.AcAuditEvent.Query().Where(acauditevent.ItemID(id)).Only(context.Background())
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("a", 1999), row.Reason, "split rune dropped, not kept as invalid UTF-8")
	assert.Len(t, row.Outcome, 300)
	assert.Len(t, row.ActorSessionUUID, 64)
	assert.Len(t, row.RemoteAddr, 100)
	assert.Len(t, row.UserAgent, 200)
	assert.Equal(t, string(domain.AcActorMCPSession), row.ActorClass)
	assert.True(t, row.SessionVerified)
	assert.Equal(t, "mcp", row.Source)

	exact := strings.Repeat("b", 2000)
	require.NoError(t, repo.RecordAcAudit(ctx, backlogdto.AcAuditEventData{ItemID: id.String(), Reason: exact, Outcome: "applied"}))
	rows, err := repo.Client.AcAuditEvent.Query().Where(acauditevent.ItemID(id), acauditevent.Outcome("applied")).Only(context.Background())
	require.NoError(t, err)
	assert.Equal(t, exact, rows.Reason, "a value exactly at the cap is stored whole")
}

func TestRecordAcAudit_should_ResolvePublicIDOnlyWhenEmpty_And_RejectBadItemID(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	item, err := repo.GetBacklogItem(context.Background(), newWaiverItem(t, repo))
	require.NoError(t, err)

	require.NoError(t, repo.RecordAcAudit(ownerCtx(), backlogdto.AcAuditEventData{ItemID: item.ID, Outcome: "applied"}))
	require.NoError(t, repo.RecordAcAudit(ownerCtx(), backlogdto.AcAuditEventData{ItemID: item.ID, ItemPublicID: "caller-supplied", Outcome: "applied"}))

	id := uuid.MustParse(item.ID)
	rows, err := repo.Client.AcAuditEvent.Query().Where(acauditevent.ItemID(id)).All(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NotEmpty(t, item.PublicIDRaw)
	got := make([]string, 0, len(rows))
	for _, r := range rows {
		got = append(got, r.ItemPublicID)
	}
	assert.ElementsMatch(t, []string{item.PublicIDRaw, "caller-supplied"}, got)

	err = repo.RecordAcAudit(ownerCtx(), backlogdto.AcAuditEventData{ItemID: "not-a-uuid", Outcome: "applied"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid item id "not-a-uuid"`)
}

func TestCreateAcAuditEvent_should_RunOnCallersTransaction(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := liveItemUUID(t, repo)
	ctx := context.Background()

	tx, err := repo.Client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.CreateAcAuditEvent(ctx, tx.AcAuditEvent, backlogdto.AcAuditEventData{ItemID: id.String(), Outcome: "applied"}))
	require.NoError(t, tx.Rollback())

	assert.Equal(t, 0, countAuditRows(t, repo, id), "the audit row rolls back with the caller's transaction")
}

func TestRecordAcAudit_should_TrimOldestRefusalsFirst_And_SkipTrimForChangeOutcomes(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := liveItemUUID(t, repo)
	base := time.Now().Add(-time.Hour)
	over := backlogstore.MaxAcAuditRefusalsPerItem + 5
	for i := 0; i < over; i++ {
		seedAuditRow(t, repo, id, fmt.Sprintf("denied:%03d", i), base.Add(time.Duration(i)*time.Second))
	}

	for _, outcome := range []string{"applied", "waiver_cleared:x"} {
		require.NoError(t, repo.RecordAcAudit(ownerCtx(), backlogdto.AcAuditEventData{ItemID: id.String(), Outcome: outcome}))
	}
	assert.Equal(t, over+2, countAuditRows(t, repo, id), "recording a real change never trims refusals")

	require.NoError(t, repo.RecordAcAudit(ownerCtx(), backlogdto.AcAuditEventData{ItemID: id.String(), Outcome: "denied:new"}))

	rows, err := repo.Client.AcAuditEvent.Query().Where(acauditevent.ItemID(id), acauditevent.OutcomeHasPrefix("denied:")).All(context.Background())
	require.NoError(t, err)
	assert.Len(t, rows, backlogstore.MaxAcAuditRefusalsPerItem)
	have := map[string]bool{}
	for _, r := range rows {
		have[r.Outcome] = true
	}
	assert.False(t, have["denied:000"], "oldest refusal evicted")
	assert.False(t, have[fmt.Sprintf("denied:%03d", over-backlogstore.MaxAcAuditRefusalsPerItem)], "still within the evicted window")
	assert.True(t, have[fmt.Sprintf("denied:%03d", over-backlogstore.MaxAcAuditRefusalsPerItem+1)], "first survivor")
	assert.True(t, have["denied:new"])

	require.NoError(t, repo.RecordAcAudit(ownerCtx(), backlogdto.AcAuditEventData{ItemID: id.String(), Outcome: "waiverish"}))
	first := fmt.Sprintf("denied:%03d", over-backlogstore.MaxAcAuditRefusalsPerItem+1)
	gone, err := repo.Client.AcAuditEvent.Query().Where(acauditevent.ItemID(id), acauditevent.Outcome(first)).Count(context.Background())
	require.NoError(t, err)
	assert.Zero(t, gone, "an outcome that only resembles waiver_cleared is a refusal and triggers the trim")
}
