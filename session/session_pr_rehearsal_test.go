package session

// Opt-in boot rehearsal for the PR maintenance (T1.5.4). It runs the real
// maintenance, its retry ticker, the bulk load and a list-path timing against a
// database ON DISK and never boots the service, so no ticker or cleaner can act
// on the worktree paths the rows point at.
//
//	SSQ_REHEARSAL_DB=<path to a COPY>         rehearse a copy of a real database
//	SSQ_REHEARSAL_SYNTHETIC_DIR=<dir on disk> build and rehearse a synthetic fixture
//
// Neither set: skipped. The run WRITES to the database it opens (links, states),
// so never point it at a live database; paths under ~/.stapler-squad (other than
// manual-builds/rehearsal/) and tmpfs are refused. Output: "REHEARSAL <label> ..." log lines.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
)

const (
	rehearsalListBudget  = 5 * time.Millisecond
	rehearsalListSamples = 200
	rehearsalListWarmup  = 20
	// start-limit window of the service unit (StartLimitBurst=10 in 600 s).
	rehearsalStartLimitWindow = 600 * time.Second
)

type rehearsalSize struct{ sessions, withPR, items, itemsWithPR int }

var syntheticRehearsalSize = rehearsalSize{sessions: 4000, withPR: 560, items: 1500, itemsWithPR: 300}

func rehearsalTarget(t *testing.T) (path, label string, synthetic bool) {
	t.Helper()
	switch {
	case os.Getenv("SSQ_REHEARSAL_DB") != "":
		path, label = os.Getenv("SSQ_REHEARSAL_DB"), "production-copy"
	case os.Getenv("SSQ_REHEARSAL_SYNTHETIC_DIR") != "":
		path = filepath.Join(os.Getenv("SSQ_REHEARSAL_SYNTHETIC_DIR"), "rehearsal.db")
		label, synthetic = "synthetic", true
	default:
		t.Skip("set SSQ_REHEARSAL_DB (a copy) or SSQ_REHEARSAL_SYNTHETIC_DIR to run the boot rehearsal")
	}
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	refuseUnsafeRehearsalPath(t, abs)
	return abs, label, synthetic
}

func refuseUnsafeRehearsalPath(t *testing.T, abs string) {
	t.Helper()
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(abs, filepath.Join(home, ".stapler-squad")+"/") &&
		!strings.Contains(abs, "/manual-builds/rehearsal/") {
		t.Fatalf("refusing %s: rehearse a COPY outside ~/.stapler-squad (or under manual-builds/rehearsal/)", abs)
	}
	for _, tmpfs := range []string{"/tmp/", "/dev/shm/", "/run/"} {
		if strings.HasPrefix(abs, tmpfs) {
			t.Fatalf("refusing %s: the rehearsal needs a database on disk, not tmpfs", abs)
		}
	}
}

