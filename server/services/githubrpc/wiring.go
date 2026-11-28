package githubrpc

import (
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/prs"
)

// NudgeWiringForTest reports the nudge collaborators the composition root installed.
func (s *GitHubUserService) NudgeWiringForTest() (PRNudger, access.NudgeRecorder, githubpkg.PRDetailFetcher, prs.PRTokenResolver) {
	return s.nudge.nudger, s.nudge.nudges, s.nudge.fetcher, s.nudge.tokens
}
