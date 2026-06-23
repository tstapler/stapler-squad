// +feature: session-artifacts
"use client";

import { useState } from "react";
import { Session } from "@/gen/session/v1/types_pb";
import * as styles from "./ArtifactsTab.css";

interface ArtifactsTabProps {
  session: Session;
}

function parsePRDisplay(url: string): string {
  const m = url.match(/github\.com\/([\w.-]+\/[\w.-]+)\/pull\/(\d+)/);
  return m ? `${m[1]}#${m[2]}` : url;
}

// safeHref ensures only http/https URLs are used as link targets (N-5 fix).
const safeHref = (url: string) => /^https?:\/\//i.test(url) ? url : "#";

export function ArtifactsTab({ session }: ArtifactsTabProps) {
  const [urlsExpanded, setUrlsExpanded] = useState(false);
  const artifacts = session.artifacts;

  // Sub-state 1: scan not yet run (server omits the field entirely)
  if (!artifacts) {
    return (
      <div className={styles.emptyState}>
        <span>Extraction pending — will populate automatically once the session starts.</span>
      </div>
    );
  }

  const hasContent =
    (artifacts.prUrls?.length ?? 0) > 0 ||
    (artifacts.commitShas?.length ?? 0) > 0 ||
    (artifacts.externalUrls?.length ?? 0) > 0;

  // Sub-state 2: scan ran but found nothing
  if (!hasContent) {
    return (
      <div className={styles.emptyState}>
        <span>No artifacts found in this session&apos;s conversation history.</span>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {(artifacts.prUrls?.length ?? 0) > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Pull Requests</h3>
          <ul className={styles.list}>
            {artifacts.prUrls.map((url) => (
              <li key={url}>
                <a
                  href={safeHref(url)}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={styles.link}
                >
                  {parsePRDisplay(url)}
                </a>
              </li>
            ))}
          </ul>
        </section>
      )}
      {(artifacts.commitShas?.length ?? 0) > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Commits</h3>
          <ul className={styles.list}>
            {artifacts.commitShas.map((sha) => (
              <li key={sha}>
                <code className={styles.sha}>{sha.slice(0, 7)}</code>
                <span className={styles.shaFull}>{sha}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
      {(artifacts.externalUrls?.length ?? 0) > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>External URLs</h3>
          {urlsExpanded ? (
            <>
              <ul className={styles.list}>
                {artifacts.externalUrls.map((url) => (
                  <li key={url}>
                    <a
                      href={safeHref(url)}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={styles.link}
                    >
                      {url.length > 60 ? url.slice(0, 60) + "…" : url}
                    </a>
                  </li>
                ))}
              </ul>
              <button
                className={styles.urlToggleButton}
                onClick={() => setUrlsExpanded(false)}
              >
                Hide URLs
              </button>
            </>
          ) : (
            <button
              className={styles.urlToggleButton}
              onClick={() => setUrlsExpanded(true)}
            >
              Show {artifacts.externalUrls.length} external URL
              {artifacts.externalUrls.length !== 1 ? "s" : ""}
            </button>
          )}
        </section>
      )}
    </div>
  );
}
