package scanstate

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/transcriptscanstate"
)

// saveChunk keeps bulk upserts under SQLite's bound-variable limit.
const saveChunk = 50

// Record is one persisted scan state.
type Record struct {
	Path string
	Position
	// State is the consumer's own checkpoint payload; empty for offset-only consumers.
	State []byte
}

// Store persists Records on an already-migrated ent client.
type Store struct {
	client *ent.Client
	now    func() time.Time
}

// NewStore wraps client; the caller must have run schema migration.
func NewStore(client *ent.Client) *Store { return &Store{client: client, now: time.Now} }

// Consumer scopes a Store to one scanner's rows.
func (s *Store) Consumer(name string) *Scoped { return &Scoped{store: s, name: name} }

// Scoped is a Store limited to one consumer name.
type Scoped struct {
	store *Store
	name  string
}

// Load returns every stored record for the consumer, keyed by path.
func (c *Scoped) Load(ctx context.Context) (map[string]Record, error) {
	rows, err := c.store.client.TranscriptScanState.Query().
		Where(transcriptscanstate.ConsumerEQ(c.name)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load %s scan state: %w", c.name, err)
	}
	out := make(map[string]Record, len(rows))
	for _, r := range rows {
		out[r.Path] = Record{
			Path: r.Path,
			Position: Position{
				Size: r.Size, ModTimeNs: r.ModTimeNs, Offset: r.Offset,
				HeadHash: uint64(r.HeadHash), TailHash: uint64(r.TailHash), //nolint:gosec // bit-pattern round trip
			},
			State: r.State,
		}
	}
	return out, nil
}

// Save upserts records. Safe to re-run.
func (c *Scoped) Save(ctx context.Context, recs []Record) error {
	now := c.store.now()
	for start := 0; start < len(recs); start += saveChunk {
		end := min(start+saveChunk, len(recs))
		creates := make([]*ent.TranscriptScanStateCreate, 0, end-start)
		for _, r := range recs[start:end] {
			creates = append(creates, c.store.client.TranscriptScanState.Create().
				SetConsumer(c.name).SetPath(r.Path).
				SetSize(r.Size).SetModTimeNs(r.ModTimeNs).SetOffset(r.Offset).
				SetHeadHash(int64(r.HeadHash)).SetTailHash(int64(r.TailHash)). //nolint:gosec // bit-pattern round trip

				SetState(r.State).SetUpdatedAt(now))
		}
		err := c.store.client.TranscriptScanState.CreateBulk(creates...).
			OnConflictColumns(transcriptscanstate.FieldConsumer, transcriptscanstate.FieldPath).
			UpdateSize().UpdateModTimeNs().UpdateOffset().UpdateHeadHash().UpdateTailHash().
			UpdateState().UpdateUpdatedAt().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("save %s scan state: %w", c.name, err)
		}
	}
	return nil
}

// Delete removes the consumer's records for paths.
func (c *Scoped) Delete(ctx context.Context, paths []string) error {
	for start := 0; start < len(paths); start += saveChunk {
		end := min(start+saveChunk, len(paths))
		_, err := c.store.client.TranscriptScanState.Delete().
			Where(transcriptscanstate.ConsumerEQ(c.name), transcriptscanstate.PathIn(paths[start:end]...)).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("delete %s scan state: %w", c.name, err)
		}
	}
	return nil
}
