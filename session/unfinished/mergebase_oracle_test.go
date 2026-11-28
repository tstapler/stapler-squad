package unfinished

// Differential oracle for merge-base parity (Story 0.1.3a). Reuses the Story
// 0.0.1 corpus (namedDivergenceCases/seededDivergenceCases), legacy walk and
// bestSet; it adds the comparison against object.Commit.MergeBase and the
// "never return a non-ancestor" invariant.

import (
	"fmt"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const mergeBaseOracleGoldenPath = "testdata/mergebase_oracle_divergences.golden"

// oracleVerdict classifies one walk's merge base against the MergeBase oracle.
func oracleVerdict(mb string, best map[string]bool, common map[int]bool) string {
	n, err := strconv.Atoi(mb)
	switch {
	case mb == "none" || mb == "err":
		return mb
	case err != nil || !common[n]:
		return "NOT-ANCESTOR"
	case best[mb]:
		return "best"
	default:
		return "common"
	}
}

func oracleCorpus() []divergenceCase {
	return append(namedDivergenceCases(), seededDivergenceCases()...)
}

func TestMergeBaseOracle_ShouldRecordGoldenAgainstMergeBase_WhenCorpusRuns(t *testing.T) {
	tally := map[string]int{}
	var lines []string
	for _, c := range oracleCorpus() {
		withMergeBaseLimit(t, c.limit)
		d := testrepo.BuildDAG(t, c.spec, nil)
		h, b := d.Hashes[c.head], d.Hashes[c.base]
		best, oracleErr := d.BestSet(h, b)
		common := c.spec.CommonAncestors(c.head, c.base)
		walks := map[string]walkOutcome{"legacy": legacyOutcome(d, h, b), "lockstep": lockstepOutcome(d, h, b)}
		verdicts := map[string]string{}
		for name, o := range walks {
			v := oracleVerdict(o.MB, best, common)
			verdicts[name] = v
			if oracleErr != nil {
				verdicts[name] = "oracle-err:" + v
			}
			tally[name+" "+verdicts[name]]++
			if v == "NOT-ANCESTOR" {
				t.Errorf("%s %s returned node %s, not a common ancestor of %d and %d", c.name, name, o.MB, c.head, c.base)
			}
		}
		if verdicts["legacy"] != "best" || verdicts["lockstep"] != "best" {
			lines = append(lines, fmt.Sprintf("%s head=%d base=%d best=%s | legacy=%s(%s) | lockstep=%s(%s)",
				c.name, c.head, c.base, sortedKeys(best), walks["legacy"].MB, verdicts["legacy"], walks["lockstep"].MB, verdicts["lockstep"]))
		}
	}
	sort.Strings(lines)
	keys := make([]string, 0, len(tally))
	for k := range tally {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sum := make([]string, 0, len(keys))
	for _, k := range keys {
		sum = append(sum, fmt.Sprintf("%s=%d", k, tally[k]))
	}
	got := fmt.Sprintf("# merge base vs object.Commit.MergeBase best set; %d lines where a walk is not in the best set\n# %s\n%s\n",
		len(lines), strings.Join(sum, " "), strings.Join(lines, "\n"))
	if *updateMergeBaseGolden {
		if err := os.WriteFile(mergeBaseOracleGoldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(mergeBaseOracleGoldenPath)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update-mergebase-golden)", err)
	}
	if string(want) != got {
		t.Errorf("oracle divergence set changed; rerun with -update-mergebase-golden\n--- got ---\n%s", got)
	}
}

func sortedKeys(m map[string]bool) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return "{" + strings.Join(ks, ",") + "}"
}

// Criss-cross: nodes 1 and 2 are both best bases of heads 5 and 6.
func TestMergeBaseOracle_ShouldReturnOneOfBestSet_WhenCrissCross(t *testing.T) {
	withMergeBaseLimit(t, 2000)
	for _, c := range namedDivergenceCases()[:2] {
		d := testrepo.BuildDAG(t, c.spec, nil)
		h, b := d.Hashes[c.head], d.Hashes[c.base]
		best, err := d.BestSet(h, b)
		if err != nil || len(best) != 2 || !best["1"] || !best["2"] {
			t.Fatalf("%s: MergeBase best set = %v err=%v, want {1,2}", c.name, best, err)
		}
		got := lockstepOutcome(d, h, b).MB
		if !best[got] {
			t.Errorf("%s: lockstep = %s, want one of {1,2}", c.name, got)
		}
		t.Logf("%s: lockstep picked %s, legacy picked %s", c.name, got, legacyOutcome(d, h, b).MB)
	}
}
