"use client";

import { useEffect, useState, useCallback } from "react";
import {
  useBacklogSourcesService,
  type ItemSource,
  type SourceSyncEvent,
} from "@/lib/hooks/useBacklogSourcesService";
import * as styles from "./BacklogSourcesSettings.css";

const PLUGIN_OPTIONS = [
  { id: "github_issues", label: "GitHub Issues" },
  { id: "github_prs", label: "GitHub Pull Requests" },
];

function formatDate(iso?: string): string {
  return iso ? new Date(iso).toLocaleString() : "never";
}

/**
 * Settings panel for configuring backlog item sources (e.g. GitHub repos to
 * sync issues/PRs from): add a source, enable/disable, sync now, view history.
 */
export function BacklogSourcesSettings() {
  const { listItemSources, createItemSource, setItemSourceEnabled, deleteItemSource, triggerSync, getSyncHistory, lastError, clearError } =
    useBacklogSourcesService();

  const [sources, setSources] = useState<ItemSource[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [historyBySource, setHistoryBySource] = useState<Record<string, SourceSyncEvent[]>>({});
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const [pluginId, setPluginId] = useState(PLUGIN_OPTIONS[0].id);
  const [displayName, setDisplayName] = useState("");
  const [owner, setOwner] = useState("");
  const [repo, setRepo] = useState("");
  const [token, setToken] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const refresh = useCallback(async () => {
    const list = await listItemSources();
    setSources(list);
    setLoading(false);
  }, [listItemSources]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const canSubmit = displayName.trim() && owner.trim() && repo.trim() && token.trim() && !submitting;

  const handleAddSource = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    clearError();
    const created = await createItemSource({
      pluginId,
      displayName: displayName.trim(),
      configJson: JSON.stringify({ owner: owner.trim(), repo: repo.trim() }),
      token: token.trim(),
    });
    setSubmitting(false);
    if (created) {
      setDisplayName("");
      setOwner("");
      setRepo("");
      setToken("");
      await refresh();
    }
  };

  const handleToggleEnabled = async (source: ItemSource) => {
    const updated = await setItemSourceEnabled(source.id, source.displayName, !source.enabled);
    if (updated) await refresh();
  };

  const handleDelete = async (source: ItemSource) => {
    if (await deleteItemSource(source.id)) await refresh();
  };

  const handleSyncNow = async (source: ItemSource) => {
    setSyncingId(source.id);
    const ok = await triggerSync(source.id);
    setSyncingId(null);
    if (ok) {
      await refresh();
      if (expandedId === source.id) {
        setHistoryBySource((prev) => ({ ...prev, [source.id]: [] }));
        const events = await getSyncHistory(source.id);
        setHistoryBySource((prev) => ({ ...prev, [source.id]: events }));
      }
    }
  };

  const handleToggleHistory = async (source: ItemSource) => {
    if (expandedId === source.id) {
      setExpandedId(null);
      return;
    }
    setExpandedId(source.id);
    if (!historyBySource[source.id]) {
      const events = await getSyncHistory(source.id);
      setHistoryBySource((prev) => ({ ...prev, [source.id]: events }));
    }
  };

  return (
    <div className={styles.container}>
      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Backlog Sources</h3>
        <p className={styles.description}>
          Sync backlog items from external sources like GitHub issues and pull requests.
        </p>

        {lastError && <div className={styles.errorMessage}>{lastError.message}</div>}

        <div className={styles.list}>
          {loading ? (
            <span className={styles.empty}>Loading…</span>
          ) : sources.length === 0 ? (
            <span className={styles.empty}>No sources configured.</span>
          ) : (
            sources.map((source) => (
              <div key={source.id} className={styles.listItem}>
                <div className={styles.listItemHeader}>
                  <span className={styles.listItemName}>{source.displayName}</span>
                  <span className={styles.listItemMeta}>{source.pluginId}</span>
                  <button
                    role="switch"
                    aria-checked={source.enabled}
                    className={`${styles.toggle} ${source.enabled ? styles.toggleOn : ""}`}
                    onClick={() => handleToggleEnabled(source)}
                    aria-label={`${source.enabled ? "Disable" : "Enable"} ${source.displayName}`}
                  />
                  <button
                    className={styles.removeBtn}
                    onClick={() => handleDelete(source)}
                    aria-label={`Remove ${source.displayName}`}
                  >
                    ✕
                  </button>
                </div>
                <span className={styles.listItemMeta}>Last synced: {formatDate(source.lastSyncedAt)}</span>
                <div className={styles.actionRow}>
                  <button
                    className={styles.smallBtn}
                    onClick={() => handleSyncNow(source)}
                    disabled={syncingId === source.id || !source.enabled}
                    title={!source.enabled ? "Enable this source to sync it" : undefined}
                  >
                    {syncingId === source.id ? "Syncing…" : "Sync now"}
                  </button>
                  <button className={styles.smallBtn} onClick={() => handleToggleHistory(source)}>
                    {expandedId === source.id ? "Hide history" : "View history"}
                  </button>
                </div>
                {expandedId === source.id && (
                  <div className={styles.historyList}>
                    {(historyBySource[source.id] ?? []).length === 0 ? (
                      <span className={styles.empty}>No sync runs yet.</span>
                    ) : (
                      historyBySource[source.id].map((ev) => (
                        <div key={ev.id}>
                          {formatDate(ev.startedAt)} — created {ev.itemsCreated}, updated {ev.itemsUpdated}, skipped{" "}
                          {ev.itemsSkipped}
                          {ev.itemsErrored > 0 && `, errored ${ev.itemsErrored}`}
                          {ev.errorMessage && ` (${ev.errorMessage})`}
                        </div>
                      ))
                    )}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </section>

      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Add a Source</h3>
        <div className={styles.form}>
          <select className={styles.select} value={pluginId} onChange={(e) => setPluginId(e.target.value)}>
            {PLUGIN_OPTIONS.map((opt) => (
              <option key={opt.id} value={opt.id}>
                {opt.label}
              </option>
            ))}
          </select>
          <input
            type="text"
            className={styles.input}
            placeholder="Display name (e.g. My Repo Issues)"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
          />
          <div className={styles.formRow}>
            <input
              type="text"
              className={styles.input}
              placeholder="Owner (e.g. acme)"
              value={owner}
              onChange={(e) => setOwner(e.target.value)}
            />
            <input
              type="text"
              className={styles.input}
              placeholder="Repo (e.g. widgets)"
              value={repo}
              onChange={(e) => setRepo(e.target.value)}
            />
          </div>
          <input
            type="password"
            className={styles.input}
            placeholder="GitHub personal access token"
            value={token}
            onChange={(e) => setToken(e.target.value)}
          />
          <button className={styles.addBtn} onClick={handleAddSource} disabled={!canSubmit}>
            {submitting ? "Adding…" : "Add Source"}
          </button>
        </div>
      </section>
    </div>
  );
}
