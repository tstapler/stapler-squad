package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// defaultClientID is the GitHub OAuth App client ID for Stapler Squad.
// Users can override this via STAPLER_SQUAD_GITHUB_CLIENT_ID.
// The default is GitHub CLI's well-known client ID for personal tooling.
const defaultClientID = "178c6fc778ccc68e1d6a"

// deviceAuthScopes are the OAuth scopes requested during device flow.
// "read:user" retrieves login; "repo" allows reading PR status.
const deviceAuthScopes = "read:user repo"

// ErrAuthorizationPending is returned by PollDeviceAuth while waiting for
// the user to complete the browser authorization step.
var ErrAuthorizationPending = errors.New("authorization_pending")

// ErrDeviceFlowExpired is returned when the device code has expired before
// the user authorized. The caller should restart the flow.
var ErrDeviceFlowExpired = errors.New("expired_token")

// DeviceAuthStart holds the values returned by the first step of the Device
// Flow: the user-visible code and the URL where they enter it.
type DeviceAuthStart struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	// ExpiresIn is how long the code is valid (seconds).
	ExpiresIn int
	// Interval is the minimum poll interval suggested by GitHub (seconds).
	Interval int
}

// clientID returns the OAuth client ID, preferring the env var override.
func clientID() string {
	if v := os.Getenv("STAPLER_SQUAD_GITHUB_CLIENT_ID"); v != "" {
		return v
	}
	return defaultClientID
}

// StartDeviceAuth initiates the GitHub Device Flow and returns the codes the
// user must enter at verification_uri. The returned DeviceAuthStart.DeviceCode
// must be passed to PollDeviceAuth.
func StartDeviceAuth(ctx context.Context) (*DeviceAuthStart, error) {
	body := url.Values{}
	body.Set("client_id", clientID())
	body.Set("scope", deviceAuthScopes)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/device/code",
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("build device code request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code: unexpected status %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode device code response: %w", err)
	}
	if result.DeviceCode == "" || result.UserCode == "" {
		return nil, errors.New("device code response missing required fields")
	}
	if result.Interval == 0 {
		result.Interval = 5
	}
	return &DeviceAuthStart{
		DeviceCode:      result.DeviceCode,
		UserCode:        result.UserCode,
		VerificationURI: result.VerificationURI,
		ExpiresIn:       result.ExpiresIn,
		Interval:        result.Interval,
	}, nil
}

// PollDeviceAuth polls GitHub's token endpoint once.
//
//   - Returns (token, nil) on success — the caller should store the token.
//   - Returns ("", ErrAuthorizationPending) if the user hasn't approved yet.
//   - Returns ("", ErrDeviceFlowExpired) if the device code has expired.
//   - Returns ("", err) for any other error.
func PollDeviceAuth(ctx context.Context, deviceCode string) (string, error) {
	body := url.Values{}
	body.Set("client_id", clientID())
	body.Set("device_code", deviceCode)
	body.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("poll token endpoint: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if result.AccessToken != "" {
		return result.AccessToken, nil
	}
	switch result.Error {
	case "authorization_pending":
		return "", ErrAuthorizationPending
	case "expired_token":
		return "", ErrDeviceFlowExpired
	case "slow_down":
		// Caller should increase poll interval; treat as pending for now.
		return "", ErrAuthorizationPending
	default:
		return "", fmt.Errorf("github device auth error: %s – %s", result.Error, result.ErrorDesc)
	}
}

// StoreTokenForDiscoveredUser fetches the GitHub login for token and stores it
// under the per-username keychain slot. Falls back to the legacy slot if the
// login cannot be determined.
func StoreTokenForDiscoveredUser(ctx context.Context, token string) error {
	login, err := GetCurrentUserLoginWithToken(ctx, token)
	if err == nil && login != "" {
		return SetKeychainTokenForAccount(login, token)
	}
	return SetKeychainToken(token)
}

// WaitForDeviceAuth polls GitHub repeatedly until the user completes
// authorization, the code expires, or ctx is cancelled.
// On success it stores the token in the OS keychain and returns it.
func WaitForDeviceAuth(ctx context.Context, da *DeviceAuthStart) (string, error) {
	interval := time.Duration(da.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(da.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}

		if time.Now().After(deadline) {
			return "", ErrDeviceFlowExpired
		}

		token, err := PollDeviceAuth(ctx, da.DeviceCode)
		if err == nil {
			_ = StoreTokenForDiscoveredUser(ctx, token)
			return token, nil
		}
		if errors.Is(err, ErrAuthorizationPending) {
			continue
		}
		return "", err
	}
}
