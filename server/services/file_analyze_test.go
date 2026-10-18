package services

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// naiveLineStarts returns the start offset of every line by scanning for "\n".
func naiveLineStarts(data []byte) []int64 {
	if len(data) == 0 {
		return nil
	}
	starts := []int64{0}
	for i, b := range data {
		if b == '\n' && i+1 < len(data) {
			starts = append(starts, int64(i)+1)
		}
	}
	return starts
}

func randomLines(rng *rand.Rand, lines, maxLen int, eol string) []byte {
	var buf bytes.Buffer
	for i := 0; i < lines; i++ {
		buf.WriteString(strings.Repeat("x", rng.Intn(maxLen+1)))
		buf.WriteString(eol)
	}
	return buf.Bytes()
}

func requireMatchesNaive(t *testing.T, data []byte, probes []int64) *Analysis {
	t.Helper()
	a, err := Analyze(context.Background(), bytes.NewReader(data))
	require.NoError(t, err)
	starts := naiveLineStarts(data)
	assert.Equal(t, int64(len(starts)), a.LineCount)
	assert.Equal(t, int64(len(data)), a.Size)
	assert.Equal(t, NewFileVersionToken(int64(len(data)), 7, data).SHA256, a.SHA256)
	assert.Len(t, a.SHA256, 64)
	r := bytes.NewReader(data)
	for _, line := range probes {
		if line >= int64(len(starts)) {
			continue
		}
		got, err := a.LineToByte(r, line)
		require.NoError(t, err)
		assert.Equal(t, starts[line], got, "line %d", line)
		back, err := a.ByteToLine(r, got)
		require.NoError(t, err)
		assert.Equal(t, line, back, "round trip line %d", line)
	}
	return a
}

func TestAnalyze_ShouldMatchNaiveScan_When_RandomLineLengthsAndSparseCheckpoints(t *testing.T) {
	t.Parallel()
	t.Run("two million LF lines", func(t *testing.T) {
		t.Parallel()
		rng := rand.New(rand.NewSource(42))
		data := randomLines(rng, 2_000_000, 12, "\n")
		a := requireMatchesNaive(t, data, []int64{0, 1, 4095, 4096, 4097, 8192, 1_234_567, 1_999_999})
		assert.Equal(t, int64(2_000_000), a.LineCount)
		assert.Equal(t, int64(2_000_000), a.LFCount)
		assert.Len(t, a.Index.Checkpoints, 489, "checkpoint every 4096 lines")
		assert.Zero(t, a.CRLFCount+a.CRCount+a.NULCount)
	})
	t.Run("random distributions", func(t *testing.T) {
		t.Parallel()
		for seed := int64(0); seed < 20; seed++ {
			rng := rand.New(rand.NewSource(seed))
			data := randomLines(rng, 1+rng.Intn(30_000), 1+rng.Intn(300), "\n")
			if seed%2 == 1 {
				data = bytes.TrimSuffix(data, []byte("\n"))
			}
			probes := make([]int64, 50)
			for i := range probes {
				probes[i] = rng.Int63n(40_000)
			}
			requireMatchesNaive(t, data, probes)
		}
	})
	t.Run("no trailing newline", func(t *testing.T) {
		t.Parallel()
		a := requireMatchesNaive(t, []byte("a\nbb\nccc"), []int64{0, 1, 2})
		assert.Equal(t, int64(3), a.LineCount)
		assert.Equal(t, int64(3), a.MaxLineLen)
	})
	t.Run("empty file", func(t *testing.T) {
		t.Parallel()
		a := requireMatchesNaive(t, nil, []int64{0})
		assert.Zero(t, a.LineCount)
		assert.Equal(t, []int64{0}, a.Index.Checkpoints)
		off, err := a.LineToByte(bytes.NewReader(nil), 0)
		require.NoError(t, err)
		assert.Zero(t, off)
	})
	t.Run("single 300 KiB line", func(t *testing.T) {
		t.Parallel()
		data := []byte(strings.Repeat("y", 300*1024))
		a := requireMatchesNaive(t, data, []int64{0})
		assert.Equal(t, int64(1), a.LineCount)
		assert.Equal(t, int64(300*1024), a.MaxLineLen)
	})
	t.Run("checkpoint boundary with trailing newline", func(t *testing.T) {
		t.Parallel()
		data := bytes.Repeat([]byte("\n"), lineIndexInterval)
		a := requireMatchesNaive(t, data, []int64{0, lineIndexInterval - 1})
		assert.Equal(t, []int64{0}, a.Index.Checkpoints)
		off, err := a.LineToByte(bytes.NewReader(data), a.LineCount)
		require.NoError(t, err)
		assert.Equal(t, a.Size, off)
		_, err = a.LineToByte(bytes.NewReader(data), a.LineCount+1)
		assert.ErrorIs(t, err, ErrLineOutOfRange)
	})
}

