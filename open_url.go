package main

import (
	"context"
	"fmt"
	"net/url"
	"runtime"

	"github.com/tstapler/stapler-squad/executor/safeexec"
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
	if err := osOpenerFunc(ctx, targetURL); err != nil {
		return fmt.Errorf("open-url: failed to open %q: %w", targetURL, err)
	}
	return nil
}
