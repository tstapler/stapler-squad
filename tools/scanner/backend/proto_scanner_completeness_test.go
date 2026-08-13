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
func TestMethodToIDCompleteness(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	protoDir := filepath.Join(repoRoot, "proto", "session", "v1")

	// Hardcoded list of proto files to check (alphabetical order)
	protoFiles := []string{
		"backlog.proto",
		"events.proto",
		"session.proto",
		"types.proto",
		"unfinished.proto",
	}

	// Collect all RPC methods found across all proto files
	foundMethods := make(map[string]string) // method name -> proto file it came from

	for _, protoFile := range protoFiles {
		fullPath := filepath.Join(protoDir, protoFile)

		// Use ScanProto to extract features, which internally extracts method names
		features, err := ScanProto(fullPath)
		if err != nil {
			t.Skipf("proto file not found: %s: %v", fullPath, err)
		}

		// Record each method found
		for _, f := range features {
			foundMethods[f.Method] = protoFile
		}
	}

	if len(foundMethods) == 0 {
		t.Fatalf("no RPC methods found in any proto files; check that proto files exist and are readable")
	}

	// Check each found method against methodToID
	var unmapped []string
	for method := range foundMethods {
		if _, exists := methodToID[method]; !exists {
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
