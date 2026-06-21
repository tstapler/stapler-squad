"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { SessionService, type AliasProto } from "@/gen/session/v1/session_pb";
import { SessionType } from "@/gen/session/v1/types_pb";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl } from "@/lib/config";
import { PROGRAMS } from "@/lib/constants/programs";
import { ALIAS_NAME_RE } from "@/lib/omnibar/detectors/AliasDetector";

const SESSION_TYPE_OPTIONS: Array<{ value: SessionType; label: string }> = [
  { value: SessionType.UNSPECIFIED, label: "Default (directory)" },
  { value: SessionType.DIRECTORY, label: "Directory" },
  { value: SessionType.NEW_WORKTREE, label: "New Worktree" },
  { value: SessionType.EXISTING_WORKTREE, label: "Existing Worktree" },
  { value: SessionType.ONE_OFF, label: "One-off" },
];
import {
  container,
  heading,
  headerRow,
  loadingText,
  emptyText,
  aliasRow,
  aliasInfo,
  aliasName as aliasNameClass,
  aliasDesc,
  aliasMeta,
  aliasActions,
  formCard,
  formTitle,
  formFields,
  field,
  label as labelClass,
  checkboxLabel,
  input,
  select,
  tagList,
  tag as tagClass,
  tagRemove,
  tagInputRow,
  formActions,
  envVarTable,
  envVarRow,
  envVarInput,
  deleteBtn,
  confirmDeleteBtn,
  advancedToggle,
  previewHint,
  fieldError,
  groupHint,
} from "./AliasesManager.css";

interface EnvVar {
  key: string;
  value: string;
}

interface AliasFormData {
  name: string;
  description: string;
  group: string;
  path: string;
  profile: string;
  program: string;
  autoYes: boolean;
  tags: string[];
  tagInput: string;
  envVars: EnvVar[];
  cliFlags: string;
  sessionType: SessionType;
  showAdvanced: boolean;
}

const emptyForm: AliasFormData = {
  name: "",
  description: "",
  group: "",
  path: "",
  profile: "",
  program: "",
  autoYes: false,
  tags: [],
  tagInput: "",
  envVars: [],
  cliFlags: "",
  sessionType: SessionType.UNSPECIFIED,
  showAdvanced: false,
};

