import React, { useState } from "react";
import { getFileIcon } from "@/lib/utils/fileIcons";
import * as styles from "./RecentFilesSection.css";

interface RecentFilesSectionProps {
  paths: string[];
  selectedPath: string | null | undefined;
  onSelect: (path: string) => void;
}

export function RecentFilesSection({
  paths,
  selectedPath,
  onSelect,
}: RecentFilesSectionProps): React.ReactElement | null {
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem("filesTab.recentCollapsed") === "true";
    } catch {
      return false;
    }
  });

  const toggle = () => {
    setCollapsed((v) => {
      const next = !v;
      try {
        localStorage.setItem("filesTab.recentCollapsed", String(next));
      } catch {}
      return next;
    });
  };

  if (paths.length === 0) {
    return null;
  }

  return (
    <div className={styles.container}>
      <button
        className={styles.heading}
        onClick={toggle}
        aria-expanded={!collapsed}
      >
        <span className={styles.chevron}>{collapsed ? "▸" : "▾"}</span>
        Recent
      </button>
      {!collapsed && paths.map((path) => {
        const basename = path.split("/").pop() ?? path;
        const parentDir = path.includes("/")
          ? path.split("/").slice(-2, -1)[0] ?? ""
          : "";
        const icon = getFileIcon(basename);
        const isSelected = path === selectedPath;

        return (
          <button
            key={path}
            className={isSelected ? styles.entrySelected : styles.entry}
            title={path}
            onClick={() => onSelect(path)}
          >
            <span className={styles.entryIcon}>{icon}</span>
            <span className={styles.entryName}>{basename}</span>
            {parentDir && (
              <span className={styles.entryDir}>{parentDir}</span>
            )}
          </button>
        );
      })}
    </div>
  );
}
