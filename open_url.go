package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/pkg/localtoken"
	"github.com/tstapler/stapler-squad/session/deeplink"
)

// localWebAppBaseURL is the address the local web UI listens on. --open-url
// always targets this host: deep links are resolved to a navigation target
// on the box the link was clicked on (see
// project_plans/backlog-deep-linking/design/ux.md's same-host case), not the
// hostname embedded in the ssq:// link itself — that hostname only matters
// to the cross-host resolver (server/services/deep_link_resolver.go).
const localWebAppBaseURL = "http://localhost:8543"

// translateDeepLinkURL parses raw as an ssq:// deep link and returns the
// local web UI URL it should navigate to, matching the existing
// ?item=<id> deep-link convention the web app already uses (see
// web-app/src/app/backlog/page.tsx's selectedItemId query param).
func translateDeepLinkURL(raw string) (string, error) {
	link, err := deeplink.ParseDeepLink(raw)
	if err != nil {
		return "", err
	}
	// ItemType/ID come from ParseDeepLink's decoded URL path segments, so a
	// crafted ssq:// link (e.g. an ID containing a percent-encoded '&') could
	// otherwise inject extra query parameters into the URL handed to the OS
	// opener. url.PathEscape/QueryEscape close that off the same way
	// web-app/src/app/resolve/page.tsx's encodeURIComponent already does on
	// the browser side.
	return fmt.Sprintf("%s/%s?item=%s", localWebAppBaseURL, url.PathEscape(link.ItemType), url.QueryEscape(link.ID)), nil
}

// osOpenerCommand returns the OS command used to open a URL in the user's
// default browser: "open" on macOS, "xdg-open" on Linux. No Go stdlib
// equivalent exists for launching the OS's default URL handler, so shelling
// out here is justified per
// the `prefer-go-git-over-subshells` skill's "still fine" exception.
func osOpenerCommand() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "open", nil
	case "linux":
		return "xdg-open", nil
	default:
		return "", fmt.Errorf("open-url: unsupported OS %q", runtime.GOOS)
	}
}

// osOpenerFunc shells out to the OS's default URL opener. Overridden in
// tests so the actual subprocess is never invoked.
var osOpenerFunc = func(ctx context.Context, targetURL string) error { //nolint:gochecknoglobals // test seam, see doc comment above
	opener, err := osOpenerCommand()
	if err != nil {
		return err
	}
	cmd := safeexec.CommandContext(ctx, opener, targetURL)
	return cmd.Run()
}

// runOpenURL implements --open-url: translate raw (an ssq:// deep link) to
// a local web UI URL and shell out to the OS's default opener. Returns an
// error with a single human-readable message on malformed input — never a
// panic — so the caller (main.go's RunE) can print it to stderr and exit
// non-zero without a stack trace.
func runOpenURL(ctx context.Context, raw string) error {
	targetURL, err := translateDeepLinkURL(raw)
	if err != nil {
		return fmt.Errorf("open-url: invalid deep link %q: %w", raw, err)
	}
	targetURL = withLocalLogin(ctx, targetURL)
	if err := osOpenerFunc(ctx, targetURL); err != nil {
		return fmt.Errorf("open-url: failed to open %q: %w", targetURL, err)
	}
	return nil
}

// withLocalLogin wraps targetURL in a one-time login URL when the server runs
// with require_local_auth, so the browser gets a session cookie without the
// token ever appearing in a URL. Falls back to targetURL unchanged when no
// token file exists or the server has no login endpoint (auth off).
func withLocalLogin(ctx context.Context, targetURL string) string {
	dir, err := config.GetConfigDir()
	if err != nil {
		return targetURL
	}
	token := localtoken.FromConfigDir(dir)
	if token == "" {
		return targetURL
	}
	target, err := url.Parse(targetURL)
	if err != nil {
		return targetURL
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, localWebAppBaseURL+"/auth/local-login/code", nil)
	if err != nil {
		return targetURL
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return targetURL
	}
	defer resp.Body.Close()
	var body struct {
		Code string `json:"code"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body) != nil || body.Code == "" {
		return targetURL
	}
	q := url.Values{"code": {body.Code}, "next": {target.RequestURI()}}
	loginURL := localWebAppBaseURL + "/auth/local-login?" + q.Encode()
	// Hand the browser a 0600 redirect page instead of the URL itself, so the
	// one-time code is never in the opener's argv (visible to other local users).
	if page, err := writeLoginRedirectPage(loginURL); err == nil {
		return (&url.URL{Scheme: "file", Path: page}).String()
	}
	return loginURL
}

// writeLoginRedirectPage writes a tiny meta-refresh page (mode 0600) that
// forwards to loginURL, and prunes stale pages from earlier runs. The embedded
// code is single-use and expires in 60s, so a leftover page is inert.
func writeLoginRedirectPage(loginURL string) (string, error) {
	const prefix = "ssq-login-"
	if old, err := filepath.Glob(filepath.Join(os.TempDir(), prefix+"*.html")); err == nil {
		for _, f := range old {
			if info, statErr := os.Stat(f); statErr == nil && time.Since(info.ModTime()) > 10*time.Minute {
				_ = os.Remove(f) //nolint:errcheck
			}
		}
	}
	f, err := os.CreateTemp("", prefix+"*.html") // 0600
	if err != nil {
		return "", err
	}
	defer f.Close()
	page := `<!doctype html><meta http-equiv="refresh" content="0;url=` + html.EscapeString(loginURL) + `">`
	if _, err := f.WriteString(page); err != nil {
		return "", err
	}
	return f.Name(), nil
}
