package jules

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// defaultBaseURL is the Jules API's alpha base URL.
const defaultBaseURL = "https://jules.googleapis.com/v1alpha"

// maxListSourcesPages bounds ListSources' nextPageToken loop so a
// misbehaving server cannot make a single call paginate forever.
const maxListSourcesPages = 10

// JulesTokenSource resolves the API key used to authenticate Jules API
// calls. Implementations (e.g. a keychain-backed source, Epic 1.2) may
// return an error satisfying errors.Is(err, ErrJulesNotConfigured) when no
// key is available.
type JulesTokenSource interface {
	APIKey(ctx context.Context) (JulesAPIKey, error)
}

// Client is a typed gateway over the three Jules API endpoints the MVP
// needs: ListSources, CreateSession, GetSession. It builds requests, sets
// authentication, classifies non-2xx responses into sentinel errors
// (errors.go), and decodes JSON — so call sites never build URLs or parse
// JSON themselves.
type Client struct {
	httpClient *http.Client
	baseURL    string
	tokens     JulesTokenSource
	limiter    *rateLimiter
}

// Option configures a Client constructed by NewClient.
type Option func(*Client)

// WithBaseURL overrides the Jules API base URL — a test seam for pointing
// the client at an httptest.Server.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = baseURL }
}

// WithHTTPClient overrides the underlying *http.Client. The rate-limit
// transport (rate_limit.go) is installed around whatever Transport the
// passed client carries (nil means http.DefaultTransport).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc == nil {
			return
		}
		c.httpClient = hc
	}
}

// NewClient builds a Client. tokens resolves the API key on every request;
// the default HTTP client has a 30s timeout, mirroring
// github/http_client.go's ghHTTPClient.
func NewClient(tokens JulesTokenSource, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    defaultBaseURL,
		tokens:     tokens,
		limiter:    newRateLimiter(),
	}
	for _, opt := range opts {
		opt(c)
	}
	next := c.httpClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	if _, alreadyWrapped := next.(*julesRateLimitTransport); !alreadyWrapped {
		c.httpClient.Transport = &julesRateLimitTransport{next: next, limiter: c.limiter}
	}
	return c
}

// IsLimited reports whether the client is currently rate limited from a
// prior 429 response.
func (c *Client) IsLimited() bool {
	return c.limiter.IsLimited()
}

// RetryAfter returns how long the client will remain rate limited. Zero
// when not limited.
func (c *Client) RetryAfter() time.Duration {
	return c.limiter.RetryAfter()
}

// newRequest builds an authenticated request against path (relative to
// baseURL), JSON-encoding body when non-nil. It resolves the API key via
// tokens, sets it on the x-goog-api-key header, and never logs it.
func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	key, err := c.tokens.APIKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("jules: resolving API key: %w", err)
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("jules: encoding request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("jules: building request: %w", err)
	}
	req.Header.Set("x-goog-api-key", key.reveal())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// do executes req and returns a classified error (errors.go) for any
// non-2xx response.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jules: request to %s: %w", req.URL.Path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, classifyJulesResponse(resp)
	}
	return resp, nil
}

// JulesSource is a GitHub repository Jules can dispatch sessions against,
// registered ahead of time through the Jules web UI's GitHub App (no create
// endpoint exists — sources are read-only via the API).
type JulesSource struct {
	Name JulesSourceName `json:"name"`
	ID   string          `json:"id"`
}

// JulesPullRequestOutput is a pull request Jules opened as the result of a
// session (AUTO_CREATE_PR automationMode).
type JulesPullRequestOutput struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// JulesSessionOutput is one element of JulesSession.Outputs. Only
// PullRequest is populated for the MVP's AUTO_CREATE_PR flow.
type JulesSessionOutput struct {
	PullRequest *JulesPullRequestOutput `json:"pullRequest,omitempty"`
}

