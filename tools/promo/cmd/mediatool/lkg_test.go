package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var lkgNow = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

// stubGH answers `run list` with listJSON and `run download` by calling download.
func stubGH(listJSON string, download func(args []string) error) ghRunner {
	return func(args ...string) ([]byte, error) {
		switch args[1] {
		case "list":
			return []byte(listJSON), nil
		case "download":
			return nil, download(args)
		}
		return nil, errors.New("unexpected gh call")
	}
}

func writeLKGManifest(t *testing.T, dir, finished string) {
	t.Helper()
	body := `{"version":1,"commit":"abc123","startedAt":"` + finished + `","finishedAt":"` + finished + `","entries":[{"name":"dashboard-lifecycle","format":"webm","status":"recorded","bytes":10,"commit":"abc123","themeId":"grimdark"}]}`
	if err := os.WriteFile(filepath.Join(dir, "media-manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLKG_NoSuccessfulRunFailsNamingArtifact(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	gh := stubGH("[]", func([]string) error { t.Fatal("download must not run"); return nil })
	code := runLKG([]string{"-out", t.TempDir()}, &out, &errb, gh, func() time.Time { return lkgNow })
	if code != 1 || !strings.Contains(errb.String(), "promo-media-lkg") {
		t.Fatalf("code=%d stderr=%q, want exit 1 naming promo-media-lkg", code, errb.String())
	}
}

func TestLKG_ExpiredArtifactFailsNamingArtifact(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	gh := stubGH(`[{"databaseId":42}]`, func([]string) error { return errors.New("no valid artifacts found") })
	code := runLKG([]string{"-out", t.TempDir()}, &out, &errb, gh, func() time.Time { return lkgNow })
	if code != 1 || !strings.Contains(errb.String(), "promo-media-lkg") || !strings.Contains(errb.String(), "expired") {
		t.Fatalf("code=%d stderr=%q, want exit 1 naming the artifact and expiry", code, errb.String())
	}
}

func TestLKG_EmptyArtifactFails(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	gh := stubGH(`[{"databaseId":42}]`, func([]string) error { return nil })
	code := runLKG([]string{"-out", t.TempDir()}, &out, &errb, gh, func() time.Time { return lkgNow })
	if code != 1 || !strings.Contains(errb.String(), "empty") {
		t.Fatalf("code=%d stderr=%q, want exit 1 for an empty artifact", code, errb.String())
	}
}

func TestLKG_AgeBound(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		finished string
		want     int
	}{
		{"27 days old passes", lkgNow.AddDate(0, 0, -27).Format(time.RFC3339), 0},
		{"28 days old fails", lkgNow.AddDate(0, 0, -28).Format(time.RFC3339), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			gh := stubGH(`[{"databaseId":7}]`, func(args []string) error {
				writeLKGManifest(t, dir, tc.finished)
				return nil
			})
			var out, errb bytes.Buffer
			code := runLKG([]string{"-out", dir, "-max-age-days", "28"}, &out, &errb, gh, func() time.Time { return lkgNow })
			if code != tc.want {
				t.Fatalf("code=%d stderr=%q, want %d", code, errb.String(), tc.want)
			}
			if tc.want == 0 && !strings.Contains(out.String(), "run=7 commit=abc123") {
				t.Fatalf("stdout %q lacks run and commit", out.String())
			}
		})
	}
}

func TestMerge_WritesMergedManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")
	mk := func(path, name string) {
		body := `{"version":1,"commit":"c1","startedAt":"2026-11-10T00:00:00Z","finishedAt":"2026-11-10T00:10:00Z","entries":[{"name":"` + name + `","format":"webm","status":"recorded","bytes":1,"commit":"c1","themeId":"grimdark"}]}`
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk(a, "one")
	mk(b, "two")
	outPath := filepath.Join(dir, "merged.json")
	var out, errb bytes.Buffer
	if code := runMerge([]string{"-manifest", a, "-manifest", b, "-out", outPath}, &out, &errb); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil || !strings.Contains(string(data), `"one"`) || !strings.Contains(string(data), `"two"`) {
		t.Fatalf("merged file %q err=%v", data, err)
	}
}
