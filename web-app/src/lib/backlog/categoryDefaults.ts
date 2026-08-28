// categoryDefaults.ts — hardcoded backlog item category taxonomy and the
// per-category automation-toggle defaults BacklogItemForm.tsx applies once,
// at category-selection time, in create mode only (docs/tasks/backlog-
// feature-improvement.md, 07-27/07-28 updates, recommended action #5).
//
// This is deliberately a small, fixed enum rather than an extensible CRUD
// settings page (unlike PipelineMode) — see
// the `interface-pollution-checklist` skill: a 4-value taxonomy with no
// evidence of a need for custom categories doesn't warrant a
// web-app/src/app/settings/categories/ CRUD page. The server only persists
// and validates the category string itself (session.IsValidBacklogCategory,
// session/domain/backlog.go); it never resolves or applies these defaults —
// applying them is purely a client-side "apply template" action that the
// user is always free to override afterward.

/** A single selectable category option for the category picker. */
export interface BacklogCategoryOption {
  value: string;
  label: string;
  description: string;
}

/** The set of automation-toggle values a category's defaults apply. */
export interface BacklogCategoryDefaults {
  skipReviewGate: boolean;
  skipPlanning: boolean;
  autoSpawnSession: boolean;
  autoCreatePR: boolean;
  /** Pipeline mode slug, or "" for the built-in default. */
  pipelineMode: string;
}

/**
 * Slug of the seeded SDD pipeline mode (session.DefaultSDDPipelineModeSlug on
 * the backend, EnsureDefaultSDDPipelineMode in session/pipeline_mode_seed.go)
 * — seeded unconditionally at boot whenever ent storage is available, so this
 * slug is safe to reference here even though it isn't guaranteed to resolve
 * to an *enabled* mode (an operator could disable or delete it later; the
 * form's existing "unknown mode" handling covers that case the same way it
 * already does for a manually-entered stale pipelineMode).
 */
const SDD_PIPELINE_MODE_SLUG = "sdd";

/** The 4-value backlog category taxonomy — keep in sync with
 * session/domain/backlog.go's BacklogCategory constants. */
export const BACKLOG_CATEGORIES: BacklogCategoryOption[] = [
  {
    value: "bugfix",
    label: "Bugfix",
    description: "Well-scoped fix — skip formal planning and spawn immediately.",
  },
  {
    value: "feature",
    label: "Feature",
    description: "New capability — full rigor, run the SDD pipeline.",
  },
  {
    value: "chore",
    label: "Chore",
    description: "Low-risk maintenance — fully automated end to end.",
  },
  {
    value: "refactor",
    label: "Refactor",
    description: "Needs human judgment — no shortcuts.",
  },
];

/**
 * Per-category automation-toggle defaults, keyed by BACKLOG_CATEGORIES'
 * value. Applied as a one-time "apply template" action — see
 * BacklogItemForm.tsx's handleCategoryChange — not a live binding; every
 * field stays fully editable afterward and a manual override always wins.
 */
export const CATEGORY_DEFAULTS: Record<string, BacklogCategoryDefaults> = {
  bugfix: {
    // Fast path — bugs are usually well-scoped, skip formal planning and
    // spawn immediately.
    autoSpawnSession: true,
    skipPlanning: true,
    skipReviewGate: false,
    autoCreatePR: false,
    pipelineMode: "",
  },
  feature: {
    // Full rigor — use the SDD pipeline mode.
    autoSpawnSession: false,
    skipPlanning: false,
    skipReviewGate: false,
    autoCreatePR: false,
    pipelineMode: SDD_PIPELINE_MODE_SLUG,
  },
  chore: {
    // Low-risk, fully automated.
    autoSpawnSession: true,
    skipPlanning: true,
    skipReviewGate: true,
    autoCreatePR: true,
    pipelineMode: "",
  },
  refactor: {
    // Needs human judgment, no shortcuts.
    autoSpawnSession: false,
    skipPlanning: false,
    skipReviewGate: false,
    autoCreatePR: false,
    pipelineMode: "",
  },
};
