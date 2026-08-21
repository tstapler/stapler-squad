package backend

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ScanHTTPHandlers walks servicesDir looking for Go files that contain
// // +http: markers. Each marker declares a plain HTTP endpoint that is not
// represented in any proto file and therefore would not be discovered by
// ScanProto.
//
// Marker format (on a single comment line):
//
//	// +http: METHOD /path/to/endpoint feature-id
//
// Example:
//
//	// +http: POST /api/v1/upload-image upload:image
//
// Files ending in .pb.go or _test.go are skipped.
func ScanHTTPHandlers(servicesDir string) ([]BackendFeature, error) {
	var features []BackendFeature

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
		if strings.HasSuffix(name, ".pb.go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		entries, err := scanFileForHTTPMarkers(path, info.ModTime())
		if err != nil {
			return err
		}
		features = append(features, entries...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return features, nil
}

// scanFileForHTTPMarkers scans a single Go file for // +http: comment lines.
func scanFileForHTTPMarkers(path string, modTime time.Time) ([]BackendFeature, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	const prefix = "// +http:"
	var features []BackendFeature

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := strings.TrimSpace(line[len(prefix):])
		// Expect: METHOD /path feature-id
		parts := strings.Fields(rest)
		if len(parts) < 3 {
			continue
		}
		httpMethod := parts[0]
		httpPath := parts[1]
		featureID := parts[2]
		if featureID == "" {
			continue
		}

		features = append(features, BackendFeature{
			ID:           featureID,
			Type:         "backend",
			Service:      "HTTPHandler",
			Method:       httpMethod + " " + httpPath,
			HTTPMethod:   httpMethod,
			HTTPPath:     httpPath,
			HandlerFile:  path,
			MarkerFound:  true,
			Tested:       false,
			TestIDs:      []string{},
			LastModified: modTime,
		})
	}
	return features, scanner.Err()
}
