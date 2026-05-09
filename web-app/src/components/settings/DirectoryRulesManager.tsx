"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import {
  SessionService,
  type DirectoryRuleProto,
  type ProfileDefaultsProto,
} from "@/gen/session/v1/session_pb";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl } from "@/lib/config";
import { PROGRAMS } from "@/lib/constants/programs";
import {
  container,
  heading,
  headerRow,
  loadingText,
  emptyText,
  ruleRow,
  ruleInfo,
  rulePath,
  ruleMeta,
  ruleActions,
  formCard,
  formTitle,
  formFields,
  field,
  label as labelClass,
  checkboxLabel,
  input,
  inputError,
  fieldError,
  select,
  overridesSection,
  tagList,
  tag as tagClass,
  tagRemove,
  tagInputRow,
  formActions,
} from "./DirectoryRulesManager.css";

interface RuleFormData {
  path: string;
  profile: string;
  showOverrides: boolean;
  overrideProgram: string;
  overrideAutoYes: boolean;
  overrideTags: string[];
  tagInput: string;
}

const emptyForm: RuleFormData = {
  path: "",
  profile: "",
  showOverrides: false,
  overrideProgram: "",
  overrideAutoYes: false,
  overrideTags: [],
  tagInput: "",
};

export function DirectoryRulesManager() {
  const [rules, setRules] = useState<DirectoryRuleProto[]>([]);
  const [profiles, setProfiles] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editingPath, setEditingPath] = useState<string | null>(null);
  const [form, setForm] = useState<RuleFormData>({ ...emptyForm });
  const [saving, setSaving] = useState(false);
  const [pathError, setPathError] = useState<string | null>(null);

  const clientRef = useRef<ReturnType<
    typeof createClient<typeof SessionService>
  > | null>(null);

  const loadRules = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      setLoading(true);
      setError(null);
      const response = await clientRef.current.getSessionDefaults({});
      const defaults = response.defaults;
      if (defaults) {
        setRules([...defaults.directoryRules]);
        setProfiles(Object.keys(defaults.profiles));
      }
    } catch (err) {
      setError(`Failed to load directory rules: ${err}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
    loadRules();
  }, [loadRules]);

  const handleEdit = (rule: DirectoryRuleProto) => {
    setEditingPath(rule.path);
    setForm({
      path: rule.path,
      profile: rule.profile,
      showOverrides: !!rule.overrides,
      overrideProgram: rule.overrides?.program ?? "",
      overrideAutoYes: rule.overrides?.autoYes ?? false,
      overrideTags: [...(rule.overrides?.tags ?? [])],
      tagInput: "",
    });
    setPathError(null);
    setShowForm(true);
  };

  const handleNewRule = () => {
    setEditingPath(null);
    setForm({ ...emptyForm });
    setPathError(null);
    setShowForm(true);
  };

  const handleCancel = () => {
    setShowForm(false);
    setEditingPath(null);
    setForm({ ...emptyForm });
    setPathError(null);
  };

  const validatePath = (path: string): string | null => {
    if (!path.trim()) return "Path is required.";
    if (!path.startsWith("/")) return "Path must be an absolute path (start with /).";
    return null;
  };

  const handleSave = async () => {
    if (!clientRef.current) return;
    const pathErr = validatePath(form.path);
    if (pathErr) {
      setPathError(pathErr);
      return;
    }
    setPathError(null);

    const overrides: Partial<ProfileDefaultsProto> | undefined = form.showOverrides
      ? {
          program: form.overrideProgram,
          autoYes: form.overrideAutoYes,
          tags: form.overrideTags,
          name: "",
          description: "",
          envVars: {},
          cliFlags: "",
        } as unknown as ProfileDefaultsProto
      : undefined;

    try {
      setSaving(true);
      setError(null);
      setSuccess(null);
      await clientRef.current.upsertDirectoryRule({
        rule: {
          path: form.path.trim(),
          profile: form.profile,
          overrides,
        } as unknown as DirectoryRuleProto,
      });
      setSuccess(`Rule for "${form.path.trim()}" saved.`);
      setTimeout(() => setSuccess(null), 3000);
      setShowForm(false);
      setEditingPath(null);
      setForm({ ...emptyForm });
      await loadRules();
    } catch (err) {
      setError(`Failed to save rule: ${err}`);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (path: string) => {
    if (!clientRef.current) return;
    if (!confirm(`Delete rule for "${path}"?`)) return;
    try {
      setError(null);
      setSuccess(null);
      await clientRef.current.deleteDirectoryRule({ path });
      setSuccess(`Rule for "${path}" deleted.`);
      setTimeout(() => setSuccess(null), 3000);
      await loadRules();
    } catch (err) {
      setError(`Failed to delete rule: ${err}`);
    }
  };

  const handleAddTag = () => {
    const trimmed = form.tagInput.trim();
    if (trimmed && !form.overrideTags.includes(trimmed)) {
      setForm({ ...form, overrideTags: [...form.overrideTags, trimmed], tagInput: "" });
    } else {
      setForm({ ...form, tagInput: "" });
    }
  };

  const handleRemoveTag = (tag: string) => {
    setForm({ ...form, overrideTags: form.overrideTags.filter((t) => t !== tag) });
  };

  const handleTagKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      handleAddTag();
    }
  };

  if (loading) {
    return (
      <div className={container}>
        <h2 className={heading}>Directory Rules</h2>
        <div className={loadingText}>Loading...</div>
      </div>
    );
  }

  return (
    <div className={container}>
      <div className={headerRow}>
        <h2 className={heading}>Directory Rules</h2>
        <button
          type="button"
          className="btn btn-primary"
          onClick={handleNewRule}
        >
          New Rule
        </button>
      </div>

      {error && <div className="alert alert-error">{error}</div>}
      {success && <div className="alert alert-success">{success}</div>}

      {rules.length === 0 && !showForm && (
        <div className={emptyText}>
          No directory rules configured. Rules auto-populate form fields based on working directory.
        </div>
      )}
      {rules.map((rule) => (
        <div key={rule.path} className={ruleRow}>
          <div className={ruleInfo}>
            <span className={rulePath}>{rule.path}</span>
            {rule.profile && (
              <span className={ruleMeta}>Profile: {rule.profile}</span>
            )}
            {rule.overrides?.program && (
              <span className={ruleMeta}>Program: {rule.overrides.program}</span>
            )}
            {rule.overrides?.autoYes && (
              <span className={ruleMeta}>Auto-yes: on</span>
            )}
            {(rule.overrides?.tags?.length ?? 0) > 0 && (
              <span className={ruleMeta}>Tags: {rule.overrides!.tags.join(", ")}</span>
            )}
          </div>
          <div className={ruleActions}>
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => handleEdit(rule)}
            >
              Edit
            </button>
            <button
              type="button"
              className="btn btn-danger"
              onClick={() => handleDelete(rule.path)}
            >
              Delete
            </button>
          </div>
        </div>
      ))}

      {showForm && (
        <div className={formCard}>
          <h3 className={formTitle}>
            {editingPath ? `Edit Rule: ${editingPath}` : "New Directory Rule"}
          </h3>
          <div className={formFields}>
            <div className={field}>
              <label className={labelClass} htmlFor="rule-path">
                Directory Path *
              </label>
              <input
                id="rule-path"
                type="text"
                className={`${input}${pathError ? ` ${inputError}` : ""}`}
                placeholder="/Users/you/projects/myrepo"
                value={form.path}
                onChange={(e) => {
                  setForm({ ...form, path: e.target.value });
                  if (pathError) setPathError(validatePath(e.target.value));
                }}
                disabled={!!editingPath}
              />
              {pathError && <span className={fieldError}>{pathError}</span>}
            </div>
            <div className={field}>
              <label className={labelClass} htmlFor="rule-profile">
                Profile (optional)
              </label>
              <select
                id="rule-profile"
                className={select}
                value={form.profile}
                onChange={(e) => setForm({ ...form, profile: e.target.value })}
              >
                <option value="">None</option>
                {profiles.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </select>
            </div>
            <div className={field}>
              <label className={checkboxLabel}>
                <input
                  type="checkbox"
                  checked={form.showOverrides}
                  onChange={(e) => setForm({ ...form, showOverrides: e.target.checked })}
                />
                Add field overrides
              </label>
            </div>
            {form.showOverrides && (
              <div className={overridesSection}>
                <div className={field}>
                  <label className={labelClass} htmlFor="rule-program">
                    Override Program
                  </label>
                  <select
                    id="rule-program"
                    className={select}
                    value={form.overrideProgram}
                    onChange={(e) => setForm({ ...form, overrideProgram: e.target.value })}
                  >
                    <option value="">Default</option>
                    {PROGRAMS.map((p) => (
                      <option key={p.value} value={p.value}>
                        {p.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div className={field}>
                  <label className={checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={form.overrideAutoYes}
                      onChange={(e) => setForm({ ...form, overrideAutoYes: e.target.checked })}
                    />
                    Auto-yes
                  </label>
                </div>
                <div className={field}>
                  <label className={labelClass}>Override Tags</label>
                  <div className={tagList}>
                    {form.overrideTags.map((t) => (
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
                    <button
                      type="button"
                      className="btn btn-secondary"
                      onClick={handleAddTag}
                    >
                      Add
                    </button>
                  </div>
                </div>
              </div>
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
      )}
    </div>
  );
}
