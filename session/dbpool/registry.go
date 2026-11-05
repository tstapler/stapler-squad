package dbpool

import (
	"context"
	"os"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

var (
	regMu       sync.Mutex
	registered  = map[*Database]struct{}{}
	metricsOnce sync.Once
)

func register(d *Database) {
	metricsOnce.Do(initMetrics)
	regMu.Lock()
	registered[d] = struct{}{}
	regMu.Unlock()
}

func unregister(d *Database) {
	regMu.Lock()
	delete(registered, d)
	regMu.Unlock()
}

// Info is one /actuator/db row. JournalMode and Synchronous were read from the
// writer connection when the database opened; reader connections use the same DSN.
type Info struct {
	UseCase     UseCase   `json:"use_case"`
	Pool        string    `json:"pool"`
	Class       Class     `json:"class"`
	Path        string    `json:"path"`
	JournalMode string    `json:"journal_mode"`
	Synchronous string    `json:"synchronous"`
	WriterConns int       `json:"writer_conns"`
	ReaderConns int       `json:"reader_conns"`
	FileBytes   int64     `json:"file_bytes"`
	WALBytes    int64     `json:"wal_bytes"`
	OpenedAt    time.Time `json:"opened_at"`
}

// Snapshot returns one row per open Database, ordered by use case then path.
// It never touches a database connection, so it cannot queue behind a writer.
func Snapshot() []Info {
	regMu.Lock()
	dbs := make([]*Database, 0, len(registered))
	for d := range registered {
		dbs = append(dbs, d)
	}
	regMu.Unlock()

	out := make([]Info, 0, len(dbs))
	for _, d := range dbs {
		out = append(out, d.info())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UseCase != out[j].UseCase {
			return out[i].UseCase < out[j].UseCase
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func (d *Database) info() Info {
	return Info{
		UseCase: d.spec.UseCase, Pool: d.spec.PoolName, Class: d.spec.Class, Path: d.spec.Path,
		JournalMode: d.journalMode, Synchronous: d.synchronous,
		WriterConns: 1, ReaderConns: d.spec.Profile.Readers,
		FileBytes: sizeOf(d.spec.Path), WALBytes: sizeOf(d.spec.Path + "-wal"), OpenedAt: d.started,
	}
}

func sizeOf(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// initMetrics registers db.journal_mode (1 when WAL) and db.synchronous
// (0 off, 1 normal, 2 full, 3 extra) against the global delegating meter.
func initMetrics() {
	meter := telemetry.GetMeter()
	mode, err1 := meter.Float64ObservableGauge("db.journal_mode", metric.WithDescription("1 when the database journal_mode is WAL, as verified at open"))
	syncG, err2 := meter.Float64ObservableGauge("db.synchronous", metric.WithDescription("PRAGMA synchronous on the writer connection: 0 off, 1 normal, 2 full, 3 extra"))
	if err1 != nil || err2 != nil {
		return
	}
	_, _ = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for _, in := range Snapshot() {
			a := metric.WithAttributes(attribute.String("db.name", in.Pool), attribute.String("db.logical", string(in.UseCase)), attribute.String("db.class", string(in.Class)))
			wal := 0.0
			if in.JournalMode == "wal" {
				wal = 1
			}
			o.ObserveFloat64(mode, wal, a)
			o.ObserveFloat64(syncG, syncValue(in.Synchronous), a)
		}
		return nil
	}, mode, syncG)
}

func syncValue(name string) float64 {
	switch name {
	case "off":
		return 0
	case "normal":
		return 1
	case "full":
		return 2
	case "extra":
		return 3
	}
	return -1
}
