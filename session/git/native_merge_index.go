package git

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// NewConflictEntries constructs the stage-1/2/3 index.Entry values for one conflicted
// path (Story 3.3.2, ConflictEntry in plan.md's Domain Glossary) — the sole construction
// point for conflicted index entries, so the go-git/git stage semantics below are never
// duplicated at a call site.
//
// A zero hash for baseHash/oursHash/theirsHash is skipped rather than turned into an
// entry: an add/add conflict has no ancestor (base), and a delete/modify conflict is
// missing whichever side deleted the path (stack.md §3's "a pure add/delete conflict
// may have only 2 of the 3 stages" finding).
//
// Never construct a conflicted entry with index.Merged: go-git's own doc comment claims
// it means "the default stage, fully merged" (i.e. real git's stage 0), but its actual
// value is 1 — identical to AncestorMode (architecture-review.md's landmine #2).
// Unconflicted entries must keep their existing Stage(0) value untouched; only
// AncestorMode/OurMode/TheirMode name the three conflict stages.
//
// This project uses []*index.Entry (matching index.Index.Entries' own field type, and
// what the encoder/sortConflictEntries actually operate on) rather than plan.md's
// prose-level "[]index.Entry" — a deliberate, no-op-for-correctness deviation that
// avoids a value/pointer conversion at every call site.
func NewConflictEntries(name string, baseHash, oursHash, theirsHash plumbing.Hash, mode filemode.FileMode) []*index.Entry {
	var entries []*index.Entry
	add := func(stage index.Stage, hash plumbing.Hash) {
		if hash == plumbing.ZeroHash {
			return
		}
		entries = append(entries, &index.Entry{
			Name:  name,
			Hash:  hash,
			Mode:  mode,
			Stage: stage,
		})
	}
	add(index.AncestorMode, baseHash)
	add(index.OurMode, oursHash)
	add(index.TheirMode, theirsHash)
	return entries
}

// sortConflictEntries sorts entries by (Name, Stage) ascending — the order real git's
// index format requires. go-git's own encoder sorts by Name only
// (plumbing/format/index/encoder.go's byName), which is not sufficient for a conflicted
// path's multiple same-Name entries (research/stack.md §3); every conflicted-index write
// in this package must call this before encoding, no exceptions.
func sortConflictEntries(entries []*index.Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Stage < entries[j].Stage
	})
}

// writeConflictedIndex writes conflictEntries (stage-1/2/3 entries for one or more
// conflicted paths, from NewConflictEntries) into worktreePath's index, replacing
// whatever stage-0 entry currently exists for each of those paths (Task 3.3.2c).
func writeConflictedIndex(worktreePath string, conflictEntries []*index.Entry) error {
	touched := make(map[string]bool, len(conflictEntries))
	for _, e := range conflictEntries {
		touched[e.Name] = true
	}
	return writeIndexEntries(worktreePath, touched, conflictEntries)
}

// writeIndexEntries is the single write path every conflicted-index mutation in this
// package funnels through (writeConflictedIndex, and abortNativeMerge's index-restore
// step in native_merge.go — its "sibling clean-write path"): load the existing index,
// drop whatever entries exist for each of touchedPaths, append newEntries, pre-sort the
// full list via sortConflictEntries, and atomically write the result back through
// AdminFileWriter rather than go-git's own non-atomic IndexStorage.SetIndex (ADR-001).
func writeIndexEntries(worktreePath string, touchedPaths map[string]bool, newEntries []*index.Entry) error {
	repo, err := openWorktreeRepo(worktreePath)
	if err != nil {
		return fmt.Errorf("writeIndexEntries: open repo: %w", err)
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		return fmt.Errorf("writeIndexEntries: load index: %w", err)
	}

	kept := idx.Entries[:0]
	for _, e := range idx.Entries {
		if !touchedPaths[e.Name] {
			kept = append(kept, e)
		}
	}
	idx.Entries = append(kept, newEntries...)
	sortConflictEntries(idx.Entries)

	return encodeAndWriteIndex(worktreePath, idx)
}

// encodeAndWriteIndex encodes idx via index.NewEncoder and writes it atomically through
// AdminFileWriter at resolveWorktreeIndexPath(worktreePath) — the shared final step for
// every index write in this package.
func encodeAndWriteIndex(worktreePath string, idx *index.Index) error {
	if idx.Version == 0 {
		idx.Version = 2
	}

	var buf bytes.Buffer
	if err := index.NewEncoder(&buf).Encode(idx); err != nil {
		return fmt.Errorf("encodeAndWriteIndex: encode: %w", err)
	}

	indexPath, err := resolveWorktreeIndexPath(worktreePath)
	if err != nil {
		return fmt.Errorf("encodeAndWriteIndex: %w", err)
	}

	w := NewAdminFileWriter(filepath.Dir(indexPath))
	if err := w.WriteFile(filepath.Base(indexPath), buf.Bytes()); err != nil {
		return fmt.Errorf("encodeAndWriteIndex: write: %w", err)
	}
	return nil
}
