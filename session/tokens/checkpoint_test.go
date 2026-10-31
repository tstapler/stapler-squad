package tokens

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/scanstate"
)

// fixtureNames are the transcripts used to compare incremental and full parses.
var fixtureNames = []string{"valid_session", "multi_model", "cache_heavy", "malformed_line", "partial_write"}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".jsonl"))
	require.NoError(t, err)
	return b
}

// normalized drops the wall-clock field so results from separate parses compare equal.
func normalized(r *ParseResult) ParseResult {
	c := *r
	c.ParsedAt = zeroTime
	return c
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func appendFile(t *testing.T, path string, data []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestParseFileFrom_IncrementalChunksEqualFullParse(t *testing.T) {
	t.Parallel()
	for _, name := range fixtureNames {
		for _, chunks := range []int{2, 3, 7} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				data := readFixture(t, name)
				path := filepath.Join(t.TempDir(), "sess.jsonl")
				p := NewParser()

				var cp *Checkpoint
				step := len(data)/chunks + 1
				for end := step; ; end += step {
					end = min(end, len(data)) // cuts land mid-line on purpose
					writeFile(t, path, data[:end])
					var err error
					_, cp, err = p.ParseFileFrom(path, cp)
					require.NoError(t, err)
					if end == len(data) {
						break
					}
				}

				got, _, err := p.ParseFileFrom(path, cp)
				require.NoError(t, err)
				want, err := p.ParseFile(path)
				require.NoError(t, err)
				assert.Equal(t, normalized(want), normalized(got))
			})
		}
	}
}

func TestParseFileFrom_ResumeReadsOnlyAppendedBytes(t *testing.T) {
	t.Parallel()
	data := readFixture(t, "valid_session")
	path := filepath.Join(t.TempDir(), "sess.jsonl")
	writeFile(t, path, data)
	p := NewParser()

	_, cp1, err := p.ParseFileFrom(path, nil)
	require.NoError(t, err)
	require.Equal(t, int64(len(data)), cp1.Pos.Offset, "newline-terminated fixture is fully consumed")

	firstInput := cp1.Result().TotalInput
	appendFile(t, path, data) // a second copy of the same turns
	res, cp2, err := p.ParseFileFrom(path, cp1)
	require.NoError(t, err)
	assert.Equal(t, int64(2*len(data)), cp2.Pos.Offset)
	assert.Equal(t, 2*cp1.Result().TotalInput, res.TotalInput, "resumed totals include old and new turns")
	assert.Equal(t, firstInput, cp1.Result().TotalInput, "prev checkpoint untouched")
	assert.Len(t, res.TurnTimeline, 2*len(cp1.Result().TurnTimeline))
}

func TestParseFileFrom_UnterminatedTailIsHeldBackFromCheckpointOnly(t *testing.T) {
	t.Parallel()
	data := readFixture(t, "valid_session")
	for len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	path := filepath.Join(t.TempDir(), "sess.jsonl")
	writeFile(t, path, data)
	p := NewParser()

	res, cp, err := p.ParseFileFrom(path, nil)
	require.NoError(t, err)
	want, err := p.ParseFile(path)
	require.NoError(t, err)
	assert.Equal(t, normalized(want), normalized(res), "published result still counts a complete unterminated last line")
	assert.Less(t, cp.Pos.Offset, int64(len(data)), "checkpoint stops before the unterminated line")

	appendFile(t, path, []byte("\n"))
	res2, cp2, err := p.ParseFileFrom(path, cp)
	require.NoError(t, err)
	want, err = p.ParseFile(path)
	require.NoError(t, err)
	assert.Equal(t, normalized(want), normalized(res2), "line counted exactly once after its newline arrives")
	assert.Equal(t, int64(len(data)+1), cp2.Pos.Offset)
}

func TestParseFileFrom_TruncatedOrReplacedFileIsReparsedFromStart(t *testing.T) {
	t.Parallel()
	short := readFixture(t, "multi_model")
	long := append(append([]byte{}, short...), short...)
	other := readFixture(t, "cache_heavy")
	for len(other) <= len(long) { // the replacement must be larger so only the fingerprint can catch it
		other = append(other, readFixture(t, "cache_heavy")...)
	}

	for name, replacement := range map[string][]byte{"truncated": short, "replaced-larger": other} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "sess.jsonl")
			writeFile(t, path, long)
			p := NewParser()
			_, cp, err := p.ParseFileFrom(path, nil)
			require.NoError(t, err)

			writeFile(t, path, replacement)
			got, _, err := p.ParseFileFrom(path, cp)
			require.NoError(t, err)
			want, err := p.ParseFile(path)
			require.NoError(t, err)
			assert.Equal(t, normalized(want), normalized(got))
		})
	}
}

func TestCheckpoint_EncodeDecodeRoundTripResumesIdentically(t *testing.T) {
	t.Parallel()
	data := readFixture(t, "multi_model")
	path := filepath.Join(t.TempDir(), "sess.jsonl")
	writeFile(t, path, data)
	p := NewParser()
	_, cp, err := p.ParseFileFrom(path, nil)
	require.NoError(t, err)

	blob, err := cp.Encode()
	require.NoError(t, err)
	restored, err := DecodeCheckpoint(blob, cp.Pos)
	require.NoError(t, err)
	assert.Equal(t, normalized(cp.Result()), normalized(restored.Result()))

	appendFile(t, path, data)
	viaOriginal, _, err := p.ParseFileFrom(path, cp)
	require.NoError(t, err)
	viaRestored, _, err := p.ParseFileFrom(path, restored)
	require.NoError(t, err)
	assert.Equal(t, normalized(viaOriginal), normalized(viaRestored))
}

