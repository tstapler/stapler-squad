"use client";

import { GlobalDefaultsForm } from "@/components/settings/GlobalDefaultsForm";
import { ProfilesManager } from "@/components/settings/ProfilesManager";
import { DirectoryRulesManager } from "@/components/settings/DirectoryRulesManager";
import { PushNotificationSettings } from "@/components/settings/PushNotificationSettings";
import * as styles from "./defaults.css";

export default function DefaultsPage() {
  return (
    <main id="main-content" className={styles.container}>
      <h1 className={styles.title}>Session Defaults</h1>
      <div className={styles.sections}>
        <section className={styles.section}>
          <GlobalDefaultsForm />
        </section>
        <section className={styles.section}>
          <ProfilesManager />
        </section>
        <section className={styles.section}>
          <DirectoryRulesManager />
        </section>
        <section className={styles.section}>
          <PushNotificationSettings />
        </section>
      </div>
    </main>
  );
}
