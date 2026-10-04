"use client";

// analytics-exempt
// +feature: settings-programs
import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { SessionService, type ProgramConfigProto } from "@/gen/session/v1/session_pb";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { useProbeProgram, type ProbeUiState } from "@/lib/hooks/useProbeProgram";
import { ProbeStatusBadge } from "@/components/ui/ProbeStatusBadge";
import { FlagCombobox } from "@/components/ui/FlagCombobox";
import { UnknownFlagsWarning } from "@/components/ui/UnknownFlagsWarning";
import { AvailableFlags } from "@/components/ui/AvailableFlags";
import { validateFlags } from "@/lib/flags/validateFlags";
import {
  container,
  heading,
  headerRow,
  loadingText,
  emptyText,
  programRow,
  programInfo,
  programTitleRow,
  programLabel,
  badgeBuiltin,
  badgeCustom,
  programDesc,
  programMeta,
  programActions,
  formCard,
  formTitle,
  formFields,
  field,
  label as labelClass,
  input,
  formActions,
  envVarTable,
  envVarRow,
  envVarInput,
  deleteBtn,
  confirmDeleteBtn,
  fieldError,
  commandRow,
  checkButton,
  hintText,
} from "./ProgramsManager.css";

// Explains the absence of suggestions; states not listed need no hint.
const FLAGS_HINTS: Partial<Record<ProbeUiState["kind"], string>> = {
  idle: "Check the command above to enable flag suggestions",
  needsConfirm: "Suggestions appear after Check reads the flags.",
};

const PROGRAM_ID_RE = /^[\w-]+$/;

interface EnvVar {
  key: string;
  value: string;
}

interface ProgramFormData {
  id: string;
  label: string;
  command: string;
  cliFlags: string;
  description: string;
  envVars: EnvVar[];
}

const emptyForm: ProgramFormData = {
  id: "",
  label: "",
  command: "",
  cliFlags: "",
  description: "",
  envVars: [],
};