func TestAnalyze_ShouldCountEolNulAndBom_When_MixedLineEndings(t *testing.T) {
	t.Parallel()
	data := []byte("\xEF\xBB\xBFa\r\nb\nc\rd\r\r\ne\x00\x00\r")
	a, err := Analyze(context.Background(), bytes.NewReader(data))
	require.NoError(t, err)
	assert.True(t, a.HasBOM)
	assert.Equal(t, int64(2), a.CRLFCount, "CRLF: after a and after the second CR")
	assert.Equal(t, int64(1), a.LFCount)
	assert.Equal(t, int64(3), a.CRCount, "lone CR after c, before the CRLF, and at EOF")
	assert.Equal(t, int64(2), a.NULCount)
	assert.Equal(t, int64(4), a.LineCount)
}

func TestAnalyze_ShouldReturnCanceledPromptly_When_ContextCancelledMidPass(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 256 MiB of generated lines; the reader cancels after 1 MiB.
	src := &cancelAfterReader{remaining: 256 << 20, cancelAt: 255 << 20, cancel: cancel}
	start := time.Now()
	_, err := Analyze(ctx, src)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), 200*time.Millisecond)
	assert.Less(t, int64(256<<20-src.remaining), int64(4<<20), "stopped within a chunk or two of cancel")
}

// cancelAfterReader yields "line\n" bytes and calls cancel once cancelAt bytes remain.
type cancelAfterReader struct {
	remaining int64
	cancelAt  int64
	cancel    func()
}

func (c *cancelAfterReader) Read(p []byte) (int, error) {
	if c.remaining <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > c.remaining {
		n = c.remaining
	}
	for i := int64(0); i < n; i++ {
		p[i] = 'a'
		if i%64 == 63 {
			p[i] = '\n'
		}
	}
	c.remaining -= n
	if c.remaining <= c.cancelAt {
		c.cancel()
	}
	return int(n), nil
}

func FuzzAnalyze_ShouldNeverPanicAndKeepIndexConsistent(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("a\r\nb\rc\n\x00"))
	f.Add([]byte("\xEF\xBB\xBFhello\nworld"))
	f.Add(bytes.Repeat([]byte("\n"), lineIndexInterval*2+1))
	f.Fuzz(func(t *testing.T, data []byte) {
		a, err := Analyze(context.Background(), bytes.NewReader(data))
		require.NoError(t, err)
		prev := int64(-1)
		for _, cp := range a.Index.Checkpoints {
			require.Greater(t, cp, prev)
			prev = cp
		}
		require.Equal(t, int64(len(data)), a.Size)
		starts := naiveLineStarts(data)
		require.Equal(t, int64(len(starts)), a.LineCount)
		r := bytes.NewReader(data)
		for n := int64(0); n < a.LineCount; n++ {
			off, err := a.LineToByte(r, n)
			require.NoError(t, err)
			require.Equal(t, starts[n], off)
			back, err := a.ByteToLine(r, off)
			require.NoError(t, err)
			require.Equal(t, n, back)
		}
		if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) && a.LineCount > 0 {
			off, _ := a.LineToByte(r, 0)
			require.Zero(t, off, "BOM leaves raw offsets unshifted")
		}
	})
}
