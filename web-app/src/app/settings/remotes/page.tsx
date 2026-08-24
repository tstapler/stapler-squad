// analytics-exempt
"use client";
// +feature: settings-remotes
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as pipeline-modes/page.tsx.

import { useCallback, useEffect, useState } from "react";
import { ConnectError } from "@connectrpc/connect";
import { useRemotesService, type RemoteConfigInfo } from "@/lib/hooks/useRemotesService";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";
import { AddRemoteForm } from "@/components/settings/AddRemoteForm";
import * as styles from "./RemotesPage.css";

function errorMessage(err: unknown): string {
  if (err instanceof ConnectError) return err.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

/**
 * Settings -> Remotes: lists configured SSH remotes with row-level Test/
 * Delete actions, and an "Add remote" form (ssh-remote-workspaces Phase 6,
 * Epic 6.1, Task 6.1.1a). Covers requirements.md AC1's UI half.
 */
export default function RemotesPage() {
  const { listRemotes, deleteRemote, testRemoteConnectionSaved } = useRemotesService();

  const [remotes, setRemotes] = useState<RemoteConfigInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);

  const [testingName, setTestingName] = useState<string | null>(null);
  const [testStatus, setTestStatus] = useState<Record<string, string>>({});

  const [confirmDeleteName, setConfirmDeleteName] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const list = await listRemotes();
      setRemotes(list);
    } catch (err) {
      setLoadError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  }, [listRemotes]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleSaved = useCallback((remote: RemoteConfigInfo) => {
    setRemotes((prev) => {
      const idx = prev.findIndex((r) => r.name === remote.name);
      if (idx === -1) return [...prev, remote];
      const next = [...prev];
      next[idx] = remote;
      return next;
    });
    setFormOpen(false);
  }, []);

  const handleTest = useCallback(
    async (name: string) => {
      setTestingName(name);
      setTestStatus((prev) => ({ ...prev, [name]: "" }));
      try {
        const result = await testRemoteConnectionSaved(name);
        if (result.success) {
          setTestStatus((prev) => ({ ...prev, [name]: "Connected" }));
        } else if (result.hostKeyUnknown) {
          setTestStatus((prev) => ({
            ...prev,
            [name]: "Host key changed or not yet trusted — re-add this remote to re-trust it.",
          }));
        } else {
          setTestStatus((prev) => ({ ...prev, [name]: result.errorMessage || "Connection failed." }));
        }
      } catch (err) {
        setTestStatus((prev) => ({ ...prev, [name]: errorMessage(err) }));
      } finally {
        setTestingName(null);
      }
    },
    [testRemoteConnectionSaved]
  );

  const handleDeleteConfirm = useCallback(
    async (name: string) => {
      setDeleting(true);
      try {
        await deleteRemote(name);
        setRemotes((prev) => prev.filter((r) => r.name !== name));
        setConfirmDeleteName(null);
      } catch (err) {
        setLoadError(errorMessage(err));
      } finally {
        setDeleting(false);
      }
    },
    [deleteRemote]
  );

  return (
    <>
      <PageViewTracker />
      <main id="main-content" className={styles.container}>
        <div className={styles.headerRow}>
          <div>
            <h1 className={styles.title}>Remotes</h1>
            <p className={styles.description}>
              Register a remote host to run sessions on a dedicated Linux box instead of this machine.
            </p>
          </div>
          {!formOpen && (
            <button className={styles.newBtn} onClick={() => setFormOpen(true)} data-testid="remotes-add-button">
              + Add remote
            </button>
          )}
        </div>

        {loadError && (
          <div className={styles.errorMessage} role="alert">
            {loadError}
          </div>
        )}

        {formOpen && (
          <div className={styles.formOverlay}>
            <AddRemoteForm onSaved={handleSaved} onCancel={() => setFormOpen(false)} />
          </div>
        )}

        <div className={styles.list}>
          {loading ? (
            <span className={styles.empty}>Loading…</span>
          ) : remotes.length === 0 ? (
            !formOpen && (
              <div className={styles.emptyState} data-testid="remotes-empty-state">
                <p className={styles.empty}>No remotes configured yet.</p>
                <p className={styles.description}>
                  Register a remote host to run sessions on a dedicated Linux box instead of this machine.
                </p>
                <button className={styles.newBtn} onClick={() => setFormOpen(true)}>
                  + Add remote
                </button>
              </div>
            )
          ) : (
            remotes.map((r) => (
              <div className={styles.listItem} key={r.name} data-testid={`remote-row-${r.name}`}>
                <div className={styles.listItemInfo}>
                  <div className={styles.listItemNameRow}>
                    <span className={styles.listItemName}>{r.name}</span>
                    <span className={styles.listItemMeta}>
                      {r.user}@{r.host}
                      {r.port ? `:${r.port}` : ""}
                    </span>
                  </div>
                  <span className={styles.listItemMeta}>{r.basePath}</span>
                  {testStatus[r.name] && (
                    <span
                      className={styles.listItemStatus}
                      role="status"
                      aria-live="polite"
                      data-testid={`remote-status-${r.name}`}
                    >
                      {testStatus[r.name]}
                    </span>
                  )}
                </div>
                <div className={styles.actionRow}>
                  {confirmDeleteName === r.name ? (
                    <>
                      <span className={styles.confirmText}>Remove {r.name}?</span>
                      <button
                        className={styles.confirmDeleteBtn}
                        onClick={() => handleDeleteConfirm(r.name)}
                        disabled={deleting}
                        data-testid={`remote-confirm-delete-${r.name}`}
                      >
                        {deleting ? "Removing…" : "Remove"}
                      </button>
                      <button
                        className={styles.smallBtn}
                        onClick={() => setConfirmDeleteName(null)}
                        disabled={deleting}
                        data-testid={`remote-cancel-delete-${r.name}`}
                      >
                        Cancel
                      </button>
                    </>
                  ) : (
                    <>
                      <button
                        className={styles.smallBtn}
                        onClick={() => handleTest(r.name)}
                        disabled={testingName === r.name}
                        data-testid={`remote-test-${r.name}`}
                      >
                        {testingName === r.name ? "Testing…" : "Test"}
                      </button>
                      <button
                        className={styles.deleteBtn}
                        onClick={() => setConfirmDeleteName(r.name)}
                        aria-label={`Delete ${r.name}`}
                        data-testid={`remote-delete-${r.name}`}
                      >
                        Delete
                      </button>
                    </>
                  )}
                </div>
              </div>
            ))
          )}
        </div>
      </main>
    </>
  );
}
