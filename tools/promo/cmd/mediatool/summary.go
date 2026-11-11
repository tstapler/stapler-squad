package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tstapler/stapler-squad/tools/promo/internal/manifest"
)

func loadMerged(paths []string) (manifest.MediaManifest, error) {
	if len(paths) == 0 {
		return manifest.MediaManifest{}, fmt.Errorf("at least one -manifest is required")
	}
	parts := make([]manifest.MediaManifest, 0, len(paths))
	for _, p := range paths {
		m, err := manifest.ReadManifest(p)
		if err != nil {
			return manifest.MediaManifest{}, err
		}
		parts = append(parts, m)
	}
	return manifest.Merge(parts...)
}

func appendSummary(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("write job summary: %w", err)
	}
	defer f.Close()
	_, err = io.WriteString(f, text)
	return err
}

func runSummary(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("summary", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var manifests stringList
	fs.Var(&manifests, "manifest", "manifest file (repeatable; shards are merged)")
	dist := fs.String("dist", "", "built site dir, for the size line")
	prev := fs.Int64("prev-dist-bytes", -1, "previous run's dist size, for the trend")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	m, err := loadMerged(manifests)
	if err != nil {
		fmt.Fprintln(stderr, "mediatool summary:", err)
		return 2
	}
	if err := emitSummary(m, *dist, *prev, stdout, getenv("GITHUB_STEP_SUMMARY")); err != nil {
		fmt.Fprintln(stderr, "mediatool summary:", err)
		return 2
	}
	return 0
}

// emitSummary appends the table to summaryPath, or prints it when no path is set.
func emitSummary(m manifest.MediaManifest, distDir string, prevDist int64, stdout io.Writer, summaryPath string) error {
	var distBytes int64
	if distDir != "" {
		var err error
		if distBytes, err = manifest.DistSize(distDir); err != nil {
			return err
		}
	}
	text := manifest.RenderSummary(m, distBytes, prevDist)
	if summaryPath == "" {
		_, err := io.WriteString(stdout, text)
		return err
	}
	return appendSummary(summaryPath, text)
}
