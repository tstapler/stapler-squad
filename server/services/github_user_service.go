package services

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	githubpkg "github.com/tstapler/stapler-squad/github"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check: GitHubUserService must implement the generated handler.
var _ sessionv1connect.GitHubUserServiceHandler = (*GitHubUserService)(nil)

// GitHubUserService implements the ConnectRPC GitHubUserServiceHandler.
type GitHubUserService struct {
	cache *githubpkg.UserPRCache
}

// NewGitHubUserService creates a new service backed by the given cache.
func NewGitHubUserService(cache *githubpkg.UserPRCache) *GitHubUserService {
	return &GitHubUserService{cache: cache}
}

// +api: github-user:list-prs
// ListUserPRs returns the current cached snapshot of open PRs.
func (s *GitHubUserService) ListUserPRs(
	ctx context.Context,
	_ *connect.Request[sessionv1.ListUserPRsRequest],
) (*connect.Response[sessionv1.ListUserPRsResponse], error) {
	authState := s.resolveAuthState(ctx)
	prs := s.cache.GetAll()
	return connect.NewResponse(&sessionv1.ListUserPRsResponse{
		Prs:       userPRsToProto(prs),
		AuthState: authState,
	}), nil
}

// +api: github-user:watch-prs
// WatchUserPRs streams PR snapshot updates.
// Pattern: send initial snapshot → register callback → forward events until disconnect.
func (s *GitHubUserService) WatchUserPRs(
	ctx context.Context,
	_ *connect.Request[sessionv1.WatchUserPRsRequest],
	stream *connect.ServerStream[sessionv1.UserPREvent],
) error {
	authState := s.resolveAuthState(ctx)

	// 1. Send initial snapshot.
	initial := s.cache.GetAll()
	if err := stream.Send(&sessionv1.UserPREvent{
		EventType: "snapshot",
		Prs:       userPRsToProto(initial),
		AuthState: authState,
	}); err != nil {
		return err
	}

	// 2. Register for updates via a buffered channel bridge.
	subID := fmt.Sprintf("%p-%d", &stream, time.Now().UnixNano())
	updateCh := make(chan []githubpkg.UserPR, 4)
	s.cache.Subscribe(subID, updateCh)
	defer s.cache.Unsubscribe(subID)

	// 3. Forward updates until client disconnects.
	for {
		select {
		case <-ctx.Done():
			return nil
		case prs, ok := <-updateCh:
			if !ok {
				return nil
			}
			if err := stream.Send(&sessionv1.UserPREvent{
				EventType: "snapshot",
				Prs:       userPRsToProto(prs),
				AuthState: authState,
			}); err != nil {
				return err
			}
		}
	}
}

// +api: github-user:get-auth-state
// GetGitHubAuthState returns the current GitHub authentication status.
func (s *GitHubUserService) GetGitHubAuthState(
	ctx context.Context,
	_ *connect.Request[sessionv1.GetGitHubAuthStateRequest],
) (*connect.Response[sessionv1.GetGitHubAuthStateResponse], error) {
	return connect.NewResponse(&sessionv1.GetGitHubAuthStateResponse{
		AuthState: s.resolveAuthState(ctx),
	}), nil
}

// resolveAuthState returns the current GitHub auth state from the cache.
// No network call is made; it reads the login stored by the background fetch.
func (s *GitHubUserService) resolveAuthState(_ context.Context) *sessionv1.GitHubAuthState {
	login := s.cache.GetCachedLogin()
	if login == "" {
		return &sessionv1.GitHubAuthState{
			Available:    false,
			ErrorMessage: "GitHub authentication not yet resolved",
		}
	}
	return &sessionv1.GitHubAuthState{Available: true, Username: login}
}

// userPRsToProto converts a slice of UserPR to proto messages.
func userPRsToProto(prs []githubpkg.UserPR) []*sessionv1.UserPR {
	if len(prs) == 0 {
		return nil
	}
	out := make([]*sessionv1.UserPR, len(prs))
	for i, pr := range prs {
		p := &sessionv1.UserPR{
			Owner:             pr.Owner,
			Repo:              pr.Repo,
			Number:            int32(pr.Number),
			Title:             pr.Title,
			HtmlUrl:           pr.URL,
			State:             pr.State,
			HeadRef:           pr.HeadRef,
			BaseRef:           pr.BaseRef,
			IsDraft:           pr.IsDraft,
			CheckConclusion:   pr.CheckConclusion,
			ApprovedCount:     int32(pr.ApprovedCount),
			ChangesReqCount:   int32(pr.ChangesReqCount),
			SessionIds:        pr.SessionIDs,
			LocalWorktreePath: pr.LocalWorktreePath,
		}
		if !pr.UpdatedAt.IsZero() {
			p.UpdatedAt = timestamppb.New(pr.UpdatedAt)
		}
		if !pr.ClosedAt.IsZero() {
			p.ClosedAt = timestamppb.New(pr.ClosedAt)
		}
		if !pr.MergedAt.IsZero() {
			p.MergedAt = timestamppb.New(pr.MergedAt)
		}
		out[i] = p
	}
	return out
}
