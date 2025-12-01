"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { SessionService } from "@/gen/session/v1/session_connect";
import { ClaudeConfigFile } from "@/gen/session/v1/session_pb";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import Editor, { OnMount } from "@monaco-editor/react";
import type { editor } from "monaco-editor";
import styles from "./config.module.css";

// Helper function to determine Monaco editor language from filename
function getLanguageFromFilename(filename: string): string {
  if (filename.endsWith('.json')) return 'json';
  if (filename.endsWith('.md')) return 'markdown';
  return 'plaintext';
}

// Validation error type
interface ValidationError {
  line: number;
  column: number;
  message: string;
  severity: 'error' | 'warning';
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

  const clientRef = useRef<any>(null);
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const monacoRef = useRef<typeof import("monaco-editor") | null>(null);

  // Check if current file requires validation (JSON files)
  const requiresValidation = selectedConfig?.name.endsWith('.json') ?? false;
  const hasValidationErrors = validationErrors.some(e => e.severity === 'error');
  const canSave = !hasValidationErrors || !requiresValidation;

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: window.location.origin,
    });
    clientRef.current = createPromiseClient(SessionService, transport);
  }, []);

  // Load configs on mount
  useEffect(() => {
    loadConfigs();
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

    try {
      setError(null);
      const response = await clientRef.current.getClaudeConfig({ filename });
      if (response.config) {
        setSelectedConfig(response.config);
        setContent(response.config.content);
        setOriginalContent(response.config.content);
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

  // Handle keyboard shortcuts (Ctrl+S to save)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Ctrl+S or Cmd+S to save
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        if (hasUnsavedChanges && !saving && canSave) {
          saveConfig();
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [hasUnsavedChanges, saving, canSave, content, selectedConfig]);

  return (
    <div className={styles.container}>
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
          </h2>
          {loading ? (
            <div>Loading...</div>
          ) : (
            <div className={styles.fileListItems}>
              {configs.map((config) => (
                <button
                  key={config.name}
                  onClick={() => loadConfig(config.name)}
                  className={`${styles.fileButton} ${
                    selectedConfig?.name === config.name ? styles.fileButtonSelected : ""
                  }`}
                >
                  {config.name}
                </button>
              ))}
            </div>
          )}
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
    </div>
  );
}
