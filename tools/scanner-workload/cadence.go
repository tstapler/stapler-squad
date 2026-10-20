package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/tstapler/stapler-squad/session/git/native"
)

// RepoCadence is the read-only measurement of one owner repo.
type RepoCadence struct {
	Name          string  `json:"name"`
	Path          string  `json:"path"`
	TotalCommits  int     `json:"total_commits"`
	WindowDays    int     `json:"window_days"`
	WindowCommits int     `json:"window_commits"`
	CommitsPerDay float64 `json:"commits_per_day"`
	TrackedFiles  int     `json:"tracked_files"`
	SizeBytes     int64   `json:"size_bytes"` // .git plus working tree, minus the skipped paths
}

// CadenceReport is the JSON the cadence subcommand prints and drive consumes.
type CadenceReport struct {
	Repos []RepoCadence `json:"repos"`
}

// PerDay maps repo name to commits per day.
func (r CadenceReport) PerDay() map[string]float64 {
	m := make(map[string]float64, len(r.Repos))
	for _, c := range r.Repos {
		m[c.Name] = c.CommitsPerDay
	}
	return m
}

// MeasureCadence walks HEAD's history with go-git (read-only) and counts all
// commits and those committed within the last windowDays before now. now is a
// parameter so tests need no wall clock.
func MeasureCadence(src string, windowDays int, now time.Time, skip []string) (RepoCadence, error) {
	c := RepoCadence{Name: filepath.Base(src), Path: src, WindowDays: windowDays}
	repo, err := native.OpenRepo(src)
	if err != nil {
		return c, err
	}
	head, err := repo.Head()
	if err != nil {
		return c, err
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return c, err
	}
	cutoff := now.AddDate(0, 0, -windowDays)
	err = iter.ForEach(func(commit *object.Commit) error {
		c.TotalCommits++
		if commit.Committer.When.After(cutoff) {
			c.WindowCommits++
		}
		return nil
	})
	if err != nil {
		return c, err
	}
	if windowDays > 0 {
		c.CommitsPerDay = float64(c.WindowCommits) / float64(windowDays)
	}
	if idx, err := repo.Storer.Index(); err == nil {
		c.TrackedFiles = len(idx.Entries)
	}
	if size, err := treeSize(src, append([]string{alwaysSkip}, skip...)); err == nil {
		c.SizeBytes = size
	}
	return c, nil
}

func readCadence(path string) (CadenceReport, error) {
	var r CadenceReport
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("parse %s: %w", path, err)
	}
	return r, nil
}

func writeJSON(v any, path string, stdout io.Writer) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if path == "" {
		_, err = stdout.Write(b)
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
