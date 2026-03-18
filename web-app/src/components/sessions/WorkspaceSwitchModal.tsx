"use client";

import { useState, useEffect, useMemo, useRef } from "react";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import {
  AvailableWorkspaceTargets,
  BookmarkTarget,
  RevisionTarget,
  WorktreeTarget,
  VCSType,
  VCSInfo,
} from "@/gen/session/v1/types_pb";
import {
  WorkspaceSwitchType,
  ChangeStrategy,
} from "@/gen/session/v1/types_pb";
import styles from "./WorkspaceSwitchModal.module.css";

interface WorkspaceSwitchModalProps {
  sessionId: string;
  sessionName: string;
  baseUrl: string;
  onClose: () => void;
  onSwitched?: () => void;
}

function getVcsIcon(type: VCSType): string {
  switch (type) {
    case VCSType.VCS_TYPE_GIT:
      return "🌿";
    case VCSType.VCS_TYPE_JUJUTSU:
      return "🔄";
    default:
      return "📁";
  }
}

function getVcsName(type: VCSType): string {
  switch (type) {
    case VCSType.VCS_TYPE_GIT:
      return "Git";
    case VCSType.VCS_TYPE_JUJUTSU:
      return "Jujutsu";
    default:
      return "VCS";
  }
}

function formatTimestamp(timestamp: Date | undefined): string {
  if (!timestamp) return "";
  const now = new Date();
  const diff = now.getTime() - timestamp.getTime();
  const hours = Math.floor(diff / (1000 * 60 * 60));
  const days = Math.floor(hours / 24);

  if (days > 0) return `${days}d ago`;
  if (hours > 0) return `${hours}h ago`;
  const minutes = Math.floor(diff / (1000 * 60));
  return minutes > 0 ? `${minutes}m ago` : "just now";
}

