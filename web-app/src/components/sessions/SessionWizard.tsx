"use client";

import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Wizard, WizardActions } from "@/components/ui/Wizard";
import { sessionSchema, SessionFormData, defaultValues } from "@/lib/validation/sessionSchema";
import styles from "./SessionWizard.module.css";

// Helper function to get program display name
function getProgramDisplay(program?: string): string {
  if (!program) return "Claude Code (default)";
  if (program === "claude") return "Claude Code";
  if (program === "aider") return "Aider";
  if (program.startsWith("aider --model")) return program;
  return program;
}

interface SessionWizardProps {
  onComplete: (data: SessionFormData) => Promise<void>;
  onCancel: () => void;
  initialData?: Partial<SessionFormData>;
}

export function SessionWizard({ onComplete, onCancel, initialData }: SessionWizardProps) {
  const [step, setStep] = useState(0);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors },
    trigger,
    watch,
  } = useForm<SessionFormData>({
    resolver: zodResolver(sessionSchema),
    defaultValues: initialData ? { ...defaultValues, ...initialData } : defaultValues,
    mode: "onChange",
  });

  // Watch all form values for the review step
  const formValues = watch();

  // Watch the program field to show/hide custom command input
  const selectedProgram = watch("program");

  const steps = ["Basic Info", "Repository", "Configuration", "Review"];

  const stepFields: Array<Array<keyof SessionFormData>> = [
    ["title", "category"],
    ["path", "workingDir", "branch"],
    ["program", "prompt", "autoYes"],
    [], // Review step has no fields to validate
  ];

  const validateStep = async () => {
    const fields = stepFields[step];
    const isValid = await trigger(fields);
    return isValid;
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
                {...register("title")}
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
                {...register("category")}
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
              Configure the git repository location. Claude Squad will create an isolated worktree for this session.
            </p>

            <div className={styles.field}>
              <label htmlFor="path">
                Repository Path <span className={styles.required}>*</span>
              </label>
              <input
                id="path"
                type="text"
                {...register("path")}
                placeholder="/Users/username/projects/my-repo"
                className={errors.path ? styles.error : ""}
              />
              {errors.path && (
                <span className={styles.errorMessage}>{errors.path.message}</span>
              )}
              <span className={styles.hint}>
                Absolute path to your git repository root
              </span>
            </div>

            <div className={styles.field}>
              <label htmlFor="workingDir">Working Directory</label>
              <input
                id="workingDir"
                type="text"
                {...register("workingDir")}
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
              <label htmlFor="branch">Git Branch</label>
              <input
                id="branch"
                type="text"
                {...register("branch")}
                placeholder="feature/my-feature"
              />
              {errors.branch && (
                <span className={styles.errorMessage}>{errors.branch.message}</span>
              )}
              <span className={styles.hint}>
                Optional: Create/use a branch with isolated worktree
              </span>
            </div>
          </div>
        )}

        {step === 2 && (
          <div className={styles.step}>
            <h2>Configuration</h2>
            <p className={styles.description}>
              Configure the AI assistant program and optional startup settings.
            </p>

            <div className={styles.field}>
              <label htmlFor="program">Program</label>
              <select id="program" {...register("program")}>
                <option value="claude">Claude Code</option>
                <option value="aider">Aider</option>
                <option value="aider --model ollama_chat/gemma3:1b">
                  Aider (Ollama Gemma 1B)
                </option>
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
                  {...register("program")}
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

            <div className={styles.field}>
              <label htmlFor="prompt">Initial Prompt</label>
              <textarea
                id="prompt"
                {...register("prompt")}
                placeholder="Optional: Initial message to send to the AI"
                rows={3}
              />
              <span className={styles.hint}>
                Optional: Message sent when session starts
              </span>
            </div>

            <div className={styles.field}>
              <label className={styles.checkbox}>
                <input type="checkbox" {...register("autoYes")} />
                <span>Auto-approve prompts (experimental mode)</span>
              </label>
              <span className={styles.hint}>
                Automatically approve all AI suggestions without confirmation
              </span>
            </div>
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
              {formValues.branch && (
                <div className={styles.reviewItem}>
                  <span className={styles.reviewLabel}>Git Branch:</span>
                  <span className={styles.reviewValue}>{formValues.branch}</span>
                  <span className={styles.hint}>A new worktree will be created</span>
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
              className={styles.buttonPrimary}
              disabled={isSubmitting}
            >
              {isSubmitting ? "Creating..." : "Create Session"}
            </button>
          )}
        </WizardActions>
      </form>
    </Wizard>
  );
}
