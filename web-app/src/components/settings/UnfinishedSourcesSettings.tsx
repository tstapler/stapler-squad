"use client";

import { useState } from "react";
import { useUnfinishedWorkConfig } from "@/lib/hooks/useUnfinishedWorkConfig";
import * as styles from "./UnfinishedSourcesSettings.css";

/**
 * Settings panel for configuring Unfinished Work sources:
 * - Auto-spider toggle
 * - Watch dirs (add/remove)
 * - Pinned repos (add/remove)
 */
export function UnfinishedSourcesSettings() {
  const { config, loading, updateConfig } = useUnfinishedWorkConfig();
  const [newWatchDir, setNewWatchDir] = useState("");
  const [newPinnedRepo, setNewPinnedRepo] = useState("");

  if (loading) return <p>Loading…</p>;
  if (!config) return <p>Failed to load config.</p>;

  const watchDirs = config.watchDirs ?? [];
  const pinnedRepos = config.pinnedRepos ?? [];
  const autoSpider = config.autoSpiderSessions ?? true;

  const handleToggleAutoSpider = () => {
    updateConfig({ autoSpiderSessions: !autoSpider });
  };

  const handleRemoveWatchDir = (dir: string) => {
    updateConfig({ watchDirs: watchDirs.filter((d) => d !== dir) });
  };

  const handleAddWatchDir = () => {
    const trimmed = newWatchDir.trim();
    if (!trimmed || watchDirs.includes(trimmed)) return;
    updateConfig({ watchDirs: [...watchDirs, trimmed] });
    setNewWatchDir("");
  };

  const handleRemovePinnedRepo = (repo: string) => {
    updateConfig({ pinnedRepos: pinnedRepos.filter((r) => r !== repo) });
  };

  const handleAddPinnedRepo = () => {
    const trimmed = newPinnedRepo.trim();
    if (!trimmed || pinnedRepos.includes(trimmed)) return;
    updateConfig({ pinnedRepos: [...pinnedRepos, trimmed] });
    setNewPinnedRepo("");
  };

  return (
    <div className={styles.container}>
      {/* Auto-spider toggle */}
      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Auto-Spider Sessions</h3>
        <p className={styles.description}>
          Automatically scan the git repos of active Stapler Squad sessions for unfinished work.
        </p>
        <div className={styles.toggleRow}>
          <button
            role="switch"
            aria-checked={autoSpider}
            className={`${styles.toggle} ${autoSpider ? styles.toggleOn : ""}`}
            onClick={handleToggleAutoSpider}
            aria-label="Toggle auto-spider sessions"
          />
          <span>{autoSpider ? "Enabled" : "Disabled"}</span>
        </div>
      </section>

      {/* Watch Dirs */}
      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Watch Directories</h3>
        <p className={styles.description}>
          Stapler Squad will scan all git repos found within these directories (depth ≤ 5).
        </p>

        <div className={styles.list}>
          {watchDirs.length === 0 ? (
            <span className={styles.empty}>No watch directories configured.</span>
          ) : (
            watchDirs.map((dir) => (
              <div key={dir} className={styles.listItem}>
                <span className={styles.listItemPath}>{dir}</span>
                <button
                  className={styles.removeBtn}
                  onClick={() => handleRemoveWatchDir(dir)}
                  aria-label={`Remove watch directory ${dir}`}
                >
                  ✕
                </button>
              </div>
            ))
          )}
        </div>

        <div className={styles.addRow}>
          <input
            type="text"
            className={styles.input}
            placeholder="/Users/you/code"
            value={newWatchDir}
            onChange={(e) => setNewWatchDir(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleAddWatchDir()}
            aria-label="New watch directory path"
          />
          <button
            className={styles.addBtn}
            onClick={handleAddWatchDir}
            disabled={!newWatchDir.trim()}
          >
            Add
          </button>
        </div>
      </section>

      {/* Pinned Repos */}
      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Pinned Repositories</h3>
        <p className={styles.description}>
          Explicitly add a specific git repository to always scan.
        </p>

        <div className={styles.list}>
          {pinnedRepos.length === 0 ? (
            <span className={styles.empty}>No pinned repositories.</span>
          ) : (
            pinnedRepos.map((repo) => (
              <div key={repo} className={styles.listItem}>
                <span className={styles.listItemPath}>{repo}</span>
                <button
                  className={styles.removeBtn}
                  onClick={() => handleRemovePinnedRepo(repo)}
                  aria-label={`Remove pinned repo ${repo}`}
                >
                  ✕
                </button>
              </div>
            ))
          )}
        </div>

        <div className={styles.addRow}>
          <input
            type="text"
            className={styles.input}
            placeholder="/Users/you/my-project"
            value={newPinnedRepo}
            onChange={(e) => setNewPinnedRepo(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleAddPinnedRepo()}
            aria-label="New pinned repository path"
          />
          <button
            className={styles.addBtn}
            onClick={handleAddPinnedRepo}
            disabled={!newPinnedRepo.trim()}
          >
            Add
          </button>
        </div>
      </section>
    </div>
  );
}
