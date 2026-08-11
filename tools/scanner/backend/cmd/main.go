package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/tools/scanner/backend"
)

// FeatureDoc is the per-feature JSON file written to docs/registry/features/backend/<domain>/<action>.json.
// Flat structure — no nesting — keeps the files simple and diff-friendly.
// Proto-derived entries populate ProtoFile; HTTP handler entries populate HTTPMethod and HTTPPath.
type FeatureDoc struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Service      string    `json:"service"`
	Method       string    `json:"method"`
	ProtoFile    string    `json:"protoFile,omitempty"`
	HTTPMethod   string    `json:"httpMethod,omitempty"`
	HTTPPath     string    `json:"httpPath,omitempty"`
	MarkerFound  bool      `json:"markerFound"`
	HandlerFile  string    `json:"handlerFile,omitempty"`
	Tested       bool      `json:"tested"`
	TestIDs      []string  `json:"testIds"`
	LastModified time.Time `json:"lastModified"`
}

// featureIDToPath converts "session:create" → "<baseDir>/session/create.json".
func featureIDToPath(baseDir, id string) string {
	domain, action, found := strings.Cut(id, ":")
	if !found {
		// No colon — place directly under baseDir.
		return filepath.Join(baseDir, id+".json")
	}
	return filepath.Join(baseDir, domain, action+".json")
}

// loadExisting reads a per-feature file if it exists, returning its testIds, tested flag,
// lastModified timestamp, and the full parsed doc (for substantive-change comparison).
func loadExisting(path string) ([]string, bool, time.Time, *FeatureDoc) {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}, false, time.Time{}, nil
	}
	var doc FeatureDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return []string{}, false, time.Time{}, nil
	}
	return doc.TestIDs, doc.Tested, doc.LastModified, &doc
}

func main() {
	protoFile := "proto/session/v1/session.proto"
	servicesDir := "server/services/"
	outputDir := "docs/registry/features/backend"

	if len(os.Args) >= 2 {
		protoFile = os.Args[1]
	}
	if len(os.Args) >= 3 {
		servicesDir = os.Args[2]
	}
	if len(os.Args) >= 4 {
		outputDir = os.Args[3]
	}

	features, err := backend.ScanProto(protoFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning proto: %v\n", err)
		os.Exit(1)
	}

	markers, err := backend.ScanMarkers(servicesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning markers: %v\n", err)
		os.Exit(1)
	}

	// Discover plain HTTP handlers declared with // +http: markers.
	httpHandlers, err := backend.ScanHTTPHandlers(servicesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning HTTP handlers: %v\n", err)
		os.Exit(1)
	}
	features = append(features, httpHandlers...)

	written := 0
	for _, f := range features {
		doc := FeatureDoc{
			ID:           f.ID,
			Type:         f.Type,
			Service:      f.Service,
			Method:       f.Method,
			ProtoFile:    f.ProtoFile,
			HTTPMethod:   f.HTTPMethod,
			HTTPPath:     f.HTTPPath,
			MarkerFound:  f.MarkerFound,
			HandlerFile:  f.HandlerFile,
			Tested:       f.Tested,
			TestIDs:      f.TestIDs,
			LastModified: f.LastModified,
		}
		if m, ok := markers[f.ID]; ok {
			doc.MarkerFound = true
			doc.HandlerFile = m.FilePath
		}

		outPath := featureIDToPath(outputDir, f.ID)

		// Preserve human-editable testIds and stable lastModified from any existing file.
		existingIDs, existingTested, existingLastMod, existingDoc := loadExisting(outPath)
		if len(existingIDs) > 0 {
			doc.TestIDs = existingIDs
			doc.Tested = existingTested
		}

		// Only update LastModified when substantive content changed.
		// Git checkout resets file mtimes, so f.LastModified (from os.Stat) drifts on every
		// CI run even when the proto/handler content is identical. Preserve the committed
		// lastModified unless an actual field changed.
		if existingDoc != nil && !existingLastMod.IsZero() {
			substantivelyChanged := existingDoc.ID != doc.ID ||
				existingDoc.Type != doc.Type ||
				existingDoc.Service != doc.Service ||
				existingDoc.Method != doc.Method ||
				existingDoc.ProtoFile != doc.ProtoFile ||
				existingDoc.HTTPMethod != doc.HTTPMethod ||
				existingDoc.HTTPPath != doc.HTTPPath ||
				existingDoc.MarkerFound != doc.MarkerFound ||
				existingDoc.HandlerFile != doc.HandlerFile
			if !substantivelyChanged {
				doc.LastModified = existingLastMod
			}
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating directory %s: %v\n", filepath.Dir(outPath), err)
			os.Exit(1)
		}

		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error marshaling %s: %v\n", f.ID, err)
			os.Exit(1)
		}
		data = append(data, '\n')

		if err := os.WriteFile(outPath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outPath, err)
			os.Exit(1)
		}
		written++
	}

	fmt.Printf("Wrote %d feature files to %s\n", written, outputDir)
}