export function ProgramsManager() {
  const [programs, setPrograms] = useState<ProgramConfigProto[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [idError, setIdError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [formData, setFormData] = useState<ProgramFormData>(emptyForm);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

  const probe = useProbeProgram(formData.command, "program-config");
  const flagsHint = FLAGS_HINTS[probe.state.kind];
  // A disabled button drops focus in browsers, so hand it back once the run settles.
  const checkButtonRef = useRef<HTMLButtonElement>(null);
  const checkClicked = useRef(false);
  const wasChecking = useRef(false);
  useEffect(() => {
    const checking = probe.state.kind === "checking";
    if (wasChecking.current && !checking && checkClicked.current && document.activeElement === document.body) {
      checkButtonRef.current?.focus();
    }
    if (!checking) checkClicked.current = false;
    wasChecking.current = checking;
  }, [probe.state.kind]);
  const [flagsFocused, setFlagsFocused] = useState(false);
  const probedFlags = useMemo(() => (probe.state.kind === "found" ? probe.state.flags : []), [probe.state]);
  // The token still being typed is not judged until the field blurs or a space follows it.
  const settledFlags =
    flagsFocused && !/\s$/.test(formData.cliFlags) ? formData.cliFlags.replace(/\S+$/, "") : formData.cliFlags;
  const unknownFlags = useMemo(() => validateFlags(settledFlags, probedFlags), [settledFlags, probedFlags]);
  const flagsDescribedBy =
    [flagsHint && "prog-flags-hint", unknownFlags.length > 0 && "prog-flags-warning"].filter(Boolean).join(" ") ||
    undefined;

  const getClient = useCallback(() => {
    return createClient(SessionService, getConnectTransport());
  }, []);

  const fetchPrograms = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const client = getClient();
      const res = await client.listProgramsConfig({});
      setPrograms(res.programs ?? []);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Failed to load programs: ${msg}`);
    } finally {
      setLoading(false);
    }
  }, [getClient]);

  useEffect(() => {
    fetchPrograms();
  }, [fetchPrograms]);

  const handleOpenCreate = () => {
    setFormData(emptyForm);
    setIsEditing(false);
    setIdError(null);
    setError(null);
    setShowForm(true);
  };

  const handleOpenEdit = (p: ProgramConfigProto) => {
    const envVars: EnvVar[] = p.env
      ? Object.entries(p.env).map(([key, value]) => ({ key, value }))
      : [];
    setFormData({
      id: p.id,
      label: p.label,
      command: p.command,
      cliFlags: p.cliFlags,
      description: p.description,
      envVars,
    });
    setIsEditing(true);
    setIdError(null);
    setError(null);
    setShowForm(true);
  };

  const handleDuplicate = (p: ProgramConfigProto) => {
    const envVars: EnvVar[] = p.env
      ? Object.entries(p.env).map(([key, value]) => ({ key, value }))
      : [];
    setFormData({
      id: `${p.id}-copy`,
      label: `${p.label} (Copy)`,
      command: p.command,
      cliFlags: p.cliFlags,
      description: p.description,
      envVars,
    });
    setIsEditing(false);
    setIdError(null);
    setError(null);
    setShowForm(true);
  };

  const handleCancelForm = () => {
    setShowForm(false);
    setFormData(emptyForm);
    setIdError(null);
  };

  const validateForm = (): boolean => {
    let valid = true;
    setIdError(null);

    const trimmedId = formData.id.trim();
    if (!trimmedId) {
      setIdError("Program ID is required.");
      valid = false;
    } else if (!PROGRAM_ID_RE.test(trimmedId)) {
      setIdError("Program ID can only contain letters, digits, hyphens, and underscores.");
      valid = false;
    } else {
      const isBuiltin = programs.some((p) => p.isBuiltin && p.id.toLowerCase() === trimmedId.toLowerCase());
      if (isBuiltin) {
        setIdError("Cannot override a built-in program ID.");
        valid = false;
      }
    }

    if (!formData.label.trim()) {
      setError("Program label is required.");
      valid = false;
    }

    if (!formData.command.trim()) {
      setError("Program command is required.");
      valid = false;
    }

    return valid;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!validateForm()) return;

    setError(null);
    setSuccess(null);

    const envMap: Record<string, string> = {};
    for (const ev of formData.envVars) {
      if (ev.key.trim()) {
        envMap[ev.key.trim()] = ev.value;
      }
    }

    try {
      const client = getClient();
      await client.upsertProgramConfig({
        program: {
          id: formData.id.trim(),
          label: formData.label.trim(),
          command: formData.command.trim(),
          cliFlags: formData.cliFlags.trim(),
          description: formData.description.trim(),
          env: envMap,
          isBuiltin: false,
        },
      });

      setSuccess(`Successfully ${isEditing ? "updated" : "created"} program "${formData.label.trim()}".`);
      setShowForm(false);
      setFormData(emptyForm);
      await fetchPrograms();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Failed to save program: ${msg}`);
    }
  };

  const handleDelete = async (id: string) => {
    setDeletingId(id);
    setError(null);
    setSuccess(null);

    try {
      const client = getClient();
      await client.deleteProgramConfig({ id });
      setSuccess(`Program "${id}" deleted.`);
      setConfirmDeleteId(null);
      await fetchPrograms();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Failed to delete program: ${msg}`);
    } finally {
      setDeletingId(null);
    }
  };

  const addEnvVar = () => {
    setFormData((prev) => ({
      ...prev,
      envVars: [...prev.envVars, { key: "", value: "" }],
    }));
  };

  const updateEnvVar = (index: number, key: string, value: string) => {
    setFormData((prev) => {
      const updated = [...prev.envVars];
      updated[index] = { key, value };
      return { ...prev, envVars: updated };
    });
  };

  const removeEnvVar = (index: number) => {
    setFormData((prev) => ({
      ...prev,
      envVars: prev.envVars.filter((_, i) => i !== index),
    }));
  };

  return (
    <div className={container} data-testid="programs-manager">
      <div className={headerRow}>
        <h2 className={heading}>Program Configurations</h2>
        {!showForm && (
          <button
            type="button"
            className={input}
            style={{ maxWidth: "fit-content", cursor: "pointer", fontWeight: 600 }}
            onClick={handleOpenCreate}
            data-testid="add-program-btn"
          >
            + Add Program
          </button>
        )}
      </div>

      {error && <div className={fieldError} data-testid="programs-error">{error}</div>}
      {success && <div style={{ color: "#10b981", fontSize: "0.875rem" }} data-testid="programs-success">{success}</div>}

      {showForm && (
        <form className={formCard} onSubmit={handleSubmit} data-testid="program-form">
          <h3 className={formTitle}>{isEditing ? "Edit Program" : "Add Program"}</h3>
          <div className={formFields}>
            <div className={field}>
              <label className={labelClass} htmlFor="prog-id">ID (unique identifier)</label>
              <input
                id="prog-id"
                type="text"
                className={input}
                value={formData.id}
                disabled={isEditing}
                onChange={(e) => setFormData({ ...formData, id: e.target.value })}
                placeholder="e.g. my-custom-agent"
                data-testid="prog-id-input"
              />
              {idError && <span className={fieldError}>{idError}</span>}
            </div>

            <div className={field}>
              <label className={labelClass} htmlFor="prog-label">Label (display name)</label>
              <input
                id="prog-label"
                type="text"
                className={input}
                value={formData.label}
                onChange={(e) => setFormData({ ...formData, label: e.target.value })}
                placeholder="e.g. My Custom Agent"
                data-testid="prog-label-input"
              />
            </div>

            <div className={field}>
              <label className={labelClass} htmlFor="prog-command">Executable Command / Path</label>
              <div className={commandRow}>
                <input
                  id="prog-command"
                  type="text"
                  className={input}
                  value={formData.command}
                  onChange={(e) => setFormData({ ...formData, command: e.target.value })}
                  onBlur={() => probe.check()}
                  onKeyDown={(e) => {
                    if (e.key !== "Enter") return;
                    e.preventDefault();
                    probe.check({ immediate: true });
                  }}
                  autoCapitalize="off"
                  autoCorrect="off"
                  spellCheck={false}
                  aria-describedby="prog-command-status-text"
                  placeholder="e.g. /usr/local/bin/my-agent or python -m myagent"
                  data-testid="prog-command-input"
                />
                <button
                  ref={checkButtonRef}
                  type="button"
                  className={checkButton}
                  onClick={() => {
                    checkClicked.current = true;
                    probe.check({ explicit: true });
                  }}
                  disabled={probe.state.kind === "checking"}
                  aria-busy={probe.state.kind === "checking"}
                  data-testid="prog-command-check"
                >
                  Check
                </button>
              </div>
              <ProbeStatusBadge
                state={probe.state}
                checkedToken={probe.checkedToken}
                onRetry={() => probe.check({ immediate: true })}
                onConfirm={() => probe.check({ explicit: true })}
                testId="prog-command-status"
                id="prog-command-status-text"
              />
            </div>

            <div className={field}>
              <label className={labelClass} htmlFor="prog-flags">Default CLI Flags (optional)</label>
              <div onFocus={() => setFlagsFocused(true)} onBlur={() => setFlagsFocused(false)}>
                <FlagCombobox
                  id="prog-flags"
                  className={input}
                  value={formData.cliFlags}
                  onChange={(cliFlags) => setFormData({ ...formData, cliFlags })}
                  flags={probedFlags}
                  placeholder="e.g. --verbose --auto"
                  testId="prog-flags-input"
                  describedBy={flagsDescribedBy}
                />
              </div>
              {flagsHint && (
                <span id="prog-flags-hint" className={hintText} data-testid="prog-flags-hint">
                  {flagsHint}
                </span>
              )}
              <UnknownFlagsWarning
                id="prog-flags-warning"
                testId="prog-flags-warning"
                program={probe.checkedToken}
                unknown={unknownFlags}
              />
              <AvailableFlags flags={probedFlags} testId="prog-available-flags" />
            </div>

            <div className={field}>
              <label className={labelClass} htmlFor="prog-desc">Description (optional)</label>
              <input
                id="prog-desc"
                type="text"
                className={input}
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                placeholder="e.g. Internal code review assistant"
                data-testid="prog-desc-input"
              />
            </div>

            <div className={field}>
              <label className={labelClass}>Environment Variables (optional)</label>
              <div className={envVarTable}>
                {formData.envVars.map((ev, idx) => (
                  <div key={idx} className={envVarRow}>
                    <input
                      type="text"
                      className={envVarInput}
                      placeholder="KEY"
                      value={ev.key}
                      onChange={(e) => updateEnvVar(idx, e.target.value, ev.value)}
                    />
                    <input
                      type="text"
                      className={envVarInput}
                      placeholder="VALUE"
                      value={ev.value}
                      onChange={(e) => updateEnvVar(idx, ev.key, e.target.value)}
                    />
                    <button
                      type="button"
                      className={deleteBtn}
                      onClick={() => removeEnvVar(idx)}
                    >
                      Remove
                    </button>
                  </div>
                ))}
                <button
                  type="button"
                  className={input}
                  style={{ width: "fit-content", cursor: "pointer", fontSize: "0.75rem" }}
                  onClick={addEnvVar}
                >
                  + Add Env Var
                </button>
              </div>
            </div>
          </div>

          <div className={formActions}>
            <button
              type="submit"
              className={input}
              style={{ backgroundColor: "#2563eb", color: "#ffffff", fontWeight: 600, cursor: "pointer" }}
              data-testid="save-program-btn"
            >
              Save Program
            </button>
            <button
              type="button"
              className={input}
              style={{ cursor: "pointer" }}
              onClick={handleCancelForm}
              data-testid="cancel-program-btn"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {loading ? (
        <div className={loadingText}>Loading programs...</div>
      ) : programs.length === 0 ? (
        <div className={emptyText}>No programs configured.</div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem" }}>
          {programs.map((p) => (
            <div key={p.id} className={programRow} data-testid={`program-row-${p.id}`}>
              <div className={programInfo}>
                <div className={programTitleRow}>
                  <span className={programLabel}>{p.label}</span>
                  <span className={p.isBuiltin ? badgeBuiltin : badgeCustom}>
                    {p.isBuiltin ? "Built-in" : "Custom"}
                  </span>
                </div>
                <div className={programDesc}>
                  Command: <code>{p.command}</code> {p.cliFlags && ` (Flags: ${p.cliFlags})`}
                </div>
                {p.description && <div className={programMeta}>{p.description}</div>}
              </div>

              <div className={programActions}>
                <button
                  type="button"
                  className={input}
                  style={{ padding: "0.25rem 0.5rem", fontSize: "0.75rem", cursor: "pointer" }}
                  onClick={() => handleDuplicate(p)}
                  data-testid={`duplicate-program-${p.id}`}
                >
                  Duplicate
                </button>

                {!p.isBuiltin && (
                  <>
                    <button
                      type="button"
                      className={input}
                      style={{ padding: "0.25rem 0.5rem", fontSize: "0.75rem", cursor: "pointer" }}
                      onClick={() => handleOpenEdit(p)}
                      data-testid={`edit-program-${p.id}`}
                    >
                      Edit
                    </button>

                    {confirmDeleteId === p.id ? (
                      <button
                        type="button"
                        className={confirmDeleteBtn}
                        disabled={deletingId === p.id}
                        onClick={() => handleDelete(p.id)}
                        data-testid={`confirm-delete-program-${p.id}`}
                      >
                        Confirm Delete
                      </button>
                    ) : (
                      <button
                        type="button"
                        className={deleteBtn}
                        onClick={() => setConfirmDeleteId(p.id)}
                        data-testid={`delete-program-${p.id}`}
                      >
                        Delete
                      </button>
                    )}
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
