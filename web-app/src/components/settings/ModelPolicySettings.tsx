"use client";
// +feature: model-policy-settings

// ModelPolicySettings — Settings -> Model Policy panel over Get/UpdateModelPolicy.
// Per-feature model and effort for background LLM work; saves apply on the next call.

import { useState, useEffect, useRef, useCallback } from "react";
import { SessionService } from "@/gen/session/v1/session_pb";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl } from "@/lib/config";
import {
  container,
  heading,
  description,
  loadingText,
  form,
  field,
  label as labelClass,
  input,
  hint,
  actions,
  saveStatus,
  saveError as saveErrorClass,
} from "./TaggingClassifierSettings.css";

const FIELDS: { key: string; label: string; hint: string }[] = [
  { key: "work", label: "Work sessions", hint: "Default-pipeline backlog work sessions." },
  { key: "review", label: "Review", hint: "Headless review when the pipeline pins no model." },
  { key: "completion_narrative", label: "Completion narrative", hint: "Session completion summaries." },
  { key: "handoff_summary", label: "Handoff summary", hint: "Session handoff summaries." },
  { key: "intent_parse", label: "Intent parse", hint: "Backlog intent parsing." },
  { key: "pr_description", label: "PR description", hint: "Drafted PR descriptions." },
  { key: "background_effort", label: "Effort", hint: "low, medium, high, xhigh, max, or none." },
];

type Values = Record<string, string>;

export function ModelPolicySettings() {
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [values, setValues] = useState<Values>({});
  const [defaults, setDefaults] = useState<Values>({});
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  const load = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      setLoading(true);
      setLoadError(null);
      const resp = await clientRef.current.getModelPolicy({});
      setValues({ ...(resp.effective?.values ?? {}) });
      setDefaults({ ...(resp.defaults?.values ?? {}) });
    } catch {
      setLoadError("Couldn't load model policy settings.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    clientRef.current = createClient(SessionService, createConnectTransport({ baseUrl: getApiBaseUrl() }));
    load();
  }, [load]);

  async function handleSave() {
    if (!clientRef.current) return;
    setSaving(true);
    setSaveError(null);
    setStatus(null);
    try {
      const resp = await clientRef.current.updateModelPolicy({ policy: { values } });
      setValues({ ...(resp.effective?.values ?? {}) });
      setStatus("Saved — applies to the next call, no restart needed.");
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <div className={container}>
        <h2 className={heading}>Model Policy</h2>
        <p className={loadingText}>Loading…</p>
      </div>
    );
  }
  if (loadError) {
    return (
      <div className={container}>
        <h2 className={heading}>Model Policy</h2>
        <div role="alert">{loadError}</div>
        <div className={actions}>
          <button type="button" className="btn btn-primary" onClick={load}>Retry</button>
        </div>
      </div>
    );
  }

  return (
    <div className={container}>
      <h2 className={heading}>Model Policy</h2>
      <p className={description}>
        Models are <code>family:haiku</code>, <code>family:sonnet</code>, <code>family:opus</code>, a
        concrete model ID, or <code>none</code> for the account default. Opus is used only if set here.
      </p>
      <div className={form}>
        {FIELDS.map((f) => (
          <div className={field} key={f.key}>
            <label htmlFor={`mp-${f.key}`} className={labelClass}>{f.label}</label>
            <input
              id={`mp-${f.key}`}
              data-testid={`model-policy-${f.key}`}
              type="text"
              className={input}
              value={values[f.key] ?? ""}
              onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
              placeholder={defaults[f.key]}
              autoComplete="off"
            />
            <p className={hint}>{f.hint} Blank resets to {defaults[f.key]}.</p>
          </div>
        ))}
        {saveError && <p role="alert" className={saveErrorClass}>{saveError}</p>}
        <div className={actions}>
          <button type="button" className="btn btn-primary" onClick={handleSave} disabled={saving}>
            {saving ? "Saving…" : "Save"}
          </button>
          {status && <span role="status" className={saveStatus}>{status}</span>}
        </div>
      </div>
    </div>
  );
}
