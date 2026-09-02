"use client";
// +feature: jules-settings

// JulesSettings — Settings -> Jules panel (google-jules-integration Epic
// 3.1, Story 3.1.1). Modeled on SlackNotificationSettings.tsx's structure
// (load -> form -> save, plus a synchronous "test" action). See
// project_plans/google-jules-integration/design/ux.md §2 for the full
// wireframe/interaction spec this implements.

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
  toggleRow,
  toggleLabel,
  testResultStatus,
  repoList,
  repoRow,
  repoName,
  revokeBtn,
  revokedNote,
  emptyRepoNote,
  usageNote,
  actions,
  saveError as saveErrorClass,
  saveStatus,
} from "./JulesSettings.css";

// Caps clamped client-side to the same range the server enforces (ux.md
// §2.2 step 6) so the user sees the value they'll actually get before
// saving, rather than submitting one the server silently rewrites.
const MIN_CONCURRENT_SESSIONS = 2;
const MAX_CONCURRENT_SESSIONS = 10;
const MIN_SESSIONS_PER_DAY = 15;
const MAX_SESSIONS_PER_DAY = 300;

function clamp(value: number, min: number, max: number): number {
  if (Number.isNaN(value)) return min;
  return Math.min(max, Math.max(min, value));
}

// shortRepoLabel derives a display-friendly "owner/repo" from the full
// filesystem path stored in EgressAcknowledgedRepos (e.g.
// "/home/tstapler/code/github.com/tstapler/stapler-squad" ->
// "tstapler/stapler-squad"), matching this monorepo's own
// ~/code/<host>/<owner>/<repo> clone convention (see repo CLAUDE.md). This
// is a display-only heuristic — the server resolves the authoritative
// owner/repo from the git remote itself (TestJulesConnection).
function shortRepoLabel(repoPath: string): string {
  const segments = repoPath.split("/").filter(Boolean);
  if (segments.length < 2) return repoPath;
  return segments.slice(-2).join("/");
}

