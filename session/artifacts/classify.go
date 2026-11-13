package artifacts

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/tstapler/stapler-squad/session/domain"
)

// URLClass is the coarse purpose of an external URL found in a transcript.
type URLClass string

const (
	URLClassDocs   URLClass = "docs"
	URLClassCI     URLClass = "ci"
	URLClassIssue  URLClass = "issue"
	URLClassDeploy URLClass = "deploy"
	URLClassOther  URLClass = "other"
)

var (
	reCIRunPath   = regexp.MustCompile(`^/[\w.-]+/[\w.-]+/actions/runs/\d+`)
	reIssuePath   = regexp.MustCompile(`^/[\w.-]+/[\w.-]+/issues/(\d+)$`)
	deploySuffix  = []string{".vercel.app", ".netlify.app", ".pages.dev", ".herokuapp.com", ".run.app", ".fly.dev", ".onrender.com"}
	docsHostParts = []string{"docs.", "pkg.go.dev", "developer.mozilla.org", "readthedocs", "godoc.org", "man7.org"}
)

// ClassifyURL sorts an external URL into docs, CI, issue, deploy or other.
func ClassifyURL(raw string) URLClass {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return URLClassOther
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case reCIRunPath.MatchString(u.Path):
		return URLClassCI
	case reIssuePath.MatchString(u.Path):
		return URLClassIssue
	}
	for _, s := range deploySuffix {
		if strings.HasSuffix(host, s) {
			return URLClassDeploy
		}
	}
	for _, p := range docsHostParts {
		if strings.Contains(host, p) || strings.HasPrefix(u.Path, "/docs/") {
			return URLClassDocs
		}
	}
	return URLClassOther
}

// parseIssueURL reads host, owner, repo and number from an .../issues/N URL.
func parseIssueURL(raw string) (IssueRecord, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return IssueRecord{}, false
	}
	m := reIssuePath.FindStringSubmatch(u.Path)
	if m == nil {
		return IssueRecord{}, false
	}
	n, _ := strconv.Atoi(m[1])
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return IssueRecord{Host: domain.NormalizeHost(u.Host), Owner: parts[0], Repo: parts[1], Number: n, Relation: "mentioned"}, true
}
