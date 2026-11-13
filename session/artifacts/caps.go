package artifacts

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/tstapler/stapler-squad/session/domain"
)

// Blob size bounds (pre-mortem #9): kinds are capped individually, oldest first,
// then the whole blob is trimmed until it fits MaxBlobBytes.
const (
	MaxBlobBytes   = 128 * 1024
	MaxResultBytes = 512

	maxPRRecords      = 100
	maxBranchRecords  = 100
	maxCommitRecords  = 300
	maxIssueRecords   = 100
	maxPlanDocRecords = 100
	maxCILinkRecords  = 100
	maxCommandRecords = 100

	maxPRURLs     = 200
	maxCommitSHAs = 300

	maxExternalURLs = 50
	maxCommands     = 30
)

var recordCaps = map[domain.RecordKind]int{
	domain.KindPR: maxPRRecords, domain.KindBranch: maxBranchRecords, domain.KindCommit: maxCommitRecords,
	domain.KindIssue: maxIssueRecords, domain.KindPlanDoc: maxPlanDocRecords, domain.KindCILink: maxCILinkRecords,
	domain.KindCommand: maxCommandRecords,
}

// enforceCaps bounds the blob in place: result text, per-kind counts, then total
// size. Records are in discovery order, so the head of each kind is the oldest.
func enforceCaps(b *domain.SessionArtifactsBlob) {
	for i, r := range b.Records {
		if c, ok := r.(domain.CommandRecord); ok && len(c.Result) > MaxResultBytes {
			c.Result = truncateBytes(c.Result, MaxResultBytes)
			b.Records[i] = c
		}
	}
	b.PRURLs = capNewest(b.PRURLs, maxPRURLs)
	b.CommitSHAs = capNewest(b.CommitSHAs, maxCommitSHAs)
	b.Records = capKinds(b.Records)
	for encodedSize(b) > MaxBlobBytes && dropOldestOfLargestKind(b) {
	}
}

func encodedSize(b *domain.SessionArtifactsBlob) int {
	raw, err := json.Marshal(b)
	if err != nil {
		return 0
	}
	return len(raw)
}

// capKinds keeps the newest records of each kind within its cap, preserving order.
func capKinds(recs domain.RecordList) domain.RecordList {
	counts := map[domain.RecordKind]int{}
	for _, r := range recs {
		counts[r.Kind()]++
	}
	skip := map[domain.RecordKind]int{}
	for k, n := range counts {
		if over := n - recordCaps[k]; over > 0 {
			skip[k] = over
		}
	}
	if len(skip) == 0 {
		return recs
	}
	out := make(domain.RecordList, 0, len(recs))
	for _, r := range recs {
		if skip[r.Kind()] > 0 {
			skip[r.Kind()]--
			continue
		}
		out = append(out, r)
	}
	return out
}

// dropOldestOfLargestKind removes a tenth (at least one) of the oldest records
// of the kind with the most records, falling back to the URL lists; it reports
// whether it removed anything.
func dropOldestOfLargestKind(b *domain.SessionArtifactsBlob) bool {
	counts := map[domain.RecordKind]int{}
	var largest domain.RecordKind
	for _, r := range b.Records {
		counts[r.Kind()]++
		if counts[r.Kind()] > counts[largest] {
			largest = r.Kind()
		}
	}
	if counts[largest] > 0 {
		drop := max(1, counts[largest]/10)
		out := make(domain.RecordList, 0, len(b.Records))
		for _, r := range b.Records {
			if r.Kind() == largest && drop > 0 {
				drop--
				continue
			}
			out = append(out, r)
		}
		b.Records = out
		return true
	}
	switch {
	case len(b.ExternalURLs) > 0:
		b.ExternalURLs = b.ExternalURLs[len(b.ExternalURLs)/10+1:]
	case len(b.PRURLs) > 0:
		b.PRURLs = b.PRURLs[len(b.PRURLs)/10+1:]
	case len(b.CommitSHAs) > 0:
		b.CommitSHAs = b.CommitSHAs[len(b.CommitSHAs)/10+1:]
	default:
		return false
	}
	return true
}

// truncateBytes cuts s to at most n bytes without splitting a multi-byte rune:
// a mid-rune cut yields invalid UTF-8, which proto marshalling rejects.
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
