"use client";

import { useEffect, useState, useCallback } from "react";
import {
  useBacklogSourcesService,
  type ItemSource,
  type SyncHistoryResult,
} from "@/lib/hooks/useBacklogSourcesService";
import { PLUGIN_SCHEMAS } from "./backlogSourceSchemas";
import * as styles from "./BacklogSourcesSettings.css";

function formatDate(iso?: string): string {
  return iso ? new Date(iso).toLocaleString() : "never";
}

/**
 * Settings panel for configuring backlog item sources (e.g. GitHub repos to
 * sync issues/PRs from): add a source, enable/disable, sync now, view history.
 */
// Non-transient sync failures (auth revoked/expired) warrant a persistent,
// row-level warning that doesn't require expanding history to notice
// (Story 4.3.2) — a rate limit or a one-off network blip does not.
function isAuthFailure(errorMessage?: string): boolean {
  if (!errorMessage) return false;
  const lower = errorMessage.toLowerCase();
  return lower.includes("401") || lower.includes("403") || lower.includes("revoked");
}

export function BacklogSourcesSettings() {
  const {
    listItemSources,
    createItemSource,
    setItemSourceEnabled,
    setForwardSyncEnabled,
    setBackwardSyncEnabled,
    setForwardSyncCloseLabel,
    deleteItemSource,
    triggerSync,
    getSyncHistory,
    lastError,
    clearError,
  } = useBacklogSourcesService();

  const [sources, setSources] = useState<ItemSource[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [historyBySource, setHistoryBySource] = useState<Record<string, SyncHistoryResult>>({});
  const [expandedId, setExpandedId] = useState<string | null>(null);
  // Local in-progress edits to the close-label input, keyed by source id —
  // committed to the backend on blur rather than per keystroke.
  const [closeLabelDrafts, setCloseLabelDrafts] = useState<Record<string, string>>({});

  const [pluginId, setPluginId] = useState(PLUGIN_SCHEMAS[0].id);
  const [displayName, setDisplayName] = useState("");
  const [fieldValues, setFieldValues] = useState<Record<string, string>>({});
  const [token, setToken] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const schema = PLUGIN_SCHEMAS.find((s) => s.id === pluginId) ?? PLUGIN_SCHEMAS[0];

  const handlePluginChange = (id: string) => {
    setPluginId(id);
    setFieldValues({});
    setToken("");
  };

  const setField = (key: string, value: string) => {
    setFieldValues((prev) => ({ ...prev, [key]: value }));
  };

  const refresh = useCallback(async () => {
    const list = await listItemSources();
    setSources(list);
    setLoading(false);

    // Eagerly load each source's sync history so the row-level
    // non-transient-failure warning (Story 4.3.2) can render without the
    // user needing to expand history first.
    const histories = await Promise.all(list.map((s) => getSyncHistory(s.id)));
    setHistoryBySource((prev) => {
      const next = { ...prev };
      list.forEach((s, i) => {
        next[s.id] = histories[i];
      });
      return next;
    });
  }, [listItemSources, getSyncHistory]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const canSubmit =
    Boolean(displayName.trim()) &&
    schema.fields.every((f) => Boolean(fieldValues[f.key]?.trim())) &&
    (!schema.requiresToken || Boolean(token.trim())) &&
    !submitting;

  const handleAddSource = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    clearError();
    const config: Record<string, string> = {};
    for (const f of schema.fields) {
      config[f.key] = fieldValues[f.key]?.trim() ?? "";
    }
    const created = await createItemSource({
      pluginId,
      displayName: displayName.trim(),
      configJson: JSON.stringify(config),
      token: schema.requiresToken ? token.trim() : "",
    });
    setSubmitting(false);
    if (created) {
      setDisplayName("");
      setFieldValues({});
      setToken("");
      await refresh();
    }
  };

  const handleToggleEnabled = async (source: ItemSource) => {
    const updated = await setItemSourceEnabled(source, !source.enabled);
    if (updated) await refresh();
  };

  const handleToggleForwardSync = async (source: ItemSource) => {
    const updated = await setForwardSyncEnabled(source, !source.forwardSyncEnabled);
    if (updated) await refresh();
  };

  // Turning backward sync OFF is unaffected — only turning it ON goes
  // through a confirmation-with-preview step (Epic 4.4, implemented later —
  // deliberately not wired here yet since it depends on Epic 2.1's
  // determineBackwardSyncTarget, in flight concurrently elsewhere).
  const handleToggleBackwardSync = async (source: ItemSource) => {
    const updated = await setBackwardSyncEnabled(source, !source.backwardSyncEnabled);
    if (updated) await refresh();
  };

  const handleCloseLabelChange = async (source: ItemSource, closeLabel: string) => {
    const updated = await setForwardSyncCloseLabel(source, closeLabel);
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
        setHistoryBySource((prev) => ({ ...prev, [source.id]: { events: [], truncated: false } }));
        const result = await getSyncHistory(source.id);
        setHistoryBySource((prev) => ({ ...prev, [source.id]: result }));
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
      const result = await getSyncHistory(source.id);
      setHistoryBySource((prev) => ({ ...prev, [source.id]: result }));
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
            sources.map((source) => {
              const mostRecentSyncError = historyBySource[source.id]?.events?.[0]?.errorMessage;
              const closeLabelValue = closeLabelDrafts[source.id] ?? source.forwardSyncCloseLabel;
              return (
              <div key={source.id} className={styles.listItem} data-testid={`source-row-${source.id}`}>
                <div className={styles.listItemHeader}>
                  <span className={styles.listItemName}>{source.displayName}</span>
                  {isAuthFailure(mostRecentSyncError) && (
                    <span
                      className={styles.authWarning}
                      data-testid={`source-row-${source.id}-auth-warning`}
                      role="alert"
                    >
                      ⚠ Sync failing — check credentials
                    </span>
                  )}
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

                <div className={styles.syncDirectionGroup}>
                  <span className={styles.subHeading}>Sync with GitHub</span>
                  <div className={styles.syncDirectionRow}>
                    <button
                      role="switch"
                      aria-checked={source.forwardSyncEnabled}
                      className={`${styles.toggle} ${source.forwardSyncEnabled ? styles.toggleOn : ""}`}
                      onClick={() => handleToggleForwardSync(source)}
                      aria-label={`${source.forwardSyncEnabled ? "Disable" : "Enable"} closing GitHub issues when done`}
                    />
                    <span>Close GitHub issues when I finish here</span>
                  </div>
                  {source.forwardSyncEnabled && (
                    <input
                      type="text"
                      className={styles.input}
                      placeholder="Label to apply on close (optional)"
                      aria-label={`Close label for ${source.displayName}`}
                      value={closeLabelValue}
                      onChange={(e) =>
                        setCloseLabelDrafts((prev) => ({ ...prev, [source.id]: e.target.value }))
                      }
                      onBlur={() => handleCloseLabelChange(source, closeLabelValue)}
                    />
                  )}
                  <div className={styles.syncDirectionRow}>
                    <button
                      role="switch"
                      aria-checked={source.backwardSyncEnabled}
                      className={`${styles.toggle} ${source.backwardSyncEnabled ? styles.toggleOn : ""}`}
                      onClick={() => handleToggleBackwardSync(source)}
                      aria-label={`${source.backwardSyncEnabled ? "Disable" : "Enable"} reflecting GitHub status back`}
                    />
                    <span>Reflect GitHub status back here</span>
                  </div>
                  {source.forwardSyncEnabled && source.backwardSyncEnabled && (
                    <div className={styles.bothDirectionsWarning}>
                      Both directions are enabled — closing this item&apos;s issue may be observed and
                      re-applied by backward sync. Verify this doesn&apos;t create a loop for items you also
                      edit manually.
                    </div>
                  )}
                </div>

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
                    {(historyBySource[source.id]?.events ?? []).length === 0 ? (
                      <span className={styles.empty}>No sync runs yet.</span>
                    ) : (
                      <>
                        {historyBySource[source.id].events.map((ev) => (
                          <div key={ev.id}>
                            {formatDate(ev.startedAt)} — created {ev.itemsCreated}, updated {ev.itemsUpdated}, skipped{" "}
                            {ev.itemsSkipped}
                            {ev.itemsErrored > 0 && `, errored ${ev.itemsErrored}`}
                            {ev.errorMessage && ` (${ev.errorMessage})`}
                          </div>
                        ))}
                        {historyBySource[source.id].truncated && (
                          <span className={styles.empty}>
                            Older sync history exists but is not shown (limited to the most recent{" "}
                            {historyBySource[source.id].events.length} runs).
                          </span>
                        )}
                      </>
                    )}
                  </div>
                )}
              </div>
              );
            })
          )}
        </div>
      </section>

      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Add a Source</h3>
        <div className={styles.form}>
          <select className={styles.select} value={pluginId} onChange={(e) => handlePluginChange(e.target.value)}>
            {PLUGIN_SCHEMAS.map((s) => (
              <option key={s.id} value={s.id}>
                {s.label}
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
            {schema.fields.map((f) => (
              <input
                key={f.key}
                type="text"
                className={styles.input}
                placeholder={f.placeholder}
                aria-label={f.label}
                value={fieldValues[f.key] ?? ""}
                onChange={(e) => setField(f.key, e.target.value)}
              />
            ))}
          </div>
          {schema.requiresToken && (
            <input
              type="password"
              className={styles.input}
              placeholder={schema.tokenLabel}
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
          )}
          <button className={styles.addBtn} onClick={handleAddSource} disabled={!canSubmit}>
            {submitting ? "Adding…" : "Add Source"}
          </button>
        </div>
      </section>
    </div>
  );
}
