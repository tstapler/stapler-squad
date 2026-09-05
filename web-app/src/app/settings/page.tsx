"use client";

// analytics-exempt
import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import * as Tabs from "@radix-ui/react-tabs";
import { GlobalDefaultsForm } from "@/components/settings/GlobalDefaultsForm";
import { ProfilesManager } from "@/components/settings/ProfilesManager";
import { DirectoryRulesManager } from "@/components/settings/DirectoryRulesManager";
import { AliasesManager } from "@/components/settings/AliasesManager";
import { PushNotificationSettings } from "@/components/settings/PushNotificationSettings";
import { SlackNotificationSettings } from "@/components/settings/SlackNotificationSettings";
import { ThemePicker } from "@/components/settings/ThemePicker";
import { InputModeSetting } from "@/components/settings/InputModeSetting";
import { ConfigPageContent } from "@/app/config/ConfigPageContent";
import { KeyboardShortcutsTab } from "./KeyboardShortcutsTab";
import { usePageView } from "@/lib/analytics/usePageView";
import { useOnboardingContext } from "@/lib/contexts/OnboardingContext";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import { routes } from "@/lib/routes";
import Link from "next/link";
import * as styles from "./settings.css";

function SettingsPageInner() {
  usePageView();
  const { triggerOnboarding } = useOnboardingContext();
  const { flags } = useFeatureFlags();
  const searchParams = useSearchParams();
  const tabParam = searchParams.get("tab");
  const validTabs = ["general", "config-files", "appearance", "keyboard-shortcuts"];
  const defaultValue = validTabs.includes(tabParam ?? "") ? (tabParam as string) : "general";

  return (
    <main id="main-content" className={styles.pageRoot}>
      <h1 className={styles.pageTitle}>Settings</h1>

      <Tabs.Root defaultValue={defaultValue}>
        <div className={styles.tabRow}>
          <Tabs.List className={styles.tabList} aria-label="Settings tabs">
            <Tabs.Trigger value="general" className={styles.tab({})}>
              General
            </Tabs.Trigger>
            <Tabs.Trigger value="config-files" className={styles.tab({})}>
              Config Files
            </Tabs.Trigger>
            <Tabs.Trigger value="appearance" className={styles.tab({})}>
              Appearance
            </Tabs.Trigger>
            <Tabs.Trigger value="keyboard-shortcuts" className={styles.tab({})}>
              Keyboard Shortcuts
            </Tabs.Trigger>
          </Tabs.List>
          {/* Pipeline Modes is a standalone route (its own list/create/edit
              management UI), not tab-panel content — linked here rather than
              embedded so it keeps its own page identity, same as Backlog
              Sources' link inside the General tab below. Previously the only
              way to reach it was typing the URL directly. */}
          <Link
            href={routes.settingsPipelineModes}
            className={styles.externalTabLink}
            data-testid="settings-pipeline-modes-tab-link"
          >
            Pipeline Modes ↗
          </Link>
        </div>

        {/* General tab */}
        <Tabs.Content value="general" className={styles.tabPanel}>
          <div className={styles.sectionGroup}>
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
              <AliasesManager />
            </section>
            {flags["backlog"] && (
              <section className={styles.section}>
                <Link href={routes.settingsBacklogSources} className={styles.helpLink}>
                  Backlog Sources (GitHub sync) →
                </Link>
              </section>
            )}
            <section className={styles.section}>
              <Link href={routes.settingsPipelineModes} className={styles.helpLink}>
                Pipeline Modes →
              </Link>
            </section>
            <section className={styles.section}>
              <Link href={routes.settingsRemotes} className={styles.helpLink} data-testid="settings-remotes-link">
                Remotes (SSH hosts) →
              </Link>
            </section>
            <section className={styles.section}>
              <Link href={routes.settingsJules} className={styles.helpLink} data-testid="settings-jules-link">
                Jules (Google cloud agent) →
              </Link>
            </section>
            {/* Help subsection */}
            <section className={styles.section}>
              <div className={styles.helpSection}>
                <div className={styles.helpSectionTitle}>Help</div>
                <div className={styles.helpRow}>
                  <button
                    className={styles.helpButton}
                    onClick={triggerOnboarding}
                  >
                    Show onboarding tour again
                  </button>
                  <Link href={routes.help} className={styles.helpLink}>
                    View documentation
                  </Link>
                </div>
              </div>
            </section>
          </div>
        </Tabs.Content>

        {/* Config Files tab */}
        <Tabs.Content value="config-files" className={styles.tabPanel}>
          <Suspense fallback={<div>Loading...</div>}>
            <ConfigPageContent />
          </Suspense>
        </Tabs.Content>

        {/* Appearance tab */}
        <Tabs.Content value="appearance" className={styles.tabPanel}>
          <div className={styles.sectionGroup}>
            <section className={styles.section}>
              <ThemePicker />
            </section>
            <section className={styles.section}>
              <InputModeSetting />
            </section>
            <section className={styles.section}>
              <PushNotificationSettings />
            </section>
            <section className={styles.section}>
              <SlackNotificationSettings />
            </section>
          </div>
        </Tabs.Content>

        {/* Keyboard Shortcuts tab */}
        <Tabs.Content value="keyboard-shortcuts" className={styles.tabPanel}>
          <Suspense fallback={<div>Loading...</div>}>
            <KeyboardShortcutsTab />
          </Suspense>
        </Tabs.Content>
      </Tabs.Root>
    </main>
  );
}

export default function SettingsPage() {
  return (
    <Suspense fallback={<div>Loading settings...</div>}>
      <SettingsPageInner />
    </Suspense>
  );
}
