// +feature: scheduled-prompts
"use client";

import { PEER_COPY } from "@/lib/copy/scheduledPromptsCopy";
import { resumeScheduledPrompts } from "@/lib/api/wakeTriggerApi";
import { useScheduledPrompts, type ScheduledPromptsState } from "@/lib/hooks/useScheduledPrompts";
import { groupLabel, rowList, section } from "./BackgroundActivity.css";
import { PeerHelpRow } from "./PeerHelpRow";
import { ScheduledPrompts } from "./ScheduledPrompts";

const RECENT_PEER_LIMIT = 5;

/** Peer-help requests as the Background tab shows them: open ones, then the recently ended (UX-S4). */
export function PeerHelpGroups({ state, now }: { state: ScheduledPromptsState; now?: number }) {
  const open = state.peers.filter((p) => p.state === "waiting" || p.state === "not_sent");
  const recent = state.peers.filter((p) => p.state === "answered" || p.state === "cancelled").slice(0, RECENT_PEER_LIMIT);
  if (open.length === 0 && recent.length === 0) return null;
  return (
    <section className={section} data-testid="peer-help-groups">
      {open.length > 0 && (
        <>
          <h3 className={groupLabel}>
            {PEER_COPY.heading} ({open.length})
          </h3>
          <ul className={rowList} aria-label={PEER_COPY.heading}>
            {open.map((p) => (
              <PeerHelpRow key={p.id} info={p} now={now} onChanged={() => void state.refresh()} />
            ))}
          </ul>
        </>
      )}
      {recent.length > 0 && (
        <>
          <h3 className={groupLabel}>{PEER_COPY.recentHeading}</h3>
          <ul className={rowList} aria-label={PEER_COPY.recentHeading} data-testid="peer-help-recent">
            {recent.map((p) => (
              <PeerHelpRow key={p.id} info={p} now={now} onChanged={() => void state.refresh()} />
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

/**
 * The agent-messaging half of the Background tab: peer-help rows and the Scheduled prompts
 * section, polled together while the tab is showing. Hidden until the server answers.
 */
export function ScheduledPromptsSection({ active }: { active: boolean }) {
  const state = useScheduledPrompts(active);
  return (
    <>
      <PeerHelpGroups state={state} />
      <ScheduledPrompts state={state} resumeFlag={resumeScheduledPrompts} />
    </>
  );
}
