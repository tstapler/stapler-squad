package artifacts

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

var (
	// `[feat-b 1a2b3c4] fix: token refresh` as printed by git commit.
	reCommitLine = regexp.MustCompile(`(?m)^\[([^\]\s]+)(?: \(root-commit\))? ([0-9a-f]{7,40})\] (.+)$`)
	// "Fixes #45", "closes owner/repo#12"
	reIssueRef = regexp.MustCompile(`(?i)\b(fix(?:es|ed)?|close[sd]?|resolve[sd]?)\s+(?:([\w.-]+)/([\w.-]+))?#(\d+)`)
	reCIURL    = regexp.MustCompile(`https?://[^\s"'<>)]+/actions/runs/\d+`)
	reIssueURL = regexp.MustCompile(`https?://[^\s"'<>)]+/issues/\d+`)
)

// fileWriteTools are the tool_use names that write a file by `file_path`.
var fileWriteTools = map[string]bool{"Write": true, "Edit": true, "MultiEdit": true, "NotebookEdit": true}

type filePathInput struct {
	FilePath string `json:"file_path"`
}

// addFileWrite records a plan document when a write tool targets one.
func (f *scanFindings) addFileWrite(c artifactContentBlock, ts time.Time) {
	var in filePathInput
	if err := json.Unmarshal(c.Input, &in); err != nil || !isPlanDocPath(in.FilePath) {
		return
	}
	f.records = append(f.records, domain.PlanDocRecord{Path: in.FilePath, FirstSeenAt: ts})
}

func isPlanDocPath(p string) bool {
	if p == "" {
		return false
	}
	slashed := "/" + strings.TrimPrefix(p, "./")
	return strings.Contains(slashed, "/project_plans/") || strings.Contains(slashed, "/docs/") ||
		strings.HasSuffix(strings.ToLower(p), ".md")
}

// addCommandRecords records the branches and issue references a Bash command
// names. Branches come from `git checkout -b`, `git switch -c` and `git push -u`.
func (f *scanFindings) addCommandRecords(command string, ts time.Time) {
	for _, seg := range commandSegments(shellFields(command)) {
		if name, action := gitBranchAction(seg); name != "" {
			f.records = append(f.records, domain.BranchRecord{Name: name, Action: action, FirstSeenAt: ts})
		}
	}
	for _, m := range reIssueRef.FindAllStringSubmatch(command, -1) {
		n, _ := strconv.Atoi(m[4])
		f.records = append(f.records, domain.IssueRecord{Owner: m[2], Repo: m[3], Number: n, Relation: strings.ToLower(m[1]), FirstSeenAt: ts})
	}
}

// commandSegments splits shell words on command separators.
func commandSegments(words []string) [][]string {
	segs := make([][]string, 0, 1)
	cur := make([]string, 0, len(words))
	for _, w := range words {
		switch w {
		case "&&", "||", ";", "|":
			segs, cur = append(segs, cur), nil
		default:
			cur = append(cur, w)
		}
	}
	return append(segs, cur)
}

// gitBranchAction returns the branch a git command creates or pushes, if any.
func gitBranchAction(seg []string) (name, action string) {
	i := indexOf(seg, "git")
	if i < 0 || i+1 >= len(seg) {
		return "", ""
	}
	args := seg[i+1:]
	switch args[0] {
	case "checkout", "switch":
		if v := flagValue(args[1:], "-b", "-B", "-c", "-C"); v != "" {
			return v, "created"
		}
	case "push":
		if hasAny(args[1:], "-u", "--set-upstream") {
			return pushedBranch(args[1:]), "pushed"
		}
	}
	return "", ""
}

// pushedBranch is the last positional argument of `git push`, which names the
// branch (`origin feat` or `origin HEAD:feat`).
func pushedBranch(args []string) string {
	var pos []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
		}
	}
	if len(pos) < 2 {
		return ""
	}
	b := pos[len(pos)-1]
	if _, after, ok := strings.Cut(b, ":"); ok {
		return after
	}
	return b
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

func hasAny(ss []string, wants ...string) bool {
	for _, w := range wants {
		if indexOf(ss, w) >= 0 {
			return true
		}
	}
	return false
}

// addResultRecords records commits, CI runs and issue links printed by a command.
func (f *scanFindings) addResultRecords(text string, ts time.Time) {
	for _, m := range reCommitLine.FindAllStringSubmatch(text, -1) {
		f.records = append(f.records, domain.CommitRecord{SHA: m[2], Subject: m[3], Branch: m[1], FirstSeenAt: ts})
	}
	for _, u := range reCIURL.FindAllString(text, -1) {
		if ClassifyURL(u) == URLClassCI {
			f.records = append(f.records, domain.CILinkRecord{URL: u, FirstSeenAt: ts})
		}
	}
	for _, u := range reIssueURL.FindAllString(text, -1) {
		if rec, ok := parseIssueURL(u); ok {
			rec.FirstSeenAt = ts
			f.records = append(f.records, rec)
		}
	}
}
