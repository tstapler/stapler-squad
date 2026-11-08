"use client";

import { Link } from "@/lib/nav";
import { formatRelativeTime } from "@/lib/utils/datetime";
import { kindLabelOf, sessionArtifactsHref, type DisplayItem } from "./artifactDisplay";
import * as s from "./ArtifactsBrowser.css";

interface Props {
  items: readonly DisplayItem[];
  /** True below the tablet breakpoint: cards instead of table rows. */
  compact: boolean;
  dimmed: boolean;
}

function SessionLink({ item }: { item: DisplayItem }) {
  return (
    <>
      <Link href={sessionArtifactsHref(item.sessionId)} className={s.sessionLink} data-testid="artifact-session-link">{item.sessionTitle || item.sessionId}</Link>
      {item.archived && <span className={s.tag}>Archived</span>}
    </>
  );
}

const when = (ms: number | undefined) => (ms === undefined ? "" : formatRelativeTime(ms));

/** Table-like rows on desktop, single-column cards on mobile; the same data in both, no hover-only content. */
export function ArtifactResults({ items, compact, dimmed }: Props) {
  if (compact) {
    return (
      <ul className={`${s.cards} ${dimmed ? s.dimmed : ""}`} data-testid="artifacts-cards">
        {items.map((item) => (
          <li key={item.key} className={s.card} data-testid="artifact-row">
            <span className={s.kind}>{kindLabelOf(item.kind)}{item.sourceLabel ? ` · ${item.sourceLabel}` : ""}</span>
            <div>{item.artifact}</div>
            <div><SessionLink item={item} /></div>
            <span className={s.muted}>{when(item.whenMs)}</span>
          </li>
        ))}
      </ul>
    );
  }
  return (
    <table className={`${s.table} ${dimmed ? s.dimmed : ""}`} data-testid="artifacts-table">
      <thead>
        <tr>
          <th scope="col" className={s.th}>Kind</th>
          <th scope="col" className={s.th}>Artifact</th>
          <th scope="col" className={s.th}>Session</th>
          <th scope="col" className={s.th}>When</th>
        </tr>
      </thead>
      <tbody>
        {items.map((item) => (
          <tr key={item.key} data-testid="artifact-row">
            <td className={s.td}>{kindLabelOf(item.kind)}{item.sourceLabel && <div className={s.muted}>{item.sourceLabel}</div>}</td>
            <td className={s.td}>{item.artifact}</td>
            <td className={s.td}><SessionLink item={item} /></td>
            <td className={s.td}>{when(item.whenMs)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
