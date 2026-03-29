"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { SessionService } from "@/gen/session/v1/session_connect";
import { ClaudeConfigFile } from "@/gen/session/v1/session_pb";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import Editor, { OnMount } from "@monaco-editor/react";
import type { editor } from "monaco-editor";
import styles from "./config.module.css";
import { getApiBaseUrl } from "@/lib/config";
import { useAuth } from "@/lib/contexts/AuthContext";
import { registerPasskey, logout as doLogout } from "@/lib/auth/passkey";

// Helper function to determine Monaco editor language from filename
function getLanguageFromFilename(filename: string): string {
  if (filename.endsWith('.json')) return 'json';
  if (filename.endsWith('.md')) return 'markdown';
  return 'plaintext';
}

// Helper function to get file icon based on filename
function getFileIcon(filename: string): string {
  if (filename.endsWith('.json')) return '📋';
  if (filename.endsWith('.md')) return '📝';
  if (filename === 'CLAUDE.md') return '🤖';
  return '📄';
}

// Validation error type
interface ValidationError {
  line: number;
  column: number;
  message: string;
  severity: 'error' | 'warning';
}

// Track modified content per file
interface FileState {
  content: string;
  originalContent: string;
}

// Server info returned by /api/server-info
interface ServerInfo {
  ca_pem_path: string;
  https_url: string;
  tls_enabled: boolean;
  hostnames: string[];
}

