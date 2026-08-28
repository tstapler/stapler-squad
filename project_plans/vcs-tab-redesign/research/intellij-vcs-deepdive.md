# IntelliJ/JetBrains VCS Tooling Deep Dive

Research for `vcs-tab-redesign`, SDD Phase 2. Scope reminder: our VCS tab is a **read-mostly
status/summary panel** for one session's git/PR state — no staging, committing, rebasing, or
conflict-resolution UI. Anything in JetBrains' tooling that requires a mutating action is noted
as out of scope, but the *display* pattern around it is still evaluated, since status display
and mutating-action UI are often fused in JetBrains' design and need to be pulled apart for us.

Current component for context: `web-app/src/components/shared/VcsWidget.tsx` — a single
`MergeabilityPill` (top-precedence, one reason shown) plus header, file list, and commit list.
Item 6 below maps directly onto replacing that pill with an all-reasons rollup; item 5 maps onto
our ahead/behind-vs-base requirement (item 7 in scope).

---

## 1. The Commit tool window

**JetBrains pattern.** The Commit tool window (`Alt+0`) is non-modal and docked, not a blocking
dialog — this is a relatively recent (2023.1-era) default, and IntelliJ still lets you switch
back to the old modal "Commit Changes" dialog via *Settings | Version Control | Commit |
uncheck "Use non-modal commit interface"*. Layout is a **changes tree on the left, diff preview
pane on the right** — selecting a file in the tree immediately renders its diff in the adjacent
pane (or a double-click / `Ctrl+D` / `F7`/"next change" opens/advances it), so review is a
single continuous scroll through tree + diff rather than a modal-per-file flow. Within a file's
diff, individual **hunks carry their own checkboxes** — a partial/hunk-level commit, not a
flat per-file stage/unstage toggle like plain git's index model.

Readiness is enforced through **Commit Checks**, opened from the gear icon (`Ctrl+O`) next to
the Commit button: "Reformat code," "Rearrange code," "Optimize imports," "Cleanup," "Update
copyright," "Check malicious dependencies," plus "Advanced" checks — run static analysis
(configurable inspection profile), TODO-comment scanning, and even a run configuration (tests)
— all gated *before* the commit completes. A commit message field with recent-message history
and template support (`git config --local commit.template <file>`) sits above the tree.

