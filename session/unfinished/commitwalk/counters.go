package commitwalk

import "sync/atomic"

// RedundancyOutcome labels the merge-base redundancy checks (isAncestorOfOther
// and repostate's reaches) that answered "not redundant" without finding the
// candidate: where a skewed committer clock can leave a redundant base in place.
type RedundancyOutcome int

// The redundancy outcomes; NumRedundancyOutcomes sizes per-outcome arrays.
const (
	RedundancyBound RedundancyOutcome = iota
	RedundancyPrunedMiss
	NumRedundancyOutcomes
)

// RedundancyOutcomeNames are the metric label values, indexed by outcome.
var RedundancyOutcomeNames = [NumRedundancyOutcomes]string{"bound", "pruned_miss"}

// mergeBaseRedundancyTotal counts the plain walk's outcomes; the id walk's live
// in repostate.GraphStats and are summed in by the repostate inputs.
var mergeBaseRedundancyTotal [NumRedundancyOutcomes]atomic.Int64

func recordRedundancyOutcome(o RedundancyOutcome) { mergeBaseRedundancyTotal[o].Add(1) }

// RedundancyTotal returns the plain walk's count for outcome o.
func RedundancyTotal(o RedundancyOutcome) int64 { return mergeBaseRedundancyTotal[o].Load() }

// decodedCommitsTotal counts full commit decodes in production paths.
var decodedCommitsTotal atomic.Int64

// RecordDecodedCommits adds n commit decodes to unfinished.repostate.decoded_commits_total.
func RecordDecodedCommits(n int64) { decodedCommitsTotal.Add(n) }

// DecodedCommitsTotal returns the commit-decode count since process start.
func DecodedCommitsTotal() int64 { return decodedCommitsTotal.Load() }
