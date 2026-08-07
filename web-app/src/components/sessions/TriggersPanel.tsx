"use client";

// +feature: triggers-panel

import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { useWorkflows, WorkflowFormData } from "@/lib/hooks/useWorkflows";
import { WorkflowProto } from "@/gen/session/v1/session_pb";
import { TriggerFormModal } from "./TriggerFormModal";
import { TriggerExecutionHistory } from "./TriggerExecutionHistory";
import {
  panel, header, titleRow, title, subtitle, refreshButton,
  headerButtons, visuallyHidden,
  tabs, tab, tabActive,
  error as errorClass, retryButton, toggleError,
  loading as loadingClass, empty, emptyStateLink,
  tableWrapper, table, th, td, tdCenter, row, rowDisabled,
  triggerName, triggerSlug,
  typeBadge, typeCron, typeGithubPush, typeWebhook,
  lastFired, neverFired,
  toggle, toggleOn, toggleOff,
  rowActions, iconButton,
  addButton, mobileAddFab, headerButtonsHiddenOnMobile,
  cardList, card, cardTop, cardMeta,
  rowCount,
  historyToggle, historyWrapper,
} from "./TriggersPanel.css";

type TriggerFilter = "all" | "github_push" | "cron" | "webhook";

const TRIGGER_TYPES: TriggerFilter[] = ["github_push", "cron", "webhook"];

function typeLabel(t: string): string {
  switch (t) {
    case "github_push": return "GitHub Push";
    case "cron": return "Cron";
    case "webhook": return "Webhook";
    default: return t;
  }
}

function typeClass(t: string): string {
  switch (t) {
    case "github_push": return typeGithubPush;
    case "cron": return typeCron;
    case "webhook": return typeWebhook;
    default: return typeCron;
  }
}

function relativeTime(iso: string | undefined): string {
  if (!iso) return "";
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const diffMs = Date.now() - then;
  const diffSec = Math.floor(diffMs / 1000);
  if (diffSec < 60) return "just now";
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  const diffDay = Math.floor(diffHr / 24);
  return `${diffDay}d ago`;
}

function timestampToIso(ts: WorkflowProto["lastFiredAt"]): string | undefined {
  if (!ts) return undefined;
  // Timestamp proto has seconds (bigint) + nanos.
  const ms = Number(ts.seconds) * 1000 + Math.floor(ts.nanos / 1e6);
  return new Date(ms).toISOString();
}

/**
 * TriggersPanel — the trigger config surface (webhook-triggers Epic 7.1): list of
 * cron/github_push/webhook Workflow rows with type badges, last-fired timestamp,
 * enable/disable toggle, and create/edit via TriggerFormModal. Extends
 * ApprovalRulesPanel's established shape (hits column → last-fired, source badge →
 * type badge, toggle, mobile FAB) per research/ux.md.
 *
 * Plain "manual" (@slug, non-triggered) Workflow rows are managed by the existing
 * WorkflowsPanel (/workflows) and are intentionally excluded here — this panel is
 * scoped to rows with an inbound/scheduled activation mechanism.
 */
