package services

import (
	"context"
	"fmt"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GitHubService handles all GitHub PR RPC methods.
//
// These methods shell out to the `gh` CLI and have no dependency on review
// queue, terminal streaming, or search. They only need to look up a session
// by ID and call PR operations on it.
type GitHubService struct {
	storage *session.Storage
}

// NewGitHubService creates a GitHubService backed by the given storage.
func NewGitHubService(storage *session.Storage) *GitHubService {
	return &GitHubService{storage: storage}
}

// findInstance loads all instances from storage and returns the one matching id.
// Returns CodeNotFound if not found.
func (gs *GitHubService) findInstance(id string) (*session.Instance, error) {
	instances, err := gs.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}
	for _, inst := range instances {
		if inst.Title == id {
			return inst, nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", id))
}

// GetPRInfo retrieves the latest PR information for a session.
func (gs *GitHubService) GetPRInfo(
	ctx context.Context,
	req *connect.Request[sessionv1.GetPRInfoRequest],
) (*connect.Response[sessionv1.GetPRInfoResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instance, err := gs.findInstance(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	if !instance.IsPRSession() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session '%s' is not a PR session", req.Msg.Id))
	}

	prInfo, err := instance.RefreshPRInfo()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to refresh PR info: %w", err))
	}

	return connect.NewResponse(&sessionv1.GetPRInfoResponse{
		PrInfo: &sessionv1.PRInfo{
			Number:       int32(prInfo.Number),
			Title:        prInfo.Title,
			Body:         prInfo.Body,
			HeadRef:      prInfo.HeadRef,
			BaseRef:      prInfo.BaseRef,
			State:        prInfo.State,
			Author:       prInfo.Author,
			Labels:       prInfo.Labels,
			HtmlUrl:      prInfo.HTMLURL,
			CreatedAt:    timestamppb.New(prInfo.CreatedAt),
			UpdatedAt:    timestamppb.New(prInfo.UpdatedAt),
			IsDraft:      prInfo.IsDraft,
			Mergeable:    prInfo.Mergeable,
			Additions:    int32(prInfo.Additions),
			Deletions:    int32(prInfo.Deletions),
			ChangedFiles: int32(prInfo.ChangedFiles),
		},
	}), nil
}

// GetPRComments retrieves all comments on the PR for a session.
func (gs *GitHubService) GetPRComments(
	ctx context.Context,
	req *connect.Request[sessionv1.GetPRCommentsRequest],
) (*connect.Response[sessionv1.GetPRCommentsResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instance, err := gs.findInstance(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	if !instance.IsPRSession() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session '%s' is not a PR session", req.Msg.Id))
	}

	comments, err := instance.GetPRComments()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get PR comments: %w", err))
	}

	protoComments := make([]*sessionv1.PRComment, 0, len(comments))
	for _, comment := range comments {
		protoComment := &sessionv1.PRComment{
			Id:        int32(comment.ID),
			Author:    comment.Author,
			Body:      comment.Body,
			CreatedAt: timestamppb.New(comment.CreatedAt),
			IsReview:  comment.IsReview,
		}
		if comment.Path != "" {
			protoComment.Path = &comment.Path
		}
		if comment.Line != 0 {
			line := int32(comment.Line)
			protoComment.Line = &line
		}
		protoComments = append(protoComments, protoComment)
	}

	return connect.NewResponse(&sessionv1.GetPRCommentsResponse{
		Comments: protoComments,
	}), nil
}

// PostPRComment posts a new comment to the PR for a session.
func (gs *GitHubService) PostPRComment(
	ctx context.Context,
	req *connect.Request[sessionv1.PostPRCommentRequest],
) (*connect.Response[sessionv1.PostPRCommentResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}
	if req.Msg.Body == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("comment body is required"))
	}

	instance, err := gs.findInstance(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	if !instance.IsPRSession() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session '%s' is not a PR session", req.Msg.Id))
	}

	if err := instance.PostComment(req.Msg.Body); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to post comment: %w", err))
	}

	return connect.NewResponse(&sessionv1.PostPRCommentResponse{
		Success: true,
		Message: fmt.Sprintf("Comment posted successfully to PR for session '%s'", req.Msg.Id),
	}), nil
}

// MergePR merges the PR for a session using the specified merge method.
func (gs *GitHubService) MergePR(
	ctx context.Context,
	req *connect.Request[sessionv1.MergePRRequest],
) (*connect.Response[sessionv1.MergePRResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instance, err := gs.findInstance(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	if !instance.IsPRSession() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session '%s' is not a PR session", req.Msg.Id))
	}

	method := "merge"
	if req.Msg.Method != nil && *req.Msg.Method != "" {
		method = *req.Msg.Method
	}

	if err := instance.MergePR(method); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to merge PR: %w", err))
	}

	return connect.NewResponse(&sessionv1.MergePRResponse{
		Success: true,
		Message: fmt.Sprintf("PR merged successfully for session '%s' using method '%s'", req.Msg.Id, method),
	}), nil
}

// ClosePR closes the PR without merging for a session.
func (gs *GitHubService) ClosePR(
	ctx context.Context,
	req *connect.Request[sessionv1.ClosePRRequest],
) (*connect.Response[sessionv1.ClosePRResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instance, err := gs.findInstance(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	if !instance.IsPRSession() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session '%s' is not a PR session", req.Msg.Id))
	}

	if err := instance.ClosePR(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to close PR: %w", err))
	}

	return connect.NewResponse(&sessionv1.ClosePRResponse{
		Success: true,
		Message: fmt.Sprintf("PR closed successfully for session '%s'", req.Msg.Id),
	}), nil
}
