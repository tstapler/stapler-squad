"use client";

// +feature: triggers-panel

import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { ConnectError, Code } from "@connectrpc/connect";
import { useWorkflows, WorkflowFormData } from "@/lib/hooks/useWorkflows";
import { WorkflowProto } from "@/gen/session/v1/session_pb";
import { protoTimestampToDate } from "@/lib/utils/timestamp";
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

function relativeTime(date: Date | null): string {
  if (!date) return "";
  const diffMs = Date.now() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  if (diffSec < 60) return "just now";
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  const diffDay = Math.floor(diffHr / 24);
  return `${diffDay}d ago`;
}

/** LastFired renders a trigger's last-fired indicator — shared between the desktop
 * table row and mobile card so both stay in sync without duplicating the ternary. */
function LastFired({ lastFiredAt, showTitle }: { lastFiredAt: WorkflowProto["lastFiredAt"]; showTitle?: boolean }) {
  const date = protoTimestampToDate(lastFiredAt);
  if (!date) return <span className={neverFired}>Never fired</span>;
  return (
    <span className={lastFired} title={showTitle ? date.toLocaleString() : undefined}>
      {relativeTime(date)}
    </span>
  );
}

/** TriggerToggle renders the enable/disable button — shared between the desktop table
 * row and mobile card. testId is optional since only the desktop row currently exposes
 * one (preserving each layout's existing locators exactly). */
function TriggerToggle({
  workflow, disabled, onToggle, testId,
}: { workflow: WorkflowProto; disabled: boolean; onToggle: () => void; testId?: string }) {
  return (
    <button
      className={`${toggle} ${workflow.enabled ? toggleOn : toggleOff}`}
      onClick={onToggle}
      disabled={disabled}
      aria-label={workflow.enabled ? `Disable trigger ${workflow.name || workflow.slug}` : `Enable trigger ${workflow.name || workflow.slug}`}
      data-testid={testId}
    >
      {workflow.enabled ? "ON" : "OFF"}
    </button>
  );
}

/** TriggerRowActions renders the Edit + History buttons — shared between the desktop
 * table row and mobile card. ariaLabel/testId props are optional since only the desktop
 * row currently exposes them (preserving each layout's existing locators exactly). */
function TriggerRowActions({
  workflow, isExpanded, onEdit, onToggleHistory, editAriaLabel, editTestId, historyTestId,
}: {
  workflow: WorkflowProto;
  isExpanded: boolean;
  onEdit: () => void;
  onToggleHistory: () => void;
  editAriaLabel?: string;
  editTestId?: string;
  historyTestId?: string;
}) {
  return (
    <div className={rowActions}>
      <button className={iconButton} onClick={onEdit} aria-label={editAriaLabel} data-testid={editTestId}>
        Edit
      </button>
      <button
        className={historyToggle}
        onClick={onToggleHistory}
        aria-expanded={isExpanded}
        data-testid={historyTestId}
      >
        {isExpanded ? "Hide history" : "History"}
      </button>
    </div>
  );
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
      const nextEnabled = !w.enabled;
      // Cron firing is gated on cronEnabled, not enabled (Scheduler never reads
      // enabled — see workflow_service.go's validateTriggerTypeFieldConsistency doc
      // comment). Keep the two fields in lockstep for cron-type rows so this single
      // toggle still controls whether a cron trigger actually fires; for
      // webhook/github_push rows only enabled matters, so cronEnabled is left alone.
      // expectedUpdatedAt (AC9's CAS precondition) is included so two tabs/users
      // clicking the same toggle in quick succession can't both silently win — the
      // second one gets a CodeAborted conflict instead.
      await updateWorkflow(w.id, {
        enabled: nextEnabled,
        ...(w.triggerType === "cron" && { cronEnabled: nextEnabled }),
        expectedUpdatedAt: w.updatedAt,
      });
      setLiveMessage(`${w.name || w.slug} ${w.enabled ? "disabled" : "enabled"}.`);
    } catch (e) {
      console.error("Failed to toggle trigger:", e);
      const isConflict = e instanceof ConnectError && e.code === Code.Aborted;
      const message = isConflict
        ? `${w.name || w.slug} was changed elsewhere — refresh and try again.`
        : `Failed to ${w.enabled ? "disable" : "enable"} ${w.name || w.slug}.`;
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
                const isExpanded = expandedId === w.id;
                return (
                  <Fragment key={w.id}>
                    <tr className={`${row} ${!w.enabled ? rowDisabled : ""}`}>
                      <td className={td}>
                        <span className={triggerName}>{w.name || w.slug}</span>
                        <span className={triggerSlug}>{w.slug}</span>
                      </td>
                      <td className={td}>
                        <span className={`${typeBadge} ${typeClass(w.triggerType)}`}>{typeLabel(w.triggerType)}</span>
                      </td>
                      <td className={td}>
                        <LastFired lastFiredAt={w.lastFiredAt} showTitle />
                      </td>
                      <td className={`${td} ${tdCenter}`}>
                        <TriggerToggle
                          workflow={w}
                          disabled={togglingId === w.id}
                          onToggle={() => void handleToggle(w)}
                          testId={`trigger-toggle-${w.id}`}
                        />
                      </td>
                      <td className={td}>
                        <TriggerRowActions
                          workflow={w}
                          isExpanded={isExpanded}
                          onEdit={() => openEdit(w)}
                          onToggleHistory={() => setExpandedId(isExpanded ? null : w.id)}
                          editAriaLabel={`Edit trigger ${w.name || w.slug}`}
                          editTestId={`trigger-edit-${w.id}`}
                          historyTestId={`trigger-history-toggle-${w.id}`}
                        />
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
          const isExpanded = expandedId === w.id;
          return (
            <div key={w.id} className={card} data-testid={`trigger-card-${w.id}`}>
              <div className={cardTop}>
                <div>
                  <span className={triggerName}>{w.name || w.slug}</span>
                  <span className={triggerSlug}>{w.slug}</span>
                </div>
                <TriggerToggle workflow={w} disabled={togglingId === w.id} onToggle={() => void handleToggle(w)} />
              </div>
              <div className={cardMeta}>
                <span className={`${typeBadge} ${typeClass(w.triggerType)}`}>{typeLabel(w.triggerType)}</span>
                <LastFired lastFiredAt={w.lastFiredAt} />
              </div>
              <TriggerRowActions
                workflow={w}
                isExpanded={isExpanded}
                onEdit={() => openEdit(w)}
                onToggleHistory={() => setExpandedId(isExpanded ? null : w.id)}
              />
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
