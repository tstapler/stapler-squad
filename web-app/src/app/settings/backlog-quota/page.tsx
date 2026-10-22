// analytics-exempt
"use client";
// +feature: settings-backlog-quota
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as remotes/page.tsx.

import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import {
  QuotaService,
  QuotaSettingsSchema,
  QuotaSource,
  UpdateQuotaConfigRequestSchema,
  type QuotaSettings,
} from "@/gen/session/v1/quota_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";
import { AlertConfirmDialog } from "@/components/settings/AlertConfirmDialog";
import { useUnsavedChangesGuard } from "./useUnsavedChangesGuard";
import * as styles from "./page.css";

type FieldKey = "budget" | "pause" | "resume" | "usd";
type ErrorKey = FieldKey | "sum";

interface FormValues {
  enabled: boolean;
  budget: string;
  pause: string;
  resume: string;
  usd: string;
}

type Errors = Partial<Record<ErrorKey, string>>;

type LoadState =
  | { phase: "loading" }
  | { phase: "failed"; reason: string }
  | { phase: "ready"; notConfigured: boolean };

interface Mismatch {
  fields: FieldKey[];
}

const FIELD_ORDER: FieldKey[] = ["budget", "pause", "resume", "usd"];
const FIELD_LABELS: Record<FieldKey, string> = {
  budget: "Assumed window token budget",
  pause: "Pause threshold (%)",
  resume: "Resume margin (%)",
  usd: "Default soft cost budget (USD)",
};
const NO_BUDGET_WARNING = "Gate is on but no budget is set, so the proactive signal never fires";
const PAUSED_DETAIL = /paused/i;
const SUM_ERROR_ID = "backlog-quota-sum-error";

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function toForm(s: QuotaSettings | undefined): FormValues | null {
  if (!s || s.pauseBelowHeadroomPct === undefined || s.resumeMarginPct === undefined) return null;
  return {
    enabled: s.enabled ?? false,
    budget: String(s.assumedWindowTokenBudget ?? BigInt(0)),
    pause: String(s.pauseBelowHeadroomPct),
    resume: String(s.resumeMarginPct),
    usd: s.defaultCostBudgetThresholdUsd === undefined ? "" : String(s.defaultCostBudgetThresholdUsd),
  };
}

function gateOnWithoutBudget(f: FormValues): boolean {
  return f.enabled && /^\s*0+\s*$/.test(f.budget);
}

function percentError(label: string, raw: string, fallback: number): string | undefined {
  if (/^\d+$/.test(raw.trim())) {
    const n = Number(raw);
    if (n >= 1 && n <= 100) return undefined;
  }
  return `${label} must be a whole number between 1 and 100; 0 would be replaced by the default (${fallback}).`;
}

function validate(f: FormValues): Errors {
  const errors: Errors = {};
  if (!/^\d+$/.test(f.budget.trim())) errors.budget = "Token budget must be a whole number, 0 or more.";
  errors.pause = percentError("Pause threshold", f.pause, 20);
  errors.resume = percentError("Resume margin", f.resume, 15);
  const usd = f.usd.trim();
  if (usd !== "" && !/^(\d+\.?\d*|\.\d+)$/.test(usd)) {
    errors.usd = "Default soft cost budget must be an amount of 0 or more, or empty for off.";
  }
  if (!errors.pause && !errors.resume && Number(f.pause) + Number(f.resume) > 100) {
    errors.sum = "Pause threshold plus resume margin must be at most 100";
  }
  return Object.fromEntries(Object.entries(errors).filter(([, v]) => v)) as Errors;
}

function toSettings(f: FormValues) {
  const usd = f.usd.trim();
  return create(QuotaSettingsSchema, {
    enabled: f.enabled,
    assumedWindowTokenBudget: BigInt(f.budget.trim()),
    pauseBelowHeadroomPct: Number(f.pause),
    resumeMarginPct: Number(f.resume),
    ...(usd === "" ? {} : { defaultCostBudgetThresholdUsd: Number(usd) }),
  });
}

function comparable(f: FormValues, key: FieldKey): string {
  const raw = f[key].trim();
  if (key === "budget") return BigInt(raw).toString();
  return raw === "" ? "" : String(Number(raw));
}

function differingFields(entered: FormValues, effective: FormValues): FieldKey[] {
  return FIELD_ORDER.filter((k) => comparable(entered, k) !== comparable(effective, k));
}

function describeEffective(f: FormValues): string {
  const budget = Number(f.budget) > 0 ? `budget ${Number(f.budget).toLocaleString("en-US")} tokens` : "no budget";
  const usd = f.usd.trim() === "" ? "" : `, warn at $${Number(f.usd).toFixed(2)}`;
  return `gate ${f.enabled ? "On" : "Off"}, ${budget}${usd}`;
}

