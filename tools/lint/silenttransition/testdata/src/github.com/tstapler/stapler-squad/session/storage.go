// Package session is a minimal fixture standing in for the real
// github.com/tstapler/stapler-squad/session package, just enough to give the
// silenttransition analyzer's test package ("a") something real to call with
// the exact import path the analyzer keys off of.
package session

import "context"

type BacklogStatus string

type BacklogItemData struct {
	ID     string
	Status string
}

type BacklogItemPrecondition struct {
	ExpectedStatus string
}

// TransitionCaller mirrors the real typed registry name.
type TransitionCaller string

// One caller per class, with the real registry's string values.
const (
	CallerOperatorArchiveRPC TransitionCaller = "operator_archive_rpc"
	CallerMCPResumeWork      TransitionCaller = "mcp_resume_work"
	CallerSessionExited      TransitionCaller = "lifecycle_session_exited"
)

type TransitionRequest struct {
	Caller TransitionCaller
	ItemID string
	To     BacklogStatus
}

type PRAssociation struct {
	URL    string
	Number int
}

type Storage struct{}

// Pre-1.5.4 name, kept by the analyzer until Task 1.5.9a.
func (s *Storage) TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error) {
	return nil, nil
}

func (s *Storage) RawTransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error) {
	return nil, nil
}

func (s *Storage) RawArchiveBacklogItem(ctx context.Context, id string, precondition *BacklogItemPrecondition, triggeredBy, note string) (*BacklogItemData, error) {
	return nil, nil
}

func (s *Storage) GatedTransition(ctx context.Context, req TransitionRequest) (*BacklogItemData, error) {
	return nil, nil
}

func (s *Storage) GatedSetPRAndTransition(ctx context.Context, req TransitionRequest, pr PRAssociation) error {
	return nil
}

func (s *Storage) UpdateItemSessionEnded(ctx context.Context, id string, endedAtUnixNano int64) error {
	return nil
}
