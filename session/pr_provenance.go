package session

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/session/deeplink"
)

// prProvenanceMarker opens every provenance comment so a reader can find it
// among ordinary PR comments. The comment carries only an opaque HostID and the
// item's ssq:// deep link. The comment is public on GitHub, so it is only
// written when the deep link names a hostname, never a bare IP address (see
// ClaimIndexRecorder.PRProvenanceComment).
const prProvenanceMarker = "<!-- ssq-provenance:v1 -->"

var (
	provenanceDeepLinkPattern = regexp.MustCompile(`ssq://[^\s<>()\[\]"']+`)
	provenanceHostIDPattern   = regexp.MustCompile(`host_[0-9A-Za-z]{26}`)
)

// PRProvenance identifies the backlog item and host a PR was created from.
type PRProvenance struct {
	HostID       HostID
	ItemDeepLink string
	ItemID       string
}

// FormatPRProvenanceComment renders the PR comment that stamps a PR with its
// originating host and item.
func FormatPRProvenanceComment(hostID HostID, itemDeepLink string) string {
	return fmt.Sprintf("%s\nTracked by Stapler Squad backlog item: %s\nHost: %s\n",
		prProvenanceMarker, itemDeepLink, hostID.String())
}

// ParsePRProvenanceComment extracts the provenance stamp from a PR comment body.
// It reports false for any comment without the marker or with an unparseable
// deep link or host ID.
func ParsePRProvenanceComment(body string) (PRProvenance, bool) {
	if !strings.Contains(body, prProvenanceMarker) {
		return PRProvenance{}, false
	}
	rawLink := provenanceDeepLinkPattern.FindString(body)
	link, err := deeplink.ParseDeepLink(rawLink)
	if err != nil {
		return PRProvenance{}, false
	}
	hostID, err := ParseHostID(provenanceHostIDPattern.FindString(body))
	if err != nil {
		return PRProvenance{}, false
	}
	return PRProvenance{HostID: hostID, ItemDeepLink: rawLink, ItemID: link.ID}, true
}

// PRProvenanceSource builds the provenance comment for an item, or reports
// false when this host cannot name itself in a deep link.
type PRProvenanceSource interface {
	PRProvenanceComment(item *BacklogItemData) (string, bool)
}

// PRProvenanceComment implements PRProvenanceSource using the same host name the
// recorder puts in claim deep links. It declines when that name is an IP
// literal: claim deep links go only to enrolled peers, but this comment is
// posted to a public PR, and must not publish a LAN address.
func (r *ClaimIndexRecorder) PRProvenanceComment(item *BacklogItemData) (string, bool) {
	if item == nil || r.deepLinkHost == "" || net.ParseIP(strings.Trim(r.deepLinkHost, "[]")) != nil {
		return "", false
	}
	return FormatPRProvenanceComment(r.identity.ID, "ssq://"+r.deepLinkHost+BacklogItemDeepLinkPath(item)), true
}

// SetPRProvenanceSource wires the source PRProvenanceComment uses. Passing nil
// disables provenance stamping.
func (s *Storage) SetPRProvenanceSource(src PRProvenanceSource) {
	if src == nil {
		s.provenance.Store(nil)
		return
	}
	s.provenance.Store(&src)
}

// PRProvenanceComment returns the provenance comment for item, or false when no
// source is wired or the source declines.
func (s *Storage) PRProvenanceComment(item *BacklogItemData) (string, bool) {
	src := s.provenance.Load()
	if src == nil {
		return "", false
	}
	return (*src).PRProvenanceComment(item)
}