function describeValue(key: FieldKey, f: FormValues): string {
  if (key === "usd") return f.usd.trim() === "" ? "off" : `$${Number(f.usd).toFixed(2)}`;
  return comparable(f, key);
}

function keepFocusedFieldVisible(el: HTMLElement) {
  const vv = window.visualViewport;
  if (vv && vv.height < window.innerHeight) el.scrollIntoView({ block: "center" });
}

interface QuotaFieldProps {
  id: FieldKey;
  value: string;
  error?: string;
  sumError?: string;
  hint: string;
  isDefault: boolean;
  inputMode: "numeric" | "decimal";
  enterKeyHint: "next" | "done";
  describedBy?: string;
  setRef: (el: HTMLInputElement | null) => void;
  onChange: (value: string) => void;
  onEnter: () => void;
}

function QuotaField(p: QuotaFieldProps) {
  const base = `backlog-quota-${p.id}`;
  const invalid = Boolean(p.error || p.sumError);
  const describedBy =
    [p.describedBy, `${base}-hint`, p.error && `${base}-error`, p.sumError && SUM_ERROR_ID].filter(Boolean).join(" ") ||
    undefined;
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && p.enterKeyHint === "next") {
      e.preventDefault();
      p.onEnter();
    }
  };
  return (
    <div className={styles.fieldRow}>
      <label htmlFor={base} className={styles.label}>
        {FIELD_LABELS[p.id]}
      </label>
      <input
        id={base}
        ref={p.setRef}
        data-testid={base}
        type="text"
        className={`${styles.input} ${invalid ? styles.inputInvalid : ""}`}
        value={p.value}
        inputMode={p.inputMode}
        pattern={p.inputMode === "numeric" ? "[0-9]*" : undefined}
        enterKeyHint={p.enterKeyHint}
        autoComplete="off"
        aria-invalid={invalid || undefined}
        aria-describedby={describedBy}
        onChange={(e) => p.onChange(e.target.value)}
        onFocus={(e) => keepFocusedFieldVisible(e.currentTarget)}
        onKeyDown={onKeyDown}
      />
      <p id={`${base}-hint`} className={styles.hint}>
        {p.hint}
        {p.isDefault && " (default value)"}
      </p>
      {p.error && (
        <p id={`${base}-error`} className={styles.fieldError}>
          {p.error}
        </p>
      )}
    </div>
  );
}

/**
 * Settings -> Backlog Quota: edits the account-wide quota gate and the default
 * soft cost budget over QuotaService. Saves apply live; "Applied" is shown only
 * after the read-back matches what was entered.
 */
