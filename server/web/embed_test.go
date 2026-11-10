package web_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/server/web"
)

// The embedded FS must contain the staged dist tree (ADR-007).
func TestEmbeddedIndexHTML(t *testing.T) {
	fsys, err := web.GetDistFS()
	if err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<html") {
		t.Fatalf("index.html does not look like HTML: %q", b[:min(len(b), 80)])
	}
	files := 0
	_ = fs.WalkDir(fsys, ".", func(_ string, d fs.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			files++
		}
		return nil
	})
	sum := sha256.Sum256(b)
	t.Logf("index.html %d bytes sha256=%s, %d files embedded", len(b), hex.EncodeToString(sum[:]), files)
}
