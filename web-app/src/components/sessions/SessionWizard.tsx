"use client";
// +feature: session-create-wizard session-title-autogen session-create-github-url

/**
 * @deprecated Use the Omnibar (OmnibarContext) for session creation.
 * This component is no longer rendered. Scheduled for removal once all
 * callers have migrated to the Omnibar flow.
 */

import { useState, useRef, useEffect, useCallback } from "react";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { Wizard, WizardActions } from "@/components/ui/Wizard";
import { AutocompleteInput } from "@/components/ui/AutocompleteInput";
import { useRepositorySuggestions } from "@/lib/hooks/useRepositorySuggestions";
import { useBranchSuggestions } from "@/lib/hooks/useBranchSuggestions";
import { useSessionDefaults } from "@/lib/hooks/useSessionDefaults";
import { sessionSchema, SessionFormData, defaultValues } from "@/lib/validation/sessionSchema";
import { getProgramDisplay, PROGRAMS, DEFAULT_PROGRAM } from "@/lib/constants/programs";
import { parseGitHubRef, isGitHubRef } from "@/lib/github/urlParser";
import { generateUniqueName } from "@/utils/sessionNameUtils";
import { SourceBadge } from "./SourceBadge";
import {
  SessionService,
  ProfileDefaultsProtoSchema,
  PromptHistoryEntry,
} from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import * as styles from "./SessionWizard.css";

interface SessionWizardProps {
  onComplete: (data: SessionFormData) => Promise<void>;
  onCancel: () => void;
  initialData?: Partial<SessionFormData>;
  existingTitles?: string[];
}

