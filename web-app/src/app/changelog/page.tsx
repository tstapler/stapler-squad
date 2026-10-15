"use client";
// +feature: changelog

// analytics-exempt
import { useEffect, useRef, useState } from "react";
import { BuildProvenance } from "@/components/changelog/BuildProvenance";
import { UpdateBadge } from "@/components/changelog/UpdateBadge";
import { fetchChangelog, type ChangelogResponse } from "@/lib/api/changelogApi";
import * as styles from "@/components/changelog/changelog.css";

const normalize = (v: string) => v.replace(/^v/, "");

export default function ChangelogPage() {
  const [data, setData] = useState<ChangelogResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const currentRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    const ctrl = new AbortController();
    fetchChangelog(ctrl.signal)
      .then((d) => {
        setData(d);
        const cur = normalize(d.current_version || "");
        const match = d.releases.find((r) => r.version === cur);
        const first = match ?? d.releases[0];
        if (first) setOpen({ [first.version]: true });
      })
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setError(e instanceof Error ? e.message : "Failed to load changelog");
      });
    return () => ctrl.abort();
  }, []);

  useEffect(() => {
    if (data && currentRef.current && typeof currentRef.current.scrollIntoView === "function") {
      currentRef.current.scrollIntoView({ block: "start" });
    }
  }, [data]);

  if (error) {
    return (
      <main id="main-content" className={styles.page}>
        <h1 className={styles.title}>Changelog</h1>
        <p role="alert">Couldn&apos;t load the changelog: {error}</p>
      </main>
    );
  }
  if (!data) {
    return (
      <main id="main-content" className={styles.page}>
        <h1 className={styles.title}>Changelog</h1>
        <p className={styles.muted}>Loading…</p>
      </main>
    );
  }

  const current = normalize(data.current_version || "");

  return (
    <main id="main-content" className={styles.page}>
      <h1 className={styles.title}>Changelog</h1>
      <BuildProvenance
        version={data.current_version}
        branch={data.branch}
        commit={data.commit}
        isSourceBuild={data.is_source_build}
      />
      <UpdateBadge />
      {data.releases.length === 0 && <p className={styles.muted}>No release notes are bundled with this build.</p>}
      {data.releases.map((rel) => {
        const isCurrent = rel.version === current;
        const isOpen = !!open[rel.version];
        const bodyId = `release-${rel.version}`;
        return (
          <section
            key={rel.version}
            id={bodyId}
            ref={isCurrent ? currentRef : undefined}
            className={`${styles.release} ${isCurrent ? styles.releaseCurrent : ""}`}
            data-testid={`release-${rel.version}`}
            aria-current={isCurrent ? "true" : undefined}
          >
            <h2 className={styles.releaseHeading}>
              <button
                type="button"
                className={styles.releaseHeader}
                aria-expanded={isOpen}
                aria-controls={`${bodyId}-body`}
                onClick={() => setOpen((o) => ({ ...o, [rel.version]: !o[rel.version] }))}
              >
                <span>v{rel.version}</span>
                {rel.date && <span className={styles.releaseDate}>{rel.date}</span>}
                {isCurrent && <span className={styles.hereChip}>You are here</span>}
              </button>
            </h2>
            {isOpen && (
              <div id={`${bodyId}-body`} className={styles.releaseBody}>
                {rel.sections.map((sec) => (
                  <div key={sec.title}>
                    <h3 className={styles.sectionTitle}>{sec.title}</h3>
                    <ul className={styles.entryList}>
                      {sec.entries.map((e, i) => (
                        <li key={i}>
                          {e.scope && <span className={styles.scope}>{e.scope}: </span>}
                          {e.text}
                          {e.prs?.map((n) => (
                            <span key={n} className={styles.muted}>
                              {" "}
                              (#{n})
                            </span>
                          ))}
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>
            )}
          </section>
        );
      })}
    </main>
  );
}
