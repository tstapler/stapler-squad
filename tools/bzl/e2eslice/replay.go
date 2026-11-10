package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Commit is one first-parent commit and the files it changed.
type Commit struct {
	SHA   string
	Files []string
}

// parseLog parses `git log --first-parent --name-only --format=@@%H`.
func parseLog(out string) []Commit {
	var cs []Commit
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "@@"):
			cs = append(cs, Commit{SHA: strings.TrimPrefix(l, "@@")})
		case l != "" && len(cs) > 0:
			cs[len(cs)-1].Files = append(cs[len(cs)-1].Files, l)
		}
	}
	return cs
}

func gitLog(repoRoot, rev string, n int) ([]Commit, error) {
	cmd := exec.Command("git", "log", rev, "--first-parent", fmt.Sprintf("-n%d", n), "--name-only", "--format=@@%H")
	cmd.Dir = repoRoot
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return parseLog(out.String()), nil
}

// ReplayStats is the Story 0.15 acceptance distribution.
type ReplayStats struct {
	Commits         int            `json:"commits"`
	Relevant        int            `json:"relevant"`
	ShareMedian     float64        `json:"share_median"`
	ShareP90        float64        `json:"share_p90"`
	AllFallbackRate float64        `json:"all_fallback_rate"`
	ByTopDir        map[string]int `json:"relevant_by_top_dir"`
}

// relevant means the commit touches non-test Go or web source (docs-only and test-only commits are zero prod change).
func relevant(files []string) bool {
	for _, f := range files {
		if strings.HasSuffix(f, ".go") && isProdGoSurface(f) {
			return true
		}
		if strings.HasPrefix(f, "web-app/src/") && !strings.Contains(f, ".test.") && !strings.Contains(f, "__tests__") {
			return true
		}
	}
	return false
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(p*float64(len(sorted)) + 0.999999)
	if i < 1 {
		i = 1
	}
	if i > len(sorted) {
		i = len(sorted)
	}
	return sorted[i-1]
}

func median(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// replay runs sel over each relevant commit and summarises the selected-spec share.
func replay(commits []Commit, sel func(files []string) Selection) ReplayStats {
	return replayCommits(commits, func(c Commit) Selection { return sel(c.Files) })
}

// replayCommits is replay for selectors that need the commit itself (line-level selection reads its diff).
func replayCommits(commits []Commit, sel func(c Commit) Selection) ReplayStats {
	st := ReplayStats{Commits: len(commits), ByTopDir: map[string]int{}}
	var shares []float64
	all := 0
	for _, c := range commits {
		if !relevant(c.Files) {
			continue
		}
		st.Relevant++
		dirs := map[string]bool{}
		for _, f := range c.Files {
			if relevant([]string{f}) {
				dirs[strings.SplitN(f, "/", 2)[0]] = true
			}
		}
		for d := range dirs {
			st.ByTopDir[d]++
		}
		s := sel(c)
		if s.All {
			all++
		}
		shares = append(shares, s.Share())
	}
	sort.Float64s(shares)
	st.ShareMedian, st.ShareP90 = median(shares), percentile(shares, 0.90)
	if st.Relevant > 0 {
		st.AllFallbackRate = float64(all) / float64(st.Relevant)
	}
	return st
}
