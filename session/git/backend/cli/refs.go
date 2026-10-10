package cli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

func parseSHA(op backend.OperationName, s string) (backend.CommitSHA, error) {
	if !shaPattern.MatchString(s) {
		return "", fmt.Errorf("git %s: unexpected output %q", op, s)
	}
	return backend.CommitSHA(s), nil
}

// isHEAD reports whether ref names the current HEAD, the only ref that can be unborn.
func isHEAD(ref backend.RefName) bool { return ref == "HEAD" || ref == "@" }

// commitish peels ref to a commit unless the caller already wrote a peel expression.
// commitRef validates a revision for the commit-only methods: no option lookalikes and no
// "rev:path" tree-ish expressions, which name trees and blobs, not commits.
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

func commitish(ref backend.RefName) string {
	if strings.HasSuffix(string(ref), "^{commit}") {
		return string(ref)
	}
	return string(ref) + "^{commit}"
}

// unbornOr maps a failed HEAD lookup to ErrUnborn when HEAD is a symbolic ref (the branch just
// has no commit), otherwise returns fallback. rev-parse alone cannot tell an unborn repository
// from a dropped connection, which is why the symbolic-ref probe exists.
func (b *Backend) unbornOr(ctx context.Context, loc backend.RepoLocation, fallback error) error {
	if _, err := b.git(ctx, loc, backend.OpHeadRef, "symbolic-ref", "-q", "HEAD"); err == nil {
		// Wrap only the command error: fallback may carry ErrRefNotFound ("unknown revision"),
		// which must not also be true for an unborn HEAD.
		var cerr *backend.CommandError
		if errors.As(fallback, &cerr) {
			return fmt.Errorf("%w: %w", backend.ErrUnborn, cerr)
		}
	}
	return fallback
}

func (b *Backend) CurrentBranch(ctx context.Context, loc backend.RepoLocation) (backend.BranchName, error) {
	res, err := b.git(ctx, loc, backend.OpCurrentBranch, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		if errors.Is(err, backend.ErrNotARepo) || errors.Is(err, backend.ErrNoLocalRunner) || errors.Is(err, backend.ErrNoRemoteRunner) {
			return "", err
		}
		return "", b.unbornOr(ctx, loc, err)
	}
	name := lastLine(res)
	switch name {
	case "":
		return "", fmt.Errorf("git %s: empty output", backend.OpCurrentBranch)
	case "HEAD":
		return "", backend.ErrDetachedHead
	}
	return backend.BranchName(name), nil
}

func (b *Backend) HeadRef(ctx context.Context, loc backend.RepoLocation) (backend.RefName, error) {
	res, err := b.git(ctx, loc, backend.OpHeadRef, "symbolic-ref", "-q", "HEAD")
	if err != nil {
		if exitCode(err) == 1 {
			return "", backend.ErrDetachedHead
		}
		return "", err
	}
	return backend.RefName(lastLine(res)), nil
}

func (b *Backend) ResolveRef(ctx context.Context, loc backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error) {
	if err := commitRef(ref); err != nil {
		return "", err
	}
	res, err := b.git(ctx, loc, backend.OpResolveRef, "rev-parse", "--verify", commitish(ref))
	if err != nil {
		if isHEAD(ref) && !errors.Is(err, backend.ErrObjectNotFound) {
			return "", b.unbornOr(ctx, loc, err)
		}
		return "", err
	}
	return parseSHA(backend.OpResolveRef, lastLine(res))
}

