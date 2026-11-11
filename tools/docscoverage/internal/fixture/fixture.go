// Package fixture builds throwaway repository trees for docscoverage tests.
package fixture

import (
	"os"
	"path/filepath"
	"testing"
)

// DocLoader is a docLoader.ts with the given slugs as its DOC_FILES literal.
func DocLoader(slugs ...string) []byte {
	out := "export const DOC_FILES = [\n"
	for _, s := range slugs {
		out += "  \"" + s + "\",\n"
	}
	return []byte(out + "];\n")
}

// Feature is a minimal frontend registry file for id.
func Feature(id, name string) []byte {
	return []byte(`{"id": "` + id + `", "type": "frontend", "name": "` + name + `", "tested": false, "testIds": []}`)
}

// Write writes files (repo-relative path -> content) under a new temp dir and returns it.
func Write(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	WriteInto(t, dir, files)
	return dir
}

// WriteInto writes files under dir.
func WriteInto(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	for p, data := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
