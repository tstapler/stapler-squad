package backlogdto

import (
	"fmt"
	"regexp"
	"strconv"
)

// PrNumberFromURLRe extracts the trailing PR number from a GitHub PR URL,
// e.g. "https://github.com/owner/repo/pull/148" -> 148.
var PrNumberFromURLRe = regexp.MustCompile(`/pull/(\d+)/?$`)

// ParsePRNumberFromURL extracts the trailing PR number from a GitHub PR URL,
// sharing the regex BackfillMissingPRNumbers uses so both parses agree.
func ParsePRNumberFromURL(prURL string) (int, error) {
	m := PrNumberFromURLRe.FindStringSubmatch(prURL)
	if m == nil {
		return 0, fmt.Errorf("no PR number found in %q", prURL)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("parsing PR number from %q: %w", prURL, err)
	}
	return n, nil
}
