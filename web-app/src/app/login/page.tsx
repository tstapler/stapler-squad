"use client";

import { useState, useEffect, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { loginWithPasskey, registerPasskey } from "@/lib/auth/passkey";
import { useAuth } from "@/lib/contexts/AuthContext";
import styles from "./login.module.css";

function LoginContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { authEnabled, authenticated, hasCredentials, setupActive, loading, refresh } = useAuth();

  const [status, setStatus] = useState<"idle" | "working" | "error">("idle");
  const [errorMsg, setErrorMsg] = useState("");

  const setupToken = searchParams.get("setup_token") ?? undefined;
  const isSetup = !hasCredentials || !!setupToken;

  // If auth is not enabled or user is already authenticated, go home
  useEffect(() => {
    if (!loading) {
      if (!authEnabled || authenticated) {
        router.replace("/");
      }
    }
  }, [loading, authEnabled, authenticated, router]);

  const handleLogin = async () => {
    setStatus("working");
    setErrorMsg("");
    try {
      await loginWithPasskey();
      await refresh();
      router.replace("/");
    } catch (e) {
      setStatus("error");
      setErrorMsg(e instanceof Error ? e.message : String(e));
    }
  };

  const handleRegister = async () => {
    setStatus("working");
    setErrorMsg("");
    try {
      await registerPasskey(setupToken);
      await refresh();
      router.replace("/");
    } catch (e) {
      setStatus("error");
      setErrorMsg(e instanceof Error ? e.message : String(e));
    }
  };

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.card}>
          <p className={styles.hint}>Checking auth status…</p>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.card}>
        <div className={styles.logo}>⚔️</div>
        <h1 className={styles.title}>Claude Squad</h1>

        {isSetup ? (
          <>
            <p className={styles.subtitle}>Set up remote access</p>
            <p className={styles.hint}>
              Register a passkey to secure remote access.
              Your browser or phone will prompt you to use Face ID, Touch ID, or a security key.
            </p>
            <button
              className={styles.button}
              onClick={handleRegister}
              disabled={status === "working"}
            >
              {status === "working" ? "Registering…" : "Register Passkey"}
            </button>
          </>
        ) : (
          <>
            <p className={styles.subtitle}>Sign in to continue</p>
            <p className={styles.hint}>
              Use your registered passkey (Face ID, Touch ID, or security key) to sign in.
            </p>
            <button
              className={styles.button}
              onClick={handleLogin}
              disabled={status === "working"}
            >
              {status === "working" ? "Signing in…" : "Sign in with Passkey"}
            </button>
          </>
        )}

        {status === "error" && (
          <p className={styles.error}>{errorMsg}</p>
        )}

        {!hasCredentials && !setupToken && (
          <p className={styles.hint} style={{ marginTop: "1rem", opacity: 0.6 }}>
            To register, use the setup URL printed in the server console.
          </p>
        )}
      </div>
    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginContent />
    </Suspense>
  );
}