// memStates is an in-memory StateStore standing in for the ent table.
type memStates struct {
	mu      sync.Mutex
	recs    map[string]scanstate.Record
	deleted []string
}

func newMemStates() *memStates { return &memStates{recs: map[string]scanstate.Record{}} }

func (m *memStates) Load(context.Context) (map[string]scanstate.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]scanstate.Record, len(m.recs))
	for k, v := range m.recs {
		out[k] = v
	}
	return out, nil
}

func (m *memStates) Save(_ context.Context, recs []scanstate.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range recs {
		m.recs[r.Path] = r
	}
	return nil
}

func (m *memStates) Delete(_ context.Context, paths []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range paths {
		delete(m.recs, p)
		m.deleted = append(m.deleted, p)
	}
	return nil
}

func TestTokenStore_RestartDoesNotReparseUnchangedTranscript(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	writeFile(t, path, readFixture(t, "valid_session"))
	states := newMemStates()
	ctx := context.Background()

	first := NewTokenStore(dir, WithStateStore(states))
	first.parseAndCache(path)
	require.NoError(t, first.FlushState(ctx))
	require.Len(t, states.recs, 1)

	second := NewTokenStore(dir, WithStateStore(states))
	second.restoreState(ctx)
	restored := second.GetByUUID("sess")
	require.NotNil(t, restored, "state restored before any parse")

	second.parseAndCache(path)
	assert.Same(t, restored, second.GetByUUID("sess"), "an unchanged file must not be reparsed after restart")
	assert.Equal(t, normalized(first.GetByUUID("sess")), normalized(restored))
}

func TestTokenStore_RestartThenAppendParsesIncrementally(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	data := readFixture(t, "multi_model")
	writeFile(t, path, data)
	states := newMemStates()
	ctx := context.Background()

	first := NewTokenStore(dir, WithStateStore(states))
	first.parseAndCache(path)
	require.NoError(t, first.FlushState(ctx))

	appendFile(t, path, data)
	second := NewTokenStore(dir, WithStateStore(states))
	second.restoreState(ctx)
	second.parseAndCache(path)

	want, err := NewParser().ParseFile(path)
	require.NoError(t, err)
	assert.Equal(t, normalized(want), normalized(second.GetByUUID("sess")))
	assert.Equal(t, int64(2*len(data)), second.cache[path].cp.Pos.Offset)
}

func TestTokenStore_WalkDropsCheckpointsOfDeletedTranscripts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keep, gone := filepath.Join(dir, "keep.jsonl"), filepath.Join(dir, "gone.jsonl")
	writeFile(t, keep, readFixture(t, "valid_session"))
	writeFile(t, gone, readFixture(t, "valid_session"))
	states := newMemStates()
	ctx := context.Background()

	first := NewTokenStore(dir, WithStateStore(states))
	first.parseAndCache(keep)
	first.parseAndCache(gone)
	require.NoError(t, first.FlushState(ctx))
	require.NoError(t, os.Remove(gone))

	second := NewTokenStore(dir, WithStateStore(states))
	second.walkAndEnqueue(ctx) // synchronous: restore, walk, prune

	assert.NotNil(t, second.GetByUUID("keep"))
	assert.Nil(t, second.GetByUUID("gone"))
	assert.Equal(t, []string{gone}, states.deleted)
}

var zeroTime = (&ParseResult{}).ParsedAt

func TestTokenStore_RestoreDoesNotClobberNewerLiveParse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	data := readFixture(t, "multi_model")
	writeFile(t, path, data)
	states := newMemStates()
	ctx := context.Background()

	first := NewTokenStore(dir, WithStateStore(states))
	first.parseAndCache(path)
	require.NoError(t, first.FlushState(ctx))

	appendFile(t, path, data)
	second := NewTokenStore(dir, WithStateStore(states))
	second.parseAndCache(path) // a live event beat the restore
	live := second.GetByUUID("sess")
	second.restoreState(ctx)

	assert.Same(t, live, second.GetByUUID("sess"), "the older persisted checkpoint must not replace the live parse")
}

func TestDecodeCheckpoint_RejectsOtherParserVersions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sess.jsonl")
	writeFile(t, path, readFixture(t, "valid_session"))
	_, cp, err := NewParser().ParseFileFrom(path, nil)
	require.NoError(t, err)

	st := cp.state
	blob, err := st.encode()
	require.NoError(t, err)
	_, err = DecodeCheckpoint(blob, cp.Pos)
	require.NoError(t, err)

	// A checkpoint written by a different parser version must be ignored.
	stale := encodeWithVersion(t, st, checkpointVersion+1)
	_, err = DecodeCheckpoint(stale, cp.Pos)
	assert.Error(t, err)
}

// encodeWithVersion serializes st as if written by parser version v.
func encodeWithVersion(t *testing.T, st *parseState, v int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	require.NoError(t, json.NewEncoder(zw).Encode(stateJSON{
		Version: v, Result: st.result, ModelCounts: st.modelCounts,
		Ring: st.ring.inOrder(), HumanTurnIndex: st.humanTurnIndex,
	}))
	require.NoError(t, zw.Close())
	return buf.Bytes()
}
