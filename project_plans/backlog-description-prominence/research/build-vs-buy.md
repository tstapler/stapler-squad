# Build vs Buy

**Verdict: Build.**

Flipping `defaultExpanded` to `true` for `DescriptionSection` and threading the prop through is a one-line default-value change plus prop-plumbing already used identically by 8+ sibling components in `web-app/src/components/backlog/detail/` (e.g. `ReviewingSection`, `ProgressHistorySection`, `PlanArtifactsSection`, `PullRequestSection`, `SessionsSection`, `VersionControlSection`, `NotesSection`, `LastReviewResultSection`, `WorkflowHistorySection`) — all built on the project's existing `Collapsible` component (`web-app/src/components/ui/Collapsible.tsx`) backed by `@radix-ui/react-accordion` (already a `web-app/package.json` dependency, `^1.2.17`). No new dependency, library, or SaaS evaluation applies here.
