package artifacts

import (
	"regexp"
)

var (
	// GitHub PR URL: https://github.com/<owner>/<repo>/pull/<number>
	rePRURL = regexp.MustCompile(`https://github\.com/[\w.-]+/[\w.-]+/pull/\d+`)
	// 40-hex commit SHA — only when following a git output keyword to avoid false positives
	// (npm hashes, Docker digests, TLS fingerprints all match bare \b[0-9a-f]{40}\b)
	reCommitSHA = regexp.MustCompile(`(?i)(?:^|\b)commit\s+([0-9a-f]{40})\b`)
	// General https:// URL
	reExternalURL = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// ExtractFromToolResult extracts artifacts from the text content of a single
// tool_result block. Callers must pass only tool_result content — never raw
// assistant text — to avoid false positives from doc links in explanations.
func ExtractFromToolResult(text string) (prURLs, commitSHAs, externalURLs []string) {
	prSet := make(map[string]struct{})
	for _, m := range rePRURL.FindAllString(text, -1) {
		if _, seen := prSet[m]; !seen {
			prSet[m] = struct{}{}
			prURLs = append(prURLs, m)
		}
	}

	shaSet := make(map[string]struct{})
	for _, sub := range reCommitSHA.FindAllStringSubmatch(text, -1) {
		sha := sub[1]
		if _, seen := shaSet[sha]; !seen {
			shaSet[sha] = struct{}{}
			commitSHAs = append(commitSHAs, sha)
		}
	}

	urlSet := make(map[string]struct{})
	for _, m := range reExternalURL.FindAllString(text, -1) {
		if _, isPR := prSet[m]; !isPR {
			if _, seen := urlSet[m]; !seen {
				urlSet[m] = struct{}{}
				externalURLs = append(externalURLs, m)
			}
		}
	}
	return
}

var (
	reGHPRCreate   = regexp.MustCompile(`gh pr create.*?--title\s+"([^"]+)"`)
	reGHPRMerge    = regexp.MustCompile(`gh pr merge\s+(\d+)`)
	reGitCommitMsg = regexp.MustCompile(`git commit.*?-m\s+"([^"]+)"`)
	reGoGet        = regexp.MustCompile(`go get\s+([\w./-]+@[\w./-]+)`)
	reNPMInstall   = regexp.MustCompile(`npm install\s+([\w@./-]+)`)
)

// ExtractFromBashCommand extracts a structured CommandArtifact from a Bash tool_use
// input command. Returns nil if no known pattern matches.
// Only call this on tool_use inputs (not tool_result outputs) to avoid double-counting.
func ExtractFromBashCommand(command string) *CommandArtifact {
	if m := reGHPRCreate.FindStringSubmatch(command); m != nil {
		return &CommandArtifact{Type: "gh_pr_create", Command: truncate(command, 200), Detail: m[1]}
	}
	if m := reGHPRMerge.FindStringSubmatch(command); m != nil {
		return &CommandArtifact{Type: "gh_pr_merge", Command: truncate(command, 200), Detail: m[1]}
	}
	if m := reGitCommitMsg.FindStringSubmatch(command); m != nil {
		return &CommandArtifact{Type: "git_commit", Command: truncate(command, 200), Detail: m[1]}
	}
	if m := reGoGet.FindStringSubmatch(command); m != nil {
		return &CommandArtifact{Type: "package_install", Command: truncate(command, 200), Detail: m[1]}
	}
	if m := reNPMInstall.FindStringSubmatch(command); m != nil {
		return &CommandArtifact{Type: "package_install", Command: truncate(command, 200), Detail: m[1]}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
