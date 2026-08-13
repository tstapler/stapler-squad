package backend

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// MarkerEntry records a // +api: comment found in a Go source file.
type MarkerEntry struct {
	FeatureID string
	FilePath  string
	FuncName  string
}

// ScanMarkers walks servicesDir looking for Go files that contain // +api: comments.
// Files ending in .pb.go or _test.go are skipped.
// Returns a map from feature ID to MarkerEntry.
func ScanMarkers(servicesDir string) (map[string]MarkerEntry, error) {
	result := make(map[string]MarkerEntry)

	err := filepath.Walk(servicesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		// Skip generated and test files
		if strings.HasSuffix(name, ".pb.go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		entries, err := scanFileForMarkers(path)
		if err != nil {
			return err
		}
		for _, e := range entries {
			result[e.FeatureID] = e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// scanFileForMarkers scans a single Go file for // +api: comment lines.
func scanFileForMarkers(path string) ([]MarkerEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []MarkerEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		const prefix = "// +api:"
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		featureID := strings.TrimSpace(line[len(prefix):])
		if featureID == "" {
			continue
		}
		entries = append(entries, MarkerEntry{
			FeatureID: featureID,
			FilePath:  path,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
