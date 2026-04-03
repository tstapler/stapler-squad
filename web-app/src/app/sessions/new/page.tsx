"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState, useEffect, Suspense } from "react";
import { SessionWizard } from "@/components/sessions/SessionWizard";
import { SessionFormData } from "@/lib/validation/sessionSchema";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { getApiBaseUrl } from "@/lib/config";
import styles from "./page.module.css";

function NewSessionContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [initialData, setInitialData] = useState<Partial<SessionFormData> | undefined>(undefined);
  const [loading, setLoading] = useState(true);

  const { createSession, getSession } = useSessionService({
    baseUrl: getApiBaseUrl(),
  });

  // Check if we're duplicating an existing session
  useEffect(() => {
    const duplicateId = searchParams.get("duplicate");
    if (duplicateId) {
      // Load session data for duplication
      getSession(duplicateId).then((session) => {
        if (session) {
          // Convert session to form data, appending "-copy" to title
          setInitialData({
            title: `${session.title}-copy`,
            path: session.path,
            workingDir: session.workingDir || "",
            branch: session.branch || "",
            program: session.program || "claude",
            category: session.category || "",
            prompt: "",
            autoYes: false,
            sessionType: (session as any).sessionType || "new_worktree",
            existingWorktree: (session as any).existingWorktree || "",
            useTitleAsBranch: true,
          });
        }
        setLoading(false);
      }).catch(() => {
        setLoading(false);
      });
    } else {
      setLoading(false);
    }
  }, [searchParams, getSession]);

  const handleComplete = async (data: SessionFormData) => {
    try {
      // If useTitleAsBranch is checked, use the session title as the branch name
      const branchName = data.useTitleAsBranch ? data.title : (data.branch || "");

      await createSession({
        title: data.title,
        path: data.path,
        workingDir: data.workingDir || "",
        branch: branchName,
        program: data.program,
        category: data.category || "",
        prompt: data.prompt || "",
        autoYes: data.autoYes,
        existingWorktree: data.existingWorktree || "",
        sessionType: data.sessionType,
      });

      // Show success indicator (for tests)
      const successIndicator = document.createElement('div');
      successIndicator.setAttribute('data-testid', 'session-created');
      successIndicator.style.display = 'none';
      document.body.appendChild(successIndicator);

      // Navigate back to home page
      router.push("/");
    } catch (error) {
      console.error("Failed to create session:", error);
      throw error;
    }
  };

  const handleCancel = () => {
    router.push("/");
  };

  if (loading) {
    return (
      <div className={styles.page}>
        <div className={styles.container}>
          <div className={styles.header}>
            <h1>Loading...</h1>
            <p>Preparing session configuration</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <div className={styles.container}>
        <div className={styles.header}>
          <h1>{initialData ? "Duplicate Session" : "Create New Session"}</h1>
          <p>
            {initialData
              ? "Review and modify the configuration from the original session"
              : "Set up a new AI coding session with custom configuration"}
          </p>
        </div>
        <SessionWizard
          onComplete={handleComplete}
          onCancel={handleCancel}
          initialData={initialData}
        />
      </div>
    </div>
  );
}

export default function NewSessionPage() {
  return (
    <Suspense fallback={
      <div className="page">
        <div className="container">
          <div className="header">
            <h1>Loading...</h1>
          </div>
        </div>
      </div>
    }>
      <NewSessionContent />
    </Suspense>
  );
}