export function SessionWizard({ onComplete, onCancel, initialData, existingTitles = [] }: SessionWizardProps) {
  const [step, setStep] = useState(0);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [creationPhase, setCreationPhase] = useState(0);
  const creationPhases = ["Creating session...", "Setting up repository...", "Starting agent...", "Almost ready..."];

  // Profile selector state
  const [selectedProfile, setSelectedProfile] = useState<string>("");

  // Save-as-profile modal state (Task 3.4)
  const [showSaveProfileModal, setShowSaveProfileModal] = useState(false);
  const [profileName, setProfileName] = useState("");
  const [profileDescription, setProfileDescription] = useState("");
  const [saveProfileError, setSaveProfileError] = useState<string | null>(null);
  const [saveProfileSuccess, setSaveProfileSuccess] = useState<string | null>(null);
  const [isSavingProfile, setIsSavingProfile] = useState(false);

  // Recent prompts state for S1-5
  const [recentPrompts, setRecentPrompts] = useState<PromptHistoryEntry[]>([]);
  const [loadingPrompts, setLoadingPrompts] = useState(false);

  // File input ref for S1-5 "Load from file" button
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Track which fields the user has manually edited
  const editedFieldsRef = useRef<Set<keyof SessionFormData>>(new Set());

  const {
    register,
    handleSubmit,
    formState: { errors },
    trigger,
    watch,
    control,
    reset,
    setValue,
    getValues,
    setError,
  } = useForm<SessionFormData>({
    resolver: zodResolver(sessionSchema),
    defaultValues: initialData ? { ...defaultValues, ...initialData } : defaultValues,
    mode: "onChange",
  });

  // Watch all form values for the review step
  const formValues = watch();

  // Watch the program field to show/hide custom command input
  const selectedProgram = watch("program");
  // S5-2: Terminal session preset — hides AI-specific fields
  const isTerminalSession = selectedProgram === "bash";

  // Watch the repository path to update branch suggestions and defaults
  const repositoryPath = watch("path");

  // Watch session type to show/hide conditional fields
  const sessionType = watch("sessionType");

  // Watch useTitleAsBranch to auto-populate branch
  const useTitleAsBranch = watch("useTitleAsBranch");
  const sessionTitle = watch("title");

  useEffect(() => {
    if (!isSubmitting) {
      setCreationPhase(0);
      return;
    }
    const timer = setInterval(() => {
      setCreationPhase(p => (p + 1) % creationPhases.length);
    }, 2500);
    return () => clearInterval(timer);
  }, [isSubmitting, creationPhases.length]);

  // Get autocomplete suggestions
  const { suggestions: repositorySuggestions, isLoading: isLoadingRepos } = useRepositorySuggestions();
  const { suggestions: branchSuggestions, isLoading: isLoadingBranches } = useBranchSuggestions({
    repositoryPath,
  });

  // Fetch session defaults based on repository path and selected profile
  const {
    defaults: resolvedDefaults,
    fieldSources,
    loading: defaultsLoading,
    profiles: availableProfiles,
  } = useSessionDefaults(repositoryPath || "", selectedProfile || undefined);

  // Track field edits via onChange wrapper
  const trackEdit = useCallback((fieldName: keyof SessionFormData) => {
    editedFieldsRef.current.add(fieldName);
  }, []);

  // S1-5: Lazy-load recent prompts on first focus
  const loadRecentPrompts = useCallback(async () => {
    if (recentPrompts.length > 0 || loadingPrompts) return;
    setLoadingPrompts(true);
    try {
      const client = createClient(SessionService, getConnectTransport());
      const response = await client.listPromptHistory({ limit: 10 });
      setRecentPrompts(response.entries);
    } catch {
      // Recent prompts are optional — ignore errors
    } finally {
      setLoadingPrompts(false);
    }
  }, [recentPrompts.length, loadingPrompts]);

  // S5-1: Auto-populate title from repo name when path changes and title is pristine
  useEffect(() => {
    if (!repositoryPath) return;
    if (editedFieldsRef.current.has("title")) return;

    let repoName: string;
    if (isGitHubRef(repositoryPath)) {
      const parsed = parseGitHubRef(repositoryPath);
      repoName = parsed?.repo || repositoryPath.split("/").pop() || "";
    } else {
      repoName = repositoryPath.split("/").pop() || "";
    }
    if (!repoName) return;

    // Strip .git suffix if present
    repoName = repoName.replace(/\.git$/, "");

    // 4 random lowercase alphanumeric chars
    const suffix = Math.random().toString(36).slice(2, 6).replace(/[^a-z0-9]/g, "x").padEnd(4, "x");
    const suggested = generateUniqueName(`${repoName}-${suffix}`, existingTitles);
    setValue("title", suggested);
  }, [repositoryPath, existingTitles, setValue]);

  // Apply resolved defaults to form, preserving user-edited fields
  useEffect(() => {
    if (!resolvedDefaults) return;

    const currentValues = getValues();
    const edited = editedFieldsRef.current;

    const newValues: Partial<SessionFormData> = { ...currentValues };

    if (!edited.has("program") && resolvedDefaults.program) {
      newValues.program = resolvedDefaults.program;
    }
    if (!edited.has("autoYes")) {
      newValues.autoYes = resolvedDefaults.autoYes;
    }

    reset({ ...defaultValues, ...newValues }, { keepDirty: true });
  }, [resolvedDefaults, reset, getValues]);

  // Clear edited fields tracking when profile changes
  const handleProfileChange = useCallback((newProfile: string) => {
    editedFieldsRef.current.clear();
    setSelectedProfile(newProfile);
  }, []);

  const steps = ["Basic Info", "Repository", "Configuration", "Review"];

  const stepFields: Array<Array<keyof SessionFormData>> = [
    ["title", "category"],
    ["path", "workingDir", "sessionType", "branch", "existingWorktree"],
    ["program", "prompt", "autoYes"],
    [], // Review step has no fields to validate
  ];

  const validateStep = async () => {
    const fields = stepFields[step];
    const isValid = await trigger(fields);
    if (!isValid) return false;

    // Cross-field check: root-level z.refine for branch doesn't fire via trigger(fields).
    // Manually enforce it here so the error surfaces on step 1 rather than silently blocking step 3.
    if (step === 1) {
      const values = getValues();
      if (values.sessionType === "new_worktree" && !values.useTitleAsBranch) {
        if (!values.branch || values.branch.trim() === "") {
          setError("branch", { type: "manual", message: "Branch name is required when creating new worktree" });
          return false;
        }
      }
    }

    return true;
  };

  const handleNext = async () => {
    const isValid = await validateStep();
    if (isValid && step < steps.length - 1) {
      setStep(step + 1);
    }
  };

  const handleBack = () => {
    if (step > 0) {
      setStep(step - 1);
    }
  };

  const onSubmit = async (data: SessionFormData) => {
    setIsSubmitting(true);
    setSubmitError(null);
    try {
      await onComplete(data);
      // If we reach here, creation was successful
      // The parent component will handle navigation
    } catch (error) {
      console.error("Failed to create session:", error);
      const errorMessage = error instanceof Error
        ? error.message
        : "Failed to create session. Please try again.";
      setSubmitError(errorMessage);
      setIsSubmitting(false);
    }
  };

  // Save as profile handler (Task 3.4)
  const handleSaveProfile = async () => {
    if (!profileName.trim()) {
      setSaveProfileError("Profile name is required");
      return;
    }

    setIsSavingProfile(true);
    setSaveProfileError(null);

    try {
      const client = createClient(SessionService, getConnectTransport());

      const profile = create(ProfileDefaultsProtoSchema, {
        name: profileName.trim(),
        description: profileDescription.trim(),
        program: formValues.program || "",
        autoYes: formValues.autoYes || false,
        tags: [],
        envVars: {},
        cliFlags: "",
      });

      await client.upsertProfile({ profile });

      setShowSaveProfileModal(false);
      setProfileName("");
      setProfileDescription("");
      setSaveProfileSuccess(`Profile "${profileName.trim()}" saved`);

      // Clear success message after 3 seconds
      setTimeout(() => setSaveProfileSuccess(null), 3000);
    } catch (err) {
      console.error("Failed to save profile:", err);
      setSaveProfileError(
        err instanceof Error ? err.message : "Failed to save profile",
      );
    } finally {
      setIsSavingProfile(false);
    }
  };

  const hasDefaults = resolvedDefaults !== null && !defaultsLoading;

  return (
    <Wizard currentStep={step} steps={steps}>
      <form onSubmit={handleSubmit(onSubmit)}>
        {step === 0 && (
          <div className={styles.step}>
            <h2>Basic Information</h2>
            <p className={styles.description}>
              Give your session a meaningful name and optionally organize it with a category for easy management.
            </p>

            <div className={styles.field}>
              <label htmlFor="title">
                Session Title <span className={styles.required}>*</span>
              </label>
              <input
                id="title"
                type="text"
                data-testid="session-title"
                {...register("title", {
                  onChange: () => trackEdit("title"),
                })}
                placeholder="feature-user-auth"
                className={errors.title ? styles.error : ""}
              />
              {errors.title && (
                <span className={styles.errorMessage}>{errors.title.message}</span>
              )}
              <span className={styles.hint}>
                A descriptive name for this coding session
              </span>
            </div>

            <div className={styles.field}>
              <label htmlFor="category">Category</label>
              <input
                id="category"
                type="text"
                {...register("category", {
                  onChange: () => trackEdit("category"),
                })}
                placeholder="e.g., Features, Bugfixes, Experiments"
              />
              <span className={styles.hint}>
                Optional: Group related sessions together
              </span>
            </div>
          </div>
        )}

        {step === 1 && (
          <div className={styles.step}>
            <h2>Repository Setup</h2>
            <p className={styles.description}>
              Configure the git repository location and worktree strategy.
            </p>

            <div className={styles.field}>
              <label htmlFor="path">
                Repository Path <span className={styles.required}>*</span>
              </label>
              <Controller
                name="path"
                control={control}
                render={({ field }) => (
                  <AutocompleteInput
                    id="path"
                    value={field.value || ""}
                    onChange={(value) => {
                      trackEdit("path");
                      field.onChange(value);
                    }}
                    onBlur={field.onBlur}
                    placeholder="/Users/username/projects/my-repo or https://github.com/owner/repo"
                    suggestions={repositorySuggestions}
                    isLoading={isLoadingRepos}
                    error={!!errors.path}
                    data-testid="session-path"
                  />
                )}
              />
              {errors.path && (
                <span className={styles.errorMessage}>{errors.path.message}</span>
              )}
              <span className={styles.hint}>
                Absolute path to your git repository root or GitHub URL
              </span>
            </div>

            <div className={styles.field}>
              <label htmlFor="workingDir">Working Directory</label>
              <input
                id="workingDir"
                type="text"
                {...register("workingDir", {
                  onChange: () => trackEdit("workingDir"),
                })}
                placeholder="src/api (optional)"
              />
              {errors.workingDir && (
                <span className={styles.errorMessage}>{errors.workingDir.message}</span>
              )}
              <span className={styles.hint}>
                Optional: Start in a subdirectory (relative path)
              </span>
            </div>

            <div className={styles.field}>
              <label htmlFor="sessionType">Session Type</label>
              <select id="sessionType" {...register("sessionType", {
                onChange: () => trackEdit("sessionType"),
              })}>
                <option value="new_worktree">Create New Worktree</option>
                <option value="existing_worktree">Use Existing Worktree</option>
                <option value="directory">Directory Only (No Worktree)</option>
              </select>
              <span className={styles.hint}>
                {sessionType === "new_worktree" && "Creates an isolated git worktree for this session"}
                {sessionType === "existing_worktree" && "Uses an existing worktree at a specific path"}
                {sessionType === "directory" && "Works directly in the repository without worktree isolation"}
              </span>
            </div>

            {sessionType === "new_worktree" && (
              <div className={styles.field}>
                <label htmlFor="branch">Git Branch</label>
                {useTitleAsBranch ? (
                  <div className={styles.branchPreview}>
                    <span className={styles.branchPreviewName}>
                      {sessionTitle || "(enter session title first)"}
                    </span>
                    <button
                      type="button"
                      className={styles.branchCustomizeButton}
                      aria-label="Customize branch name"
                      onClick={() => setValue("useTitleAsBranch", false)}
                    >
                      ✏️ Customize
                    </button>
                  </div>
                ) : (
                  <>
                    <Controller
                      name="branch"
                      control={control}
                      render={({ field }) => (
                        <AutocompleteInput
                          id="branch"
                          value={field.value || ""}
                          onChange={field.onChange}
                          onBlur={field.onBlur}
                          placeholder="feature/my-feature"
                          suggestions={branchSuggestions}
                          isLoading={isLoadingBranches}
                          error={!!errors.branch}
                        />
                      )}
                    />
                    {errors.branch && (
                      <span className={styles.errorMessage}>{errors.branch.message}</span>
                    )}
                    <div className={styles.branchCustomHint}>
                      <button
                        type="button"
                        className={styles.branchCustomizeButton}
                        onClick={() => { setValue("useTitleAsBranch", true); setValue("branch", ""); }}
                      >
                        Use session name instead
                      </button>
                    </div>
                  </>
                )}
                <span className={styles.hint}>
                  {useTitleAsBranch
                    ? "Branch name is automatically set from session title"
                    : "Custom branch name for the new worktree"}
                </span>
              </div>
            )}

            {sessionType === "existing_worktree" && (
              <div className={styles.field}>
                <label htmlFor="existingWorktree">
                  Existing Worktree Path <span className={styles.required}>*</span>
                </label>
                <input
                  id="existingWorktree"
                  type="text"
                  {...register("existingWorktree", {
                    onChange: () => trackEdit("existingWorktree"),
                  })}
                  placeholder="/path/to/existing/worktree"
                  className={errors.existingWorktree ? styles.error : ""}
                />
                {errors.existingWorktree && (
                  <span className={styles.errorMessage}>{errors.existingWorktree.message}</span>
                )}
                <span className={styles.hint}>
                  Absolute path to an existing git worktree
                </span>
              </div>
            )}
          </div>
        )}

        {step === 2 && (
          <div className={styles.step}>
            <h2>
              {isTerminalSession ? "Terminal Configuration" : "Configuration"}
              {hasDefaults && !isTerminalSession && (
                <span className={styles.defaultsNotice}>Pre-filled from defaults</span>
              )}
            </h2>
            <p className={styles.description}>
              {isTerminalSession
                ? "Configure the terminal session settings."
                : "Configure the AI assistant program and optional startup settings."}
            </p>

            {availableProfiles.length > 0 && (
              <div className={styles.field}>
                <label htmlFor="profile">Profile (optional)</label>
                <select
                  id="profile"
                  value={selectedProfile}
                  onChange={(e) => handleProfileChange(e.target.value)}
                >
                  <option value="">None</option>
                  {availableProfiles.map((name) => (
                    <option key={name} value={name}>{name}</option>
                  ))}
                </select>
                <span className={styles.hint}>
                  Apply a saved configuration profile
                </span>
              </div>
            )}

            <div className={styles.field}>
              <label htmlFor="program">
                Program
                <SourceBadge
                  source={fieldSources.program}
                  detail={selectedProfile || undefined}
                />
              </label>
              <select id="program" {...register("program", {
                onChange: () => trackEdit("program"),
              })}>
                {PROGRAMS.map((p) => (
                  <option key={p.value} value={p.value}>{p.label}</option>
                ))}
                <option value="custom">Custom Command...</option>
              </select>
              <span className={styles.hint}>
                AI assistant to run in this session
              </span>
            </div>

            {selectedProgram === "custom" && (
              <div className={styles.field}>
                <label htmlFor="customCommand">
                  Custom Command <span className={styles.required}>*</span>
                </label>
                <input
                  id="customCommand"
                  type="text"
                  {...register("program", {
                    onChange: () => trackEdit("program"),
                  })}
                  placeholder="Enter custom command (e.g., aider --model gpt-4)"
                  className={errors.program ? styles.error : ""}
                />
                {errors.program && (
                  <span className={styles.errorMessage}>{errors.program.message}</span>
                )}
                <span className={styles.hint}>
                  Full command to execute for this session
                </span>
              </div>
            )}

            {/* S1-5: InitialPrompt — injected via CLAUDE.md; hidden for terminal sessions */}
            {!isTerminalSession && (
              <div className={styles.field}>
                <label htmlFor="initialPrompt">Initial Prompt</label>
                {recentPrompts.length > 0 && (
                  <select
                    defaultValue=""
                    onChange={(e) => {
                      if (e.target.value) {
                        setValue("initialPrompt", e.target.value);
                        trackEdit("initialPrompt");
                      }
                    }}
                  >
                    <option value="">📋 Load a recent prompt…</option>
                    {recentPrompts.map((p) => (
                      <option key={p.id} value={p.text}>
                        {(p.label || p.text).slice(0, 80)}
                      </option>
                    ))}
                  </select>
                )}
                <div style={{ display: "flex", gap: "8px", alignItems: "flex-start" }}>
                  <textarea
                    id="initialPrompt"
                    data-testid="initial-prompt-textarea"
                    {...register("initialPrompt", {
                      onChange: () => trackEdit("initialPrompt"),
                    })}
                    placeholder="Optional: Full task description or context injected via CLAUDE.md (up to 10,000 chars)"
                    rows={5}
                    onFocus={loadRecentPrompts}
                    style={{ flex: 1 }}
                  />
                  <div>
                    <input
                      ref={fileInputRef}
                      type="file"
                      accept=".txt,.md"
                      style={{ display: "none" }}
                      onChange={(e) => {
                        const file = e.target.files?.[0];
                        if (!file) return;
                        const reader = new FileReader();
                        reader.onload = (ev) => {
                          const content = ev.target?.result as string;
                          setValue("initialPrompt", content);
                          trackEdit("initialPrompt");
                        };
                        reader.readAsText(file);
                        // Reset so the same file can be re-selected
                        e.target.value = "";
                      }}
                    />
                    <button
                      type="button"
                      onClick={() => fileInputRef.current?.click()}
                      title="Load from file (.txt, .md)"
                      style={{
                        background: "none",
                        border: `1px solid var(--border-color)`,
                        borderRadius: "6px",
                        padding: "6px 8px",
                        cursor: "pointer",
                        fontSize: "1rem",
                        lineHeight: 1,
                      }}
                    >
                      📎
                    </button>
                  </div>
                </div>
                {errors.initialPrompt && (
                  <span className={styles.errorMessage}>{errors.initialPrompt.message}</span>
                )}
                <span className={styles.hint}>
                  {(formValues.initialPrompt || "").length > 0
                    ? `${(formValues.initialPrompt || "").length.toLocaleString()} / 10,000 characters`
                    : "Optional: Context injected via CLAUDE.md at session start"}
                </span>
              </div>
            )}

            {/* Legacy quick prompt — hidden for terminal sessions */}
            {!isTerminalSession && (
              <div className={styles.field}>
                <label htmlFor="prompt">Quick Prompt</label>
                <textarea
                  id="prompt"
                  {...register("prompt", {
                    onChange: () => trackEdit("prompt"),
                  })}
                  placeholder="Optional: Short message appended as CLI argument"
                  rows={2}
                />
                <span className={styles.hint}>
                  Optional: Short message passed directly to the CLI (max 1,000 chars)
                </span>
              </div>
            )}

            {/* Auto-approve — hidden for terminal sessions */}
            {!isTerminalSession && (
              <div className={styles.field}>
                <label className={styles.checkbox}>
                  <input
                    type="checkbox"
                    data-testid="auto-yes-checkbox"
                    {...register("autoYes", {
                      onChange: () => trackEdit("autoYes"),
                    })}
                  />
                  <span>Auto-approve prompts (experimental mode)</span>
                </label>
                <span className={styles.hint}>
                  Automatically approve all AI suggestions without confirmation
                </span>
                {formValues.autoYes && (
                  <span style={{ color: 'var(--warning, #d97706)', fontSize: '0.8125rem', marginTop: '0.25rem', display: 'block' }} role="alert">
                    Warning: Claude will execute file changes and run commands without asking for your approval.
                  </span>
                )}
              </div>
            )}
          </div>
        )}

        {step === 3 && (
          <div className={styles.step}>
            <h2>Review Configuration</h2>
            <p className={styles.description}>
              Please review your session configuration before creating.
            </p>

            <div className={styles.reviewSection}>
              <h3>Basic Information</h3>
              <div className={styles.reviewItem}>
                <span className={styles.reviewLabel}>Session Title:</span>
                <span className={styles.reviewValue}>{formValues.title || "(Not set)"}</span>
              </div>
              {formValues.category && (
                <div className={styles.reviewItem}>
                  <span className={styles.reviewLabel}>Category:</span>
                  <span className={styles.reviewValue}>{formValues.category}</span>
                </div>
              )}
            </div>

            <div className={styles.reviewSection}>
              <h3>Repository Setup</h3>
              <div className={styles.reviewItem}>
                <span className={styles.reviewLabel}>Repository Path:</span>
                <span className={styles.reviewValue}>{formValues.path || "(Not set)"}</span>
              </div>
              {formValues.workingDir && (
                <div className={styles.reviewItem}>
                  <span className={styles.reviewLabel}>Working Directory:</span>
                  <span className={styles.reviewValue}>{formValues.workingDir}</span>
                </div>
              )}
              <div className={styles.reviewItem}>
                <span className={styles.reviewLabel}>Session Type:</span>
                <span className={styles.reviewValue}>
                  {formValues.sessionType === "new_worktree" && "Create New Worktree"}
                  {formValues.sessionType === "existing_worktree" && "Use Existing Worktree"}
                  {formValues.sessionType === "directory" && "Directory Only"}
                </span>
              </div>
              {formValues.sessionType === "new_worktree" && (
                <div className={styles.reviewItem}>
                  <span className={styles.reviewLabel}>Git Branch:</span>
                  <span className={styles.reviewValue}>
                    {formValues.useTitleAsBranch ? formValues.title : formValues.branch}
                  </span>
                  <span className={styles.hint}>
                    A new worktree will be created on this branch
                  </span>
                </div>
              )}
              {formValues.sessionType === "existing_worktree" && formValues.existingWorktree && (
                <div className={styles.reviewItem}>
                  <span className={styles.reviewLabel}>Existing Worktree:</span>
                  <span className={styles.reviewValue}>{formValues.existingWorktree}</span>
                </div>
              )}
            </div>

            <div className={styles.reviewSection}>
              <h3>Configuration</h3>
              <div className={styles.reviewItem}>
                <span className={styles.reviewLabel}>Program:</span>
                <span className={styles.reviewValue}>{getProgramDisplay(formValues.program)}</span>
              </div>
              {formValues.prompt && (
                <div className={styles.reviewItem}>
                  <span className={styles.reviewLabel}>Initial Prompt:</span>
                  <span className={styles.reviewValue}>{formValues.prompt}</span>
                </div>
              )}
              <div className={styles.reviewItem}>
                <span className={styles.reviewLabel}>Auto-approve:</span>
                <span className={styles.reviewValue}>{formValues.autoYes ? "Yes" : "No"}</span>
              </div>
            </div>

            {saveProfileSuccess && (
              <div className={styles.successMessage}>{saveProfileSuccess}</div>
            )}
          </div>
        )}

        {submitError && (
          <div className={styles.submitError}>
            <strong>Error:</strong> {submitError}
          </div>
        )}

        <WizardActions>
          {step > 0 && (
            <button
              type="button"
              onClick={handleBack}
              className={styles.buttonSecondary}
              disabled={isSubmitting}
            >
              Back
            </button>
          )}
          <button
            type="button"
            onClick={onCancel}
            className={styles.buttonSecondary}
            disabled={isSubmitting}
          >
            Cancel
          </button>
          {step === 3 && (
            <button
              type="button"
              onClick={() => setShowSaveProfileModal(true)}
              className={styles.buttonSecondary}
              disabled={isSubmitting}
            >
              Save as Profile...
            </button>
          )}
          {step < steps.length - 1 ? (
            <button
              type="button"
              onClick={handleNext}
              className={styles.buttonPrimary}
            >
              Next
            </button>
          ) : (
            <button
              type="submit"
              data-testid="create-session-button"
              className={styles.buttonPrimary}
              disabled={isSubmitting}
            >
              {isSubmitting ? creationPhases[creationPhase] : "Create Session"}
            </button>
          )}
        </WizardActions>
      </form>

      {/* Save as Profile Modal (Task 3.4) */}
      {showSaveProfileModal && (
        <div className={styles.modalOverlay} onClick={() => setShowSaveProfileModal(false)}>
          <div className={styles.modalContent} onClick={(e) => e.stopPropagation()}>
            <h3>Save as Profile</h3>
            <p className={styles.description}>
              Save the current configuration as a reusable profile.
            </p>

            <div className={styles.field}>
              <label htmlFor="profileName">
                Profile Name <span className={styles.required}>*</span>
              </label>
              <input
                id="profileName"
                type="text"
                value={profileName}
                onChange={(e) => setProfileName(e.target.value)}
                placeholder="e.g., my-work-defaults"
                autoFocus
              />
            </div>

            <div className={styles.field}>
              <label htmlFor="profileDescription">Description</label>
              <textarea
                id="profileDescription"
                value={profileDescription}
                onChange={(e) => setProfileDescription(e.target.value)}
                placeholder="Optional description for this profile"
                rows={2}
              />
            </div>

            {saveProfileError && (
              <div className={styles.submitError}>
                {saveProfileError}
              </div>
            )}

            <div className={styles.modalActions}>
              <button
                type="button"
                onClick={() => {
                  setShowSaveProfileModal(false);
                  setSaveProfileError(null);
                  setProfileName("");
                  setProfileDescription("");
                }}
                className={styles.buttonSecondary}
                disabled={isSavingProfile}
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleSaveProfile}
                className={styles.buttonPrimary}
                disabled={isSavingProfile || !profileName.trim()}
              >
                {isSavingProfile ? "Saving..." : "Save Profile"}
              </button>
            </div>
          </div>
        </div>
      )}
    </Wizard>
  );
}
