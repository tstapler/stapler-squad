package services

// Fixtures used by os_root_semantics_test.go and the transport spike and benchmarks.

import (
	"bytes"
	"math/rand"
	"os"
	"testing"
)

const (
	fixtureSeed      = 42
	searchBenchQuery = "TODO"
)

var fixtureWords = []string{"alpha", "beta", "gamma", "delta", "func", "return", "TODO", "value", "index", "config"}

func fixtureText(rng *rand.Rand, lines int) []byte {
	var b bytes.Buffer
	for l := 0; l < lines; l++ {
		n := 3 + rng.Intn(8)
		for w := 0; w < n; w++ {
			b.WriteString(fixtureWords[rng.Intn(len(fixtureWords))])
			b.WriteByte(' ')
		}
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func mustMkdir(tb testing.TB, p string) {
	tb.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		tb.Fatal(err)
	}
}

func writeFixtureFile(tb testing.TB, p string, data []byte) {
	tb.Helper()
	if err := os.WriteFile(p, data, 0o644); err != nil {
		tb.Fatal(err)
	}
}
