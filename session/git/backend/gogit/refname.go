package gogit

import (
	"fmt"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// optArg mirrors the CLI backend's guard: an empty value, or one git would read as an option, is
// an invalid argument before anything runs.
func optArg(name, v string) error {
	if v == "" {
		return fmt.Errorf("%w: %s is empty", backend.ErrInvalidArgument, name)
	}
	if strings.HasPrefix(v, "-") {
		return fmt.Errorf("%w: %s %q starts with '-'", backend.ErrInvalidArgument, name, v)
	}
	return nil
}

// commitRef mirrors the CLI backend's validation for the commit-only methods: no peel suffix
// other than ^{commit}, and no "rev:path" tree-ish expressions.
func commitRef(ref backend.RefName) error {
	if err := optArg("ref", string(ref)); err != nil {
		return err
	}
	if strings.Contains(strings.TrimSuffix(string(ref), "^{commit}"), "^{") {
		return fmt.Errorf("%w: ref %q has a peel suffix other than ^{commit}", backend.ErrInvalidArgument, ref)
	}
	if strings.Contains(string(ref), ":") {
		return fmt.Errorf("%w: ref %q is a tree-ish expression, not a commit", backend.ErrInvalidArgument, ref)
	}
	return nil
}

// validRefName implements `git check-ref-format`. oneLevel allows a single component, which
// only pseudo-refs such as FETCH_HEAD are. Besides matching git, it keeps ".." and absolute
// components out of the file paths go-git builds from a ref name.
func validRefName(name string, oneLevel bool) bool {
	if name == "" || name == "@" || strings.HasSuffix(name, ".") || strings.Contains(name, "..") ||
		strings.Contains(name, "@{") || (!oneLevel && !strings.Contains(name, "/")) {
		return false
	}
	for _, comp := range strings.Split(name, "/") {
		if comp == "" || strings.HasPrefix(comp, ".") || strings.HasSuffix(comp, ".lock") {
			return false
		}
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(" ~^:?*[\\", r) {
			return false
		}
	}
	return true
}

// isPseudoRefName is git's refname_is_safe test for names outside refs/: upper-case letters and
// underscores only (HEAD, ORIG_HEAD, FETCH_HEAD, ...).
func isPseudoRefName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r < 'A' || r > 'Z') && r != '_' {
			return false
		}
	}
	return true
}

// revParseRules are git's ref_rev_parse_rules: the names a short ref expands to, in order.
var revParseRules = []string{"%s", "refs/%s", "refs/tags/%s", "refs/heads/%s", "refs/remotes/%s", "refs/remotes/%s/HEAD"} //nolint:gochecknoglobals read-only

func applyRule(rule, short string) string { return strings.Replace(rule, "%s", short, 1) }

// shortenUnambiguous is git's shorten_unambiguous_ref in strict mode (core.warnAmbiguousRefs,
// default true): the shortest name that no other rule also resolves, so a branch and a tag both
// called main shorten to "heads/main". exists says whether a full ref name resolves.
//
// git matches the last rule (refs/remotes/%s/HEAD) with sscanf, whose greedy %s swallows the
// whole suffix, so that rule's short name always equals the ref's own remote name and is always
// rejected as ambiguous with itself; it is therefore never tried here.
func shortenUnambiguous(refname string, exists func(string) (bool, error)) (string, error) {
	for i := len(revParseRules) - 2; i > 0; i-- {
		prefix, _, _ := strings.Cut(revParseRules[i], "%s")
		if !strings.HasPrefix(refname, prefix) || len(refname) == len(prefix) {
			continue
		}
		short := refname[len(prefix):]
		ambiguous := false
		for j, rule := range revParseRules {
			if j == i {
				continue
			}
			ok, err := exists(applyRule(rule, short))
			if err != nil {
				return "", err
			}
			if ok {
				ambiguous = true
				break
			}
		}
		if !ambiguous {
			return short, nil
		}
	}
	return refname, nil
}
