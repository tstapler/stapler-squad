import { Skeleton } from "../ui/Skeleton";
import * as styles from "./SessionCard.css";

export function SessionCardSkeleton() {
  return (
    <div className={styles.card}>
      <div className={styles.header}>
        <div className={styles.titleRow}>
          <Skeleton width="60%" height={24} />
          <Skeleton width={80} height={24} />
        </div>
        <Skeleton width="30%" height={16} style={{ marginTop: "0.5rem" }} />
      </div>

      <div className={styles.body}>
        <div className={styles.info}>
          <div className={styles.infoRow}>
            <Skeleton width="25%" height={16} />
            <Skeleton width="50%" height={16} />
          </div>
          <div className={styles.infoRow}>
            <Skeleton width="25%" height={16} />
            <Skeleton width="40%" height={16} />
          </div>
          <div className={styles.infoRow}>
            <Skeleton width="25%" height={16} />
            <Skeleton width="70%" height={16} />
          </div>
        </div>

        <div className={styles.diffStats}>
          <Skeleton width={60} height={20} />
          <Skeleton width={60} height={20} />
        </div>
      </div>

      <div className={styles.footer}>
        <div className={styles.timestamps}>
          <Skeleton width="40%" height={14} />
          <Skeleton width="40%" height={14} />
        </div>

        <div className={styles.desktopActions}>
          <Skeleton width={80} height={36} />
        </div>
      </div>
    </div>
  );
}
