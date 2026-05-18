import React from "react";
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
  if (paths.length === 0) {
    return null;
  }

  return (
    <div className={styles.container}>
      <div className={styles.heading}>Recent</div>
      {paths.map((path) => {
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
