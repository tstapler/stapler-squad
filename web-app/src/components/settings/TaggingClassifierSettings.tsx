"use client";
// +feature: tagging-classifier-settings

// TaggingClassifierSettings — Settings -> Tag Classification panel. Load ->
// form -> save over Get/UpdateTaggingClassifierConfig, modeled on
// JulesSettings.tsx's structure. Covers the session-tag LLM fallback's model
// hierarchy: primary model plus ordered fallbacks (the free-proxy story —
// name the proxy's model as primary or fallback; ANTHROPIC_BASE_URL already
// flows to the headless subprocess).

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
  inputRow,
  input,
  hint,
  warningText,
  actions,
  saveStatus,
  saveError as saveErrorClass,
  fallbackRow,
  fallbackName,
  removeBtn,
  emptyNote,
} from "./TaggingClassifierSettings.css";

const MAX_FALLBACKS = 5;

export function TaggingClassifierSettings() {
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [model, setModel] = useState("");
  const [fallbacks, setFallbacks] = useState<string[]>([]);
  const [fallbackInput, setFallbackInput] = useState("");
  const [envOverrideActive, setEnvOverrideActive] = useState(false);

  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveStatusMessage, setSaveStatusMessage] = useState<string | null>(
    null,
  );

  const clientRef = useRef<ReturnType<
    typeof createClient<typeof SessionService>
  > | null>(null);

  const loadConfig = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      setLoading(true);
      setLoadError(null);
      const response =
        await clientRef.current.getTaggingClassifierConfig({});
      const cfg = response.config;
      if (cfg) {
        setModel(cfg.model || "");
        setFallbacks([...cfg.fallbackModels]);
      }
      setEnvOverrideActive(response.envOverrideActive);
    } catch {
      setLoadError("Couldn't load tag classification settings.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
    loadConfig();
  }, [loadConfig]);

  function handleAddFallback() {
    const name = fallbackInput.trim();
    if (!name || fallbacks.includes(name) || fallbacks.length >= MAX_FALLBACKS) return;
    setFallbacks([...fallbacks, name]);
    setFallbackInput("");
  }

  function handleRemoveFallback(name: string) {
    setFallbacks(fallbacks.filter((f) => f !== name));
  }

  async function handleSave() {
    if (!clientRef.current) return;
    setSaving(true);
    setSaveError(null);
    setSaveStatusMessage(null);
    try {
      const resp = await clientRef.current.updateTaggingClassifierConfig({
        config: { model: model.trim(), fallbackModels: fallbacks },
      });
      const saved = resp.config;
      if (saved) {
        setModel(saved.model || "");
        setFallbacks([...saved.fallbackModels]);
      }
      setSaveStatusMessage("Saved — applies to the next classification, no restart needed.");
      await loadConfig();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <div className={container}>
        <h2 className={heading}>Tag Classification</h2>
        <p className={loadingText}>Loading…</p>
      </div>
    );
  }

  if (loadError) {
    return (
      <div className={container}>
        <h2 className={heading}>Tag Classification</h2>
        <div role="alert">{loadError}</div>
        <div className={actions}>
          <button type="button" className="btn btn-primary" onClick={loadConfig}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={container}>
      <h2 className={heading}>Tag Classification</h2>
      <p className={description}>
        Sessions the sync tagging rules can&apos;t confidently tag fall back to
        a cheap LLM call — batched, at most one call per poll tick, and each
        session is classified once then left alone (re-run one manually from
        its tag editor).
      </p>

      {envOverrideActive && (
        <p className={warningText} role="alert">
          Environment overrides are active (STAPLER_SQUAD_TAGGING_MODEL or
          STAPLER_SQUAD_TAGGING_FALLBACK_MODELS). They win over this form —
          unset them for these saved values to take effect.
        </p>
      )}

      <div className={form}>
        <div className={field}>
          <label htmlFor="tagging-model" className={labelClass}>
            Primary model
          </label>
          <input
            id="tagging-model"
            type="text"
            className={input}
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder="haiku"
            aria-describedby="tagging-model-hint"
            autoComplete="off"
          />
          <p id="tagging-model-hint" className={hint}>
            Passed as --model to the headless CLI. Blank resets to the default
            (haiku). To use a free proxy, point ANTHROPIC_BASE_URL at it and
            name its model here.
          </p>
        </div>

        <div className={field}>
          <span className={labelClass}>Fallback models (tried in order)</span>
          {fallbacks.length === 0 ? (
            <p className={emptyNote}>
              None — a failed primary call degrades the batch to Unclassified.
            </p>
          ) : (
            <ul role="list">
              {fallbacks.map((name) => (
                <li key={name} className={fallbackRow} role="listitem">
                  <span className={fallbackName}>{name}</span>
                  <button
                    type="button"
                    className={removeBtn}
                    onClick={() => handleRemoveFallback(name)}
                    aria-label={`Remove fallback model ${name}`}
                  >
                    Remove
                  </button>
                </li>
              ))}
            </ul>
          )}
          <div className={inputRow}>
            <input
              id="tagging-fallback-input"
              type="text"
              className={input}
              value={fallbackInput}
              onChange={(e) => setFallbackInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  handleAddFallback();
                }
              }}
              placeholder="e.g. proxy-free"
              aria-label="Add a fallback model"
              autoComplete="off"
            />
            <button
              type="button"
              className="btn btn-secondary"
              onClick={handleAddFallback}
              disabled={
                !fallbackInput.trim() || fallbacks.length >= MAX_FALLBACKS
              }
            >
              Add
            </button>
          </div>
          <p className={hint}>
            First success wins; at most {MAX_FALLBACKS} fallbacks. The first
            model that answers keeps classifying even when the paid tier is
            unreachable.
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
          {saveStatusMessage && (
            <span role="status" className={saveStatus}>
              {saveStatusMessage}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
