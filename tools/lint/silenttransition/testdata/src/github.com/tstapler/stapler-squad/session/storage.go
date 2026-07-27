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

type Storage struct{}

func (s *Storage) TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error) {
	return nil, nil
}

func (s *Storage) UpdateItemSessionEnded(ctx context.Context, id string, endedAtUnixNano int64) error {
	return nil
}