export function WorkspaceSwitchModal({
  sessionId,
  sessionName,
  baseUrl,
  onClose,
  onSwitched,
}: WorkspaceSwitchModalProps) {
  const [targets, setTargets] = useState<AvailableWorkspaceTargets | null>(null);
  const [vcsInfo, setVcsInfo] = useState<VCSInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState("");
  const [selectedTarget, setSelectedTarget] = useState<{
    type: "bookmark" | "revision" | "worktree";
    value: string;
  } | null>(null);
  const [changeStrategy, setChangeStrategy] = useState<ChangeStrategy>(
    ChangeStrategy.KEEP_AS_WIP
  );
  const [isSwitching, setSwitching] = useState(false);
  const [switchError, setSwitchError] = useState<string | null>(null);

  const modalRef = useRef<HTMLDivElement>(null);
  useFocusTrap(modalRef, true);

  const client = useMemo(
    () =>
      createPromiseClient(
        SessionService,
        createConnectTransport({ baseUrl })
      ),
    [baseUrl]
  );

  const fetchData = async () => {
    setLoading(true);
    setError(null);

    try {
      const [infoResponse, targetsResponse] = await Promise.all([
        client.getWorkspaceInfo({ id: sessionId }),
        client.listWorkspaceTargets({ id: sessionId }),
      ]);

      if (infoResponse.error) {
        setError(infoResponse.error);
        return;
      }
      if (targetsResponse.error) {
        setError(targetsResponse.error);
        return;
      }

      setVcsInfo(infoResponse.vcsInfo ?? null);
      setTargets(targetsResponse.targets ?? null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load workspace data");
      console.error("Error fetching workspace data:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, [sessionId, baseUrl]);

  // Filter targets based on search input
  const filteredBookmarks = useMemo(() => {
    if (!targets?.bookmarks) return [];
    const lowerFilter = filter.toLowerCase();
    return targets.bookmarks.filter((b) =>
      b.name.toLowerCase().includes(lowerFilter)
    );
  }, [targets?.bookmarks, filter]);

  const filteredRevisions = useMemo(() => {
    if (!targets?.recentRevisions) return [];
    const lowerFilter = filter.toLowerCase();
    return targets.recentRevisions.filter(
      (r) =>
        r.shortId.toLowerCase().includes(lowerFilter) ||
        r.description.toLowerCase().includes(lowerFilter) ||
        r.author.toLowerCase().includes(lowerFilter)
    );
  }, [targets?.recentRevisions, filter]);

  const filteredWorktrees = useMemo(() => {
    if (!targets?.worktrees) return [];
    const lowerFilter = filter.toLowerCase();
    return targets.worktrees.filter(
      (w) =>
        w.name.toLowerCase().includes(lowerFilter) ||
        w.path.toLowerCase().includes(lowerFilter)
    );
  }, [targets?.worktrees, filter]);

  const handleSwitch = async () => {
    if (!selectedTarget) return;

    setSwitching(true);
    setSwitchError(null);

    try {
      let switchType: WorkspaceSwitchType;
      switch (selectedTarget.type) {
        case "bookmark":
          switchType = WorkspaceSwitchType.REVISION;
          break;
        case "revision":
          switchType = WorkspaceSwitchType.REVISION;
          break;
        case "worktree":
          switchType = WorkspaceSwitchType.WORKTREE;
          break;
      }

      const response = await client.switchWorkspace({
        id: sessionId,
        switchType,
        target: selectedTarget.value,
        changeStrategy,
        createIfMissing: false,
      });

      if (!response.success) {
        setSwitchError(response.message || "Failed to switch workspace");
        return;
      }

      // Success!
      onSwitched?.();
      onClose();
    } catch (err) {
      setSwitchError(err instanceof Error ? err.message : "Failed to switch workspace");
      console.error("Error switching workspace:", err);
    } finally {
      setSwitching(false);
    }
  };

  // Handle escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const renderContent = () => {
    if (loading) {
      return <div className={styles.loading}>Loading workspace information...</div>;
    }

    if (error) {
      return (
        <div className={styles.error}>
          <span className={styles.errorIcon}>⚠️</span>
          <span className={styles.errorMessage}>{error}</span>
          <button className={styles.retryButton} onClick={fetchData}>
            Retry
          </button>
        </div>
      );
    }

    return (
      <>
        {/* Current workspace info */}
        {vcsInfo && (
          <div className={styles.currentInfo}>
            <div className={styles.currentInfoTitle}>Current Workspace</div>
            <div className={styles.currentInfoValue}>
              {vcsInfo.currentBookmark || vcsInfo.currentRevision || "(detached)"}
            </div>
          </div>
        )}

        {/* Warning if there are uncommitted changes */}
        {vcsInfo?.hasUncommittedChanges && (
          <div className={styles.warningBanner}>
            <span className={styles.warningIcon}>⚠️</span>
            <span className={styles.warningText}>
              You have uncommitted changes ({vcsInfo.modifiedFileCount} files).
              Choose how to handle them below.
            </span>
          </div>
        )}

        {/* Filter input */}
        <input
          type="text"
          className={styles.filterInput}
          placeholder="Filter branches, revisions, worktrees..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          autoFocus
        />

        {/* Bookmarks/Branches section */}
        {filteredBookmarks.length > 0 && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>
              <span className={styles.sectionIcon}>⎇</span>
              {targets?.vcsType === VCSType.VCS_TYPE_JUJUTSU ? "Bookmarks" : "Branches"}
            </h3>
            <div className={styles.targetList}>
              {filteredBookmarks.map((bookmark) => {
                const isCurrent = bookmark.name === vcsInfo?.currentBookmark;
                const isSelected =
                  selectedTarget?.type === "bookmark" &&
                  selectedTarget.value === bookmark.name;
                return (
                  <div
                    key={bookmark.name}
                    className={`${styles.targetItem} ${isSelected ? styles.selected : ""} ${isCurrent ? styles.current : ""}`}
                    onClick={() =>
                      !isCurrent &&
                      setSelectedTarget({ type: "bookmark", value: bookmark.name })
                    }
                  >
                    <span className={styles.targetItemIcon}>⎇</span>
                    <span className={styles.targetItemName}>{bookmark.name}</span>
                    {bookmark.revisionId && (
                      <span className={styles.targetItemMeta}>
                        {bookmark.revisionId.substring(0, 8)}
                      </span>
                    )}
                    {isCurrent && <span className={styles.currentBadge}>Current</span>}
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Recent revisions section */}
        {filteredRevisions.length > 0 && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>
              <span className={styles.sectionIcon}>📝</span>
              Recent Revisions
            </h3>
            <div className={styles.targetList}>
              {filteredRevisions.map((revision) => {
                const isCurrent = revision.isCurrent;
                const isSelected =
                  selectedTarget?.type === "revision" &&
                  selectedTarget.value === revision.id;
                return (
                  <div
                    key={revision.id}
                    className={`${styles.targetItem} ${isSelected ? styles.selected : ""} ${isCurrent ? styles.current : ""}`}
                    onClick={() =>
                      !isCurrent &&
                      setSelectedTarget({ type: "revision", value: revision.id })
                    }
                  >
                    <span className={styles.targetItemIcon}>📝</span>
                    <span className={styles.targetItemName}>{revision.shortId}</span>
                    <span className={styles.targetItemMeta}>
                      {revision.description.substring(0, 40)}
                      {revision.description.length > 40 ? "..." : ""}
                    </span>
                    {isCurrent && <span className={styles.currentBadge}>Current</span>}
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Worktrees section */}
        {filteredWorktrees.length > 0 && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>
              <span className={styles.sectionIcon}>📂</span>
              Worktrees
            </h3>
            <div className={styles.targetList}>
              {filteredWorktrees.map((worktree) => {
                const isCurrent = worktree.isCurrent;
                const isSelected =
                  selectedTarget?.type === "worktree" &&
                  selectedTarget.value === worktree.path;
                return (
                  <div
                    key={worktree.path}
                    className={`${styles.targetItem} ${isSelected ? styles.selected : ""} ${isCurrent ? styles.current : ""}`}
                    onClick={() =>
                      !isCurrent &&
                      setSelectedTarget({ type: "worktree", value: worktree.path })
                    }
                  >
                    <span className={styles.targetItemIcon}>📂</span>
                    <span className={styles.targetItemName}>{worktree.name}</span>
                    <span className={styles.targetItemMeta}>{worktree.bookmark}</span>
                    {isCurrent && <span className={styles.currentBadge}>Current</span>}
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Empty state */}
        {filteredBookmarks.length === 0 &&
          filteredRevisions.length === 0 &&
          filteredWorktrees.length === 0 && (
            <div className={styles.emptyState}>
              {filter
                ? "No matching targets found"
                : "No branches, revisions, or worktrees available"}
            </div>
          )}

        {/* Change strategy section */}
        {vcsInfo?.hasUncommittedChanges && selectedTarget && (
          <div className={styles.strategySection}>
            <h3 className={styles.sectionTitle}>
              <span className={styles.sectionIcon}>⚙️</span>
              Handle Uncommitted Changes
            </h3>
            <div className={styles.strategyOptions}>
              <div
                className={`${styles.strategyOption} ${changeStrategy === ChangeStrategy.KEEP_AS_WIP ? styles.selected : ""}`}
                onClick={() => setChangeStrategy(ChangeStrategy.KEEP_AS_WIP)}
              >
                <input
                  type="radio"
                  className={styles.strategyRadio}
                  checked={changeStrategy === ChangeStrategy.KEEP_AS_WIP}
                  readOnly
                />
                <div className={styles.strategyContent}>
                  <div className={styles.strategyLabel}>Keep as WIP</div>
                  <div className={styles.strategyDescription}>
                    Save changes as a separate WIP revision (JJ) or stash them (Git)
                  </div>
                </div>
              </div>
              <div
                className={`${styles.strategyOption} ${changeStrategy === ChangeStrategy.BRING_ALONG ? styles.selected : ""}`}
                onClick={() => setChangeStrategy(ChangeStrategy.BRING_ALONG)}
              >
                <input
                  type="radio"
                  className={styles.strategyRadio}
                  checked={changeStrategy === ChangeStrategy.BRING_ALONG}
                  readOnly
                />
                <div className={styles.strategyContent}>
                  <div className={styles.strategyLabel}>Bring Along</div>
                  <div className={styles.strategyDescription}>
                    Keep changes in the new location (JJ) or stash pop after switch (Git)
                  </div>
                </div>
              </div>
              <div
                className={`${styles.strategyOption} ${changeStrategy === ChangeStrategy.ABANDON ? styles.selected : ""}`}
                onClick={() => setChangeStrategy(ChangeStrategy.ABANDON)}
              >
                <input
                  type="radio"
                  className={styles.strategyRadio}
                  checked={changeStrategy === ChangeStrategy.ABANDON}
                  readOnly
                />
                <div className={styles.strategyContent}>
                  <div className={styles.strategyLabel}>Abandon</div>
                  <div className={styles.strategyDescription}>
                    Discard all uncommitted changes (cannot be undone!)
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Switch error */}
        {switchError && (
          <div className={styles.error}>
            <span className={styles.errorIcon}>❌</span>
            <span className={styles.errorMessage}>{switchError}</span>
          </div>
        )}
      </>
    );
  };

  return (
    <div className={styles.modalOverlay} onClick={onClose}>
      <div
        className={styles.modal}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="workspace-switch-title"
        ref={modalRef}
      >
        {/* Header */}
        <div className={styles.header}>
          <h2 className={styles.headerTitle} id="workspace-switch-title">
            <span className={styles.vcsIcon}>
              {getVcsIcon(targets?.vcsType ?? VCSType.VCS_TYPE_UNSPECIFIED)}
            </span>
            Switch Workspace - {sessionName}
          </h2>
          <button className={styles.closeButton} onClick={onClose}>
            ✕
          </button>
        </div>

        {/* Content */}
        <div className={styles.content}>{renderContent()}</div>

        {/* Footer */}
        <div className={styles.footer}>
          <div />
          <div className={styles.footerButtons}>
            <button className={styles.cancelButton} onClick={onClose}>
              Cancel
            </button>
            <button
              className={styles.switchButton}
              disabled={!selectedTarget || isSwitching}
              onClick={handleSwitch}
            >
              {isSwitching ? "Switching..." : "Switch Workspace"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
