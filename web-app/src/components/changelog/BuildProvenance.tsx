"use client";
// +feature: changelog

import * as styles from "./changelog.css";

interface BuildProvenanceProps {
  version: string;
  branch?: string;
  commit?: string;
  isSourceBuild: boolean;
}

export function BuildProvenance({ version, branch, commit, isSourceBuild }: BuildProvenanceProps) {
  return (
    <dl className={styles.provenance} data-testid="build-provenance">
      <div className={styles.provenanceItem}>
        <dt className={styles.provenanceLabel}>Version</dt>
        <dd className={styles.provenanceValue}>{version || "unknown"}</dd>
      </div>
      {commit && (
        <div className={styles.provenanceItem}>
          <dt className={styles.provenanceLabel}>Commit</dt>
          <dd className={styles.provenanceValue}>
            <code>{commit}</code>
          </dd>
        </div>
      )}
      {branch && (
        <div className={styles.provenanceItem}>
          <dt className={styles.provenanceLabel}>Branch</dt>
          <dd className={styles.provenanceValue}>
            <code>{branch}</code>
          </dd>
        </div>
      )}
      {isSourceBuild && (
        <span className={styles.sourceBadge} data-testid="source-build-badge">
          Source build
        </span>
      )}
    </dl>
  );
}
