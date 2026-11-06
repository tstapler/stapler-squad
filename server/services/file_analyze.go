package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
)

// Constants from ADR-036 "Spike outcomes" (S-T2, Task 0.1.7b): indexing plus
// hashing 64 MB costs about 50 ms, so the 4,096-line checkpoint interval and the
// 64 MiB hash cap are confirmed as proposed.
const (
	lineIndexInterval = 4096
	analyzeChunkBytes = 256 * 1024
)

// Analysis is everything one streaming pass over a file establishes. Offsets are
// raw file offsets: a BOM never shifts them.
type Analysis struct {
	Size       int64
	LineCount  int64
	Index      LineIndex
	LFCount    int64 // "\n" not preceded by "\r"
	CRLFCount  int64
	CRCount    int64 // "\r" not followed by "\n"
	NULCount   int64
	MaxLineLen int64 // longest line in bytes, excluding its "\n"
	HasBOM     bool
	SHA256     string // lowercase hex of the whole file
}

// analyzer is the byte-at-a-time state machine behind Analyze.
type analyzer struct {
	a         Analysis
	h         hash.Hash
	pos       int64
	lineStart int64
	prevCR    bool
}

func (z *analyzer) feed(buf []byte) {
	_, _ = z.h.Write(buf)
	for _, b := range buf {
		switch b {
		case '\n':
			z.endLine()
		case '\r':
			if z.prevCR {
				z.a.CRCount++
			}
			z.prevCR = true
		case 0:
			z.a.NULCount++
			z.loneCR()
		default:
			z.loneCR()
		}
		z.pos++
	}
}

func (z *analyzer) loneCR() {
	if z.prevCR {
		z.a.CRCount++
		z.prevCR = false
	}
}

func (z *analyzer) endLine() {
	if z.prevCR {
		z.a.CRLFCount++
		z.prevCR = false
	} else {
		z.a.LFCount++
	}
	z.noteLineLen(z.pos - z.lineStart)
	z.a.LineCount++
	z.lineStart = z.pos + 1
	if z.a.LineCount%lineIndexInterval == 0 {
		z.a.Index.Checkpoints = append(z.a.Index.Checkpoints, z.lineStart)
	}
}

func (z *analyzer) noteLineLen(n int64) {
	if n > z.a.MaxLineLen {
		z.a.MaxLineLen = n
	}
}

func (z *analyzer) finish() *Analysis {
	z.loneCR()
	if z.pos > z.lineStart {
		z.noteLineLen(z.pos - z.lineStart)
		z.a.LineCount++
	}
	// A trailing newline on a checkpoint boundary starts no line.
	cps := z.a.Index.Checkpoints
	for len(cps) > 1 && cps[len(cps)-1] >= z.pos {
		cps = cps[:len(cps)-1]
	}
	z.a.Index.Checkpoints = cps
	z.a.Size = z.pos
	z.a.SHA256 = hex.EncodeToString(z.h.Sum(nil))
	return &z.a
}

// Analyze makes one streaming pass over r: line index, EOL/NUL statistics, the
// longest line and the whole-file sha256. It checks ctx between chunks.
func Analyze(ctx context.Context, r io.Reader) (*Analysis, error) {
	z := &analyzer{h: sha256.New()}
	z.a.Index = LineIndex{Interval: lineIndexInterval, Checkpoints: []int64{0}}
	buf := make([]byte, analyzeChunkBytes)
	for first := true; ; first = false {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := io.ReadFull(r, buf)
		if first && n >= 3 {
			z.a.HasBOM = buf[0] == 0xEF && buf[1] == 0xBB && buf[2] == 0xBF
		}
		z.feed(buf[:n])
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return z.finish(), nil
		}
		if err != nil {
			return nil, err
		}
	}
}
