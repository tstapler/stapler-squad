"use client";
// +feature: llm-backend-settings

// LLMBackendSettings — Settings -> LLM Backends panel. Sets the global default
// backend, per-feature overrides, the consolette router URL and per-backend
// model-name maps over Get/UpdateLLMBackendSettings. Changes apply live.

import { useState, useEffect, useRef, useCallback } from "react";
import { LLMBackendService, type LLMBackendStatus } from "@/gen/session/v1/llm_backend_pb";
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
  inputTouch,
  buttonTouch,
  featureRow,
  featureName,
  select,
} from "./LLMBackendSettings.css";

const ALIASES = ["haiku", "sonnet", "opus"];
const DEFAULT_OPTION = "";

type ModelMaps = Record<string, Record<string, string>>;

function describeCaps(b: LLMBackendStatus): string {
  const caps = [
    b.supportsResume && "resume",
    b.supportsSystemPrompt && "system prompt",
    b.supportsToolRestriction && "tool restriction",
    b.supportsWorkdir && "workdir",
  ].filter(Boolean);
  return `${b.available ? "available" : "unavailable"}; ${caps.join(", ") || "plain text only"}`;
}

export function LLMBackendSettings() {
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [backends, setBackends] = useState<LLMBackendStatus[]>([]);
  const [featureKeys, setFeatureKeys] = useState<string[]>([]);
  const [defaultBackend, setDefaultBackend] = useState(DEFAULT_OPTION);
  const [perFeature, setPerFeature] = useState<Record<string, string>>({});
  const [consoletteUrl, setConsoletteUrl] = useState("");
  const [anthropicUrl, setAnthropicUrl] = useState("");
  const [modelMaps, setModelMaps] = useState<ModelMaps>({});
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);

  const clientRef = useRef<ReturnType<typeof createClient<typeof LLMBackendService>> | null>(null);

  const load = useCallback(async (silent = false) => {
    if (!clientRef.current) return;
    try {
      if (!silent) setLoading(true);
      setLoadError(null);
      const resp = await clientRef.current.getLLMBackendSettings({});
      const s = resp.settings;
      setBackends(resp.backends);
      setFeatureKeys(resp.featureKeys);
      setDefaultBackend(s?.defaultBackend ?? DEFAULT_OPTION);
      setPerFeature({ ...(s?.perFeature ?? {}) });
      setConsoletteUrl(s?.consoletteBaseUrl ?? "");
      setAnthropicUrl(s?.anthropicBaseUrl ?? "");
      const maps: ModelMaps = {};
      for (const m of s?.modelMaps ?? []) maps[m.backend] = { ...m.aliases };
      setModelMaps(maps);
    } catch {
      setLoadError("Couldn't load LLM backend settings.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    clientRef.current = createClient(LLMBackendService, createConnectTransport({ baseUrl: getApiBaseUrl() }));
    load();
  }, [load]);

  function setOverride(feature: string, backend: string) {
    setPerFeature((prev) => {
      const next = { ...prev };
      if (backend === DEFAULT_OPTION) delete next[feature];
      else next[feature] = backend;
      return next;
    });
  }

  function setAlias(backend: string, alias: string, value: string) {
    setModelMaps((prev) => ({ ...prev, [backend]: { ...prev[backend], [alias]: value } }));
  }

  async function handleSave() {
    if (!clientRef.current) return;
    setSaving(true);
    setSaveError(null);
    setStatus(null);
    try {
      await clientRef.current.updateLLMBackendSettings({
        settings: {
          defaultBackend,
          perFeature,
          consoletteBaseUrl: consoletteUrl.trim(),
          anthropicBaseUrl: anthropicUrl.trim(),
          modelMaps: Object.entries(modelMaps).map(([backend, aliases]) => ({
            backend,
            aliases: Object.fromEntries(Object.entries(aliases).filter(([, v]) => v.trim() !== "")),
          })),
        },
      });
      setStatus("Saved — applies to the next call, no restart needed.");
      await load(true);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <div className={container}>
        <h2 className={heading}>LLM Backends</h2>
        <p className={loadingText}>Loading…</p>
      </div>
    );
  }

  if (loadError) {
    return (
      <div className={container}>
        <h2 className={heading}>LLM Backends</h2>
        <div role="alert" className={saveErrorClass}>{loadError}</div>
        <div className={actions}>
          <button type="button" className={`btn btn-primary ${buttonTouch}`} onClick={() => load()}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  const options = (
    <>
      {backends.map((b) => (
        <option key={b.name} value={b.name}>
          {b.name} ({b.available ? "available" : "unavailable"})
        </option>
      ))}
    </>
  );
  const translatable = backends.filter((b) => b.name !== "claude");

  return (
    <div className={container}>
      <h2 className={heading}>LLM Backends</h2>
      <p className={description}>
        Choose which backend serves headless LLM features. If a backend is
        unavailable or lacks a capability a call needs (resume, tool
        restriction, working directory), the call falls back to claude and the
        reason is logged and recorded.
      </p>

      <div className={form}>
        <div className={field}>
          <label htmlFor="llm-default-backend" className={labelClass}>
            Default backend
          </label>
          <select
            id="llm-default-backend"
            className={select}
            value={defaultBackend}
            onChange={(e) => setDefaultBackend(e.target.value)}
          >
            <option value={DEFAULT_OPTION}>claude (default)</option>
            {options}
          </select>
        </div>

        <div className={field}>
          <span className={labelClass}>Per-feature overrides</span>
          {featureKeys.map((key) => (
            <div key={key} className={featureRow}>
              <label htmlFor={`llm-feature-${key}`} className={featureName}>
                {key}
              </label>
              <select
                id={`llm-feature-${key}`}
                className={select}
                value={perFeature[key] ?? DEFAULT_OPTION}
                onChange={(e) => setOverride(key, e.target.value)}
              >
                <option value={DEFAULT_OPTION}>use default</option>
                {options}
              </select>
            </div>
          ))}
        </div>

        <div className={field}>
          <label htmlFor="llm-consolette-url" className={labelClass}>
            Consolette router URL
          </label>
          <input
            id="llm-consolette-url"
            type="url"
            inputMode="url"
            className={`${input} ${inputTouch}`}
            value={consoletteUrl}
            onChange={(e) => setConsoletteUrl(e.target.value)}
            placeholder="http://127.0.0.1:47000"
            autoComplete="off"
          />
          <p className={hint}>Anthropic-compatible router. Reported unavailable while it does not answer.</p>
        </div>

        <div className={field}>
          <label htmlFor="llm-anthropic-url" className={labelClass}>
            Anthropic HTTP API base URL (rules generation)
          </label>
          <input
            id="llm-anthropic-url"
            type="url"
            inputMode="url"
            className={`${input} ${inputTouch}`}
            value={anthropicUrl}
            onChange={(e) => setAnthropicUrl(e.target.value)}
            placeholder="https://api.anthropic.com"
            autoComplete="off"
          />
          <p className={hint}>The capacity monitor probe always uses api.anthropic.com, regardless of this.</p>
        </div>

        {translatable.map((b) => (
          <div key={b.name} className={field}>
            <span className={labelClass}>{b.name} model names</span>
            {ALIASES.map((alias) => (
              <div key={alias} className={featureRow}>
                <label htmlFor={`llm-model-${b.name}-${alias}`} className={featureName}>
                  {alias}
                </label>
                <input
                  id={`llm-model-${b.name}-${alias}`}
                  type="text"
                  className={`${input} ${inputTouch}`}
                  value={modelMaps[b.name]?.[alias] ?? ""}
                  onChange={(e) => setAlias(b.name, alias, e.target.value)}
                  placeholder="backend default"
                  autoComplete="off"
                />
              </div>
            ))}
            <p className={hint}>{describeCaps(b)}</p>
          </div>
        ))}

        {saveError && (
          <p role="alert" className={saveErrorClass}>
            {saveError}
          </p>
        )}

        <div className={actions}>
          <button type="button" className={`btn btn-primary ${buttonTouch}`} onClick={handleSave} disabled={saving}>
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
