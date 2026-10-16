import type { BacklogItemStatus } from "@/lib/hooks/useBacklogService";
import { useBacklogStages, type BacklogStageInfo } from "@/lib/hooks/useBacklogStages";
import * as styles from "./StageTracker.css";

/** The 5 always-visible lifecycle stages (Domain Glossary: "Lifecycle Stage"). */
export type Stage = "idea" | "ready" | "in_progress" | "review" | "done";

export const STAGE_ORDER: readonly Stage[] = ["idea", "ready", "in_progress", "review", "done"];

const STAGE_LABELS: Record<Stage, string> = {
  idea: "Idea",
  ready: "Ready",
  in_progress: "In Progress",
  review: "Review",
  done: "Done",
};

export interface StageDisplay {
  activeStage: Stage;
  /** Modifier badge text (e.g. "Queued", "PR pending"), rendered on the active node only. */
  modifier?: string;
  archived: boolean;
}

/**
 * Pure derivation from item.status to the tracker's 5-node display. `queued`
 * and `pr_pending` never add a 6th node — they render as a modifier badge on
 * an existing node (In Progress / Review respectively). `refining` folds
 * into Idea. `archived` never guesses which stage the item was archived
 * from — the tracker renders as a dimmed neutral state with an "Archived"
 * ribbon overlay instead (see StageTracker's render logic below).
 *
 * `stages` (Epic 2.9's `useBacklogStages()`, defaulted to the built-in set)
 * is consulted only for a status that matches none of the fixed cases below:
 * a status matching a real, currently-configured custom stage folds into the
 * Idea node with a modifier badge naming that stage (the same "no 6th node,
 * use a modifier instead" treatment `queued`/`pr_pending` already get) rather
 * than being lumped in with genuinely unresolvable data. A status matching
 * nothing in `stages` at all (a deleted/unknown stage) keeps today's dimmed
 * "Archived"-styled fallback unchanged.
 */
export function deriveStageDisplay(
  status: BacklogItemStatus,
  stages: readonly BacklogStageInfo[] = []
): StageDisplay {
  switch (status) {
    case "idea":
    case "refining":
      return { activeStage: "idea", archived: false };
    case "ready":
      return { activeStage: "ready", archived: false };
    case "queued":
      return { activeStage: "in_progress", modifier: "Queued", archived: false };
    case "in_progress":
      return { activeStage: "in_progress", archived: false };
    case "review":
      return { activeStage: "review", archived: false };
    case "pr_pending":
      return { activeStage: "review", modifier: "PR pending", archived: false };
    case "done":
      return { activeStage: "done", archived: false };
    case "archived":
      // The pre-archive stage isn't reconstructable from status alone — the
      // caller renders a neutral/dimmed tracker with the ribbon overlay
      // rather than guessing a stage (Story 2.1.1 AC).
      return { activeStage: "idea", archived: true };
    default: {
      const customStage = stages.find((s) => s.slug === status);
      if (customStage) {
        return { activeStage: "idea", modifier: customStage.name, archived: false };
      }
      // Unknown/future server-defined status with no matching configured
      // stage either: don't crash, don't guess a stage — same defensive
      // posture as getStatusClass's archived fallback.
      return { activeStage: "idea", archived: true };
    }
  }
}

export interface StageTrackerProps {
  status: BacklogItemStatus;
}

/** Compact horizontal stepper bound strictly to item.status — always exactly 5 nodes. */
export function StageTracker({ status }: StageTrackerProps) {
  const { stages } = useBacklogStages();
  const { activeStage, modifier, archived } = deriveStageDisplay(status, stages);
  const activeIndex = STAGE_ORDER.indexOf(activeStage);

  return (
    <div
      className={styles.container}
      data-testid="stage-tracker"
      aria-label={archived ? "Lifecycle stage: Archived" : `Lifecycle stage: ${STAGE_LABELS[activeStage]}`}
      role="status"
      aria-live="polite"
      aria-atomic="true"
    >
      <ol className={`${styles.track} ${archived ? styles.trackDimmed : ""}`}>
        {STAGE_ORDER.map((stage, index) => {
          const nodeState = archived
            ? "pending"
            : index < activeIndex
              ? "done"
              : index === activeIndex
                ? "active"
                : "pending";
          const isActive = !archived && stage === activeStage;

          return (
            <li
              key={stage}
              className={styles.nodeVariants[nodeState]}
              data-testid={`stage-node-${stage}`}
              aria-current={isActive ? "step" : undefined}
            >
              <span>{STAGE_LABELS[stage]}</span>
              {isActive && modifier && (
                <span className={styles.modifierBadge} data-testid="stage-modifier-badge">
                  {modifier}
                </span>
              )}
            </li>
          );
        })}
      </ol>
      {archived && (
        <div className={styles.archivedRibbon} data-testid="stage-archived-ribbon">
          Archived
        </div>
      )}
    </div>
  );
}