func TestRehearsal_ShouldRecordCountsAndBudgets_WhenProductionDBCopyProvided(t *testing.T) {
	path, label, synthetic := rehearsalTarget(t)
	if synthetic {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		for _, f := range []string{path, path + "-wal", path + "-shm"} {
			_ = os.Remove(f)
		}
	}
	ctx := context.Background()
	repo, err := NewEntRepository(WithDatabasePath(path))
	require.NoError(t, err)
	defer repo.Close()
	if synthetic {
		buildSyntheticRehearsalDB(t, repo, syntheticRehearsalSize)
	}
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	rep := &rehearsalReport{t: t, label: label}
	// The log writer starts its rotation goroutine on the first log line; it is not the ticker's.
	opts := []goleak.Option{goleak.IgnoreCurrent(), goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun")}

	rep.add("sessions", rehearsalCount(t, repo, "sessions"))
	rep.add("sessions_with_pr_number", rehearsalCount(t, repo, "sessions WHERE github_pr_number > 0"))
	rep.add("sessions_with_pr_number_and_empty_url_(synthesized_at_seed)", rehearsalCount(t, repo, "sessions WHERE github_pr_number > 0 AND (github_pr_url IS NULL OR github_pr_url = '')"))
	rep.add("items_with_pr_number", rehearsalCount(t, repo, "backlog_items WHERE pr_number > 0"))
	recordRehearsalBaselines(t, repo, rep)

	before := measureListPath(t, storage)
	rep.addDuration("load_before_ms", before.load)
	rep.addDuration("list_p99_before_ms", before.p99)

	rehearseTickerCancel(t, repo, opts)
	linksBefore := rehearsalCount(t, repo, "session_prs")
	require.Zero(t, linksBefore, "cancelled and failing passes must import nothing")

	maint := rehearseFaultedMaintenance(t, repo)
	rep.addDuration("maintenance_first_pass_after_4_busy_faults_ms", maint)
	assert.Less(t, maint, rehearsalStartLimitWindow, "maintenance must finish well inside the start-limit window")
	rep.add("links_after", rehearsalCount(t, repo, "session_prs"))
	rep.add("session_links_after", rehearsalCount(t, repo, "session_prs WHERE session_uuid != ''"))
	rep.add("item_links_after", rehearsalCount(t, repo, "session_prs WHERE backlog_item_id != ''"))
	rep.add("states_after", rehearsalCount(t, repo, "pull_request_states"))
	emptyAfter := rehearsalCount(t, repo, "pull_request_states WHERE url = ''")
	rep.add("states_with_empty_url_after", emptyAfter)
	assert.Zero(t, emptyAfter, "every seeded row carries a synthesized or stored URL")

	start := time.Now()
	report, err := repo.runPRBackfill(ctx, nil)
	require.NoError(t, err)
	rep.addDuration("maintenance_steady_state_pass_ms", time.Since(start))
	rep.add("steady_state_created_session_links", report.CreatedSessionLinks)
	rep.add("skipped_no_repo", report.SkippedNoRepo)
	rep.add("skipped_no_uuid", report.SkippedNoUUID)
	rep.add("skipped_invalid", report.SkippedInvalid)
	rep.add("skipped_item_bad_ref", report.SkippedItemBadRef)
	rep.add("skipped_item_invalid", report.SkippedItemInvalid)
	rep.add("deferred", report.Deferred)

	after := measureListPath(t, storage)
	rep.addDuration("load_after_ms", after.load)
	rep.addDuration("list_p99_after_ms", after.p99)
	delta := after.p99 - before.p99
	rep.addDuration("list_p99_delta_ms", delta)
	assert.LessOrEqual(t, delta, rehearsalListBudget, "ListSessions-path p99 must stay within +5 ms")

	start = time.Now()
	data, err := storage.ListInstanceDataWithWorktree()
	require.NoError(t, err)
	rep.addDuration("sweeper_read_pass_ms", time.Since(start))
	rep.add("sweeper_read_rows", len(data))
	rep.flush()
}

// rehearsalReport collects key=value lines and writes them in one block.
type rehearsalReport struct {
	t     *testing.T
	label string
	lines []string
}

func (r *rehearsalReport) add(k string, v any) { r.lines = append(r.lines, fmt.Sprintf("%s=%v", k, v)) }

func (r *rehearsalReport) addDuration(k string, d time.Duration) {
	r.add(k, fmt.Sprintf("%.2f", float64(d.Microseconds())/1000))
}

func (r *rehearsalReport) flush() {
	out := fmt.Sprintf("REHEARSAL %s\n  %s", r.label, strings.Join(r.lines, "\n  "))
	r.t.Log("\n" + out)
	if f := os.Getenv("SSQ_REHEARSAL_OUT"); f != "" {
		require.NoError(r.t, os.WriteFile(f, []byte(out+"\n"), 0o600))
	}
}

type listTiming struct{ load, p99 time.Duration }

// measureListPath times LoadInstances (the bulk load) and the p99 of one
// list-request's work: converting every loaded instance's snapshot to the domain
// Session, where PR views are read. It stands in for the ListSessions RPC.
func measureListPath(t *testing.T, s *Storage) listTiming {
	t.Helper()
	start := time.Now()
	insts, err := s.LoadInstances()
	require.NoError(t, err)
	load := time.Since(start)
	for n := 0; n < rehearsalListWarmup; n++ {
		for _, inst := range insts {
			_ = InstanceToSession(inst)
		}
	}
	samples := make([]time.Duration, 0, rehearsalListSamples)
	for n := 0; n < rehearsalListSamples; n++ {
		begin := time.Now()
		for _, inst := range insts {
			_ = InstanceToSession(inst)
		}
		samples = append(samples, time.Since(begin))
	}
	sort.Slice(samples, func(a, b int) bool { return samples[a] < samples[b] })
	return listTiming{load: load, p99: samples[len(samples)*99/100]}
}

// rehearseTickerCancel: a failing first pass leaves the retry ticker running;
// cancelling the context stops it and leaves no goroutine behind.
func rehearseTickerCancel(t *testing.T, repo *EntRepository, opts []goleak.Option) {
	t.Helper()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	clock := &prMaintenanceClock{}
	storage.runPRStartupMaintenance(ctx, clock.config(func(PRRef) error { return errors.New("database is locked") }))
	require.Equal(t, int64(1), storage.PRMaintenanceFailures())
	cancel()
	storage.StopPRMaintenance()
	require.NoError(t, goleak.Find(opts...))
}

// rehearseFaultedMaintenance injects a busy fault on the first 4 attempts, checks
// the service would still boot (nil, counted, nothing imported), then fires the
// ticker and returns how long its successful import pass took. A success stops the ticker.
func rehearseFaultedMaintenance(t *testing.T, repo *EntRepository) time.Duration {
	t.Helper()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	failures := 4
	clock := &prMaintenanceClock{}
	require.NoError(t, func() error {
		storage.runPRStartupMaintenance(context.Background(), clock.config(func(PRRef) error {
			if failures > 0 {
				failures--
				return errors.New("database is locked")
			}
			return nil
		}))
		return nil
	}())
	require.Equal(t, int64(1), storage.PRMaintenanceFailures())
	require.Zero(t, rehearsalCount(t, repo, "session_prs"), "no link may persist from a failed pass")
	start := time.Now()
	clock.tick <- time.Now()
	storage.prMaintenance.wg.Wait() // the ticker stops itself after its first success
	elapsed := time.Since(start)
	storage.StopPRMaintenance()
	require.True(t, repo.prVerifyCountersTrusted.Load())
	return elapsed
}

func rehearsalCount(t *testing.T, repo *EntRepository, fromWhere string) int {
	t.Helper()
	var rows entsql.Rows
	require.NoError(t, repo.driver.Query(context.Background(), "SELECT count(*) FROM "+fromWhere, []any{}, &rows))
	defer rows.Close()
	var n int
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&n))
	return n
}

