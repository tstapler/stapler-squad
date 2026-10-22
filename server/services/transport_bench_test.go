package services

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// S-T2 (ADR-036) prototypes: single-pass index+sha256, streaming splice, and the
// 64 MiB literal scan with the per-file cap lifted. Run with:
//
//	go test -bench 'BenchmarkTransport|BenchmarkScan64' -run '^$' -benchmem ./server/services &
//
// Numbers are recorded in ADR-036 and never asserted in CI.

const (
	mib            = 1 << 20
	indexEveryLine = 1024 // sparse line index: one offset per 1024 lines
	ioChunk        = 1 << 20
	longLineChars  = 200
)

type lineStyle struct {
	name string
	eol  string
	line int // chars per line before the terminator
}

var lineStyles = []lineStyle{{"LF", "\n", 60}, {"CRLF", "\r\n", 60}, {"long200", "\n", longLineChars}}

// transportFile returns (creating once per dir) the deterministic sample file for
// style and size; the caller owns dir (b.TempDir of the top-level benchmark).
func transportFile(b *testing.B, dir string, style lineStyle, sizeMB int) string {
	b.Helper()
	p := filepath.Join(dir, fmt.Sprintf("%s-%d.txt", style.name, sizeMB))
	if _, err := os.Stat(p); err == nil {
		return p
	}
	f, err := os.Create(p)
	if err != nil {
		b.Fatal(err)
	}
	w := bufio.NewWriterSize(f, ioChunk)
	rng := rand.New(rand.NewSource(fixtureSeed))
	line := make([]byte, style.line)
	for written := 0; written < sizeMB*mib; {
		for i := range line {
			line[i] = byte('a' + rng.Intn(26))
		}
		if rng.Intn(5000) == 0 {
			copy(line, "TODO")
		}
		w.Write(line)
		w.WriteString(style.eol)
		written += style.line + len(style.eol)
	}
	if err := w.Flush(); err != nil {
		b.Fatal(err)
	}
	if err := f.Close(); err != nil {
		b.Fatal(err)
	}
	return p
}

// indexAndHash reads r once, returning sha256, line count and a sparse line-start index.
func indexAndHash(r io.Reader) (sum [32]byte, lines int, index []int64, err error) {
	h := sha256.New()
	buf := make([]byte, ioChunk)
	var off int64
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			chunk := buf[:n]
			for {
				i := bytes.IndexByte(chunk, '\n')
				if i < 0 {
					break
				}
				lines++
				if lines%indexEveryLine == 0 {
					index = append(index, off+int64(i)+1)
				}
				off += int64(i) + 1
				chunk = chunk[i+1:]
			}
			off += int64(len(chunk))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return sum, 0, nil, rerr
		}
	}
	copy(sum[:], h.Sum(nil))
	return sum, lines, index, nil
}

// splice writes prefix + insert + suffix of src (replacing delLen bytes at off) to a
// temp file in dir, hashing old and new content in the same pass.
func splice(src, dir string, off, delLen int64, insert []byte) (oldSum, newSum [32]byte, err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	tmp, err := os.CreateTemp(dir, ".x.ssq-tmp-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	oldH, newH := sha256.New(), sha256.New()
	out := io.MultiWriter(tmp, newH)
	r := io.TeeReader(in, oldH)
	buf := make([]byte, ioChunk)
	if _, err = io.CopyBuffer(out, io.LimitReader(r, off), buf); err != nil {
		return
	}
	if _, err = out.Write(insert); err != nil {
		return
	}
	if _, err = io.CopyN(io.Discard, r, delLen); err != nil {
		return
	}
	if _, err = io.CopyBuffer(out, r, buf); err != nil {
		return
	}
	if err = tmp.Sync(); err != nil {
		return
	}
	copy(oldSum[:], oldH.Sum(nil))
	copy(newSum[:], newH.Sum(nil))
	return
}

func BenchmarkTransportIndexHash(b *testing.B) {
	dir := b.TempDir()
	for _, st := range lineStyles {
		for _, mb := range []int{1, 16, 64, 256} {
			b.Run(fmt.Sprintf("%s/%dMB", st.name, mb), func(b *testing.B) {
				p := transportFile(b, dir, st, mb)
				fi, _ := os.Stat(p)
				b.SetBytes(fi.Size())
				b.ReportAllocs()
				for b.Loop() {
					f, err := os.Open(p)
					if err != nil {
						b.Fatal(err)
					}
					if _, _, _, err := indexAndHash(f); err != nil {
						b.Fatal(err)
					}
					f.Close()
				}
			})
		}
	}
}

func BenchmarkTransportSplice(b *testing.B) {
	dir := b.TempDir()
	insert := bytes.Repeat([]byte("x"), 100)
	for _, st := range lineStyles {
		for _, mb := range []int{1, 16, 64, 256} {
			b.Run(fmt.Sprintf("%s/%dMB", st.name, mb), func(b *testing.B) {
				p := transportFile(b, dir, st, mb)
				fi, _ := os.Stat(p)
				b.SetBytes(fi.Size())
				b.ReportAllocs()
				for b.Loop() {
					if _, _, err := splice(p, filepath.Dir(p), fi.Size()/2, 10, insert); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkScan64MiB measures the SearchContent prototype scanner (Task 0.1.2b) on
// one 64 MiB file with the per-file size cap lifted: whole-file read plus regexp,
// versus streaming chunks with a literal search.
func BenchmarkScan64MiB(b *testing.B) {
	p := transportFile(b, b.TempDir(), lineStyles[0], 64)
	re := regexp.MustCompile(regexp.QuoteMeta(searchBenchQuery))
	b.Run("readall+regexp", func(b *testing.B) {
		b.SetBytes(64 * mib)
		b.ReportAllocs()
		for b.Loop() {
			data, err := os.ReadFile(p)
			if err != nil {
				b.Fatal(err)
			}
			if !re.Match(data) {
				b.Fatal("no match")
			}
		}
	})
	b.Run("stream+bytes.Index", func(b *testing.B) {
		b.SetBytes(64 * mib)
		b.ReportAllocs()
		needle := []byte(searchBenchQuery)
		for b.Loop() {
			f, err := os.Open(p)
			if err != nil {
				b.Fatal(err)
			}
			found, buf := 0, make([]byte, ioChunk+len(needle))
			carry := 0
			for {
				n, rerr := f.Read(buf[carry:])
				chunk := buf[:carry+n]
				found += bytes.Count(chunk, needle)
				carry = min(len(needle)-1, len(chunk))
				copy(buf, chunk[len(chunk)-carry:])
				if rerr != nil {
					break
				}
			}
			f.Close()
			if found == 0 {
				b.Fatal("no match")
			}
		}
	})
}
