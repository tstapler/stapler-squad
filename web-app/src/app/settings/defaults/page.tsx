"use client";

import { GlobalDefaultsForm } from "@/components/settings/GlobalDefaultsForm";
import { ProfilesManager } from "@/components/settings/ProfilesManager";
import { DirectoryRulesManager } from "@/components/settings/DirectoryRulesManager";
import { PushNotificationSettings } from "@/components/settings/PushNotificationSettings";
import { ThemePicker } from "@/components/settings/ThemePicker";
import { usePageView } from "@/lib/analytics/usePageView";
import * as styles from "./defaults.css";

export default function DefaultsPage() {
  usePageView();
  return (
    <main id="main-content" className={styles.container}>
      <h1 className={styles.title}>Session Defaults</h1>
      <div className={styles.sections}>
        {/* Story 1.4.5 — Theme picker at top of settings */}
        <section className={styles.section}>
          <ThemePicker />
        </section>
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
