"use client";

import { useState, useEffect, useRef } from "react";
import { SessionService } from "@/gen/session/v1/session_connect";
import { ClaudeConfigFile } from "@/gen/session/v1/session_pb";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import Editor from "@monaco-editor/react";
import styles from "./config.module.css";

// Helper function to determine Monaco editor language from filename
function getLanguageFromFilename(filename: string): string {
  if (filename.endsWith('.json')) return 'json';
  if (filename.endsWith('.md')) return 'markdown';
  return 'plaintext';
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

  const clientRef = useRef<any>(null);

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
                  <button
                    onClick={saveConfig}
                    disabled={!hasUnsavedChanges || saving}
                    className="btn btn-primary"
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
                height="600px"
                language={getLanguageFromFilename(selectedConfig.name)}
                theme="vs-dark"
                value={content}
                onChange={(value) => setContent(value || "")}
                options={{
                  minimap: { enabled: true },
                  fontSize: 14,
                  lineNumbers: 'on',
                  scrollBeyondLastLine: false,
                  automaticLayout: true,
                }}
              />
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
