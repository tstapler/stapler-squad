package gitoracle

import (
	"context"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// ReadOps are the read operations of the refs and diffstatus cohorts, ready for AssertParity.
// A cohort test may use all of them or pick the ones it implements.
func ReadOps() []Op {
	read := func(name string, run func(ctx context.Context, b backend.Backend, loc backend.Local) (any, error)) Op {
		return Op{Name: name, Run: run}
	}
	return []Op{
		read("CurrentBranch", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.CurrentBranch(ctx, l)
		}),
		read("HeadRef", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) { return b.HeadRef(ctx, l) }),
		read("ResolveRef(HEAD)", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.ResolveRef(ctx, l, "HEAD")
		}),
		read("ResolveRef(missing)", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.ResolveRef(ctx, l, "no-such-branch")
		}),
		read("RefExists(main)", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.RefExists(ctx, l, "refs/heads/main")
		}),
		read("RepoRoot", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) { return b.RepoRoot(ctx, l) }),
		read("GitDir", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) { return b.GitDir(ctx, l) }),
		read("CommonDir", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) { return b.CommonDir(ctx, l) }),
		read("ListRefs", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.ListRefs(ctx, l, backend.ListRefsRequest{})
		}),
		read("ListBranches", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.ListBranches(ctx, l, backend.ListBranchesRequest{})
		}),
		read("IsDirty", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.IsDirty(ctx, l, backend.IntentDisplay)
		}),
		read("Status", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.Status(ctx, l, backend.IntentDisplay)
		}),
		read("ListUntracked", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.ListUntracked(ctx, l)
		}),
		read("ListWorktrees", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.ListWorktrees(ctx, l)
		}),
	}
}

// CreateBranchOp is a mutating example op: it cuts branch "oracle-new" from HEAD in the main
// worktree. Mutating ops get a fresh repository per side and a repository-state and fsck
// comparison, so each place costs a rebuild; this one stays in "main".
func CreateBranchOp() Op {
	return Op{Name: "CreateBranch", Mutating: true, Places: []string{"main"}, Run: func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
		return nil, b.CreateBranch(ctx, l, backend.CreateBranchRequest{Name: "oracle-new", Base: "HEAD"})
	}}
}
