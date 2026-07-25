package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	githubpkg "github.com/tstapler/stapler-squad/github"
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

// +api: github-user:start-device-auth
// StartGitHubDeviceAuth initiates the GitHub Device Flow OAuth.
func (s *GitHubUserService) StartGitHubDeviceAuth(
	ctx context.Context,
	_ *connect.Request[sessionv1.StartGitHubDeviceAuthRequest],
) (*connect.Response[sessionv1.StartGitHubDeviceAuthResponse], error) {
	da, err := githubpkg.StartDeviceAuth(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("start device auth: %w", err))
	}
	return connect.NewResponse(&sessionv1.StartGitHubDeviceAuthResponse{
		DeviceCode:      da.DeviceCode,
		UserCode:        da.UserCode,
		VerificationUri: da.VerificationURI,
		ExpiresIn:       int32(da.ExpiresIn),
		Interval:        int32(da.Interval),
	}), nil
}

// +api: github-user:poll-device-auth
// PollGitHubDeviceAuth polls GitHub's token endpoint once. The frontend calls
// this on an interval until status is COMPLETE or EXPIRED.
func (s *GitHubUserService) PollGitHubDeviceAuth(
	ctx context.Context,
	req *connect.Request[sessionv1.PollGitHubDeviceAuthRequest],
) (*connect.Response[sessionv1.PollGitHubDeviceAuthResponse], error) {
	if req.Msg.DeviceCode == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("device_code is required"))
	}

	token, err := githubpkg.PollDeviceAuth(ctx, req.Msg.DeviceCode)
	if err == nil {
		// Discover the username and store per-account in the keychain.
		if storeErr := githubpkg.StoreTokenForDiscoveredUser(ctx, token); storeErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("store token in keychain: %w", storeErr))
		}
		s.cache.InvalidateLoginCache()
		_ = s.cache.Refresh(ctx)
		return connect.NewResponse(&sessionv1.PollGitHubDeviceAuthResponse{
			Status:    sessionv1.DeviceAuthStatus_DEVICE_AUTH_STATUS_COMPLETE,
			AuthState: s.resolveAuthState(ctx),
		}), nil
	}

	if errors.Is(err, githubpkg.ErrAuthorizationPending) {
		return connect.NewResponse(&sessionv1.PollGitHubDeviceAuthResponse{
			Status: sessionv1.DeviceAuthStatus_DEVICE_AUTH_STATUS_PENDING,
		}), nil
	}
	if errors.Is(err, githubpkg.ErrDeviceFlowExpired) {
		return connect.NewResponse(&sessionv1.PollGitHubDeviceAuthResponse{
			Status: sessionv1.DeviceAuthStatus_DEVICE_AUTH_STATUS_EXPIRED,
		}), nil
	}
	return connect.NewResponse(&sessionv1.PollGitHubDeviceAuthResponse{
		Status: sessionv1.DeviceAuthStatus_DEVICE_AUTH_STATUS_ERROR,
		Error:  err.Error(),
	}), nil
}

// +api: github-user:revoke-token
// RevokeGitHubToken removes a per-account keychain token (or the legacy slot) and resets auth state.
func (s *GitHubUserService) RevokeGitHubToken(
	ctx context.Context,
	req *connect.Request[sessionv1.RevokeGitHubTokenRequest],
) (*connect.Response[sessionv1.RevokeGitHubTokenResponse], error) {
	if req.Msg.Username != "" {
		_ = githubpkg.DeleteKeychainTokenForAccount(req.Msg.Username)
	} else {
		_ = githubpkg.DeleteKeychainToken()
	}
	s.cache.InvalidateLoginCache()
	_ = s.cache.Refresh(ctx)
	return connect.NewResponse(&sessionv1.RevokeGitHubTokenResponse{}), nil
}

// +api: github-user:list-accounts
// ListGitHubAccounts returns all connected GitHub accounts.
func (s *GitHubUserService) ListGitHubAccounts(
	_ context.Context,
	_ *connect.Request[sessionv1.ListGitHubAccountsRequest],
) (*connect.Response[sessionv1.ListGitHubAccountsResponse], error) {
	accounts := s.buildAccountList()
	return connect.NewResponse(&sessionv1.ListGitHubAccountsResponse{
		Accounts: accounts,
	}), nil
}

// resolveAuthState returns the current GitHub auth state from the cache.
// No network call is made; it reads the logins stored by the background fetch.
func (s *GitHubUserService) resolveAuthState(_ context.Context) *sessionv1.GitHubAuthState {
	logins := s.cache.GetCachedLogins()
	if len(logins) == 0 {
		return &sessionv1.GitHubAuthState{
			Available:    false,
			ErrorMessage: "GitHub authentication not yet resolved",
		}
	}
	return &sessionv1.GitHubAuthState{
		Available: true,
		Username:  logins[0],
		Accounts:  s.buildAccountList(),
	}
}

func (s *GitHubUserService) buildAccountList() []*sessionv1.GitHubAccount {
	logins := s.cache.GetCachedLogins()
	accounts := make([]*sessionv1.GitHubAccount, 0, len(logins))
	for _, login := range logins {
		isEnv := login == "env:GITHUB_TOKEN" || login == "env:GH_TOKEN"
		accounts = append(accounts, &sessionv1.GitHubAccount{
			Username:   login,
			IsEnvToken: isEnv,
		})
	}
	return accounts
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
