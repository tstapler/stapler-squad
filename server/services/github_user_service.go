package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check: GitHubUserService must implement the generated handler.
var _ sessionv1connect.GitHubUserServiceHandler = (*GitHubUserService)(nil)

// pendingDeviceAuth stashes the host/clientID for an in-flight device code so
// PollGitHubDeviceAuth can resolve them without the proto needing to carry
// them on every poll request.
type pendingDeviceAuth struct {
	host     string
	clientID string
}

// GitHubUserService implements the ConnectRPC GitHubUserServiceHandler.
type GitHubUserService struct {
	cache           *githubpkg.UserPRCache
	enterpriseHosts []config.GitHubEnterpriseHost

	pendingMu sync.Mutex
	pending   map[string]pendingDeviceAuth // device_code -> host/clientID
}

// NewGitHubUserService creates a new service backed by the given cache.
func NewGitHubUserService(cache *githubpkg.UserPRCache, enterpriseHosts []config.GitHubEnterpriseHost) *GitHubUserService {
	return &GitHubUserService{
		cache:           cache,
		enterpriseHosts: enterpriseHosts,
		pending:         make(map[string]pendingDeviceAuth),
	}
}

// clientIDForHost looks up the registered OAuth App client_id for an
// enterprise host. Returns "" for github.com (StartDeviceAuth falls back to
// the default/env-var client_id in that case).
func (s *GitHubUserService) clientIDForHost(host string) string {
	if githubpkg.IsGitHubCom(host) {
		return ""
	}
	for _, h := range s.enterpriseHosts {
		if githubpkg.NormalizeHost(h.Host) == host {
			return h.ClientID
		}
	}
	return ""
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
	req *connect.Request[sessionv1.StartGitHubDeviceAuthRequest],
) (*connect.Response[sessionv1.StartGitHubDeviceAuthResponse], error) {
	host := githubpkg.NormalizeHost(req.Msg.Host)
	clientID := s.clientIDForHost(host)
	da, err := githubpkg.StartDeviceAuth(ctx, host, clientID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("start device auth: %w", err))
	}

	s.pendingMu.Lock()
	s.pending[da.DeviceCode] = pendingDeviceAuth{host: host, clientID: clientID}
	s.pendingMu.Unlock()

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

	s.pendingMu.Lock()
	pend, ok := s.pending[req.Msg.DeviceCode]
	s.pendingMu.Unlock()
	if !ok {
		// Unknown device_code (e.g. server restarted mid-flow) — fall back to github.com.
		pend = pendingDeviceAuth{host: githubpkg.NormalizeHost(""), clientID: ""}
	}

	token, err := githubpkg.PollDeviceAuth(ctx, pend.host, pend.clientID, req.Msg.DeviceCode)
	if err == nil {
		s.pendingMu.Lock()
		delete(s.pending, req.Msg.DeviceCode)
		s.pendingMu.Unlock()

		// Discover the username and store per-account in the keychain.
		if storeErr := githubpkg.StoreTokenForDiscoveredUser(ctx, pend.host, token); storeErr != nil {
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
		s.pendingMu.Lock()
		delete(s.pending, req.Msg.DeviceCode)
		s.pendingMu.Unlock()
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
		_ = githubpkg.DeleteKeychainTokenForAccount(req.Msg.Host, req.Msg.Username)
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

	// EnterpriseHosts drives the omnibar's GHE URL detector, so it must reflect
	// every host the user can actually reach — not just hosts with a
	// statically configured OAuth App (s.enterpriseHosts), which omits hosts
	// added via AddGitHubAccountWithToken/AddGitHubAccountFromCLI (e.g. gh CLI
	// import) since those never touch s.enterpriseHosts.
	seen := make(map[string]bool, len(s.enterpriseHosts)+len(accounts))
	hosts := make([]string, 0, len(s.enterpriseHosts)+len(accounts))
	addHost := func(host string) {
		host = githubpkg.NormalizeHost(host)
		if host == "" || githubpkg.IsGitHubCom(host) || seen[host] {
			return
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	for _, h := range s.enterpriseHosts {
		addHost(h.Host)
	}
	for _, a := range accounts {
		addHost(a.Host)
	}
	return connect.NewResponse(&sessionv1.ListGitHubAccountsResponse{
		Accounts:        accounts,
		EnterpriseHosts: hosts,
	}), nil
}

// +api: github-user:add-account-with-token
// AddGitHubAccountWithToken validates a personal access token against the
// host's /user endpoint and stores it in the keychain on success. Use this
// for hosts that don't support OAuth Device Flow (e.g. some GHES instances).
func (s *GitHubUserService) AddGitHubAccountWithToken(
	ctx context.Context,
	req *connect.Request[sessionv1.AddGitHubAccountWithTokenRequest],
) (*connect.Response[sessionv1.AddGitHubAccountWithTokenResponse], error) {
	token := strings.TrimSpace(req.Msg.Token)
	if token == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("token is required"))
	}
	host := githubpkg.NormalizeHost(req.Msg.Host)

	login, err := githubpkg.GetCurrentUserLoginWithToken(ctx, host, token)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("validate token: %w", err))
	}
	if login == "" {
		// CodePermissionDenied, not CodeUnauthenticated: the latter triggers the
		// frontend's global session-expired redirect to /login (createAuthInterceptor
		// in web-app/src/lib/config.ts), which would yank the user off this form
		// instead of showing the inline "token rejected" error.
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("token was rejected — check the token and host"))
	}
	if err := githubpkg.SetKeychainTokenForAccount(host, login, token); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("store token: %w", err))
	}

	s.cache.InvalidateLoginCache()
	_ = s.cache.Refresh(ctx)
	return connect.NewResponse(&sessionv1.AddGitHubAccountWithTokenResponse{
		AuthState: s.resolveAuthState(ctx),
	}), nil
}