Sources:
- [Commit and push changes to Git repository](https://www.jetbrains.com/help/idea/commit-and-push-changes.html)
- [Commit Changes dialog](https://www.jetbrains.com/help/idea/commit-changes-dialog.html)
- [Running commit checks before commit](https://intellij-support.jetbrains.com/hc/en-us/community/posts/18301393941778-Running-commit-checks-before-commit)

**Recommendation for our tab.** The specific mutating pieces (hunk checkboxes, commit checks,
message templates) are out of scope — we don't stage or commit. But the **tree-plus-adjacent-
detail pairing** is directly reusable as an interaction pattern for our file list: clicking a
changed file in our (already-present) `VcsWidgetFileList` should update an adjacent diff/detail
pane in place rather than navigating away, mirroring the "select in tree → render on the right"
model, instead of our current `onNavigateToFile`/`onViewDiff` pattern that jumps to a separate
Diff tab. Given we already have a dedicated Diff tab per the task brief, treat this as a
**nice-to-have inline preview for the file list, not a requirement** — don't duplicate the full
diff experience in the status panel. The "is this commit ready" checklist concept (readiness
gating before an action) is the strongest transferable idea, but it belongs to the **mergeability
rollup** (item 6 below), not to a commit flow we don't have.

---

## 2. The Log tab / Git Log

**JetBrains pattern.** The Log tab uses a **four-region layout** (the task brief called it
three-pane; JetBrains' own docs describe Branches | Commits | Changed Files, with Commit Details
stacked below Changed Files): a **Branches pane** (left) listing local/remote branches and
favorites, a **Commits pane** (center, largest) rendering the commit graph, a **Changed Files
pane** (upper right) listing files touched by the selected commit, and a **Commit Details pane**
(lower right) showing message, hash, author (with email link), date/time, GPG signature status,
root, and branches.

Branch/tag decoration is rendered as **colored labels directly on graph nodes**: yellow = current
branch head, green = local branches, violet = remote branches. Commits on the current branch get
a light-blue row background (others: white), and commits authored by the current user render in
**bold** — an at-a-glance "is this mine / is this where HEAD is" cue with zero extra chrome.
Multi-repo projects add a colored left-edge stripe per repository root.

Filtering is compositional, not a single search box: **by branch** (including `-<branch>` glob
exclusion), **by user**, **by date/range**, **by path** (folder-scoped), and **by free text**
(matches message, hash, or regex) — all as independent toolbar filter chips that combine.
Clicking a commit populates Changed Files and Commit Details instantly (no navigation); a
`Ctrl+F` "Go to Hash/Branch/Tag" jumps directly to any ref.

Sources:
- [Log tab](https://www.jetbrains.com/help/idea/log-tab.html)
- [Investigate changes in Git repository](https://www.jetbrains.com/help/idea/investigate-changes.html)

**Recommendation for our tab.** We are not building a graph view — a single-session status
panel has no branch-topology question to answer (there's exactly one branch of interest: this
session's). The transferable pieces are narrower but real:
- **Decorate, don't caption.** JetBrains encodes "current branch," "mine," and "where HEAD is"
  as color/weight on the row itself rather than a text label ("You are here"). Apply the same
  idea to our `VcsWidgetCommitList`: bold or accent the HEAD commit, rather than (or in addition
  to) a separate caption row — cheaper to scan on mobile than reading each row's text.
- **Click-to-populate-adjacent-detail**, same as item 1: clicking a commit row should show its
  file list / stats inline or in an adjacent region, not navigate away, if we ever add a
  commit-detail affordance to the commit list.
- Multi-axis filtering (branch/user/date/path) does **not** translate — there's only one branch
  and effectively one author-of-interest (the agent) in scope for a single-session panel. Skip.

---

## 3. Local Changes vs. Log — the pending/immutable split

**JetBrains pattern.** JetBrains keeps these as genuinely separate UI surfaces, not just a
filter on one timeline. **Local Changes** (now its own top-level tab, split out of the old
combined Git tool window in recent versions) shows only uncommitted, mutable state — edits,
adds, deletes in the working tree — organized into **changelists**: named, user-created groups
of uncommitted changes (e.g. one changelist per task), with exactly one **active changelist**
that is what the next commit will include. Changelists can carry an associated IDE "context"
(open files, breakpoints) that gets frozen/restored on activation. The **Log tab**, by contrast,
shows only what's already committed — immutable history, one unified graph across all branches.
The **Incoming tab** is a third surface: changes that exist on the remote but aren't fetched/
merged locally yet, shown as its own changelist-shaped list (with the caveat, confirmed against
the docs, that JetBrains' own help page doesn't spell out whether population requires an
explicit fetch first — treat that as unverified rather than assumed).

Sources:
- [Group changes into changelists](https://www.jetbrains.com/help/idea/managing-changelists.html)
- [Log tab](https://www.jetbrains.com/help/idea/log-tab.html)
- [Repository and Incoming tabs](https://www.jetbrains.com/help/idea/version-control-tool-window-repository-and-incoming-tabs.html)
- [The "Local Changes" and "Stash" tabs have been separated from the Git window](https://intellij-support.jetbrains.com/hc/en-us/community/posts/22614979848594-The-Local-Changes-and-Stash-tabs-have-been-separated-from-the-Git-window)

**Recommendation — does our single-panel status view need the same split?**
No, and this is a considered "don't adopt," not an oversight. JetBrains' split exists because
Local Changes is a **staging area you actively curate** (multiple changelists, one active,
task-scoped grouping) — that's inherently a multi-step authoring workflow, which is explicitly
out of scope for us. Our panel has no staging concept: everything the agent has touched in the
working tree *is* "the change," full stop, and there's no competing "which changelist is
active" question. A **merged timeline is the right model for us**: one chronological list, top
to bottom, uncommitted work at the top (visually distinguished — e.g. an "Uncommitted" or
"Working tree" heading/badge) flowing into already-committed commits below, all in the single
`VcsWidgetFileList`/`VcsWidgetCommitList` pairing we already have. The one thing worth
borrowing from the split is the **visual distinction, not a structural one**: uncommitted rows
should look different (different badge/weight, e.g. "not yet committed") from committed rows in
the same list, the way JetBrains uses row color/weight in the Log rather than a fully separate
component, so a viewer can tell "pending" from "done" at a glance without a second surface to
check. The Incoming-tab concept (remote-ahead work not yet local) is a genuinely separate
question — see item 5 — and is worth its own indicator, but that's an ahead/behind-count problem,
not a reason to fork our single timeline into two panels.

---

## 4. Annotate/blame gutter

**JetBrains pattern.** Invoked via right-click in the editor gutter → "Annotate with Git Blame"
(assignable to a shortcut). Once active, **every line** in the editor gets an inline annotation
strip showing (configurable via right-click → View): revision id, commit date, author name
(full/first/last/initials/email — separately configurable), with lines from the most recent
revision bolded and asterisked, and each distinct commit given its own background color so
same-commit line ranges are visually grouped without re-reading each line. **Hovering** an
annotation pops up full commit details (message, files changed, link to remote host). **Clicking**
an annotation jumps straight into the Log tab, pre-focused on that exact commit.

Sources:
- [Investigate changes in Git repository](https://www.jetbrains.com/help/idea/investigate-changes.html)
- [Annotate with Git Blame: Commit — JetBrains Guide](https://www.jetbrains.com/guide/java/tips/annotate-git-commit/)

**Recommendation.** Out of scope as a feature — we don't render source with a line gutter in
this panel, and per-line blame is squarely the Diff tab's territory, not the status panel's.
The interaction pattern worth carrying over, though, is the **hover-for-summary /
click-for-full-detail two-tier disclosure**: cheap information (author, date, one-line summary)
on hover/tap, full detail only on a deliberate click. That maps well to our **commit list rows**
(`VcsWidgetCommitList`): show author + relative date + subject line by default, and use the same
"hover shows more, click/tap opens full detail" tiering there rather than always rendering full
detail inline — keeps rows compact on mobile while still one interaction away from full context.
Note the color-per-commit grouping trick doesn't apply to us since we don't render arbitrary
source lines.

---

## 5. Update Project / incoming changes indicator

**JetBrains pattern.** The **VCS widget** in the status bar shows the current branch plus a blue
arrow icon annotated with the count of incoming (fetched-but-not-merged, or not-yet-fetched,
depending on version) commits, and a green arrow with the outgoing (local-ahead) commit count —
both counts visible without opening any panel. The dedicated **Incoming tab** (Repository tool
window) lists those incoming changes as a changelist-shaped list a user can review before
running **Update Project** (`Ctrl+T` / `Git | Update Project`), which performs the actual
fetch+merge/rebase.

Sources:
- [Sync with a remote Git repository (fetch, pull, update)](https://www.jetbrains.com/help/idea/sync-with-a-remote-repository.html)
- [Repository and Incoming tabs](https://www.jetbrains.com/help/idea/version-control-tool-window-repository-and-incoming-tabs.html)
- [User interface](https://www.jetbrains.com/help/idea/guided-tour-around-the-user-interface.html)

**Recommendation.** This maps directly and cleanly onto our existing "ahead/behind vs. base"
scope item. Adopt the **paired-arrow-with-count** idiom at the header/status level (already
close to what an ahead/behind badge naturally looks like): e.g. `↓3 ↑2` for 3 commits behind
base / 2 ahead, always visible without a click, exactly like JetBrains' status-bar widget rather
than buried in a submenu. Do **not** adopt the Incoming tab itself (a reviewable list of
not-yet-merged upstream commits) — that requires an Update/merge *action* to resolve, which is
out of scope; we're a display of the ahead/behind fact, not a tool for acting on it. If we ever
want to let a viewer see *what* the behind-commits are (not just the count), that's a Diff-tab
or Log-style drill-down, not something to cram into the summary panel itself — surface the
count here, link out for detail, the same separation JetBrains draws between the widget (count)
and the Incoming tab (content).

---

## 6. Surfacing "why can't I merge/push"

**JetBrains pattern.** JetBrains does not centralize this into one rollup panel — it's
distributed across several targeted, situational UI moments, each with distinct copy and
visual treatment:
- **Diverged branches**: the VCS widget itself displays a **"Branches have diverged"** warning
  state when local and remote (or two repo roots) have independently moved forward — this is a
  named, specific message, not a generic "cannot push."
- **Rebase-needed / update-required**: surfaced procedurally, through the *Update Project*
  action's own dialog offering merge vs. rebase strategy choice, rather than as a persistent
  static badge.
- **Merge conflicts**: on a Git operation that produces conflicts, a **"Merge Conflicts" node
  appears directly in the Changes tree** of the Commit tool window with an inline link to
  resolve — conflict state is shown *where the files already live* in the UI, not as a separate
  banner.
- **Push rejected (non-fast-forward)**: surfaced as a notification/balloon at the point of the
  attempted push, not as a standing indicator (JetBrains' own docs don't describe a persistent
  "you will be rejected on push" precondition badge — this is reactive, discovered only when you
  try).

Sources:
- [Manage Git branches](https://www.jetbrains.com/help/idea/manage-branches.html)
- [Resolve Git conflicts](https://www.jetbrains.com/help/idea/resolve-conflicts.html)
- [IDEA says "Branches have diverged" after update — YouTrack IDEA-227366](https://youtrack.jetbrains.com/issue/IDEA-227366)

**Recommendation.** This is where JetBrains' model is the *weakest* fit and the task brief's
instinct — replace the single top-precedence pill with a GitLab-style **all-reasons rollup** —
is the right call, not a JetBrains-style scattered/reactive approach. JetBrains can afford
scattered, situational messaging because it's an interactive client where the user is one click
from *fixing* each condition in place (resolve conflicts right in the Changes tree, choose a
rebase strategy in the Update dialog). We have no such affordance — our panel is read-only, so
a viewer can't act on a conflict node the way an IntelliJ user can. That means every blocking
condition **must** be legible from the summary alone, since there's no follow-up action screen
to discover it in. Concretely:
- Keep (and build out) the **all-reasons rollup** direction already implied by replacing the
  single `MergeabilityPill`: list every active reason (diverged, conflicts, checks failing,
  behind base, review required) simultaneously, not top-precedence-wins.
- Borrow only the **specificity of copy**, not the placement: JetBrains' "Branches have
  diverged" beats a generic "cannot merge" — each of our rollup reasons should carry a distinct,
  named label (e.g. "Diverged from base," "Merge conflicts," "Checks failing") rather than one
  collapsed status string.
- Do **not** adopt the "surface it inline where the files already live" pattern (the Changes-
  tree conflict node) as a substitute for the rollup — that pattern depends on the viewer being
  able to click through and resolve it there, which we don't support. For us, "where the
  conflicting files are" is informational at most (maybe: which files show a conflict marker in
  our file list), never a call-to-action.
- The reactive, discovered-on-attempt push-rejection pattern doesn't translate at all — we have
  no push action to fail against. Any "would fail to push" state we know about (e.g. diverged)
  must be proactively surfaced in the rollup, not deferred to a future action attempt.

---

## Summary table

| Area | Adopt | Adapt | Skip |
|---|---|---|---|
| 1. Commit tool window | — | tree→adjacent-detail pairing for file list (nice-to-have) | hunk checkboxes, commit checks, message templates (all mutating) |
| 2. Log tab | decorate HEAD/mine via color/weight, not caption | click→adjacent detail | multi-axis branch/user/date/path filtering (no graph to filter) |
| 3. Local Changes vs. Log | — | merged single timeline, visually distinguish pending vs. committed rows | separate Local-Changes-style panel/changelists |
| 4. Annotate/blame gutter | hover-summary / click-full-detail tiering on commit rows | — | in-gutter blame display itself (belongs to Diff tab) |
| 5. Update Project / incoming | `↓N ↑N` always-visible ahead/behind badge | — | Incoming-tab content list, Update action itself |
| 6. Why can't I merge/push | all-reasons rollup (already the design direction), specific named copy per reason | — | scattered/reactive placement, inline actionable conflict nodes, discovered-on-push-attempt pattern |
