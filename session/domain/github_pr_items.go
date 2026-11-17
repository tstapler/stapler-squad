package domain

// GitHubCheckItem is one itemized CI check from a PR's statusCheckRollup — the
// data a single-string conclusion collapses; exported so callers that want the
// itemized view (e.g. the VCS tab's "why blocked" rollup) don't need to.
type GitHubCheckItem struct {
	Name       string
	Context    string
	State      string
	Status     string
	Conclusion string
}

// GitHubReviewItem is one PR review — the data approved/changes-requested
// counts collapse, exported so callers can also read the reviewer's Body text
// (e.g. a CHANGES_REQUESTED review's stated reason).
type GitHubReviewItem struct {
	Author string
	State  string
	Body   string
}