// +api: github-user:list-cli-hosts
// ListGitHubCLIHosts discovers hosts the local gh CLI is already
// authenticated to, so the UI can offer one-click imports.
func (s *GitHubUserService) ListGitHubCLIHosts(
	_ context.Context,
	_ *connect.Request[sessionv1.ListGitHubCLIHostsRequest],
) (*connect.Response[sessionv1.ListGitHubCLIHostsResponse], error) {
	cliHosts, err := githubpkg.ListCLIHosts()
	if err != nil {
		return connect.NewResponse(&sessionv1.ListGitHubCLIHostsResponse{
			GhAvailable: false,
		}), nil
	}

	connected := make(map[string]bool)
	for _, a := range s.cache.GetCachedAccounts() {
		connected[githubpkg.NormalizeHost(a.Host)+"|"+a.Login] = true
	}

	hosts := make([]*sessionv1.GitHubCLIHost, 0, len(cliHosts))
	for _, h := range cliHosts {
		hosts = append(hosts, &sessionv1.GitHubCLIHost{
			Host:         h.Host,
			Username:     h.Username,
			AlreadyAdded: connected[h.Host+"|"+h.Username],
		})
	}
	return connect.NewResponse(&sessionv1.ListGitHubCLIHostsResponse{
		Hosts:       hosts,
		GhAvailable: true,
	}), nil
}

// +api: github-user:add-account-from-cli
// AddGitHubAccountFromCLI fetches the token gh CLI already holds for a host
// (discovered via ListGitHubCLIHosts), validates it, and stores it in the
// keychain — same outcome as AddGitHubAccountWithToken without manual paste.
func (s *GitHubUserService) AddGitHubAccountFromCLI(
	ctx context.Context,
	req *connect.Request[sessionv1.AddGitHubAccountFromCLIRequest],
) (*connect.Response[sessionv1.AddGitHubAccountWithTokenResponse], error) {
	host := githubpkg.NormalizeHost(req.Msg.Host)

	token, err := githubpkg.GetCLIToken(ctx, host)
	if err != nil || token == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("gh CLI has no token for %s: %w", host, err))
	}

	login, err := githubpkg.GetCurrentUserLoginWithToken(ctx, host, token)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("validate token: %w", err))
	}
	if login == "" {
		// CodePermissionDenied, not CodeUnauthenticated: see AddGitHubAccountWithToken.
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("gh CLI token was rejected by the host"))
	}
	if err := githubpkg.SetKeychainTokenForAccount(host, login, token); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("store token: %w", err))
	}

	s.cache.InvalidateLoginCache()
	_ = s.cache.Refresh(ctx)
	return connect.NewResponse(&sessionv1.AddGitHubAccountWithTokenResponse{
		AuthState: s.resolveAuthState(ctx),
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
	cached := s.cache.GetCachedAccounts()
	accounts := make([]*sessionv1.GitHubAccount, 0, len(cached))
	for _, a := range cached {
		isEnv := a.Login == "env:GITHUB_TOKEN" || a.Login == "env:GH_TOKEN"
		accounts = append(accounts, &sessionv1.GitHubAccount{
			Username:   a.Login,
			IsEnvToken: isEnv,
			Host:       a.Host,
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
