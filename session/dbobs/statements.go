package dbobs

import (
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// maxStatementKeys bounds the per-(statement, caller) table so statements
// with variable-length IN lists or batch inserts cannot grow it without limit.
const maxStatementKeys = 512

// topStatementsReported is how many rows Snapshot returns per pool.
const topStatementsReported = 25

// callerDepth is how many application frames identify a caller.
const callerDepth = 3

// StatementStat aggregates one (statement, caller) pair. It answers "who
// issues the writes that queue behind the single writer connection".
type StatementStat struct {
	Statement  string  `json:"statement"`
	Caller     string  `json:"caller"`
	Count      int64   `json:"count"`
	TotalMs    float64 `json:"total_ms"`
	MaxMs      float64 `json:"max_ms"`
	PoolWaitMs float64 `json:"pool_wait_ms"`
}

// stmtKey holds the normalized statement (see NormalizeStatement), so IN lists
// and multi-row VALUES of varying length share one key.
type stmtKey struct{ statement, caller string }

type stmtAgg struct {
	count             int64
	total, max, waits time.Duration
}

type statementTable struct {
	mu      sync.Mutex
	rows    map[stmtKey]*stmtAgg
	dropped int64 // new keys refused because the table was full
}

func (t *statementTable) add(k stmtKey, elapsed, wait time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.rows == nil {
		t.rows = make(map[stmtKey]*stmtAgg)
	}
	a := t.rows[k]
	if a == nil {
		if len(t.rows) >= maxStatementKeys {
			t.dropped++
			return
		}
		a = &stmtAgg{}
		t.rows[k] = a
	}
	a.count++
	a.total += elapsed
	a.waits += wait
	if elapsed > a.max {
		a.max = elapsed
	}
}

func (t *statementTable) droppedKeys() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.dropped
}

// top returns the n rows with the greatest total duration.
func (t *statementTable) top(n int) []StatementStat {
	t.mu.Lock()
	out := make([]StatementStat, 0, len(t.rows))
	for k, a := range t.rows {
		out = append(out, StatementStat{
			Statement: k.statement, Caller: k.caller, Count: a.count,
			TotalMs: ms(int64(a.total)), MaxMs: ms(int64(a.max)), PoolWaitMs: ms(int64(a.waits)),
		})
	}
	t.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].TotalMs > out[j].TotalMs })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// callerOf names the application frames that issued the query, skipping the
// ent-generated builders, this package, dbsplit, database/sql and the runtime.
// "Repo.Method<-Service.Method<-..." is enough to tell writers apart.
func callerOf() string {
	var pcs [24]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	var names []string
	for len(names) < callerDepth {
		f, more := frames.Next()
		if isApplicationFrame(f.Function, f.File) {
			names = append(names, shortFunc(f.Function))
		}
		if !more {
			break
		}
	}
	if len(names) == 0 {
		return "unknown"
	}
	return strings.Join(names, "<-")
}

const modulePrefix = "github.com/tstapler/stapler-squad/"

func isApplicationFrame(fn, file string) bool {
	if !strings.HasPrefix(fn, modulePrefix) {
		return false
	}
	rest := fn[len(modulePrefix):]
	for _, infra := range []string{"session/ent.", "session/ent/", "session/dbsplit.", "session/dbobs."} {
		if strings.HasPrefix(rest, infra) {
			// dbobs tests call the driver directly and need a visible caller.
			return strings.HasSuffix(file, "_test.go") && strings.HasPrefix(rest, "session/dbobs.")
		}
	}
	return true
}

func shortFunc(fn string) string {
	fn = strings.TrimPrefix(fn, modulePrefix)
	return strings.NewReplacer("(*", "", ")", "").Replace(fn)
}
