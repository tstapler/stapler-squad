// +feature: unfinished-github-prs
"use client";

import { useMemo } from "react";
import { type UserPR } from "@/gen/session/v1/types_pb";
import { PRCard } from "./PRCard";
import * as styles from "./PRCard.css";

interface PRGroupedListProps {
  prs: UserPR[];
}

export function PRGroupedList({ prs }: PRGroupedListProps) {
  const groups = useMemo(() => {
    const map = new Map<string, UserPR[]>();
    for (const pr of prs) {
      const key = `${pr.owner}/${pr.repo}`;
      const group = map.get(key) ?? [];
      group.push(pr);
      map.set(key, group);
    }
    return Array.from(map.entries()).sort(([a], [b]) => a.localeCompare(b));
  }, [prs]);

  if (groups.length === 0) return null;

  if (groups.length === 1) {
    return (
      <div className={styles.repoGroupSection}>
        {groups[0][1].map((pr) => (
          <PRCard key={`${pr.owner}/${pr.repo}#${pr.number}`} pr={pr} />
        ))}
      </div>
    );
  }

  return (
    <>
      {groups.map(([repoKey, repoPRs]) => (
        <div key={repoKey} className={styles.repoGroupSection}>
          <div className={styles.repoGroupHeader}>{repoKey}</div>
          {repoPRs.map((pr) => (
            <PRCard key={`${pr.owner}/${pr.repo}#${pr.number}`} pr={pr} />
          ))}
        </div>
      ))}
    </>
  );
}
