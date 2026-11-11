package main

import (
	"fmt"
	"regexp"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/collect"
	"github.com/tstapler/stapler-squad/tools/bzl/parity/ghci"
)

// prFiles lists the changed paths of one PR through the read-only GH interface.
func prFiles(gh ghci.GH, repo string, pr int) ([]string, error) {
	var files []string
	for p := 1; p <= 30; p++ {
		b, err := gh.API(fmt.Sprintf("repos/%s/pulls/%d/files?per_page=100&page=%d", repo, pr, p))
		if err != nil {
			return nil, err
		}
		page, err := decodeFiles(b)
		if err != nil {
			return nil, err
		}
		files = append(files, page...)
		if len(page) < 100 {
			break
		}
	}
	return files, nil
}

// MeasureT2 records time-to-green p50 (seconds) and runner-minutes p50 for the partial-change class and for all
// PRs of the window, from the Bazel workflows' runs. A class with fewer than minRuns SHAs is UNMEASURED.
func MeasureT2(d *collect.Doc, gh ghci.GH, repo string, workflows []string, n, minRuns int, globalRegex string) error {
	re, err := regexp.Compile(globalRegex)
	if err != nil {
		return fmt.Errorf("global regex: %w", err)
	}
	runs, err := ghci.FetchRuns(gh, repo, "pull_request", 40)
	if err != nil {
		return err
	}
	shas := ghci.TimeToGreen(runs, workflows, n)
	classes := map[string][]ghci.SHA{"all": shas}
	for _, s := range shas {
		if len(s.PRs) == 0 {
			continue
		}
		files, err := prFiles(gh, repo, s.PRs[0])
		if err != nil {
			return err
		}
		global := false
		for _, f := range files {
			global = global || re.MatchString(f)
		}
		if !global {
			classes["partial"] = append(classes["partial"], s)
		}
	}
	for name, ss := range classes {
		src := fmt.Sprintf("gh api, %d PR head SHAs, workflows %v", len(ss), workflows)
		ttgKey, rmKey := "T2-"+name+"-ttg", "T2-"+name+"-runner-min"
		if len(ss) < minRuns {
			why := fmt.Sprintf("%d qualifying PR runs, need %d", len(ss), minRuns)
			d.Unmeasured(ttgKey, why, collect.Meta{Source: src})
			d.Unmeasured(rmKey, why, collect.Meta{Source: src})
			continue
		}
		var ttg, rm []float64
		for _, s := range ss {
			m, err := ghci.RunnerMinutes(gh, repo, s.RunIDs)
			if err != nil {
				return err
			}
			ttg, rm = append(ttg, s.TTGS), append(rm, m)
		}
		d.Set(ttgKey, collect.Median(ttg), collect.Meta{Source: src})
		d.Set(rmKey, collect.Median(rm), collect.Meta{Source: src})
	}
	return nil
}
