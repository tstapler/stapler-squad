// Package ghci reads CI timings from the GitHub Actions API (read-only) for the P10 and T2 collectors.
// All access goes through the GH interface so tests run on a fixture-backed fake.
package ghci

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"time"
)

// GH is read-only access to `gh api`.
type GH interface {
	API(endpoint string) ([]byte, error)
}

// ExecGH shells out to `gh api` with GET only.
type ExecGH struct{}

// API implements GH.
func (ExecGH) API(endpoint string) ([]byte, error) {
	out, err := exec.Command("gh", "api", "--method", "GET", endpoint).Output() //nolint:norawexec // read-only gh call, waited on
	if err != nil {
		return nil, fmt.Errorf("gh api %s: %w", endpoint, err)
	}
	return out, nil
}

// FakeGH serves canned responses keyed by the exact endpoint string.
type FakeGH struct {
	Responses map[string]json.RawMessage `json:"responses"`
}

// LoadFake reads a fixture {"responses": {"<endpoint>": <json>}}.
func LoadFake(file string) (*FakeGH, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var f FakeGH
	return &f, json.Unmarshal(b, &f)
}

// API implements GH.
func (f *FakeGH) API(endpoint string) ([]byte, error) {
	if r, ok := f.Responses[endpoint]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("fixture has no response for %q", endpoint)
}

// Run is the slice of a workflow run the collectors need.
type Run struct {
	ID           int64  `json:"id"`
	WorkflowID   int64  `json:"workflow_id"`
	RunNumber    int    `json:"run_number"`
	RunAttempt   int    `json:"run_attempt"`
	Path         string `json:"path"`
	HeadSHA      string `json:"head_sha"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	RunStartedAt string `json:"run_started_at"`
	UpdatedAt    string `json:"updated_at"`
	PRs          []struct {
		Number int `json:"number"`
	} `json:"pull_requests"`
}

// Workflow is the file name of the run's workflow.
func (r Run) Workflow() string { return path.Base(r.Path) }

// FetchRuns pages through `actions/runs?event=<event>` (100 per page) until pages are exhausted or max reached.
func FetchRuns(gh GH, repo, event string, maxPages int) ([]Run, error) {
	var all []Run
	for p := 1; p <= maxPages; p++ {
		b, err := gh.API(fmt.Sprintf("repos/%s/actions/runs?event=%s&per_page=100&page=%d", repo, event, p))
		if err != nil {
			return nil, err
		}
		var doc struct {
			Runs []Run `json:"workflow_runs"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return nil, err
		}
		all = append(all, doc.Runs...)
		if len(doc.Runs) < 100 {
			break
		}
	}
	return all, nil
}

// SHA is one head SHA whose required workflows all finished green.
type SHA struct {
	HeadSHA string
	PRs     []int
	TTGS    float64
	PerWF   map[string]float64
	RunIDs  []int64
	End     time.Time
}

func parse(ts string) (time.Time, error) { return time.Parse(time.RFC3339, ts) }

// TimeToGreen groups runs of the given workflow files by head SHA (definition of ci-time-to-green.sh): the latest
// run of each workflow must be completed with success or skipped, and the SHA's time-to-green is the last
// updated_at minus the first run_started_at. The newest n SHAs by end time are returned (n <= 0 means all).
func TimeToGreen(runs []Run, workflows []string, n int) []SHA {
	want := map[string]bool{}
	for _, w := range workflows {
		want[w] = true
	}
	bySHA := map[string][]Run{}
	for _, r := range runs {
		if want[r.Workflow()] {
			bySHA[r.HeadSHA] = append(bySHA[r.HeadSHA], r)
		}
	}
	var out []SHA
	for sha, rs := range bySHA {
		latest := map[int64]Run{}
		for _, r := range rs {
			if cur, ok := latest[r.WorkflowID]; !ok || r.RunNumber > cur.RunNumber {
				latest[r.WorkflowID] = r
			}
		}
		s := SHA{HeadSHA: sha, PerWF: map[string]float64{}}
		var start, end time.Time
		ok := true
		for _, r := range latest {
			s0, e1 := parse(r.RunStartedAt)
			e0, e2 := parse(r.UpdatedAt)
			if r.Status != "completed" || (r.Conclusion != "success" && r.Conclusion != "skipped") || e1 != nil || e2 != nil {
				ok = false
				break
			}
			if start.IsZero() || s0.Before(start) {
				start = s0
			}
			if e0.After(end) {
				end = e0
			}
			s.PerWF[r.Workflow()] = e0.Sub(s0).Seconds()
			s.RunIDs = append(s.RunIDs, r.ID)
			for _, pr := range r.PRs {
				s.PRs = appendUnique(s.PRs, pr.Number)
			}
		}
		if !ok {
			continue
		}
		s.TTGS, s.End = end.Sub(start).Seconds(), end
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].End.After(out[j].End) })
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func appendUnique(xs []int, v int) []int {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}

// RunnerMinutes sums the durations of every completed job of the given runs, in minutes.
func RunnerMinutes(gh GH, repo string, runIDs []int64) (float64, error) {
	total := 0.0
	for _, id := range runIDs {
		b, err := gh.API(fmt.Sprintf("repos/%s/actions/runs/%d/jobs?per_page=100", repo, id))
		if err != nil {
			return 0, err
		}
		var doc struct {
			Jobs []struct {
				StartedAt   string `json:"started_at"`
				CompletedAt string `json:"completed_at"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return 0, err
		}
		for _, j := range doc.Jobs {
			s, e1 := parse(j.StartedAt)
			e, e2 := parse(j.CompletedAt)
			if e1 == nil && e2 == nil {
				total += e.Sub(s).Minutes()
			}
		}
	}
	return total, nil
}
