// Package deeplink parses the ssq:// URL scheme used for backlog item deep
// links (ssq://<hostname>/<type>/<version>/<id>). See
// project_plans/backlog-deep-linking/implementation/plan.md Story 2.1.
package deeplink

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Scheme is the URL scheme deep links use.
const Scheme = "ssq"

// SupportedVersions enumerates the deep-link path versions this binary can
// resolve. ParseDeepLink rejects any version not present here with
// ErrUnsupportedVersion rather than returning a DeepLink value carrying an
// unrecognized version — an unsupported-version DeepLink is never
// constructible.
var SupportedVersions = map[string]bool{
	"v1": true,
}

// ErrMalformed indicates the raw string is not a well-formed ssq:// deep
// link: wrong scheme, missing hostname, or missing/empty path segments.
var ErrMalformed = errors.New("deeplink: malformed ssq:// URL")

// ErrUnsupportedVersion indicates the URL is well-formed but names a
// deep-link version this binary does not recognize.
var ErrUnsupportedVersion = errors.New("deeplink: unsupported version")

// DeepLink is the parsed form of an ssq://<hostname>/<type>/<version>/<id>
// link. A DeepLink value is only ever returned by ParseDeepLink on success,
// so every field is guaranteed non-empty and Version is guaranteed to be a
// member of SupportedVersions.
type DeepLink struct {
	Hostname string
	ItemType string
	Version  string
	ID       string
}

// ParseDeepLink parses raw as an ssq://<hostname>/<type>/<version>/<id>
// deep link. It returns ErrMalformed for anything that isn't well-formed
// (wrong scheme, missing hostname, missing/empty path segments) and
// ErrUnsupportedVersion when the URL is well-formed but names a version
// this binary doesn't recognize — the version check is folded into parsing
// itself so a DeepLink with an unsupported version is never returned.
func ParseDeepLink(raw string) (DeepLink, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return DeepLink{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if u.Scheme != Scheme {
		return DeepLink{}, fmt.Errorf("%w: scheme %q, want %q", ErrMalformed, u.Scheme, Scheme)
	}
	if u.Host == "" {
		return DeepLink{}, fmt.Errorf("%w: missing hostname", ErrMalformed)
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) != 3 {
		return DeepLink{}, fmt.Errorf("%w: expected 3 path segments (type/version/id), got %d", ErrMalformed, len(segments))
	}
	itemType, version, id := segments[0], segments[1], segments[2]
	if itemType == "" || version == "" || id == "" {
		return DeepLink{}, fmt.Errorf("%w: empty path segment", ErrMalformed)
	}

	if !SupportedVersions[version] {
		return DeepLink{}, fmt.Errorf("%w: %q", ErrUnsupportedVersion, version)
	}

	return DeepLink{
		Hostname: u.Host,
		ItemType: itemType,
		Version:  version,
		ID:       id,
	}, nil
}
