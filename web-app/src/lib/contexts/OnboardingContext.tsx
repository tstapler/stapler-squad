"use client";

import { createContext, useContext, ReactNode } from "react";
import { useOnboarding } from "@/components/onboarding/useOnboarding";
import { OnboardingModal } from "@/components/onboarding/OnboardingModal";

interface OnboardingContextValue {
  triggerOnboarding: () => void;
}

const OnboardingContext = createContext<OnboardingContextValue | null>(null);

export function useOnboardingContext(): OnboardingContextValue {
  const context = useContext(OnboardingContext);
  if (!context) {
    throw new Error("useOnboardingContext must be used within an OnboardingProvider");
  }
  return context;
}

interface OnboardingProviderProps {
  children: ReactNode;
}

export function OnboardingProvider({ children }: OnboardingProviderProps) {
  const { showOnboarding, setOnboardingComplete, resetOnboarding } = useOnboarding();

  const value: OnboardingContextValue = {
    triggerOnboarding: resetOnboarding,
  };

  return (
    <OnboardingContext.Provider value={value}>
      <OnboardingModal isOpen={showOnboarding} onClose={setOnboardingComplete} />
      {children}
    </OnboardingContext.Provider>
  );
}
