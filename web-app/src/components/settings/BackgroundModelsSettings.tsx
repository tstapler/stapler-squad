"use client";
// +feature: background-models-settings

// BackgroundModelsSettings — Settings -> Background Models panel. Load -> form -> save
// over Get/UpdateBackgroundModels. A blank field means "use the built-in default"
// (shown as the placeholder); changes apply on the next call, no restart.

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

type Overrides = Record<string, string>;

export function BackgroundModelsSettings() {
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [features, setFeatures] = useState<Overrides>({});
  const [stages, setStages] = useState<Overrides>({});
  const [effort, setEffort] = useState("");
  const [featureDefaults, setFeatureDefaults] = useState<Overrides>({});
  const [stageDefaults, setStageDefaults] = useState<Overrides>({});
  const [effortLevels, setEffortLevels] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);

  const clientRef = useRef<ReturnType<
    typeof createClient<typeof SessionService>
  > | null>(null);

  const load = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      setLoading(true);
      setLoadError(null);
      const resp = await clientRef.current.getBackgroundModels({});
      setFeatures({ ...resp.settings?.features });
      setStages({ ...resp.settings?.stages });
      setEffort(resp.settings?.effort ?? "");
      setFeatureDefaults({ ...resp.featureDefaults });
      setStageDefaults({ ...resp.stageDefaults });
      setEffortLevels([...resp.effortLevels]);
    } catch {
      setLoadError("Couldn't load background model settings.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
    load();
  }, [load]);

  async function handleSave() {
    if (!clientRef.current) return;
    setSaving(true);
    setSaveError(null);
    setStatus(null);
    try {
      await clientRef.current.updateBackgroundModels({
        settings: { features, stages, effort },
      });
      setStatus("Saved — applies to the next call, no restart needed.");
      await load();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }

  const renderRows = (
    prefix: string,
    values: Overrides,
    defaults: Overrides,
    set: (v: Overrides) => void,
  ) =>
    Object.keys(defaults)
      .sort()
      .map((key) => (
        <div className={field} key={key}>
          <label htmlFor={`${prefix}-${key}`} className={labelClass}>
            {key}
          </label>
          <input
            id={`${prefix}-${key}`}
            type="text"
            className={input}
            value={values[key] ?? ""}
            onChange={(e) => set({ ...values, [key]: e.target.value })}
            placeholder={defaults[key] || "account default"}
            autoComplete="off"
          />
        </div>
      ));

  if (loading) {
    return (
      <div className={container}>
        <h2 className={heading}>Background Models</h2>
        <p className={loadingText}>Loading…</p>
      </div>
    );
  }

  if (loadError) {
    return (
      <div className={container}>
        <h2 className={heading}>Background Models</h2>
        <div role="alert">{loadError}</div>
        <div className={actions}>
          <button type="button" className="btn btn-primary" onClick={load}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={container}>
      <h2 className={heading}>Background Models</h2>
      <p className={description}>
        Unattended LLM work is pinned to a cheaper model instead of the account
        default. Leave a field blank to use the built-in default shown in it.
        Opus is never a default — enter it here to opt in.
      </p>
      <div className={form}>
        <h3>Headless features</h3>
        {renderRows("bgm-feature", features, featureDefaults, setFeatures)}
        <h3>Backlog stages</h3>
        {renderRows("bgm-stage", stages, stageDefaults, setStages)}
        <div className={field}>
          <label htmlFor="bgm-effort" className={labelClass}>
            Effort (work sessions)
          </label>
          <select
            id="bgm-effort"
            className={input}
            value={effort}
            onChange={(e) => setEffort(e.target.value)}
          >
            <option value="">unset (CLI default)</option>
            {effortLevels.map((l) => (
              <option key={l} value={l}>
                {l}
              </option>
            ))}
          </select>
          <p className={hint}>
            Passed as --effort to claude work sessions only; other programs are
            unchanged.
          </p>
        </div>

        {saveError && (
          <p role="alert" className={saveErrorClass}>
            {saveError}
          </p>
        )}
        <div className={actions}>
          <button
            type="button"
            className="btn btn-primary"
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? "Saving…" : "Save"}
          </button>
          {status && (
            <span role="status" className={saveStatus}>
              {status}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