// recordRehearsalBaselines records the PR-history and issue-close denominators
// that need only the database (the live `gh issue view` half is a manual step).
func recordRehearsalBaselines(t *testing.T, repo *EntRepository, rep *rehearsalReport) {
	t.Helper()
	rep.add("baseline_sessions_with_multiple_distinct_artifact_pr_urls", rehearsalCount(t, repo,
		"sessions WHERE session_artifacts != '' AND json_valid(session_artifacts) AND "+
			"(SELECT count(DISTINCT value) FROM json_each(json_extract(session_artifacts, '$.pr_urls'))) > 1"))
	rep.add("baseline_done_items_with_github_issue_url", rehearsalCount(t, repo,
		"backlog_items WHERE status = 'done' AND external_url LIKE '%/issues/%'"))
}

func buildSyntheticRehearsalDB(t *testing.T, repo *EntRepository, size rehearsalSize) {
	t.Helper()
	ctx := context.Background()
	const chunk = 200
	uuids := make([]string, 0, size.sessions)
	for from := 0; from < size.sessions; from += chunk {
		to := min(from+chunk, size.sessions)
		builders := make([]*ent.SessionCreate, 0, to-from)
		for i := from; i < to; i++ {
			uuids = append(uuids, fmt.Sprintf("syn-uuid-%05d", i))
			builders = append(builders, syntheticSession(repo, i, size.withPR))
		}
		require.NoError(t, repo.Client.Session.CreateBulk(builders...).Exec(ctx))
	}
	for from := 0; from < size.items; from += chunk {
		to := min(from+chunk, size.items)
		builders := make([]*ent.BacklogItemCreate, 0, to-from)
		for i := from; i < to; i++ {
			builders = append(builders, syntheticItem(repo, i, size.itemsWithPR))
		}
		require.NoError(t, repo.Client.BacklogItem.CreateBulk(builders...).Exec(ctx))
	}
	items, err := repo.Client.BacklogItem.Query().Where(backlogitem.PrNumberGT(0)).Limit(size.itemsWithPR).All(ctx)
	require.NoError(t, err)
	for i, it := range items {
		_, err := repo.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: it.ID.String(), SessionUUID: uuids[i], SessionRole: SessionRoleWork})
		require.NoError(t, err)
	}
}

// syntheticSession: the first withPR sessions carry a legacy PR (some sharing a PR,
// every 9th without a stored URL, 1 without owner/repo; the poisoned-row classes
// (link cap, validator) only occur in real data).
func syntheticSession(repo *EntRepository, i, withPR int) *ent.SessionCreate {
	b := repo.Client.Session.Create().
		SetTitle(fmt.Sprintf("syn-%05d", i)).SetUUID(fmt.Sprintf("syn-uuid-%05d", i)).
		SetPath(fmt.Sprintf("/nonexistent/syn/%05d", i)).SetProgram("claude").SetStatus(int(domain.Paused))
	if i >= withPR {
		return b
	}
	j := i
	if i%25 == 0 {
		j = i - 1 // shares the previous session's PR
	}
	owner, repoName, number := fmt.Sprintf("org%d", j%12), fmt.Sprintf("repo%d", j%7), 1000+j
	if i == 3 {
		owner, repoName = "", ""
	}
	b.SetGithubOwner(owner).SetGithubRepo(repoName).SetGithubPrNumber(number).SetGithubPrStatusTerminal(i%3 == 0)
	if i%9 != 0 {
		b.SetGithubPrURL(fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repoName, number))
	}
	return b
}

func syntheticItem(repo *EntRepository, i, withPR int) *ent.BacklogItemCreate {
	statuses := []string{"done", "pr_pending", "archived", "in_progress"}
	b := repo.Client.BacklogItem.Create().SetTitle(fmt.Sprintf("syn-item-%05d", i)).SetPriority(3).SetStatus(statuses[i%len(statuses)])
	if i < withPR {
		b.SetPrNumber(5000 + i).SetPrURL(fmt.Sprintf("https://github.com/org%d/repo%d/pull/%d", i%12, i%7, 5000+i))
	}
	if i%4 == 0 {
		b.SetExternalURL(fmt.Sprintf("https://github.com/org%d/repo%d/issues/%d", i%12, i%7, i))
	}
	return b
}