export default function ConfigEditorPage() {
  const [configs, setConfigs] = useState<ClaudeConfigFile[]>([]);
  const [selectedConfig, setSelectedConfig] = useState<ClaudeConfigFile | null>(null);
  const [content, setContent] = useState("");
  const [originalContent, setOriginalContent] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [validationErrors, setValidationErrors] = useState<ValidationError[]>([]);
  const [isValidating, setIsValidating] = useState(false);
  // Per-file state tracking for multi-file navigation
  const [fileStates, setFileStates] = useState<Record<string, FileState>>({});
  const [serverInfo, setServerInfo] = useState<ServerInfo | null>(null);
  const [copiedField, setCopiedField] = useState<string | null>(null);

  const clientRef = useRef<ReturnType<typeof createPromiseClient<typeof SessionService>> | null>(null);
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const monacoRef = useRef<typeof import("monaco-editor") | null>(null);

  // Check if current file requires validation (JSON files)
  const requiresValidation = selectedConfig?.name.endsWith('.json') ?? false;
  const hasValidationErrors = validationErrors.some(e => e.severity === 'error');
  const canSave = !hasValidationErrors || !requiresValidation;

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
    });
    clientRef.current = createPromiseClient(SessionService, transport);
  }, []);

  // Load configs on mount
  useEffect(() => {
    loadConfigs();
  }, []);

  // Load server info on mount
  useEffect(() => {
    fetch('/api/server-info')
      .then(r => r.json())
      .then((data: ServerInfo) => setServerInfo(data))
      .catch(() => { /* server-info not critical, ignore errors */ });
  }, []);

  // Validate JSON content and update Monaco markers
  const validateContent = useCallback((value: string, filename: string) => {
    if (!filename.endsWith('.json')) {
      setValidationErrors([]);
      return;
    }

    setIsValidating(true);
    const errors: ValidationError[] = [];

    try {
      JSON.parse(value);
      // JSON is valid - clear errors
    } catch (e) {
      if (e instanceof SyntaxError) {
        // Extract line/column from JSON parse error message
        const match = e.message.match(/at position (\d+)/);
        let line = 1;
        let column = 1;

        if (match) {
          const position = parseInt(match[1], 10);
          // Convert position to line/column
          const lines = value.substring(0, position).split('\n');
          line = lines.length;
          column = lines[lines.length - 1].length + 1;
        }

        errors.push({
          line,
          column,
          message: e.message.replace(/^JSON\.parse: /, ''),
          severity: 'error',
        });
      }
    }

    setValidationErrors(errors);
    setIsValidating(false);

    // Update Monaco editor markers
    if (monacoRef.current && editorRef.current) {
      const model = editorRef.current.getModel();
      if (model) {
        const markers = errors.map(err => ({
          severity: err.severity === 'error'
            ? monacoRef.current!.MarkerSeverity.Error
            : monacoRef.current!.MarkerSeverity.Warning,
          startLineNumber: err.line,
          startColumn: err.column,
          endLineNumber: err.line,
          endColumn: err.column + 1,
          message: err.message,
        }));
        monacoRef.current.editor.setModelMarkers(model, 'json-validator', markers);
      }
    }
  }, []);

  // Editor mount handler
  const handleEditorMount: OnMount = (editor, monaco) => {
    editorRef.current = editor;
    monacoRef.current = monaco;

    // Enable JSON validation in Monaco
    monaco.languages.json.jsonDefaults.setDiagnosticsOptions({
      validate: true,
      allowComments: false,
      schemas: [],
      enableSchemaRequest: false,
    });

    // Validate initial content if JSON file
    if (selectedConfig?.name.endsWith('.json') && content) {
      validateContent(content, selectedConfig.name);
    }
  };

  // Validate on content change (debounced)
  useEffect(() => {
    if (!selectedConfig) return;

    const timeoutId = setTimeout(() => {
      validateContent(content, selectedConfig.name);
    }, 300); // 300ms debounce

    return () => clearTimeout(timeoutId);
  }, [content, selectedConfig, validateContent]);

  // Clear validation errors when switching files
  useEffect(() => {
    setValidationErrors([]);
  }, [selectedConfig?.name]);

  const loadConfigs = async () => {
    if (!clientRef.current) return;

    try {
      setLoading(true);
      setError(null);
      const response = await clientRef.current.listClaudeConfigs({});
      setConfigs(response.configs);
    } catch (err) {
      setError(`Failed to load configs: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const loadConfig = async (filename: string) => {
    if (!clientRef.current) return;

    // Save current file state before switching
    if (selectedConfig && content !== undefined) {
      setFileStates(prev => ({
        ...prev,
        [selectedConfig.name]: {
          content,
          originalContent,
        }
      }));
    }

    // Check if we have cached state for this file
    const cachedState = fileStates[filename];
    if (cachedState) {
      // Find the config in our list
      const config = configs.find(c => c.name === filename);
      if (config) {
        setSelectedConfig(config);
        setContent(cachedState.content);
        setOriginalContent(cachedState.originalContent);
        return;
      }
    }

    try {
      setError(null);
      const response = await clientRef.current.getClaudeConfig({ filename });
      if (response.config) {
        setSelectedConfig(response.config);
        setContent(response.config.content);
        setOriginalContent(response.config.content);
        // Cache the initial state
        setFileStates(prev => ({
          ...prev,
          [filename]: {
            content: response.config!.content,
            originalContent: response.config!.content,
          }
        }));
      }
    } catch (err) {
      setError(`Failed to load config: ${err}`);
    }
  };

  const saveConfig = async () => {
    if (!selectedConfig || !clientRef.current) return;

    try {
      setSaving(true);
      setError(null);
      setSuccessMessage(null);

      await clientRef.current.updateClaudeConfig({
        filename: selectedConfig.name,
        content: content,
      });

      setOriginalContent(content);
      // Update cached file state to reflect saved content
      setFileStates(prev => ({
        ...prev,
        [selectedConfig.name]: {
          content,
          originalContent: content,
        }
      }));
      setSuccessMessage(`✓ Saved ${selectedConfig.name}`);

      // Auto-clear success message after 3 seconds
      setTimeout(() => setSuccessMessage(null), 3000);
    } catch (err) {
      setError(`Failed to save config: ${err}`);
    } finally {
      setSaving(false);
    }
  };

  const hasUnsavedChanges = content !== originalContent;

  // Check if a specific file has unsaved changes
  const fileHasUnsavedChanges = useCallback((filename: string): boolean => {
    // If it's the current file, use the live state
    if (selectedConfig?.name === filename) {
      return content !== originalContent;
    }
    // Otherwise check cached state
    const state = fileStates[filename];
    return state ? state.content !== state.originalContent : false;
  }, [selectedConfig, content, originalContent, fileStates]);

  // Count total unsaved files
  const unsavedFilesCount = configs.filter(c => fileHasUnsavedChanges(c.name)).length;

  // Passkey management
  const { authEnabled, authenticated, hasCredentials, refresh: refreshAuth } = useAuth();
  const [passkeyStatus, setPasskeyStatus] = useState<"idle" | "working" | "success" | "error">("idle");
  const [passkeyMsg, setPasskeyMsg] = useState("");

  const handleRegisterPasskey = async () => {
    setPasskeyStatus("working");
    setPasskeyMsg("");
    try {
      await registerPasskey();
      await refreshAuth();
      setPasskeyStatus("success");
      setPasskeyMsg("Passkey registered successfully.");
      setTimeout(() => { setPasskeyStatus("idle"); setPasskeyMsg(""); }, 4000);
    } catch (e) {
      setPasskeyStatus("error");
      setPasskeyMsg(e instanceof Error ? e.message : String(e));
    }
  };

  const handleRevokeAll = async () => {
    setPasskeyStatus("working");
    setPasskeyMsg("");
    try {
      await doLogout();
      await refreshAuth();
      setPasskeyStatus("success");
      setPasskeyMsg("All sessions revoked.");
      setTimeout(() => { setPasskeyStatus("idle"); setPasskeyMsg(""); }, 4000);
    } catch (e) {
      setPasskeyStatus("error");
      setPasskeyMsg(e instanceof Error ? e.message : String(e));
    }
  };

  const copyToClipboard = (text: string, field: string) => {
    navigator.clipboard.writeText(text).then(() => {
      setCopiedField(field);
      setTimeout(() => setCopiedField(null), 2000);
    });
  };

  // Handle keyboard shortcuts (Ctrl+S to save, Ctrl+1-9 for file switching)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Ctrl+S or Cmd+S to save
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        if (hasUnsavedChanges && !saving && canSave) {
          saveConfig();
        }
        return;
      }

      // Ctrl+1-9 or Cmd+1-9 for quick file switching
      if ((e.ctrlKey || e.metaKey) && e.key >= '1' && e.key <= '9') {
        e.preventDefault();
        const index = parseInt(e.key, 10) - 1;
        if (index < configs.length) {
          loadConfig(configs[index].name);
        }
        return;
      }

      // Ctrl+[ or Cmd+[ for previous file
      if ((e.ctrlKey || e.metaKey) && e.key === '[') {
        e.preventDefault();
        if (selectedConfig && configs.length > 0) {
          const currentIndex = configs.findIndex(c => c.name === selectedConfig.name);
          const prevIndex = currentIndex > 0 ? currentIndex - 1 : configs.length - 1;
          loadConfig(configs[prevIndex].name);
        }
        return;
      }

      // Ctrl+] or Cmd+] for next file
      if ((e.ctrlKey || e.metaKey) && e.key === ']') {
        e.preventDefault();
        if (selectedConfig && configs.length > 0) {
          const currentIndex = configs.findIndex(c => c.name === selectedConfig.name);
          const nextIndex = currentIndex < configs.length - 1 ? currentIndex + 1 : 0;
          loadConfig(configs[nextIndex].name);
        }
        return;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [hasUnsavedChanges, saving, canSave, content, selectedConfig, configs, loadConfig]);

  return (
    <main id="main-content" className={styles.container}>
      <h1 className={styles.title}>
        📝 Claude Config Editor
      </h1>

      {error && (
        <div className="alert alert-error">
          {error}
        </div>
      )}

      {successMessage && (
        <div className="alert alert-success">
          {successMessage}
        </div>
      )}

      <div className={styles.content}>
        {/* File list */}
        <div className={styles.fileList}>
          <h2 className={styles.sectionTitle}>
            Config Files
            {unsavedFilesCount > 0 && (
              <span className={styles.unsavedCount}>
                {unsavedFilesCount} unsaved
              </span>
            )}
          </h2>
          {loading ? (
            <div>Loading...</div>
          ) : (
            <div className={styles.fileListItems}>
              {configs.map((config, index) => (
                <button
                  key={config.name}
                  onClick={() => loadConfig(config.name)}
                  className={`${styles.fileButton} ${
                    selectedConfig?.name === config.name ? styles.fileButtonSelected : ""
                  } ${fileHasUnsavedChanges(config.name) ? styles.fileButtonModified : ""}`}
                  title={`${config.name}${index < 9 ? ` (Ctrl+${index + 1})` : ''}`}
                >
                  <span className={styles.fileIcon}>{getFileIcon(config.name)}</span>
                  <span className={styles.fileName}>{config.name}</span>
                  {fileHasUnsavedChanges(config.name) && (
                    <span className={styles.fileModifiedDot}>●</span>
                  )}
                  {index < 9 && (
                    <span className={styles.fileShortcut}>{index + 1}</span>
                  )}
                </button>
              ))}
            </div>
          )}
          {/* Keyboard shortcuts help */}
          <div className={styles.shortcutsHelp}>
            <div className={styles.shortcutsTitle}>Shortcuts</div>
            <div className={styles.shortcutItem}>
              <kbd>Ctrl+S</kbd> Save
            </div>
            <div className={styles.shortcutItem}>
              <kbd>Ctrl+1-9</kbd> Switch file
            </div>
            <div className={styles.shortcutItem}>
              <kbd>Ctrl+[/]</kbd> Prev/Next
            </div>
          </div>
        </div>

        {/* Editor */}
        <div className={styles.editor}>
          {selectedConfig ? (
            <>
              <div className={styles.editorHeader}>
                <h2 className={styles.editorTitle}>
                  {selectedConfig.name}
                  {hasUnsavedChanges && (
                    <span className={styles.modifiedIndicator}>
                      [modified]
                    </span>
                  )}
                </h2>
                <div className={styles.buttonGroup}>
                  {requiresValidation && (
                    <span className={hasValidationErrors ? styles.validationBadgeError : styles.validationBadgeValid}>
                      {isValidating ? '⏳ Validating...' : hasValidationErrors ? `❌ ${validationErrors.length} error(s)` : '✓ Valid JSON'}
                    </span>
                  )}
                  <button
                    onClick={saveConfig}
                    disabled={!hasUnsavedChanges || saving || !canSave}
                    className="btn btn-primary"
                    title={!canSave ? 'Fix validation errors before saving' : ''}
                  >
                    {saving ? "Saving..." : "Save"}
                  </button>
                  <button
                    onClick={() => setContent(originalContent)}
                    disabled={!hasUnsavedChanges}
                    className="btn btn-danger"
                  >
                    Discard
                  </button>
                </div>
              </div>
              <Editor
                height={validationErrors.length > 0 ? "500px" : "600px"}
                language={getLanguageFromFilename(selectedConfig.name)}
                theme="vs-dark"
                value={content}
                onChange={(value) => setContent(value || "")}
                onMount={handleEditorMount}
                options={{
                  readOnly: saving,
                  minimap: { enabled: true },
                  fontSize: 14,
                  lineNumbers: 'on',
                  folding: true,
                  tabSize: 2,
                  automaticLayout: true,
                  scrollBeyondLastLine: false,
                  wordWrap: 'on',
                  formatOnPaste: true,
                  formatOnType: true,
                  bracketPairColorization: {
                    enabled: true,
                  },
                }}
              />
              {/* Validation Error Panel */}
              {validationErrors.length > 0 && (
                <div className={styles.validationPanel}>
                  <div className={styles.validationPanelHeader}>
                    <span className={styles.validationPanelTitle}>
                      ⚠️ Validation Errors ({validationErrors.length})
                    </span>
                  </div>
                  <div className={styles.validationPanelContent}>
                    {validationErrors.map((err, index) => (
                      <div
                        key={index}
                        className={`${styles.validationError} ${err.severity === 'error' ? styles.validationErrorSeverity : styles.validationWarningSeverity}`}
                        onClick={() => {
                          // Jump to error location in editor
                          if (editorRef.current) {
                            editorRef.current.setPosition({ lineNumber: err.line, column: err.column });
                            editorRef.current.focus();
                            editorRef.current.revealLineInCenter(err.line);
                          }
                        }}
                      >
                        <span className={styles.validationErrorLocation}>
                          Line {err.line}, Col {err.column}:
                        </span>
                        <span className={styles.validationErrorMessage}>
                          {err.message}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </>
          ) : (
            <div className={styles.emptyState}>
              Select a config file to edit
            </div>
          )}
        </div>
      </div>

      <div className={styles.networkSection}>
        <h2 className={styles.sectionTitle}>🌐 Network &amp; Remote Access</h2>
        <div className={styles.networkCard}>
          {serverInfo ? (
            <>
              <div className={styles.networkRow}>
                <span className={styles.networkLabel}>HTTPS URL</span>
                {serverInfo.https_url ? (
                  <div className={styles.networkValue}>
                    <span className={styles.networkValueText} title={serverInfo.https_url}>
                      {serverInfo.https_url}
                    </span>
                    <a href={serverInfo.https_url} target="_blank" rel="noreferrer" className={styles.networkLink}>
                      Open
                    </a>
                    <button
                      className={styles.networkCopyBtn}
                      onClick={() => copyToClipboard(serverInfo.https_url, 'httpsUrl')}
                    >
                      {copiedField === 'httpsUrl' ? 'Copied!' : 'Copy'}
                    </button>
                  </div>
                ) : (
                  <span className={styles.networkDisabledNote}>Remote access not enabled (start with --remote-access)</span>
                )}
              </div>
              <div className={styles.networkRow}>
                <span className={styles.networkLabel}>CA Certificate Path</span>
                {serverInfo.tls_enabled && serverInfo.ca_pem_path ? (
                  <div className={styles.networkValue}>
                    <span className={styles.networkValueText} title={serverInfo.ca_pem_path}>
                      {serverInfo.ca_pem_path}
                    </span>
                    <button
                      className={styles.networkCopyBtn}
                      onClick={() => copyToClipboard(serverInfo.ca_pem_path, 'caPath')}
                    >
                      {copiedField === 'caPath' ? 'Copied!' : 'Copy'}
                    </button>
                    <a href="/auth/ca.pem" download="stapler-squad-ca.pem" className={styles.networkLink}>
                      Download
                    </a>
                  </div>
                ) : (
                  <span className={styles.networkDisabledNote}>TLS not active</span>
                )}
              </div>
              <div className={styles.networkRow}>
                <span className={styles.networkLabel}>Detected Hostnames</span>
                {serverInfo.hostnames && serverInfo.hostnames.length > 0 ? (
                  <div className={styles.hostnamesList}>
                    {serverInfo.hostnames.map((hn, idx) => (
                      <div key={idx} className={styles.hostnameItem}>
                        <span className={styles.hostnameText}>{hn}</span>
                        <button
                          className={styles.networkCopyBtn}
                          onClick={() => copyToClipboard(hn, `hostname-${idx}`)}
                        >
                          {copiedField === `hostname-${idx}` ? 'Copied!' : 'Copy'}
                        </button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <span className={styles.networkDisabledNote}>No LAN hostnames detected</span>
                )}
              </div>
            </>
          ) : (
            <span className={styles.networkDisabledNote}>Loading…</span>
          )}
        </div>
      </div>

      {authEnabled && (
        <div className={styles.securitySection}>
          <h2 className={styles.sectionTitle}>🔐 Passkey Security</h2>
          <div className={styles.securityCard}>
            <div className={styles.securityRow}>
              <span className={styles.securityLabel}>Authentication</span>
              <span className={styles.statusEnabled}>Active</span>
            </div>
            <div className={styles.securityRow}>
              <span className={styles.securityLabel}>Passkeys registered</span>
              <span>{hasCredentials ? "✓ Yes" : "None"}</span>
            </div>
            <div className={styles.securityActions}>
              {authenticated && (
                <button
                  onClick={handleRegisterPasskey}
                  disabled={passkeyStatus === "working"}
                  className="btn btn-primary"
                >
                  {passkeyStatus === "working" ? "Working…" : "Register New Passkey"}
                </button>
              )}
              <button
                onClick={handleRevokeAll}
                disabled={passkeyStatus === "working" || !authenticated}
                className="btn btn-danger"
              >
                Sign Out All Sessions
              </button>
            </div>
            {passkeyStatus === "error" && (
              <p className={styles.securityError}>{passkeyMsg}</p>
            )}
            {passkeyStatus === "success" && (
              <p className={styles.securitySuccess}>{passkeyMsg}</p>
            )}
          </div>
        </div>
      )}
    </main>
  );
}
