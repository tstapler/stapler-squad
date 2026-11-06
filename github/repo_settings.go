package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

// RepoSettingsStatus is the outcome class of a repository-settings lookup.
// Only RepoSettingsOK carries trustworthy setting values.
type RepoSettingsStatus int

const (
	// RepoSettingsUnspecified is the zero value: no lookup was made.
	RepoSettingsUnspecified RepoSettingsStatus = iota
	RepoSettingsOK
	RepoSettingsRateLimited
	// RepoSettingsNoPermission means the token could not read the setting
	// (GitHub omits allow_auto_merge for tokens without push access).
	RepoSettingsNoPermission
	RepoSettingsNotFound
	RepoSettingsError
)

func (s RepoSettingsStatus) String() string {
	switch s {
	case RepoSettingsOK:
		return "OK"
	case RepoSettingsRateLimited:
		return "RATE_LIMITED"
	case RepoSettingsNoPermission:
		return "NO_PERMISSION"
	case RepoSettingsNotFound:
		return "NOT_FOUND"
	case RepoSettingsError:
		return "ERROR"
	default:
		return "UNSPECIFIED"
	}
}

// RepoSettingsResult is the result of RepoSettings. The setting values are
// unexported so "false" and "not asked" cannot be conflated: read them via
// AutoMerge/SquashMerge, which report ok=false unless Status is OK.
type RepoSettingsResult struct {
	Status RepoSettingsStatus
	// Reason is a short machine-readable detail for non-OK results
	// (e.g. "timeout", "circuit_open", "http_502").
	Reason string
	// CheckedAt is when the data was last fetched (OK) or the attempt was made
	// (failure). Zero for RepoSettingsUnspecified.
	CheckedAt time.Time
	// Stale is true when Status is OK but the latest refresh failed, so the
	// values come from an earlier successful fetch.
	Stale bool

	autoMerge   bool
	squashMerge bool
}

// AutoMerge returns the repo's allow_auto_merge setting; ok is false unless
// Status is OK, in which case allowed is always false.
func (r RepoSettingsResult) AutoMerge() (allowed, ok bool) {
	if r.Status != RepoSettingsOK {
		return false, false
	}
	return r.autoMerge, true
}

// SquashMerge returns the repo's allow_squash_merge setting with the same
// ok semantics as AutoMerge.
func (r RepoSettingsResult) SquashMerge() (allowed, ok bool) {
	if r.Status != RepoSettingsOK {
		return false, false
	}
	return r.squashMerge, true
}

const (
	repoSettingsCallSite     = "repo.settings"
	repoSettingsMaxBodyBytes = 1 << 20
	// repoSettingsDefaultBackoff applies to a 429/403 that carries no
	// Retry-After or X-RateLimit-Reset hint.
	repoSettingsDefaultBackoff = time.Minute
)

// repoSettingsFetch is one parsed HTTP exchange, before caching.
type repoSettingsFetch struct {
	status      RepoSettingsStatus
	reason      string
	autoMerge   bool
	squashMerge bool
	etag        string
	notModified bool
	// backoff is how long a RateLimited result should suppress re-fetching.
	backoff time.Duration
}

// fetchRepoSettings performs a single GET /repos/{owner}/{repo}, sending
// If-None-Match when etag is non-empty, and classifies the response.
func (c *repoSettingsLookup) fetchRepoSettings(ctx context.Context, ref RepoRef, etag string) repoSettingsFetch {
	ctx = WithGitHubCallOrigin(WithGitHubCallSite(ctx, repoSettingsCallSite), OriginInteractive)
	path := "repos/" + ref.Owner() + "/" + ref.Repo()
	req, err := newGHRequestForHostWithToken(ctx, ref.Host(), path, c.token(ctx, ref.Host()))
	if err != nil {
		return repoSettingsFetch{status: RepoSettingsError, reason: "bad_request"}
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return classifyRepoSettingsTransportError(ctx, err)
	}
	defer resp.Body.Close()
	return classifyRepoSettingsResponse(resp)
}

func classifyRepoSettingsTransportError(ctx context.Context, err error) repoSettingsFetch {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return repoSettingsFetch{status: RepoSettingsError, reason: "timeout"}
	}
	// rateLimitTransport refuses to dispatch while the shared limiter is tripped.
	if limited, until := DefaultRateLimiter.IsLimited(); limited {
		return repoSettingsFetch{status: RepoSettingsRateLimited, reason: "client_rate_limited", backoff: time.Until(until)}
	}
	return repoSettingsFetch{status: RepoSettingsError, reason: "network"}
}

func classifyRepoSettingsResponse(resp *http.Response) repoSettingsFetch {
	switch code := resp.StatusCode; {
	case code == http.StatusNotModified:
		return repoSettingsFetch{status: RepoSettingsOK, notModified: true}
	case code == http.StatusOK:
		return parseRepoSettingsBody(resp)
	case code == http.StatusTooManyRequests, code == http.StatusForbidden && isRateLimitedResponse(resp):
		return repoSettingsFetch{status: RepoSettingsRateLimited, reason: httpReason(code), backoff: rateLimitBackoff(resp)}
	case code == http.StatusUnauthorized, code == http.StatusForbidden:
		return repoSettingsFetch{status: RepoSettingsNoPermission, reason: httpReason(code)}
	case code == http.StatusNotFound:
		return repoSettingsFetch{status: RepoSettingsNotFound, reason: httpReason(code)}
	default:
		return repoSettingsFetch{status: RepoSettingsError, reason: httpReason(code)}
	}
}

func parseRepoSettingsBody(resp *http.Response) repoSettingsFetch {
	var body struct {
		AllowAutoMerge   *bool `json:"allow_auto_merge"`
		AllowSquashMerge *bool `json:"allow_squash_merge"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, repoSettingsMaxBodyBytes)).Decode(&body); err != nil {
		return repoSettingsFetch{status: RepoSettingsError, reason: "bad_body"}
	}
	// GitHub only returns allow_auto_merge to tokens with push access; its
	// absence means "could not read", never "disabled".
	if body.AllowAutoMerge == nil {
		return repoSettingsFetch{status: RepoSettingsNoPermission, reason: "field_absent"}
	}
	f := repoSettingsFetch{
		status:    RepoSettingsOK,
		autoMerge: *body.AllowAutoMerge,
		etag:      resp.Header.Get("ETag"),
	}
	if body.AllowSquashMerge != nil {
		f.squashMerge = *body.AllowSquashMerge
	}
	return f
}

// isRateLimitedResponse distinguishes a secondary/primary rate-limit 403 from
// a permission 403.
func isRateLimitedResponse(resp *http.Response) bool {
	return resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0"
}

func rateLimitBackoff(resp *http.Response) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if s := resp.Header.Get("X-RateLimit-Reset"); s != "" {
		if epoch, err := strconv.ParseInt(s, 10, 64); err == nil {
			if d := time.Until(time.Unix(epoch, 0)); d > 0 {
				return d
			}
		}
	}
	return repoSettingsDefaultBackoff
}

func httpReason(code int) string { return "http_" + strconv.Itoa(code) }
