// +feature: unfinished-github-prs
"use client";

import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";
import { type UserPR } from "@/gen/session/v1/types_pb";
import { orderPRs, prKey, type PRGroup, type PRSortBy } from "@/lib/unfinished/prOrdering";
import { PRCard } from "./PRCard";
import * as styles from "./PRCard.css";
import * as listStyles from "./PRGroupedList.css";

interface FrozenOrder {
  /** Filter/sort/search signature this order was frozen under. */
  stateKey: string;
  groups: { key: string; prs: UserPR[] }[];
}

const freeze = (groups: PRGroup[], stateKey: string): FrozenOrder => ({
  stateKey,
  groups: groups.map((g) => ({ key: g.key, prs: g.prs })),
});

const flatKeys = (groups: { prs: UserPR[] }[]) => groups.flatMap((g) => g.prs.map(prKey));
const countPRs = (groups: { prs: UserPR[] }[]) => groups.reduce((n, g) => n + g.prs.length, 0);

/** PRs added, removed, or at a different position between the frozen order and the live one. */
function changedCount(frozen: FrozenOrder, live: PRGroup[]): number {
  const before = flatKeys(frozen.groups);
  const after = flatKeys(live);
  const beforeSet = new Set(before);
  const afterSet = new Set(after);
  const removed = before.filter((k) => !afterSet.has(k)).length;
  const added = after.filter((k) => !beforeSet.has(k)).length;
  const beforeCommon = before.filter((k) => afterSet.has(k));
  const afterCommon = after.filter((k) => beforeSet.has(k));
  const moved = beforeCommon.filter((k, i) => afterCommon[i] !== k).length;
  return removed + added + moved;
}

function groupTitle(key: string): string {
  return key.startsWith("github.com/") ? key.slice("github.com/".length) : key;
}

function spansMultipleHostsOrAccounts(prs: UserPR[]): boolean {
  const hosts = new Set(prs.map((p) => (p.host || "github.com").toLowerCase()));
  const accounts = new Set(prs.map((p) => `${p.host}|${p.accountLogin}`));
  return hosts.size > 1 || accounts.size > 1;
}

interface PRGroupedListProps {
  /** Filtered PRs, any order; this component orders them. */
  prs: UserPR[];
  sortBy: PRSortBy;
  /** Changes whenever filter, sort or search changes; a new value applies the live order. */
  stateKey: string;
  /** Where focus lands after "Refresh list" when the previously focused card is gone. */
  fallbackFocusRef?: React.RefObject<HTMLElement | null>;
}

/**
 * Renders PRs grouped by repo. After the first paint of a given `stateKey` the ORDER and
 * MEMBERSHIP are frozen so the list does not shuffle under the user; card content still
 * updates in place, and a status line offers "Refresh list" to apply the new order.
 */
export function PRGroupedList({ prs, sortBy, stateKey, fallbackFocusRef }: PRGroupedListProps) {
  const liveGroups = useMemo(() => orderPRs(prs, sortBy), [prs, sortBy]);
  const liveByKey = useMemo(() => new Map(prs.map((p) => [prKey(p), p])), [prs]);
  const showHostAccount = useMemo(() => spansMultipleHostsOrAccounts(prs), [prs]);

  const [applied, setApplied] = useState<FrozenOrder>(() => freeze(liveGroups, stateKey));
  // An empty first paint has nothing to protect; follow live data until there is something.
  const followLive = applied.stateKey !== stateKey || countPRs(applied.groups) === 0;
  const frozen = followLive ? freeze(liveGroups, stateKey) : applied;
  if (followLive && countPRs(frozen.groups) > 0) setApplied(frozen);

  const containerRef = useRef<HTMLDivElement>(null);
  const lastFocusedKey = useRef<string | null>(null);
  const pendingFocus = useRef(false);

  const onFocusCapture = useCallback((e: React.FocusEvent) => {
    const card = (e.target as HTMLElement).closest<HTMLElement>("[data-pr-key]");
    if (card) lastFocusedKey.current = card.dataset.prKey ?? null;
  }, []);

  const applyLiveOrder = useCallback(() => {
    pendingFocus.current = true;
    setApplied(freeze(liveGroups, stateKey));
  }, [liveGroups, stateKey]);

  useLayoutEffect(() => {
    if (!pendingFocus.current) return;
    pendingFocus.current = false;
    const key = lastFocusedKey.current;
    const card = key
      ? Array.from(containerRef.current?.querySelectorAll<HTMLElement>("[data-pr-key]") ?? []).find(
          (el) => el.dataset.prKey === key
        )
      : undefined;
    (card?.querySelector<HTMLElement>("a") ?? fallbackFocusRef?.current)?.focus();
  }, [applied, fallbackFocusRef]);

  const changed = changedCount(frozen, liveGroups);
  const groups = frozen.groups;

  return (
    <div ref={containerRef} className={listStyles.groupList} onFocusCapture={onFocusCapture}>
      <div role="status" className={listStyles.changesLine} data-testid="pr-list-changes">
        {changed > 0 && (
          <>
            <span>
              {changed} {changed === 1 ? "PR" : "PRs"} changed.
            </span>
            <button type="button" className={listStyles.refreshListButton} onClick={applyLiveOrder}>
              Refresh list
            </button>
          </>
        )}
      </div>
      {groups.map((group) => (
        <div key={group.key} className={styles.repoGroupSection}>
          {groups.length > 1 && <div className={styles.repoGroupHeader}>{groupTitle(group.key)}</div>}
          {group.prs.map((frozenPR) => {
            const key = prKey(frozenPR);
            return <PRCard key={key} pr={liveByKey.get(key) ?? frozenPR} showHostAccount={showHostAccount} />;
          })}
        </div>
      ))}
    </div>
  );
}