func (b *Backend) RefExists(ctx context.Context, loc backend.RepoLocation, ref backend.RefName) (bool, error) {
	if err := commitRef(ref); err != nil {
		return false, err
	}
	_, err := b.git(ctx, loc, backend.OpRefExists, "rev-parse", "--verify", "--quiet", commitish(ref))
	if err == nil {
		return true, nil
	}
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

func (b *Backend) RepoRoot(ctx context.Context, loc backend.RepoLocation) (backend.RepoRoot, error) {
	res, err := b.git(ctx, loc, backend.OpRepoRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return backend.RepoRoot(lastLine(res)), nil
}

func (b *Backend) GitDir(ctx context.Context, loc backend.RepoLocation) (backend.GitDir, error) {
	res, err := b.git(ctx, loc, backend.OpGitDir, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return "", err
	}
	return backend.GitDir(lastLine(res)), nil
}

func (b *Backend) CommonDir(ctx context.Context, loc backend.RepoLocation) (backend.CommonDir, error) {
	res, err := b.git(ctx, loc, backend.OpCommonDir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return backend.CommonDir(lastLine(res)), nil
}

func (b *Backend) ListRefs(ctx context.Context, loc backend.RepoLocation, req backend.ListRefsRequest) ([]backend.RefName, error) {
	args := []string{"for-each-ref", "--format=%(refname)"}
	if req.Limit > 0 {
		args = append(args, "--count="+strconv.Itoa(req.Limit))
	}
	if req.Pattern != "" {
		if err := optArg("pattern", string(req.Pattern)); err != nil {
			return nil, err
		}
		args = append(args, string(req.Pattern))
	}
	res, err := b.git(ctx, loc, backend.OpListRefs, args...)
	if err != nil {
		return nil, err
	}
	var refs []backend.RefName
	for _, ln := range nonEmptyLines(res.text()) {
		refs = append(refs, backend.RefName(ln))
	}
	return refs, nil
}

func (b *Backend) MergeBase(ctx context.Context, loc backend.RepoLocation, req backend.MergeBaseRequest) (backend.CommitSHA, error) {
	if err := optArg("left", string(req.Left)); err != nil {
		return "", err
	}
	if err := optArg("right", string(req.Right)); err != nil {
		return "", err
	}
	res, err := b.git(ctx, loc, backend.OpMergeBase, "merge-base", string(req.Left), string(req.Right))
	if err != nil {
		if exitCode(err) == 1 {
			return "", fmt.Errorf("%w: %w", backend.ErrNoMergeBase, err)
		}
		return "", err
	}
	return parseSHA(backend.OpMergeBase, lastLine(res))
}

func rangeArg(rng backend.RangeSpec) string {
	switch {
	case rng.Exclude != "" && rng.Include != "":
		return string(rng.Exclude) + ".." + string(rng.Include)
	case rng.Exclude != "":
		return string(rng.Exclude) + "..HEAD"
	case rng.Include != "":
		return string(rng.Include)
	}
	return "HEAD"
}

func checkRange(rng backend.RangeSpec) error {
	if rng.Exclude != "" {
		if err := optArg("exclude", string(rng.Exclude)); err != nil {
			return err
		}
	}
	if rng.Include != "" {
		return optArg("include", string(rng.Include))
	}
	return nil
}

func (b *Backend) CountCommits(ctx context.Context, loc backend.RepoLocation, rng backend.RangeSpec) (int, error) {
	if err := checkRange(rng); err != nil {
		return 0, err
	}
	res, err := b.git(ctx, loc, backend.OpCountCommits, "rev-list", "--count", rangeArg(rng))
	if err != nil {
		return 0, err
	}
	n, convErr := strconv.Atoi(lastLine(res))
	if convErr != nil {
		return 0, fmt.Errorf("git %s: unexpected output %q", backend.OpCountCommits, lastLine(res))
	}
	return n, nil
}

const logSep = "\x1f"

func (b *Backend) Log(ctx context.Context, loc backend.RepoLocation, req backend.LogRequest) ([]backend.LogEntry, error) {
	if err := checkRange(req.Range); err != nil {
		return nil, err
	}
	args := []string{"log", "--format=%H%x1f%s"}
	if req.Limit > 0 {
		args = append(args, "-n", strconv.Itoa(req.Limit))
	}
	args = append(args, rangeArg(req.Range))
	res, err := b.git(ctx, loc, backend.OpLog, args...)
	if err != nil {
		return nil, err
	}
	var entries []backend.LogEntry
	for _, ln := range nonEmptyLines(res.text()) {
		sha, subject, _ := strings.Cut(ln, logSep)
		entries = append(entries, backend.LogEntry{SHA: backend.CommitSHA(sha), Subject: subject})
	}
	return entries, nil
}

func (b *Backend) GetConfig(ctx context.Context, loc backend.RepoLocation, key backend.ConfigKey) (string, error) {
	if err := optArg("key", string(key)); err != nil {
		return "", err
	}
	res, err := b.git(ctx, loc, backend.OpGetConfig, "config", "--get", string(key))
	if err != nil {
		if exitCode(err) == 1 {
			return "", fmt.Errorf("%w: %s", backend.ErrConfigUnset, key)
		}
		return "", err
	}
	return lastLine(res), nil
}

func (b *Backend) SetConfig(ctx context.Context, loc backend.RepoLocation, req backend.SetConfigRequest) error {
	if err := optArg("key", string(req.Key)); err != nil {
		return err
	}
	// "--" ends option parsing so values such as -1 (core.compression) pass through verbatim.
	_, err := b.git(ctx, loc, backend.OpSetConfig, "config", "--", string(req.Key), req.Value)
	return err
}

func (b *Backend) SetRemoteURL(ctx context.Context, loc backend.RepoLocation, req backend.SetRemoteURLRequest) error {
	if err := optArg("remote", string(req.Remote)); err != nil {
		return err
	}
	if err := optArg("url", string(req.URL)); err != nil {
		return err
	}
	_, err := b.git(ctx, loc, backend.OpSetRemoteURL, "remote", "set-url", string(req.Remote), string(req.URL))
	return err
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	return out
}
