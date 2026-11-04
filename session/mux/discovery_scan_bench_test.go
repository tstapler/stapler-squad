package mux

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// populateTempDir fills dir with decoy entries and n stale (non-socket)
// ssq-mux-*.sock files, approximating a busy /tmp.
func populateTempDir(tb testing.TB, dir string, decoys, n int) {
	tb.Helper()
	for i := 0; i < decoys; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("decoy-%04d.tmp", i)), nil, 0o600); err != nil {
			tb.Fatal(err)
		}
	}
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("ssq-mux-%d.sock", i)), nil, 0o600); err != nil {
			tb.Fatal(err)
		}
	}
}

// BenchmarkDiscoveryScanUnchangedDir measures the steady-state poll tick:
// nothing in the socket directory changed since the last scan.
func BenchmarkDiscoveryScanUnchangedDir(b *testing.B) {
	for _, decoys := range []int{200, 2000} {
		b.Run(fmt.Sprintf("tmp_entries=%d", decoys), func(b *testing.B) {
			dir := b.TempDir()
			b.Setenv("TMPDIR", dir)
			populateTempDir(b, dir, decoys, 5)
			d := NewDiscovery()
			if _, err := d.Scan(); err != nil { // warm: first scan is always full
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := d.Scan(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
