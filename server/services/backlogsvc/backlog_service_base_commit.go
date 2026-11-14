package backlogsvc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session/backlog/pipeline"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// baseCommitStamp describes one ItemSession whose pre-work base commit is being recorded.
type baseCommitStamp struct {
	Caller  string // log prefix, e.g. "SpawnSessionFromItem"
	Item    *backlogdto.BacklogItemData
	Session backlogdto.ItemSessionSummary
	Dir     string // directory whose HEAD is the base
	// RaisesNotification is set for work sessions, whose base feeds the PR-ownership
	// guard; a failure there is escalated to the operator, not only logged.
	RaisesNotification bool
}

// stampBaseCommit records Dir's HEAD as the session's pre-work base commit and returns
// it, or "" when it could not be captured. Every ItemSession-creating spawn path stamps
// through here so a failure is never dropped: the PR-ownership guard
// (verifyPRBelongsToItem) and the review gate's base..HEAD diff both fail closed
// without a base. A failure is logged with item/session context and, for work
// sessions, raised as an operator notification (the existing surface for spawn defects).
func (s *BacklogService) stampBaseCommit(ctx context.Context, st baseCommitStamp) string {
	baseSHA, err := pipeline.GetGitHeadSHA(st.Dir)
	if err == nil && baseSHA == "" {
		err = errors.New("HEAD resolved to an empty SHA")
	}
	if err != nil {
		s.reportBaseCommitUnstamped(st, "read HEAD", err)
		return ""
	}
	if err := s.storage.SetItemSessionBaseCommit(ctx, st.Session.ID, baseSHA); err != nil {
		s.reportBaseCommitUnstamped(st, "persist base commit", err)
		return ""
	}
	return baseSHA
}

// hasDotGit reports whether dir itself holds a `.git` directory or gitdir file.
func hasDotGit(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func (s *BacklogService) reportBaseCommitUnstamped(st baseCommitStamp, step string, err error) {
	log.Warn("["+st.Caller+"] base commit not recorded on item session",
		"item", st.Item.ID, "session", st.Session.SessionUUID, "dir", st.Dir, "step", step,
		"work_session", st.RaisesNotification, "error", err)
	if !st.RaisesNotification || s.eventBus == nil {
		return
	}
	s.eventBus.Publish(events.NewNotificationEvent(
		st.Item.ID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		services.DerivePriority(false, true),
		"Work session has no base commit",
		fmt.Sprintf("%s: could not %s in %s: %v. PR-ownership checks and review diffs for this session have no base to compare against and will fail closed.",
			st.Item.Title, step, st.Dir, err),
		map[string]string{"item_id": st.Item.ID, "session_uuid": st.Session.SessionUUID},
	))
}
