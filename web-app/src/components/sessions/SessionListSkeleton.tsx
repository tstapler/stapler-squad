import { SessionCardSkeleton } from "./SessionCardSkeleton";
import * as styles from "./SessionList.css";

interface SessionListSkeletonProps {
  count?: number;
}

export function SessionListSkeleton({ count = 6 }: SessionListSkeletonProps) {
  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div style={{ display: "flex", alignItems: "center", gap: "1rem" }}>
          <div
            style={{ width: "120px", height: "32px", background: "var(--hover-background)", borderRadius: "4px" }}
          />
        </div>

        <div className={styles.filters}>
          <div
            style={{ width: "200px", height: "38px", background: "var(--hover-background)", borderRadius: "4px" }}
          />
          <div
            style={{ width: "150px", height: "38px", background: "var(--hover-background)", borderRadius: "4px" }}
          />
          <div
            style={{ width: "150px", height: "38px", background: "var(--hover-background)", borderRadius: "4px" }}
          />
        </div>
      </div>

      <div className={styles.sessionList}>
        <div className={styles.categoryGroup}>
          <div
            style={{
              width: "200px",
              height: "24px",
              background: "var(--hover-background)",
              borderRadius: "4px",
              marginBottom: "1rem",
            }}
          />
          <div className={styles.categoryContent}>
            {Array.from({ length: count }).map((_, i) => (
              <SessionCardSkeleton key={i} />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
