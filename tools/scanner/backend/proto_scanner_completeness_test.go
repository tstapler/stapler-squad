package backend

import (
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// TestMethodToIDCompleteness verifies that every RPC method found in all proto files
// in proto/session/v1/ has a corresponding entry in methodToID.
// This catches regressions when new RPCs are added without updating the map.
//
// Globs proto/session/v1/*.proto from disk rather than hardcoding a file list --
// a hardcoded list here is exactly the bug class this test exists to catch, just one
// enumeration removed: ssh-remote-workspaces Phase 6 Epic 6.3 found remote.proto's RPCs
// invisible to registry-generate-backend (Makefile), prune-stale-backend.sh, AND
// validate-registry.sh -- THREE separate hardcoded proto lists, all missing it -- and this
// test's own list (a fourth) was no different until now. Globbing means a fifth hardcoded
// list can never again drift from what actually exists on disk.
func TestMethodToIDCompleteness(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	protoDir := filepath.Join(repoRoot, "proto", "session", "v1")

	protoFiles, err := filepath.Glob(filepath.Join(protoDir, "*.proto"))
	if err != nil {
		t.Fatalf("failed to glob proto files in %s: %v", protoDir, err)
	}
	if len(protoFiles) == 0 {
		t.Fatalf("no .proto files found in %s; check the path is correct", protoDir)
	}
	sort.Strings(protoFiles)

	// Collect all RPC methods found across all proto files
	foundMethods := make(map[string]string) // method name -> proto file it came from

	for _, fullPath := range protoFiles {
		protoFile := filepath.Base(fullPath)

		// Use ScanProto to extract features, which internally extracts method names
		features, err := ScanProto(fullPath)
		if err != nil {
			t.Fatalf("failed to scan proto file %s: %v", fullPath, err)
		}

		// Record each method found
		for _, f := range features {
			foundMethods[f.Method] = protoFile
		}
	}

	if len(foundMethods) == 0 {
		t.Fatalf("no RPC methods found in any proto files; check that proto files exist and are readable")
	}

	// knownPreexistingGaps are methods with NO methodToID entry that ScanProto's fallback
	// (id = method name verbatim, see proto_scanner.go's ScanProto) already handles: each has
	// a real, currently-generated per-feature file under docs/registry/features/backend/ using
	// that PascalCase fallback id (e.g. "GetInsightsSummary.json"), just not the kebab-case
	// "scope:action" id convention every properly-mapped RPC uses. Surfaced by switching this
	// test from a hardcoded proto file list to a glob (ssh-remote-workspaces Phase 6 Epic
	// 6.3) -- pre-existing debt from before that change, not something it introduced.
	// Allowlisted rather than silently ignored (so this test stays a real regression guard for
	// every method NOT in this list) and rather than mass-fixed here (giving each a proper
	// kebab-case id would rename 10 already-committed registry files' ids, which
	// registry-diff/validate-registry.sh would then read as 10 RPCs removed + 10 added --
	// a real, separate cleanup, out of scope for a completeness-test fix).
	knownPreexistingGaps := map[string]bool{
		"ListGitHubAccounts":       true, // github_user.proto
		"PollGitHubDeviceAuth":     true, // github_user.proto
		"RevokeGitHubToken":        true, // github_user.proto
		"StartGitHubDeviceAuth":    true, // github_user.proto
		"GetInsightsSummary":       true, // insights.proto
		"GetSessionTurnTimeline":   true, // insights.proto
		"ListSessionTokens":        true, // insights.proto
		"WatchInsights":            true, // insights.proto
		"GetSessionSummary":        true, // session_summary.proto
		"RegenerateSessionSummary": true, // session_summary.proto
	}

	// Check each found method against methodToID
	var unmapped []string
	for method := range foundMethods {
		if _, exists := methodToID[method]; !exists && !knownPreexistingGaps[method] {
			unmapped = append(unmapped, method)
		}
	}

	if len(unmapped) > 0 {
		sort.Strings(unmapped)
		t.Errorf("%d RPC method(s) found in proto files but missing from methodToID map:\n", len(unmapped))
		for _, m := range unmapped {
			protoFile := foundMethods[m]
			t.Logf("  - %s (from %s)", m, protoFile)
		}
		t.Log("\nAdd these entries to methodToID in proto_scanner.go:")
		for _, m := range unmapped {
			protoFile := foundMethods[m]
			t.Logf(`  "%s": "feature:id-for-%s", // from %s`, m, m, protoFile)
		}
	}
}