export function AliasesManager() {
  const [aliases, setAliases] = useState<AliasProto[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nameError, setNameError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [form, setForm] = useState<AliasFormData>({ ...emptyForm });
  const [saving, setSaving] = useState(false);
  const [pendingDeleteName, setPendingDeleteName] = useState<string | null>(null);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const nameInputRef = useRef<HTMLInputElement | null>(null);
  const pendingDeleteTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const successBannerTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Timer cleanup on unmount
  useEffect(() => {
    return () => {
      if (pendingDeleteTimerRef.current) clearTimeout(pendingDeleteTimerRef.current);
      if (successBannerTimerRef.current) clearTimeout(successBannerTimerRef.current);
    };
  }, []);

  const loadAliases = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      setLoading(true);
      setError(null);
      const response = await clientRef.current.listAliases({});
      setAliases(response.aliases ?? []);
    } catch (err) {
      setError(`Failed to load aliases: ${err}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
    loadAliases();
  }, [loadAliases]);

  // Focus name input when creating new alias
  useEffect(() => {
    if (showForm && editingName === null && nameInputRef.current) {
      nameInputRef.current.focus();
    }
  }, [showForm, editingName]);

  const handleEdit = (alias: AliasProto) => {
    setEditingName(alias.name);
    const envVarsList: EnvVar[] = Object.entries(alias.envVars ?? {}).map(([key, value]) => ({ key, value }));
    const hasAdvanced = envVarsList.length > 0 || !!alias.cliFlags;
    setForm({
      name: alias.name,
      description: alias.description,
      group: alias.group,
      path: alias.path,
      profile: alias.profile,
      program: alias.program,
      autoYes: alias.autoYes,
      tags: [...alias.tags],
      tagInput: "",
      envVars: envVarsList,
      cliFlags: alias.cliFlags,
      sessionType: alias.sessionType,
      showAdvanced: hasAdvanced,
    });
    setNameError(null);
    setShowForm(true);
  };

  const handleNewAlias = () => {
    setEditingName(null);
    setForm({ ...emptyForm });
    setNameError(null);
    setShowForm(true);
  };

  const handleCancel = () => {
    setShowForm(false);
    setEditingName(null);
    setNameError(null);
    setForm({ ...emptyForm });
  };

  const handleSave = async () => {
    if (!clientRef.current) return;

    const trimmedName = form.name.trim();

    if (!trimmedName) {
      setNameError("Name is required.");
      return;
    }

    if (!ALIAS_NAME_RE.test(trimmedName)) {
      setNameError("Name may only contain letters, digits, hyphens, and underscores.");
      return;
    }

    // Case-insensitive collision check in create mode only
    if (editingName === null) {
      const collision = aliases.find(
        (a) => a.name.toLowerCase() === trimmedName.toLowerCase()
      );
      if (collision) {
        setNameError(`An alias named "@${collision.name}" already exists. Edit it instead.`);
        return;
      }
    }

    // Build envVars map (skip blank keys)
    const envVarsMap: { [key: string]: string } = {};
    for (const ev of form.envVars) {
      if (ev.key.trim()) {
        envVarsMap[ev.key.trim()] = ev.value;
      }
    }

    try {
      setSaving(true);
      setError(null);
      setSuccess(null);
      await clientRef.current.upsertAlias({
        alias: {
          name: trimmedName,
          description: form.description,
          group: form.group,
          path: form.path,
          profile: form.profile,
          program: form.program,
          autoYes: form.autoYes,
          tags: form.tags,
          envVars: envVarsMap,
          cliFlags: form.cliFlags,
          sessionType: form.sessionType,
        } as unknown as AliasProto,
      });
      if (successBannerTimerRef.current) clearTimeout(successBannerTimerRef.current);
      setSuccess(`Alias "@${trimmedName}" saved.`);
      successBannerTimerRef.current = setTimeout(() => setSuccess(null), 3000);
      setShowForm(false);
      setEditingName(null);
      setNameError(null);
      setForm({ ...emptyForm });
      await loadAliases();
    } catch (err) {
      setError(`Failed to save alias: ${err}`);
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteClick = (name: string) => {
    if (pendingDeleteTimerRef.current) clearTimeout(pendingDeleteTimerRef.current);
    setPendingDeleteName(name);
    pendingDeleteTimerRef.current = setTimeout(() => {
      setPendingDeleteName(null);
    }, 3000);
  };

  const handleDeleteConfirm = async (name: string) => {
    if (!clientRef.current) return;
    if (pendingDeleteTimerRef.current) clearTimeout(pendingDeleteTimerRef.current);
    setPendingDeleteName(null);
    try {
      setError(null);
      setSuccess(null);
      await clientRef.current.deleteAlias({ name });
      if (successBannerTimerRef.current) clearTimeout(successBannerTimerRef.current);
      setSuccess(`Alias "@${name}" deleted.`);
      successBannerTimerRef.current = setTimeout(() => setSuccess(null), 3000);
      await loadAliases();
    } catch (err) {
      setError(`Failed to delete alias: ${err}`);
    }
  };

  const handleAddTag = () => {
    const trimmed = form.tagInput.trim();
    if (trimmed && !form.tags.includes(trimmed)) {
      setForm({ ...form, tags: [...form.tags, trimmed], tagInput: "" });
    } else {
      setForm({ ...form, tagInput: "" });
    }
  };

  const handleRemoveTag = (t: string) => {
    setForm({ ...form, tags: form.tags.filter((tag) => tag !== t) });
  };

  const handleTagKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      handleAddTag();
    }
  };

  const handleAddEnvVar = () => {
    setForm({ ...form, envVars: [...form.envVars, { key: "", value: "" }] });
  };

  const handleEnvVarChange = (index: number, fieldName: "key" | "value", val: string) => {
    const updated = form.envVars.map((ev, i) => (i === index ? { ...ev, [fieldName]: val } : ev));
    setForm({ ...form, envVars: updated });
  };

  const handleRemoveEnvVar = (index: number) => {
    setForm({ ...form, envVars: form.envVars.filter((_, i) => i !== index) });
  };

  if (loading) {
    return (
      <div className={container}>
        <h2 className={heading}>Aliases</h2>
        <div className={loadingText}>Loading...</div>
      </div>
    );
  }

  return (
    <div className={container}>
      <div className={headerRow}>
        <h2 className={heading}>Aliases</h2>
        <button
          type="button"
          className="btn btn-primary"
          onClick={handleNewAlias}
          disabled={saving}
        >
          New Alias
        </button>
      </div>

      {error && <div className="alert alert-error" role="alert">{error}</div>}
      {success && <div className="alert alert-success" role="status">{success}</div>}

      {aliases.length === 0 && !showForm && (
        <div className={emptyText}>No aliases configured.</div>
      )}

      {aliases.map((alias) => (
        <div key={alias.name} className={aliasRow} data-testid={`alias-row-${alias.name}`}>
          <div className={aliasInfo}>
            <span className={aliasNameClass}>@{alias.name}</span>
            {alias.description && <span className={aliasDesc}>{alias.description}</span>}
            {alias.group && <span className={aliasMeta}>Group: {alias.group}</span>}
            {alias.path && <span className={aliasMeta}>Path: {alias.path}</span>}
            {alias.program && <span className={aliasMeta}>Program: {alias.program}</span>}
            {alias.sessionType !== SessionType.UNSPECIFIED && (
              <span className={aliasMeta}>
                Type: {SESSION_TYPE_OPTIONS.find((o) => o.value === alias.sessionType)?.label ?? ""}
              </span>
            )}
            {alias.autoYes && <span className={aliasMeta}>Auto-yes: on</span>}
            {alias.tags.length > 0 && <span className={aliasMeta}>Tags: {alias.tags.join(", ")}</span>}
          </div>
          <div className={aliasActions}>
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => handleEdit(alias)}
            >
              Edit
            </button>
            {pendingDeleteName === alias.name ? (
              <button
                type="button"
                className={confirmDeleteBtn}
                onClick={() => handleDeleteConfirm(alias.name)}
                aria-label={`Confirm delete alias ${alias.name}`}
                data-testid={`alias-confirm-delete-${alias.name}`}
              >
                Confirm delete?
              </button>
            ) : (
              <button
                type="button"
                className={deleteBtn}
                onClick={() => handleDeleteClick(alias.name)}
                aria-label={`Delete alias ${alias.name}`}
                data-testid={`alias-delete-${alias.name}`}
              >
                Delete
              </button>
            )}
          </div>
        </div>
      ))}

      {showForm && (
        <section aria-labelledby="alias-form-title">
          <div className={formCard}>
            <h3 id="alias-form-title" className={formTitle}>
              {editingName ? `Edit Alias: ${editingName}` : "New Alias"}
            </h3>
            <div className={formFields}>
              {/* Name */}
              <div className={field}>
                <label className={labelClass} htmlFor="alias-name">
                  Name *
                </label>
                <input
                  id="alias-name"
                  ref={nameInputRef}
                  type="text"
                  className={input}
                  placeholder="e.g. myproject"
                  value={form.name}
                  onChange={(e) => { setForm({ ...form, name: e.target.value }); setNameError(null); }}
                  disabled={!!editingName}
                  aria-required="true"
                />
                <div className={previewHint}>Preview: @{form.name || "name"}</div>
                {nameError && <div className={fieldError} role="alert">{nameError}</div>}
              </div>

              {/* Description */}
              <div className={field}>
                <label className={labelClass} htmlFor="alias-desc">
                  Description
                </label>
                <input
                  id="alias-desc"
                  type="text"
                  className={input}
                  placeholder="Short description"
                  value={form.description}
                  onChange={(e) => setForm({ ...form, description: e.target.value })}
                />
              </div>

              {/* Group */}
              <div className={field}>
                <label className={labelClass} htmlFor="alias-group">
                  Group
                </label>
                <input
                  id="alias-group"
                  type="text"
                  className={input}
                  placeholder="e.g. work, personal"
                  value={form.group}
                  onChange={(e) => setForm({ ...form, group: e.target.value })}
                />
                <div className={groupHint}>Used to organize aliases in the palette.</div>
              </div>

              {/* Path */}
              <div className={field}>
                <label className={labelClass} htmlFor="alias-path">
                  Path
                </label>
                <input
                  id="alias-path"
                  type="text"
                  className={input}
                  placeholder="e.g. ~/code/myproject"
                  value={form.path}
                  onChange={(e) => setForm({ ...form, path: e.target.value })}
                />
              </div>

              {/* Profile */}
              <div className={field}>
                <label className={labelClass} htmlFor="alias-profile">
                  Profile
                </label>
                <input
                  id="alias-profile"
                  type="text"
                  className={input}
                  placeholder="e.g. fast-mode"
                  value={form.profile}
                  onChange={(e) => setForm({ ...form, profile: e.target.value })}
                />
              </div>

              {/* Program */}
              <div className={field}>
                <label className={labelClass} htmlFor="alias-program">
                  Program
                </label>
                <select
                  id="alias-program"
                  className={select}
                  value={form.program}
                  onChange={(e) => setForm({ ...form, program: e.target.value })}
                >
                  <option value="">Default</option>
                  {PROGRAMS.map((p) => (
                    <option key={p.value} value={p.value}>
                      {p.label}
                    </option>
                  ))}
                </select>
              </div>

              {/* Session Type */}
              <div className={field}>
                <label className={labelClass} htmlFor="alias-session-type">
                  Session Type
                </label>
                <select
                  id="alias-session-type"
                  className={select}
                  value={form.sessionType}
                  onChange={(e) => setForm({ ...form, sessionType: Number(e.target.value) as SessionType })}
                >
                  {SESSION_TYPE_OPTIONS.map((opt) => (
                    <option key={opt.value} value={opt.value}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </div>

              {/* Auto-yes */}
              <div className={field}>
                <label className={checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={form.autoYes}
                    onChange={(e) => setForm({ ...form, autoYes: e.target.checked })}
                  />
                  Auto-yes
                </label>
              </div>

              {/* Tags */}
              <div className={field}>
                <label className={labelClass}>Tags</label>
                <div className={tagList}>
                  {form.tags.map((t) => (
                    <span key={t} className={tagClass}>
                      {t}
                      <button
                        type="button"
                        className={tagRemove}
                        onClick={() => handleRemoveTag(t)}
                        aria-label={`Remove tag ${t}`}
                      >
                        x
                      </button>
                    </span>
                  ))}
                </div>
                <div className={tagInputRow}>
                  <input
                    type="text"
                    className={input}
                    placeholder="Add a tag..."
                    value={form.tagInput}
                    onChange={(e) => setForm({ ...form, tagInput: e.target.value })}
                    onKeyDown={handleTagKeyDown}
                  />
                  <button type="button" className="btn btn-secondary" onClick={handleAddTag}>
                    Add
                  </button>
                </div>
              </div>

              {/* Advanced section */}
              <div className={field}>
                <label className={checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={form.showAdvanced}
                    onChange={(e) => setForm({ ...form, showAdvanced: e.target.checked })}
                  />
                  <span className={advancedToggle}>Advanced</span>
                </label>
              </div>

              {form.showAdvanced && (
                <>
                  {/* Env Vars */}
                  <div className={field}>
                    <label className={labelClass}>Environment Variables</label>
                    <div className={envVarTable}>
                      {form.envVars.map((ev, i) => (
                        <div key={i} className={envVarRow}>
                          <input
                            type="text"
                            className={envVarInput}
                            placeholder="KEY"
                            value={ev.key}
                            onChange={(e) => handleEnvVarChange(i, "key", e.target.value)}
                            aria-label={`Environment variable key ${i + 1}`}
                          />
                          <input
                            type="text"
                            className={envVarInput}
                            placeholder="value"
                            value={ev.value}
                            onChange={(e) => handleEnvVarChange(i, "value", e.target.value)}
                            aria-label={`Environment variable value ${i + 1}`}
                          />
                          <button
                            type="button"
                            className={deleteBtn}
                            onClick={() => handleRemoveEnvVar(i)}
                            aria-label={`Remove environment variable ${ev.key || String(i + 1)}`}
                          >
                            Remove
                          </button>
                        </div>
                      ))}
                    </div>
                    <button
                      type="button"
                      className="btn btn-secondary"
                      onClick={handleAddEnvVar}
                    >
                      Add variable
                    </button>
                  </div>

                  {/* CLI Flags */}
                  <div className={field}>
                    <label className={labelClass} htmlFor="alias-cliflags">
                      CLI Flags
                    </label>
                    <input
                      id="alias-cliflags"
                      type="text"
                      className={input}
                      placeholder="e.g. --verbose"
                      value={form.cliFlags}
                      onChange={(e) => setForm({ ...form, cliFlags: e.target.value })}
                    />
                  </div>
                </>
              )}
            </div>

            <div className={formActions}>
              <button
                type="button"
                className="btn btn-primary"
                onClick={handleSave}
                disabled={saving}
              >
                {saving ? "Saving..." : "Save"}
              </button>
              <button
                type="button"
                className="btn btn-secondary"
                onClick={handleCancel}
              >
                Cancel
              </button>
            </div>
          </div>
        </section>
      )}
    </div>
  );
}