export function TriggersPanel() {
  const { workflows, loading, error, createWorkflow, updateWorkflow, refresh } = useWorkflows();

  const [typeFilter, setTypeFilter] = useState<TriggerFilter>("all");
  const [modalOpen, setModalOpen] = useState(false);
  const [editTrigger, setEditTrigger] = useState<WorkflowProto | null>(null);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [liveMessage, setLiveMessage] = useState("");
  const [togglingId, setTogglingId] = useState<string | null>(null);
  const [toggleErrorMsg, setToggleErrorMsg] = useState<string | null>(null);
  const toggleErrorTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (toggleErrorTimeoutRef.current) clearTimeout(toggleErrorTimeoutRef.current);
    };
  }, []);

  const triggers = useMemo(
    () => workflows.filter((w) => TRIGGER_TYPES.includes(w.triggerType as TriggerFilter)),
    [workflows]
  );

  const visibleTriggers = useMemo(
    () => (typeFilter === "all" ? triggers : triggers.filter((w) => w.triggerType === typeFilter)),
    [triggers, typeFilter]
  );

  function openCreate() {
    setEditTrigger(null);
    setModalOpen(true);
  }

  function openEdit(w: WorkflowProto) {
    setEditTrigger(w);
    setModalOpen(true);
  }

  async function handleSave(data: WorkflowFormData) {
    if (editTrigger) {
      await updateWorkflow(editTrigger.id, data);
    } else {
      await createWorkflow(data);
    }
  }

  async function handleToggle(w: WorkflowProto) {
    setTogglingId(w.id);
    setToggleErrorMsg(null);
    try {
      await updateWorkflow(w.id, { cronEnabled: !w.cronEnabled });
      setLiveMessage(`${w.name || w.slug} ${w.cronEnabled ? "disabled" : "enabled"}.`);
    } catch (e) {
      console.error("Failed to toggle trigger:", e);
      const message = `Failed to ${w.cronEnabled ? "disable" : "enable"} ${w.name || w.slug}.`;
      setLiveMessage(message);
      // Visible (not just screen-reader-only) error for sighted users — transient,
      // clears itself after a few seconds or on the next toggle attempt.
      setToggleErrorMsg(message);
      if (toggleErrorTimeoutRef.current) clearTimeout(toggleErrorTimeoutRef.current);
      toggleErrorTimeoutRef.current = setTimeout(
        () => setToggleErrorMsg((prev) => (prev === message ? null : prev)),
        5000
      );
    } finally {
      setTogglingId(null);
    }
  }

  return (
    <div className={panel} data-testid="triggers-panel">
      <div className={header}>
        <div className={titleRow}>
          <h2 className={title}>Triggers</h2>
          <div className={headerButtons}>
            <button
              className={`${addButton} ${headerButtonsHiddenOnMobile}`}
              onClick={openCreate}
              data-testid="add-trigger-button"
            >
              + Add Trigger
            </button>
            <button
              onClick={() => void refresh()}
              className={refreshButton}
              disabled={loading}
              aria-label="Refresh triggers"
              title="Refresh triggers"
            >
              {loading ? "⟳" : "↻"}
            </button>
          </div>
        </div>
        <p className={subtitle}>
          Automated triggers create sessions from GitHub pushes, schedules, or inbound webhooks.
        </p>
      </div>

      <span aria-live="polite" aria-atomic="true" className={visuallyHidden}>
        {liveMessage}
      </span>

      {toggleErrorMsg && (
        <div className={toggleError} role="alert" data-testid="trigger-toggle-error">
          {toggleErrorMsg}
        </div>
      )}

      <div className={tabs}>
        {(["all", ...TRIGGER_TYPES] as TriggerFilter[]).map((t) => {
          const count = t === "all" ? triggers.length : triggers.filter((w) => w.triggerType === t).length;
          return (
            <button
              key={t}
              className={`${tab} ${typeFilter === t ? tabActive : ""}`}
              onClick={() => setTypeFilter(t)}
              data-testid={`trigger-tab-${t}`}
            >
              {t === "all" ? "All" : typeLabel(t)} ({count})
            </button>
          );
        })}
      </div>

      {error && (
        <div className={errorClass}>
          Failed to load triggers: {error.message}
          <button onClick={() => void refresh()} className={retryButton}>Retry</button>
        </div>
      )}

      <div className={tableWrapper}>
        {loading && visibleTriggers.length === 0 ? (
          <div className={loadingClass}>Loading triggers…</div>
        ) : visibleTriggers.length === 0 ? (
          <div className={empty} data-testid="triggers-empty-state">
            <p>No triggers configured{typeFilter !== "all" ? ` for ${typeLabel(typeFilter)}` : ""} yet.</p>
            <p>
              <button
                className={emptyStateLink}
                onClick={openCreate}
              >
                Add Trigger
              </button>
              {" "}to create sessions automatically from GitHub pushes, schedules, or webhooks.
            </p>
          </div>
        ) : (
          <table className={table}>
            <thead>
              <tr>
                <th className={th}>Name</th>
                <th className={th}>Type</th>
                <th className={th}>Last fired</th>
                <th className={th}>Enabled</th>
                <th className={th}></th>
              </tr>
            </thead>
            <tbody>
              {visibleTriggers.map((w) => {
                const lastFiredIso = timestampToIso(w.lastFiredAt);
                const isExpanded = expandedId === w.id;
                return (
                  <Fragment key={w.id}>
                    <tr className={`${row} ${!w.cronEnabled ? rowDisabled : ""}`}>
                      <td className={td}>
                        <span className={triggerName}>{w.name || w.slug}</span>
                        <span className={triggerSlug}>{w.slug}</span>
                      </td>
                      <td className={td}>
                        <span className={`${typeBadge} ${typeClass(w.triggerType)}`}>{typeLabel(w.triggerType)}</span>
                      </td>
                      <td className={td}>
                        {lastFiredIso ? (
                          <span className={lastFired} title={new Date(lastFiredIso).toLocaleString()}>
                            {relativeTime(lastFiredIso)}
                          </span>
                        ) : (
                          <span className={neverFired}>Never fired</span>
                        )}
                      </td>
                      <td className={`${td} ${tdCenter}`}>
                        <button
                          className={`${toggle} ${w.cronEnabled ? toggleOn : toggleOff}`}
                          onClick={() => void handleToggle(w)}
                          disabled={togglingId === w.id}
                          aria-label={w.cronEnabled ? `Disable trigger ${w.name || w.slug}` : `Enable trigger ${w.name || w.slug}`}
                          data-testid={`trigger-toggle-${w.id}`}
                        >
                          {w.cronEnabled ? "ON" : "OFF"}
                        </button>
                      </td>
                      <td className={td}>
                        <div className={rowActions}>
                          <button
                            className={iconButton}
                            onClick={() => openEdit(w)}
                            aria-label={`Edit trigger ${w.name || w.slug}`}
                            data-testid={`trigger-edit-${w.id}`}
                          >
                            Edit
                          </button>
                          <button
                            className={historyToggle}
                            onClick={() => setExpandedId(isExpanded ? null : w.id)}
                            aria-expanded={isExpanded}
                            data-testid={`trigger-history-toggle-${w.id}`}
                          >
                            {isExpanded ? "Hide history" : "History"}
                          </button>
                        </div>
                      </td>
                    </tr>
                    {isExpanded && (
                      <tr>
                        <td className={td} colSpan={5}>
                          <div className={historyWrapper}>
                            <TriggerExecutionHistory workflowId={w.id} />
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* ── Mobile card layout ── */}
      <div className={cardList}>
        {visibleTriggers.map((w) => {
          const lastFiredIso = timestampToIso(w.lastFiredAt);
          const isExpanded = expandedId === w.id;
          return (
            <div key={w.id} className={card} data-testid={`trigger-card-${w.id}`}>
              <div className={cardTop}>
                <div>
                  <span className={triggerName}>{w.name || w.slug}</span>
                  <span className={triggerSlug}>{w.slug}</span>
                </div>
                <button
                  className={`${toggle} ${w.cronEnabled ? toggleOn : toggleOff}`}
                  onClick={() => void handleToggle(w)}
                  disabled={togglingId === w.id}
                  aria-label={w.cronEnabled ? `Disable trigger ${w.name || w.slug}` : `Enable trigger ${w.name || w.slug}`}
                >
                  {w.cronEnabled ? "ON" : "OFF"}
                </button>
              </div>
              <div className={cardMeta}>
                <span className={`${typeBadge} ${typeClass(w.triggerType)}`}>{typeLabel(w.triggerType)}</span>
                {lastFiredIso ? (
                  <span className={lastFired}>{relativeTime(lastFiredIso)}</span>
                ) : (
                  <span className={neverFired}>Never fired</span>
                )}
              </div>
              <div className={rowActions}>
                <button className={iconButton} onClick={() => openEdit(w)}>Edit</button>
                <button className={historyToggle} onClick={() => setExpandedId(isExpanded ? null : w.id)} aria-expanded={isExpanded}>
                  {isExpanded ? "Hide history" : "History"}
                </button>
              </div>
              {isExpanded && (
                <div className={historyWrapper}>
                  <TriggerExecutionHistory workflowId={w.id} />
                </div>
              )}
            </div>
          );
        })}
      </div>

      {visibleTriggers.length > 0 && (
        <div className={rowCount}>
          {visibleTriggers.length} trigger{visibleTriggers.length !== 1 ? "s" : ""}
          {typeFilter !== "all" && ` (filtered from ${triggers.length} total)`}
        </div>
      )}

      <button
        className={mobileAddFab}
        onClick={openCreate}
        aria-label="Add trigger"
        data-testid="add-trigger-fab"
      >
        +
      </button>

      <TriggerFormModal
        open={modalOpen}
        editTrigger={editTrigger}
        onSave={handleSave}
        onClose={() => { setModalOpen(false); setEditTrigger(null); }}
      />
    </div>
  );
}
