package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// IsDirty ignores intent: the CLI is the authoritative answer for both Display and Destructive.
func (b *Backend) IsDirty(ctx context.Context, loc backend.RepoLocation, _ backend.Intent) (bool, error) {
	res, err := b.git(ctx, loc, backend.OpIsDirty, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return res.text() != "", nil
}

func (b *Backend) Status(ctx context.Context, loc backend.RepoLocation, _ backend.Intent) (backend.StatusResult, error) {
	res, err := b.gitZ(ctx, loc, backend.OpStatus, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all")
	if err != nil {
		return backend.StatusResult{}, err
	}
	out := res.out
	if res.combined {
		// The first record is always "# branch.oid"; anything before it is stderr banner.
		if i := strings.Index(string(out), "# branch.oid "); i > 0 {
			out = out[i:]
		}
	}
	if err := checkZ(result{out: out, combined: res.combined}, backend.OpStatus, false); err != nil {
		return backend.StatusResult{}, err
	}
	return parseStatusV2(out)
}

func (b *Backend) ListUntracked(ctx context.Context, loc backend.RepoLocation) ([]backend.RepoPath, error) {
	res, err := b.gitZ(ctx, loc, backend.OpListUntracked, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	// Records carry no marker, so a leading banner cannot be told from a file named "hint: x".
	if err := checkZ(res, backend.OpListUntracked, true); err != nil {
		return nil, err
	}
	var out []backend.RepoPath
	for _, p := range strings.Split(string(res.out), "\x00") {
		if p != "" { // names may begin or end with spaces or newlines: no trimming
			out = append(out, backend.RepoPath(p))
		}
	}
	return out, nil
}

func diffArgs(spec backend.DiffSpec, extra ...string) ([]string, error) {
	// Pinned against user config: no color escapes, external diff drivers or textconv filters.
	args := append([]string{"diff", "--no-color", "--no-ext-diff", "--no-textconv"}, extra...)
	if spec.Staged {
		args = append(args, "--cached")
	}
	if spec.Base != "" {
		if err := optArg("base", string(spec.Base)); err != nil {
			return nil, err
		}
	}
	if spec.Head != "" {
		if err := optArg("head", string(spec.Head)); err != nil {
			return nil, err
		}
	}
	switch {
	case spec.FromMergeBase && spec.Base != "":
		head := spec.Head
		if head == "" {
			head = "HEAD"
		}
		args = append(args, string(spec.Base)+"..."+string(head))
	case spec.Base != "" && spec.Head != "":
		args = append(args, string(spec.Base)+".."+string(spec.Head))
	case spec.Base != "":
		args = append(args, string(spec.Base))
	case spec.Head != "":
		args = append(args, string(spec.Head))
	}
	if len(spec.Paths) > 0 {
		args = append(args, "--")
		args = append(args, paths(spec.Paths)...)
	}
	return args, nil
}

func (b *Backend) Diff(ctx context.Context, loc backend.RepoLocation, spec backend.DiffSpec) (string, error) {
	args, err := diffArgs(spec, "--src-prefix=a/", "--dst-prefix=b/") // ignore diff.noprefix / mnemonicPrefix
	if err != nil {
		return "", err
	}
	res, err := b.git(ctx, loc, backend.OpDiff, args...)
	if err != nil {
		return "", err
	}
	return string(res.out), nil
}

func (b *Backend) DiffNumstat(ctx context.Context, loc backend.RepoLocation, spec backend.DiffSpec) ([]backend.NumstatRow, error) {
	args, err := diffArgs(spec, "--numstat", "-z")
	if err != nil {
		return nil, err
	}
	res, err := b.gitZ(ctx, loc, backend.OpDiffNumstat, args...)
	if err != nil {
		return nil, err
	}
	// A banner would corrupt the first record's counts (a loud parse error); an unterminated
	// tail is the same signal at the other end.
	if err := checkZ(res, backend.OpDiffNumstat, false); err != nil {
		return nil, err
	}
	return parseNumstatZ(res.out)
}

// parseNumstatZ parses `diff --numstat -z`: "added\tdeleted\tpath\0", or for a rename
// "added\tdeleted\t\0old\0new\0".
func parseNumstatZ(out []byte) ([]backend.NumstatRow, error) {
	recs := strings.Split(string(out), "\x00")
	var rows []backend.NumstatRow
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if rec == "" {
			continue
		}
		f := strings.SplitN(rec, "\t", 3)
		if len(f) != 3 {
			return nil, fmt.Errorf("git numstat: malformed record %q", rec)
		}
		row := backend.NumstatRow{Path: backend.RepoPath(f[2])}
		if f[0] == "-" && f[1] == "-" {
			row.Binary = true
		} else {
			var err1, err2 error
			row.Added, err1 = strconv.Atoi(f[0])
			row.Deleted, err2 = strconv.Atoi(f[1])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("git numstat: malformed counts in %q", rec)
			}
		}
		if f[2] == "" { // rename: the next two records are old and new path
			if i+2 >= len(recs) {
				return nil, fmt.Errorf("git numstat: truncated rename record %q", rec)
			}
			row.OrigPath, row.Path = backend.RepoPath(recs[i+1]), backend.RepoPath(recs[i+2])
			i += 2
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// parseStatusV2 parses `status --porcelain=v2 --branch -z`.
func parseStatusV2(out []byte) (backend.StatusResult, error) {
	var st backend.StatusResult
	recs := strings.Split(string(out), "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if rec == "" {
			continue
		}
		switch rec[0] {
		case '#':
			parseStatusHeader(&st, rec)
		case '1':
			f, err := statusFields(rec, 9)
			if err != nil {
				return st, err
			}
			st.Files = append(st.Files, fileStatus(f[1], f[2], backend.RepoPath(f[8]), ""))
		case '2':
			f, err := statusFields(rec, 10)
			if err != nil {
				return st, err
			}
			if i+1 >= len(recs) {
				return st, fmt.Errorf("git status: rename record %q has no original path", rec)
			}
			i++
			st.Files = append(st.Files, fileStatus(f[1], f[2], backend.RepoPath(f[9]), backend.RepoPath(recs[i])))
		case 'u':
			f, err := statusFields(rec, 11)
			if err != nil {
				return st, err
			}
			fs := fileStatus(f[1], f[2], backend.RepoPath(f[10]), "")
			fs.Unmerged = true
			st.Files = append(st.Files, fs)
		case '?':
			st.Files = append(st.Files, backend.FileStatus{Path: backend.RepoPath(strings.TrimPrefix(rec, "? ")), Untracked: true})
		case '!':
			st.Files = append(st.Files, backend.FileStatus{Path: backend.RepoPath(strings.TrimPrefix(rec, "! ")), Ignored: true})
		default:
			return st, fmt.Errorf("git status: unrecognised record %q", rec)
		}
	}
	return st, nil
}

func statusFields(rec string, n int) ([]string, error) {
	f := strings.SplitN(rec, " ", n)
	if len(f) != n {
		return nil, fmt.Errorf("git status: malformed record %q", rec)
	}
	return f, nil
}

func fileStatus(xy, sub string, path, orig backend.RepoPath) backend.FileStatus {
	fs := backend.FileStatus{Path: path, OrigPath: orig, SubmoduleDir: strings.HasPrefix(sub, "S")}
	if len(xy) == 2 {
		fs.Index, fs.Worktree = backend.StatusCode(xy[0]), backend.StatusCode(xy[1])
	}
	return fs
}

func parseStatusHeader(st *backend.StatusResult, rec string) {
	key, val, _ := strings.Cut(strings.TrimPrefix(rec, "# "), " ")
	switch key {
	case "branch.oid":
		if val == "(initial)" {
			st.Unborn = true
		} else {
			st.HeadOID = backend.CommitSHA(val)
		}
	case "branch.head":
		if val == "(detached)" {
			st.Detached = true
		} else {
			st.Branch = backend.BranchName(val)
		}
	case "branch.upstream":
		st.Upstream = backend.RefName(val)
	case "branch.ab":
		var ahead, behind int
		if _, err := fmt.Sscanf(val, "+%d -%d", &ahead, &behind); err == nil {
			st.Ahead, st.Behind = ahead, behind
		}
	}
}