// JulesSession is a Jules API session resource, as returned by CreateSession
// and GetSession.
type JulesSession struct {
	Name       JulesSessionName     `json:"name"`
	ID         string               `json:"id"`
	State      JulesSessionState    `json:"state"`
	Title      string               `json:"title,omitempty"`
	Outputs    []JulesSessionOutput `json:"outputs,omitempty"`
	CreateTime string               `json:"createTime,omitempty"`
	UpdateTime string               `json:"updateTime,omitempty"`
	URL        string               `json:"url,omitempty"`
}

// CreateSessionRequest is the input to Client.CreateSession. The MVP is
// fire-and-forget (requirements.md): RequirePlanApproval and AutomationMode
// are not caller-controlled — CreateSession hardcodes them.
type CreateSessionRequest struct {
	Prompt         string
	Source         JulesSourceName
	StartingBranch GitHubBranchRef
}

// julesSourceContext / julesGitHubRepoContext / julesCreateSessionWire are
// the wire-shaped request body for POST /v1alpha/sessions — kept unexported
// and separate from CreateSessionRequest so the public API stays flat while
// the wire nesting (sourceContext.githubRepoContext.startingBranch) is
// exactly what the Jules API documents (research/stack.md §Sessions).
type julesGitHubRepoContext struct {
	StartingBranch GitHubBranchRef `json:"startingBranch"`
}

type julesSourceContext struct {
	Source            JulesSourceName        `json:"source"`
	GitHubRepoContext julesGitHubRepoContext `json:"githubRepoContext"`
}

type julesCreateSessionWire struct {
	Prompt              string             `json:"prompt"`
	SourceContext       julesSourceContext `json:"sourceContext"`
	RequirePlanApproval bool               `json:"requirePlanApproval"`
	AutomationMode      string             `json:"automationMode"`
}

// julesListSourcesResponse is the wire shape of GET /v1alpha/sources.
type julesListSourcesResponse struct {
	Sources       []JulesSource `json:"sources"`
	NextPageToken string        `json:"nextPageToken,omitempty"`
}

// ListSources lists every GitHub source registered against the caller's
// Jules account, following nextPageToken until it is empty (capped at
// maxListSourcesPages to bound a misbehaving server).
func (c *Client) ListSources(ctx context.Context) ([]JulesSource, error) {
	var all []JulesSource
	pageToken := ""
	for i := 0; i < maxListSourcesPages; i++ {
		path := "/sources"
		if pageToken != "" {
			path += "?pageToken=" + url.QueryEscape(pageToken)
		}
		req, err := c.newRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.do(req)
		if err != nil {
			return nil, err
		}
		var body julesListSourcesResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("jules: decoding ListSources response: %w", decodeErr)
		}
		all = append(all, body.Sources...)
		if body.NextPageToken == "" {
			break
		}
		pageToken = body.NextPageToken
	}
	return all, nil
}

// CreateSession starts a Jules session against source's starting branch with
// prompt, fire-and-forget (requirePlanApproval=false,
// automationMode=AUTO_CREATE_PR — no caller knob for MVP).
func (c *Client) CreateSession(ctx context.Context, in CreateSessionRequest) (*JulesSession, error) {
	wire := julesCreateSessionWire{
		Prompt: in.Prompt,
		SourceContext: julesSourceContext{
			Source: in.Source,
			GitHubRepoContext: julesGitHubRepoContext{
				StartingBranch: in.StartingBranch,
			},
		},
		RequirePlanApproval: false,
		AutomationMode:      "AUTO_CREATE_PR",
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/sessions", wire)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var session JulesSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("jules: decoding CreateSession response: %w", err)
	}
	return &session, nil
}

// GetSession fetches the current state of a session by resource name, e.g.
// "sessions/abc". A 404 surfaces as ErrJulesSessionNotFound so the poller
// can end a vanished session rather than retry forever.
func (c *Client) GetSession(ctx context.Context, name JulesSessionName) (*JulesSession, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/"+string(name), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var session JulesSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("jules: decoding GetSession response: %w", err)
	}
	return &session, nil
}
