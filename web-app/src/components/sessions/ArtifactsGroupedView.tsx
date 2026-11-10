"use client";

import { useId, useMemo, useState, type ReactNode } from "react";
import type { SessionArtifacts, SessionPullRequest } from "@/gen/session/v1/types_pb";
import { PullRequestChip } from "@/components/shared/PullRequestChip";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { groupArtifacts, REDACTED, type ArtifactItem, type PrGroup } from "@/lib/artifacts/groupArtifacts";
import { prKey } from "@/lib/pr/pullRequests";
import * as s from "./ArtifactsGroupedView.css";

/** A group longer than this shows its newest items first behind "Show all (N)". */
export const GROUP_ITEM_CAP = 50;
/** Beyond this many groups only the active PR's group starts open. */
const OPEN_ALL_UP_TO = 3;

const safeHref = (url: string) => (/^https?:\/\//i.test(url) ? url : "#");

function ItemRow({ item }: { item: ArtifactItem }) {
  const { announce } = useAnnounce();
  const copy = () => {
    void (async () => {
      try {
        await navigator.clipboard.writeText(item.copyValue ?? "");
        announce(`Copied ${item.kindLabel} ${item.title}`, "polite", "artifact-copy");
      } catch {
        announce(`Couldn't copy ${item.kindLabel}`, "assertive", "artifact-copy");
      }
    })();
  };
  let control: ReactNode;
  if (item.redacted) control = <span className={s.value} data-testid="artifact-redacted">{REDACTED}</span>;
  else if (item.href) control = <a className={s.control} href={safeHref(item.href)} target="_blank" rel="noopener noreferrer">{item.title}</a>;
  else if (item.copyValue) control = <button type="button" className={s.control} aria-label={item.copyName} onClick={copy}>{/* analytics-exempt */}{item.title}</button>;
  else control = <span className={s.value}>{item.title}</span>;
  return (
    <li className={s.item} data-testid={`artifact-item-${item.kind}`}>
      <div className={s.itemLine}>
        <span className={s.kind}>{item.kindLabel}</span>
        {control}
      </div>
      {item.meta && !item.redacted && <span className={s.meta}>{item.meta}</span>}
    </li>
  );
}

function Group({ title, count, chip, defaultOpen, testId, children }: { title: string; count: number; chip?: ReactNode; defaultOpen: boolean; testId: string; children: ReactNode }) {
  const [open, setOpen] = useState(defaultOpen);
  const bodyId = useId();
  return (
    <section className={s.group} data-testid={testId}>
      <div className={s.groupHeader}>
        <button type="button" className={s.disclosure} aria-expanded={open} aria-controls={bodyId} onClick={() => setOpen((v) => !v)}>{/* analytics-exempt */}
          <span aria-hidden="true">{open ? "▾" : "▸"}</span>
          <span>{title}</span>
          <span className={s.count}>{count} {count === 1 ? "item" : "items"}</span>
        </button>
        {chip}
      </div>
      <div id={bodyId} hidden={!open}>{open && children}</div>
    </section>
  );
}

function Items({ items }: { items: readonly ArtifactItem[] }) {
  const [all, setAll] = useState(false);
  const shown = all || items.length <= GROUP_ITEM_CAP ? items : items.slice(-GROUP_ITEM_CAP);
  return (
    <>
      <ul className={s.items}>{shown.map((it) => <ItemRow key={it.key} item={it} />)}</ul>
      {items.length > GROUP_ITEM_CAP && (
        <button type="button" className={s.showAll} aria-pressed={all} onClick={() => setAll((v) => !v)}>{/* analytics-exempt */}
          {all ? "Show newest 50" : `Show all (${items.length})`}
        </button>
      )}
    </>
  );
}

function PrHeader({ group, tracked }: { group: PrGroup; tracked: SessionPullRequest | undefined }) {
  // A PR the session does not track has no state to show, so it gets a plain link, never a "Checking" chip.
  return tracked
    ? <PullRequestChip pullRequests={[tracked]} onOpenList={() => {}} />
    : <a className={s.prLink} href={group.url} target="_blank" rel="noopener noreferrer" aria-label={`Open #${group.number} ${group.owner}/${group.repo} on GitHub`}>Open on GitHub</a>;
}

interface Props {
  artifacts: SessionArtifacts;
  pullRequests: readonly SessionPullRequest[];
}

/** v2 records grouped by the PR the session created (ux.md S7 "Grouped view"). */
export function ArtifactsGroupedView({ artifacts, pullRequests }: Props) {
  const grouping = useMemo(() => groupArtifacts(artifacts), [artifacts]);
  const trackedByKey = useMemo(() => new Map(pullRequests.map((p) => [prKey(p), p])), [pullRequests]);
  const { prGroups, unassigned, mentioned, skipped } = grouping;
  const openAll = prGroups.length + (unassigned.length > 0 ? 1 : 0) <= OPEN_ALL_UP_TO;
  const activeKey = prGroups.find((g) => trackedByKey.get(g.key)?.isActive)?.key ?? prGroups[prGroups.length - 1]?.key;

  return (
    <div className={s.groups} data-testid="artifacts-grouped">
      <p className={s.note}>Grouped by when each item appeared relative to PR creation.</p>
      {skipped > 0 && <p className={s.note} role="status">{skipped} {skipped === 1 ? "item" : "items"} couldn&apos;t be shown. Rescan to retry.</p>}
      {prGroups.map((g) => (
        <Group
          key={g.key}
          testId={`artifact-group-${g.number}`}
          title={`#${g.number} ${g.owner}/${g.repo}`}
          count={g.items.length}
          defaultOpen={openAll || g.key === activeKey}
          chip={<PrHeader group={g} tracked={trackedByKey.get(g.key)} />}
        >
          <Items items={g.items} />
        </Group>
      ))}
      {unassigned.length > 0 && (
        <Group key="unassigned" testId="artifact-group-unassigned" title="Unassigned" count={unassigned.length} defaultOpen={openAll || prGroups.length === 0}>
          <Items items={unassigned} />
        </Group>
      )}
      {mentioned.length > 0 && (
        <Group key="mentioned" testId="artifact-group-mentioned" title="Mentioned pull requests" count={mentioned.length} defaultOpen={false}>
          <p className={s.note}>Mentioned, not created by this session.</p>
          <ul className={s.items}>
            {mentioned.map((m) => (
              <li key={m.key} className={s.item}>
                <a className={s.control} href={safeHref(m.url)} target="_blank" rel="noopener noreferrer">#{m.number} {m.owner}/{m.repo}</a>
              </li>
            ))}
          </ul>
        </Group>
      )}
    </div>
  );
}