export default function BacklogQuotaPage() {
  const client = useMemo(() => createClient(QuotaService, getConnectTransport()), []);
  const { flagList } = useFeatureFlags();

  const [load, setLoad] = useState<LoadState>({ phase: "loading" });
  const [baseline, setBaseline] = useState<FormValues | null>(null);
  const [form, setForm] = useState<FormValues | null>(null);
  const [errors, setErrors] = useState<Errors>({});
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [mismatch, setMismatch] = useState<Mismatch | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [warningNote, setWarningNote] = useState<string | null>(null);
  const [focusRequest, setFocusRequest] = useState<{ key: FieldKey } | null>(null);

  const fieldRefs = useRef<Partial<Record<FieldKey, HTMLInputElement | null>>>({});
  const mismatchRef = useRef<HTMLDivElement>(null);
  const hadMismatch = useRef(false);

  const dirty = Boolean(form && baseline && JSON.stringify(form) !== JSON.stringify(baseline));
  const guard = useUnsavedChangesGuard({ dirty, savePending: saving });

  const refresh = useCallback(async () => {
    setLoad({ phase: "loading" });
    try {
      const res = await client.getQuotaConfig({});
      const values = toForm(res.settings);
      if (!values || res.quotaSource === QuotaSource.UNSPECIFIED) {
        throw new Error("the server response was incomplete");
      }
      setBaseline(values);
      setForm(values);
      setLoad({ phase: "ready", notConfigured: res.quotaSource === QuotaSource.NOT_CONFIGURED });
    } catch (err) {
      setLoad({ phase: "failed", reason: errorMessage(err) });
    }
  }, [client]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (mismatch && !hadMismatch.current) mismatchRef.current?.focus();
    hadMismatch.current = mismatch !== null;
  }, [mismatch]);

  useEffect(() => {
    if (!focusRequest) return;
    fieldRefs.current[focusRequest.key]?.focus();
    setFocusRequest(null);
  }, [focusRequest]);

  const edit = (patch: Partial<FormValues>, errorKeys: ErrorKey[] = []) => {
    if (!form) return;
    const next = { ...form, ...patch };
    const before = gateOnWithoutBudget(form);
    const after = gateOnWithoutBudget(next);
    setWarningNote(before === after ? null : after ? NO_BUDGET_WARNING : "Budget warning cleared");
    setNote(null);
    setForm(next);
    if (errorKeys.length) setErrors((prev) => ({ ...prev, ...Object.fromEntries(errorKeys.map((k) => [k, undefined])) }));
  };

  const focusField = (key: ErrorKey) => fieldRefs.current[key === "sum" ? "pause" : key]?.focus();

  const submit = async () => {
    if (!form || !baseline || saving) return;
    const found = validate(form);
    if (Object.keys(found).length > 0) {
      setErrors(found);
      setSaveError(null);
      setMismatch(null);
      focusField(Object.keys(found)[0] as ErrorKey);
      return;
    }
    const resumesBacklog =
      baseline.enabled &&
      !form.enabled &&
      flagList.some((f) => f.name === "backlog" && PAUSED_DETAIL.test(f.statusDetail));
    setErrors({});
    setSaving(true);
    setSaveError(null);
    setNote(null);
    try {
      const res = await client.updateQuotaConfig(
        create(UpdateQuotaConfigRequestSchema, { settings: toSettings(form) }),
      );
      const readBack = toForm(res.settings);
      if (!readBack) throw new Error("the server did not return the stored settings");
      const fields = differingFields(form, readBack);
      setBaseline(readBack);
      if (fields.length > 0) {
        setMismatch({ fields });
      } else {
        setMismatch(null);
        setForm(readBack);
        setNote(resumesBacklog ? "Applied. Backlog resumed" : "Applied");
      }
    } catch (err) {
      setSaveError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const reloadEffective = () => {
    if (!baseline || !mismatch) return;
    setForm(baseline);
    setFocusRequest({ key: mismatch.fields[0] });
    setMismatch(null);
    setNote("Reloaded effective values");
  };

  const revert = () => {
    if (!baseline) return;
    setForm(baseline);
    setErrors({});
    setSaveError(null);
    setMismatch(null);
    setNote(null);
    setWarningNote(null);
  };

  if (load.phase !== "ready" || !form || !baseline) {
    return (
      <div className={styles.container} data-testid="backlog-quota-page">
        <PageViewTracker />
        <h1 className={styles.title}>Backlog Quota</h1>
        <div aria-busy={load.phase === "loading"} data-testid="backlog-quota-load">
          {load.phase === "failed" && (
            <div role="alert" className={styles.errorBox} data-testid="backlog-quota-load-error">
              <span>{`Couldn't load quota settings: ${load.reason}.`}</span>
              <div className={styles.errorActions}>
                {// analytics-exempt
                <button type="button" className={styles.button} onClick={() => void refresh()} data-testid="backlog-quota-retry">
                  Retry
                </button>}
              </div>
            </div>
          )}
        </div>
      </div>
    );
  }

  const errorEntries = Object.entries(errors).filter(([, v]) => v) as Array<[ErrorKey, string]>;
  const statusMessage = saving ? "Saving..." : (note ?? warningNote ?? (dirty ? "Unsaved changes" : ""));
  const showWarning = gateOnWithoutBudget(form);
  const fieldProps = (id: FieldKey, extra: Partial<QuotaFieldProps> & Pick<QuotaFieldProps, "hint">) => ({
    id,
    value: form[id],
    error: errors[id],
    sumError: id === "pause" || id === "resume" ? errors.sum : undefined,
    isDefault: load.notConfigured && id !== "usd" && id !== "budget",
    inputMode: "numeric" as const,
    enterKeyHint: "next" as const,
    setRef: (el: HTMLInputElement | null) => {
      fieldRefs.current[id] = el;
    },
    onChange: (value: string) => edit({ [id]: value }, [id, "sum"]),
    onEnter: () => fieldRefs.current[FIELD_ORDER[FIELD_ORDER.indexOf(id) + 1]]?.focus(),
    ...extra,
  });
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    void submit();
  };

  return (
    <div className={styles.container} data-testid="backlog-quota-page">
      <PageViewTracker />
      <h1 className={styles.title}>Backlog Quota</h1>
      <p className={styles.staticLine} data-testid="backlog-quota-effective">
        {`Effective now: ${describeEffective(baseline)}`}
      </p>
      {load.notConfigured && (
        <p className={styles.staticLine} data-testid="backlog-quota-not-configured">
          Quota is not configured. Defaults apply: gate Off, no budget.
        </p>
      )}
      {showWarning && (
        <div id="backlog-quota-warning" className={styles.warningBox} data-testid="backlog-quota-warning">
          <span aria-hidden="true">(!)</span>
          <span>{NO_BUDGET_WARNING}</span>
        </div>
      )}
      <form className={styles.form} onSubmit={onSubmit} noValidate data-testid="backlog-quota-form">
        <div className={`${styles.fieldRow} ${styles.fullWidth}`}>
          <label className={styles.switchRow}>
            <input
              type="checkbox"
              role="switch"
              className={styles.switchInput}
              checked={form.enabled}
              onChange={(e) => edit({ enabled: e.target.checked })}
              aria-describedby="backlog-quota-gate-help"
              data-testid="backlog-quota-enabled"
            />
            <span>{`Quota gate: ${form.enabled ? "On" : "Off"}`}</span>
          </label>
          <p id="backlog-quota-gate-help" className={styles.hint}>
            Turning the gate on also throttles foreground dispatch. Toggling counts as a manual override with a
            notification grace window. Turning it off resumes backlog if the gate had paused it.
          </p>
        </div>
        <div className={styles.fullWidth}>
          <QuotaField
            {...fieldProps("budget", {
              hint: "0 = proactive headroom signal disabled",
              describedBy: showWarning ? "backlog-quota-warning" : undefined,
            })}
          />
        </div>
        <QuotaField {...fieldProps("pause", { hint: "Range 1-100" })} />
        <QuotaField {...fieldProps("resume", { hint: "Range 1-100" })} />
        {errors.sum && (
          <p id={SUM_ERROR_ID} className={`${styles.fieldError} ${styles.fullWidth}`}>
            {errors.sum}
          </p>
        )}
        <div className={styles.fullWidth}>
          <QuotaField
            {...fieldProps("usd", {
              hint: "Empty = off; 0 is a valid threshold. Crossing is written to the server log; no notification is sent.",
              inputMode: "decimal",
              enterKeyHint: "done",
            })}
          />
        </div>
        <div className={`${styles.actions} ${styles.fullWidth}`}>
          <button
            type="submit"
            className={styles.primaryButton}
            aria-disabled={saving || undefined}
            data-testid="backlog-quota-save"
          >
            Save
          </button>
          {// analytics-exempt
          <button
            type="button"
            className={styles.button}
            disabled={!dirty}
            onClick={revert}
            data-testid="backlog-quota-revert"
          >
            Revert
          </button>}
          <div role="status" className={styles.status} data-testid="backlog-quota-status">
            {statusMessage}
          </div>
        </div>
      </form>

      {errorEntries.length > 0 ? (
        <div role="alert" className={styles.errorBox} data-testid="backlog-quota-errors">
          <span>Fix these before saving:</span>
          <ul className={styles.errorList}>
            {errorEntries.map(([key, message]) => (
              <li key={key}>
                {// analytics-exempt
                <button type="button" className={styles.errorLink} onClick={() => focusField(key)}>
                  {message}
                </button>}
              </li>
            ))}
          </ul>
        </div>
      ) : saveError ? (
        <div role="alert" className={styles.errorBox} data-testid="backlog-quota-save-error">
          <span>{`Couldn't save: ${saveError}`}</span>
          <div className={styles.errorActions}>
            {// analytics-exempt
            <button type="button" className={styles.button} onClick={() => void submit()}>
              Retry
            </button>}
          </div>
        </div>
      ) : mismatch ? (
        <div role="alert" tabIndex={-1} ref={mismatchRef} className={styles.errorBox} data-testid="backlog-quota-mismatch">
          <span>The server stored different values than you entered:</span>
          <ul className={styles.errorList}>
            {mismatch.fields.map((k) => (
              <li key={k}>{`${FIELD_LABELS[k]}: Entered ${describeValue(k, form)}, effective now ${describeValue(k, baseline)}`}</li>
            ))}
          </ul>
          <div className={styles.errorActions}>
            {// analytics-exempt
            <button type="button" className={styles.button} onClick={reloadEffective} data-testid="backlog-quota-mismatch-reload">
              Reload effective values
            </button>}
            {// analytics-exempt
            <button type="button" className={styles.button} onClick={() => void submit()} data-testid="backlog-quota-mismatch-save-again">
              Save again
            </button>}
          </div>
        </div>
      ) : null}

      {guard.promptOpen && (
        <AlertConfirmDialog
          heading="Discard unsaved changes?"
          confirmLabel="Discard"
          cancelLabel="Keep editing"
          onConfirm={guard.discard}
          onCancel={guard.keepEditing}
        >
          Your quota edits have not been saved.
        </AlertConfirmDialog>
      )}
    </div>
  );
}