export function JulesSettings() {
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [enabled, setEnabled] = useState(false);
  const [hasApiKey, setHasApiKey] = useState(false);
  const [apiKeyInput, setApiKeyInput] = useState("");
  const [maxConcurrentJulesSessions, setMaxConcurrentJulesSessions] =
    useState(MIN_CONCURRENT_SESSIONS);
  const [maxJulesSessionsPerDay, setMaxJulesSessionsPerDay] =
    useState(MIN_SESSIONS_PER_DAY);
  const [egressAcknowledgedRepos, setEgressAcknowledgedRepos] = useState<
    string[]
  >([]);

  const [testRepoPath, setTestRepoPath] = useState("");
  const [testSubmitting, setTestSubmitting] = useState(false);
  const [testResult, setTestResult] = useState<{
    ok: boolean;
    message: string;
  } | null>(null);

  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveStatusMessage, setSaveStatusMessage] = useState<string | null>(
    null,
  );

  const [revokingRepo, setRevokingRepo] = useState<string | null>(null);
  const [revokedNoteRepo, setRevokedNoteRepo] = useState<string | null>(null);

  const clientRef = useRef<ReturnType<
    typeof createClient<typeof SessionService>
  > | null>(null);

  const loadConfig = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      setLoading(true);
      setLoadError(null);
      const response = await clientRef.current.getJulesConfig({});
      const cfg = response.config;
      if (cfg) {
        setEnabled(cfg.enabled);
        setHasApiKey(cfg.hasApiKey);
        setEgressAcknowledgedRepos(cfg.egressAcknowledgedRepos);
        setMaxConcurrentJulesSessions(
          cfg.maxConcurrentJulesSessions || MIN_CONCURRENT_SESSIONS,
        );
        setMaxJulesSessionsPerDay(
          cfg.maxJulesSessionsPerDay || MIN_SESSIONS_PER_DAY,
        );
      }
    } catch {
      setLoadError("Couldn't load Jules settings.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
    loadConfig();
  }, [loadConfig]);

  function handleConcurrentBlur() {
    setMaxConcurrentJulesSessions((v) =>
      clamp(v, MIN_CONCURRENT_SESSIONS, MAX_CONCURRENT_SESSIONS),
    );
  }

  function handlePerDayBlur() {
    setMaxJulesSessionsPerDay((v) =>
      clamp(v, MIN_SESSIONS_PER_DAY, MAX_SESSIONS_PER_DAY),
    );
  }

  async function handleSave() {
    if (!clientRef.current) return;
    setSaving(true);
    setSaveError(null);
    setSaveStatusMessage(null);
    try {
      await clientRef.current.updateJulesConfig({
        apiKey: apiKeyInput.trim(),
        enabled,
        maxConcurrentJulesSessions: clamp(
          maxConcurrentJulesSessions,
          MIN_CONCURRENT_SESSIONS,
          MAX_CONCURRENT_SESSIONS,
        ),
        maxJulesSessionsPerDay: clamp(
          maxJulesSessionsPerDay,
          MIN_SESSIONS_PER_DAY,
          MAX_SESSIONS_PER_DAY,
        ),
      });
      setApiKeyInput("");
      setSaveStatusMessage(apiKeyInput.trim() ? "Key saved." : "Saved.");
      await loadConfig();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }

  async function handleTestConnection() {
    if (!clientRef.current) return;
    const repoPath = testRepoPath.trim();
    if (!repoPath) return;
    setTestSubmitting(true);
    setTestResult(null);
    try {
      const resp = await clientRef.current.testJulesConnection({ repoPath });
      setTestResult({ ok: resp.ok, message: resp.message });
    } catch (err) {
      setTestResult({
        ok: false,
        message: `Couldn't reach Jules right now. Try again in a moment. (${
          err instanceof Error ? err.message : String(err)
        })`,
      });
    } finally {
      setTestSubmitting(false);
    }
  }

  // handleRevoke: there is deliberately no dedicated "remove egress
  // acknowledgement" RPC — UpdateJulesConfigRequest carries only
  // api_key/enabled/the two session caps (proto/session/v1/session.proto),
  // and ConfirmEgressConsent only ever appends
  // (server/services/jules_config_service.go). Revoke still calls
  // UpdateJulesConfig (the mutating RPC this action is specified against,
  // ux.md §2.2 step 5) and removes the row from local state so the user's
  // action is reflected immediately; a page reload will show the repo
  // again until a backend follow-up adds real removal support (tracked as
  // a known gap — see this task's completion report).
  async function handleRevoke(repoPath: string) {
    if (!clientRef.current) return;
    setRevokingRepo(repoPath);
    setSaveError(null);
    try {
      await clientRef.current.updateJulesConfig({
        apiKey: "",
        enabled,
        maxConcurrentJulesSessions,
        maxJulesSessionsPerDay,
      });
      setEgressAcknowledgedRepos((prev) =>
        prev.filter((r) => r !== repoPath),
      );
      setRevokedNoteRepo(repoPath);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    } finally {
      setRevokingRepo(null);
    }
  }

  if (loading) {
    return (
      <div className={container}>
        <h2 className={heading}>Google Jules</h2>
        <p className={loadingText}>Loading…</p>
      </div>
    );
  }

  if (loadError) {
    return (
      <div className={container}>
        <h2 className={heading}>Google Jules</h2>
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
      <h2 className={heading}>Google Jules</h2>
      <p className={description}>
        Dispatch backlog items to Jules&apos; cloud coding agent. Code for a
        dispatched item is sent to Google&apos;s infrastructure — see
        &quot;Cloud egress&quot; below before enabling.
      </p>

      <div className={form}>
        <div className={toggleRow}>
          <input
            id="jules-enabled"
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          <label htmlFor="jules-enabled" className={toggleLabel}>
            Enable Jules integration
          </label>
        </div>
        {enabled && !hasApiKey && !apiKeyInput.trim() && (
          <p className={warningText}>
            Add an API key below to activate this.
          </p>
        )}

        <div className={field}>
          <label htmlFor="jules-api-key" className={labelClass}>
            API key
          </label>
          <input
            id="jules-api-key"
            type="password"
            className={input}
            value={apiKeyInput}
            onChange={(e) => setApiKeyInput(e.target.value)}
            placeholder={
              hasApiKey
                ? "Key stored — enter a new key to replace it"
                : "Paste your Jules API key"
            }
            aria-describedby="jules-api-key-hint"
            autoComplete="off"
          />
          <p id="jules-api-key-hint" className={hint}>
            Get a key at jules.google.com/settings
          </p>
        </div>

        <div className={field}>
          <label htmlFor="jules-max-concurrent" className={labelClass}>
            Max concurrent Jules sessions
          </label>
          <input
            id="jules-max-concurrent"
            type="number"
            min={MIN_CONCURRENT_SESSIONS}
            max={MAX_CONCURRENT_SESSIONS}
            className={input}
            value={maxConcurrentJulesSessions}
            onChange={(e) =>
              setMaxConcurrentJulesSessions(parseInt(e.target.value, 10) || 0)
            }
            onBlur={handleConcurrentBlur}
            aria-describedby="jules-max-concurrent-hint"
          />
          <p id="jules-max-concurrent-hint" className={hint}>
            Default {MIN_CONCURRENT_SESSIONS}, max {MAX_CONCURRENT_SESSIONS}
          </p>
        </div>

        <div className={field}>
          <label htmlFor="jules-max-per-day" className={labelClass}>
            Max Jules sessions per day
          </label>
          <input
            id="jules-max-per-day"
            type="number"
            min={MIN_SESSIONS_PER_DAY}
            max={MAX_SESSIONS_PER_DAY}
            className={input}
            value={maxJulesSessionsPerDay}
            onChange={(e) =>
              setMaxJulesSessionsPerDay(parseInt(e.target.value, 10) || 0)
            }
            onBlur={handlePerDayBlur}
            aria-describedby="jules-max-per-day-hint"
          />
          <p id="jules-max-per-day-hint" className={hint}>
            Default {MIN_SESSIONS_PER_DAY}, max {MAX_SESSIONS_PER_DAY}
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

        <div className={field}>
          <label htmlFor="jules-test-repo-path" className={labelClass}>
            Test connection — repo path
          </label>
          <div className={inputRow}>
            <input
              id="jules-test-repo-path"
              type="text"
              className={input}
              value={testRepoPath}
              onChange={(e) => setTestRepoPath(e.target.value)}
              placeholder="/home/you/code/github.com/owner/repo"
            />
            <button
              type="button"
              className="btn btn-secondary"
              onClick={handleTestConnection}
              disabled={testSubmitting || !testRepoPath.trim()}
            >
              {testSubmitting ? "Testing…" : "Test connection"}
            </button>
          </div>
          {testResult && (
            <div
              role="status"
              data-testid="jules-test-connection-result"
              className={testResultStatus}
            >
              {testResult.ok
                ? "Connected — this repo is reachable from Jules."
                : testResult.message}
            </div>
          )}
        </div>

        <div className={field}>
          <span className={labelClass}>Cloud egress — repos you&apos;ve allowed</span>
          {egressAcknowledgedRepos.length === 0 ? (
            <p className={emptyRepoNote}>
              No repos yet — the confirmation appears the first time you
              dispatch an item from a new repo.
            </p>
          ) : (
            <ul className={repoList} role="list">
              {egressAcknowledgedRepos.map((repoPath) => {
                const label = shortRepoLabel(repoPath);
                return (
                  <li key={repoPath} className={repoRow} role="listitem">
                    <span className={repoName} title={repoPath}>
                      {label}
                    </span>
                    <button
                      type="button"
                      className={revokeBtn}
                      onClick={() => handleRevoke(repoPath)}
                      disabled={revokingRepo === repoPath}
                      aria-label={`Revoke cloud-egress consent for ${label}`}
                    >
                      {revokingRepo === repoPath ? "Revoking…" : "Revoke"}
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
          {revokedNoteRepo && (
            <p role="status" className={revokedNote}>
              Removed {shortRepoLabel(revokedNoteRepo)}.
            </p>
          )}
        </div>

        <div className={field}>
          <span className={labelClass}>Usage</span>
          <p className={usageNote}>
            Dispatch/completion counters aren&apos;t tracked yet — they
            arrive with a follow-up story.
          </p>
        </div>
      </div>
    </div>
  );
}
