package services

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// Task 1.6.7b: recorded, never gated. Run in the background, numbers go to ADR-036 "Spike outcomes":
//
//	go test ./server/services -run '^$' -bench 'BenchmarkRange|BenchmarkAnalyze|BenchmarkLargeFileSearch' -benchtime 3x -benchmem &

func rangeBenchOps(b *testing.B, dir string) *FileOps {
	b.Helper()
	ops := NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: dir}, Flags: &stubFilesFlags{rangeRead: true}}, osRootBackend{})
	b.Cleanup(ops.analysis.Close)
	return ops
}

func awaitAnalysis(b *testing.B, ops *FileOps, name string) {
	b.Helper()
	fr, err := ops.readFileFacts("test-session", name)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := ops.analysis.Await(context.Background(), fr.Src.Key, ops.analysisRunnerFor(fr.Src)); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkAnalyze times the production background pass (sparse line index plus sha256) on a cold analysis cache.
func BenchmarkAnalyze(b *testing.B) {
	dir := b.TempDir()
	for _, mb := range []int{1, 16, 64, 256} {
		b.Run(fmt.Sprintf("LF/%dMB", mb), func(b *testing.B) {
			p := transportFile(b, dir, lineStyles[0], mb)
			b.SetBytes(int64(mb) * mib)
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				ops := rangeBenchOps(b, filepath.Dir(p))
				b.StartTimer()
				awaitAnalysis(b, ops, filepath.Base(p))
				ops.analysis.Close()
			}
		})
	}
}

// BenchmarkRangeRead times one 128 KiB window at pseudo-random lines of an analyzed file.
func BenchmarkRangeRead(b *testing.B) {
	dir := b.TempDir()
	for _, mb := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("LF/%dMB", mb), func(b *testing.B) {
			p := transportFile(b, dir, lineStyles[0], mb)
			ops := rangeBenchOps(b, filepath.Dir(p))
			awaitAnalysis(b, ops, filepath.Base(p))
			lines := int64(mb) * mib / int64(len(lineStyles[0].eol)+lineStyles[0].line)
			rng := rand.New(rand.NewSource(fixtureSeed))
			b.ReportAllocs()
			for b.Loop() {
				req := linesReq(filepath.Base(p), 1+rng.Int63n(lines), sessionv1.RangeDir_RANGE_DIR_AROUND, 400, 128<<10)
				if _, err := ops.ReadRange(context.Background(), req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLargeFileSearch searches one 63 MiB file (under rangeHashCapBytes) with large_files on, through the production walker.
func BenchmarkLargeFileSearch(b *testing.B) {
	dir := b.TempDir()
	transportFile(b, dir, lineStyles[0], 63)
	r, err := os.OpenRoot(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	opts := literalOpts(searchBenchQuery)
	opts.LargeFiles = true
	b.SetBytes(63 * mib)
	b.ReportAllocs()
	for b.Loop() {
		matches := 0
		stats, err := NewWalkSearcher().Search(context.Background(), r.FS(), opts, func(ms []SearchMatch) error { matches += len(ms); return nil })
		if err != nil {
			b.Fatal(err)
		}
		if matches == 0 || stats.LargeFilesSkipped != 0 {
			b.Fatalf("matches=%d skipped=%d", matches, stats.LargeFilesSkipped)
		}
	}
}
