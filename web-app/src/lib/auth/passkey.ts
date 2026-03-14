/**
 * Passkey authentication client utilities.
 * Uses @simplewebauthn/browser to handle the browser WebAuthn ceremony.
 */

import {
  startRegistration,
  startAuthentication,
} from "@simplewebauthn/browser";

export interface AuthStatus {
  auth_enabled: boolean;
  has_credentials: boolean;
  authenticated: boolean;
  setup_active: boolean;
}

/** Returns the /auth base URL using the current origin. */
function authBase(): string {
  if (typeof window !== "undefined") {
    return window.location.origin + "/auth";
  }
  return "http://localhost:8543/auth";
}

/** Fetch the current auth status from the server. */
export async function getAuthStatus(): Promise<AuthStatus> {
  const resp = await fetch(`${authBase()}/status`, {
    credentials: "include",
  });
  if (!resp.ok) {
    throw new Error(`auth status failed: ${resp.statusText}`);
  }
  return resp.json();
}

/**
 * Register a new passkey.
 * If a setupToken is provided it is passed as a query param (first-device bootstrap).
 */
export async function registerPasskey(setupToken?: string): Promise<void> {
  const base = authBase();

  // 1. Begin registration – server returns ceremony key + WebAuthn options
  const beginParams = setupToken ? `?setup_token=${setupToken}` : "";
  const beginResp = await fetch(`${base}/register/begin${beginParams}`, {
    method: "POST",
    credentials: "include",
  });
  if (!beginResp.ok) {
    const text = await beginResp.text();
    throw new Error(`begin registration failed: ${text}`);
  }
  const { ceremony_key, options } = await beginResp.json();

  // 2. Browser prompts user to create a passkey
  const credential = await startRegistration({ optionsJSON: options });

  // 3. Finish registration – send credential back to server
  const finishResp = await fetch(
    `${base}/register/finish?ceremony_key=${ceremony_key}${setupToken ? `&setup_token=${setupToken}` : ""}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(credential),
      credentials: "include",
    }
  );
  if (!finishResp.ok) {
    const text = await finishResp.text();
    throw new Error(`finish registration failed: ${text}`);
  }
}

/** Authenticate with an existing passkey. */
export async function loginWithPasskey(): Promise<void> {
  const base = authBase();

  // 1. Begin login
  const beginResp = await fetch(`${base}/login/begin`, {
    method: "POST",
    credentials: "include",
  });
  if (!beginResp.ok) {
    const text = await beginResp.text();
    throw new Error(`begin login failed: ${text}`);
  }
  const { ceremony_key, options } = await beginResp.json();

  // 2. Browser prompts user to sign with their passkey
  const credential = await startAuthentication({ optionsJSON: options });

  // 3. Finish login
  const finishResp = await fetch(
    `${base}/login/finish?ceremony_key=${ceremony_key}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(credential),
      credentials: "include",
    }
  );
  if (!finishResp.ok) {
    const text = await finishResp.text();
    throw new Error(`finish login failed: ${text}`);
  }
}

/** Log out (revoke the current session cookie). */
export async function logout(): Promise<void> {
  await fetch(`${authBase()}/logout`, {
    method: "POST",
    credentials: "include",
  });
}
