package github

import "testing"

func TestParseGitHubRefWithHosts_PatternsCached(t *testing.T) {
	hosts := []string{"ghe.example.com"}
	const in = "https://ghe.example.com/owner/repo/pull/12"
	if _, err := ParseGitHubRefWithHosts(in, hosts); err != nil {
		t.Fatal(err)
	}
	// Compiling the 10 URL regexes costs hundreds of allocations; a cached call stays far below.
	if allocs := testing.AllocsPerRun(100, func() { _, _ = ParseGitHubRefWithHosts(in, hosts) }); allocs > 40 {
		t.Fatalf("allocs/op = %v, patterns are being recompiled per call", allocs)
	}
}

func BenchmarkParseGitHubRefWithHosts(b *testing.B) {
	hosts := []string{"ghe.example.com"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ParseGitHubRefWithHosts("https://ghe.example.com/owner/repo/pull/12", hosts)
	}
}
