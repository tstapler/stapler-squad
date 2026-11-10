package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/tools/promo/internal/manifest"
)

// ghRunner runs `gh <args>` and returns stdout; injected so tests stub it.
type ghRunner func(args ...string) ([]byte, error)

func execGH(args ...string) ([]byte, error) {
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return out, fmt.Errorf("gh %v: %w: %s", args, err, ee.Stderr)
		}
		return out, fmt.Errorf("gh %v: %w", args, err)
	}
	return out, nil
}

// lkgFlags configures the last-known-good artifact lookup.
type lkgFlags struct {
	out        string
	workflow   string
	branch     string
	name       string
	maxAgeDays int
}

type lkgRun struct {
	DatabaseID int64 `json:"databaseId"`
}

// runLKG downloads the last-known-good media artifact from the newest successful
// run of the deploy workflow on the default branch. A missing, expired or empty
// artifact is always an error naming the artifact: callers must not skip silently.
func runLKG(args []string, stdout, stderr io.Writer, gh ghRunner, now func() time.Time) int {
	var f lkgFlags
	fs := flag.NewFlagSet("lkg", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&f.out, "out", "", "directory to download the artifact into")
	fs.StringVar(&f.workflow, "workflow", "deploy-website.yml", "workflow whose newest successful run holds the artifact")
	fs.StringVar(&f.branch, "branch", "main", "branch the run must be on")
	fs.StringVar(&f.name, "name", "promo-media-lkg", "artifact name")
	fs.IntVar(&f.maxAgeDays, "max-age-days", 0, "fail when the recorded media is this many days old or older (0 disables)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if f.out == "" {
		fmt.Fprintln(stderr, "mediatool lkg: -out is required")
		return 2
	}
	if err := fetchLKG(f, stdout, gh, now); err != nil {
		fmt.Fprintln(stderr, "mediatool lkg:", err)
		return 1
	}
	return 0
}

func fetchLKG(f lkgFlags, stdout io.Writer, gh ghRunner, now func() time.Time) error {
	listed, err := gh("run", "list", "--workflow", f.workflow, "--branch", f.branch,
		"--status", "success", "--limit", "1", "--json", "databaseId")
	if err != nil {
		return fmt.Errorf("list runs of %s: %w", f.workflow, err)
	}
	var runs []lkgRun
	if err := json.Unmarshal(listed, &runs); err != nil {
		return fmt.Errorf("parse run list: %w", err)
	}
	if len(runs) == 0 {
		return fmt.Errorf("no successful %s run on %s: artifact %s is unavailable", f.workflow, f.branch, f.name)
	}
	id := fmt.Sprint(runs[0].DatabaseID)
	if _, err := gh("run", "download", id, "-n", f.name, "-D", f.out); err != nil {
		return fmt.Errorf("artifact %s of run %s is missing or expired: %w", f.name, id, err)
	}
	m, err := manifest.ReadManifest(filepath.Join(f.out, "media-manifest.json"))
	if err != nil {
		return fmt.Errorf("artifact %s of run %s is empty or has no readable media-manifest.json: %w", f.name, id, err)
	}
	if len(m.Entries) == 0 {
		return fmt.Errorf("artifact %s of run %s has a manifest with no entries", f.name, id)
	}
	if err := checkAge(f, m, now()); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "run=%s commit=%s finished=%s\n", id, m.Commit, m.FinishedAt.Format(time.RFC3339))
	return nil
}

func checkAge(f lkgFlags, m manifest.MediaManifest, now time.Time) error {
	if f.maxAgeDays <= 0 {
		return nil
	}
	recorded := m.FinishedAt
	if recorded.IsZero() {
		recorded = m.StartedAt
	}
	if recorded.IsZero() {
		return fmt.Errorf("artifact %s manifest has no timestamp, cannot apply the %d-day age bound", f.name, f.maxAgeDays)
	}
	age := now.Sub(recorded)
	if age >= time.Duration(f.maxAgeDays)*24*time.Hour {
		return fmt.Errorf("last-known-good media is %d days old (limit %d): recorded %s", int(age.Hours()/24), f.maxAgeDays, recorded.Format(time.RFC3339))
	}
	return nil
}

// runMerge merges shard manifests into one file (the site reads media-manifest.json).
func runMerge(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var manifests stringList
	fs.Var(&manifests, "manifest", "shard manifest (repeatable)")
	out := fs.String("out", "", "merged manifest path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		fmt.Fprintln(stderr, "mediatool merge: -out is required")
		return 2
	}
	m, err := loadMerged(manifests)
	if err != nil {
		fmt.Fprintln(stderr, "mediatool merge:", err)
		return 2
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err == nil {
		err = os.WriteFile(*out, append(data, '\n'), 0o644)
	}
	if err != nil {
		fmt.Fprintln(stderr, "mediatool merge:", err)
		return 2
	}
	fmt.Fprintf(stdout, "merged %d manifests, %d entries -> %s\n", len(manifests), len(m.Entries), *out)
	return 0
}
