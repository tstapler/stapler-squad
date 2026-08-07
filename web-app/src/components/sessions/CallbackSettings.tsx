"use client";

// +feature: callback-settings

import { useState } from "react";
import { useCallbackConfig, CallbackConfigUpdateData } from "@/lib/hooks/useCallbackConfig";
import {
  panel, header, title, subtitle,
  fieldRow, fieldLabelRow, fieldLabel, configuredBadge, notConfiguredBadge,
  inputRow, input, clearButton, hint,
  saveRow, saveButton, statusMessage, errorBanner,
} from "./CallbackSettings.css";

interface CallbackFieldConfig {
  key: keyof CallbackConfigUpdateData;
  configuredKey: "onSessionCompleteConfigured" | "onSessionStaleConfigured" | "onQueueItemCreatedConfigured";
  label: string;
  hint: string;
  testId: string;
}

const FIELDS: CallbackFieldConfig[] = [
  {
    key: "onSessionCompleteUrl",
    configuredKey: "onSessionCompleteConfigured",
    label: "On session complete",
    hint: "POSTed when a trigger-created session's backlog item transitions to done.",
    testId: "callback-on-session-complete",
  },
  {
    key: "onSessionStaleUrl",
    configuredKey: "onSessionStaleConfigured",
    label: "On session stale",
    hint: "POSTed when a session is marked stuck/stale.",
    testId: "callback-on-session-stale",
  },
  {
    key: "onQueueItemCreatedUrl",
    configuredKey: "onQueueItemCreatedConfigured",
    label: "On review-queue item created",
    hint: "POSTed when a new item lands in the human review queue.",
    testId: "callback-on-queue-item-created",
  },
];

/**
 * CallbackSettings — the outbound-callback URL config surface (webhook-triggers
 * Epic 5.1/7.3). Same masked-placeholder-on-edit convention as TriggerFormModal's
 * webhook secret field: GetCallbackConfig only ever returns booleans ("is a URL
 * configured"), never the URL itself, so this form never displays a real URL —
 * only whether one is set, plus an editable field to replace or clear it.
 */
export function CallbackSettings() {
  const { config, loading, error, updateConfig } = useCallbackConfig();
  const [edits, setEdits] = useState<Record<string, string | undefined>>({});
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [savedMessage, setSavedMessage] = useState<string | null>(null);

  function setEdit(key: string, value: string | undefined) {
    setEdits((prev) => ({ ...prev, [key]: value }));
    setSavedMessage(null);
  }

  const hasEdits = Object.keys(edits).length > 0;

  async function handleSave() {
    setSaving(true);
    setSaveError(null);
    try {
      const payload: CallbackConfigUpdateData = {};
      for (const f of FIELDS) {
        if (edits[f.key] !== undefined) {
          (payload as Record<string, string>)[f.key] = edits[f.key] as string;
        }
      }
      await updateConfig(payload);
      setEdits({});
      setSavedMessage("Callback settings saved.");
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : "Failed to save callback settings.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className={panel} data-testid="callback-settings">
      <div className={header}>
        <h2 className={title}>Outbound Callbacks</h2>
        <p className={subtitle}>
          Global lifecycle-event webhooks. URLs are never displayed once saved — only whether one is configured.
        </p>
      </div>

      {error && <div className={errorBanner}>Failed to load callback settings: {error.message}</div>}
      {saveError && <div className={errorBanner} role="alert">{saveError}</div>}

      {loading && !config ? (
        <p className={hint}>Loading…</p>
      ) : (
        <>
          {FIELDS.map((f) => {
            const configured = config?.[f.configuredKey] ?? false;
            const isEdited = edits[f.key] !== undefined;
            return (
              <div className={fieldRow} key={f.key}>
                <div className={fieldLabelRow}>
                  <label className={fieldLabel} htmlFor={`cb-${f.key}`}>{f.label}</label>
                  <span className={configured ? configuredBadge : notConfiguredBadge}>
                    {configured ? "Configured" : "Not configured"}
                  </span>
                </div>
                <div className={inputRow}>
                  <input
                    id={`cb-${f.key}`}
                    className={input}
                    type="url"
                    data-testid={f.testId}
                    placeholder={configured ? "•••• (unchanged) — enter a new URL to replace it" : "https://example.com/hook"}
                    value={isEdited ? (edits[f.key] ?? "") : ""}
                    onChange={(e) => setEdit(f.key, e.target.value)}
                  />
                  {configured && (
                    <button
                      type="button"
                      className={clearButton}
                      onClick={() => setEdit(f.key, "")}
                      data-testid={`${f.testId}-clear`}
                    >
                      {isEdited && edits[f.key] === "" ? "Will clear" : "Clear"}
                    </button>
                  )}
                </div>
                <span className={hint}>{f.hint}</span>
              </div>
            );
          })}

          <div className={saveRow}>
            <button
              className={saveButton}
              onClick={() => void handleSave()}
              disabled={saving || !hasEdits}
              data-testid="callback-settings-save"
            >
              {saving ? "Saving…" : "Save"}
            </button>
            {savedMessage && <span className={statusMessage}>{savedMessage}</span>}
          </div>
        </>
      )}
    </div>
  );
}
