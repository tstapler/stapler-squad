"use client";

import { ReactNode } from "react";
import {
  wizard,
  steps as stepsClass,
  step,
  stepNumber,
  stepLabel,
  stepConnector,
  active,
  activeStepNumber,
  activeStepLabel,
  completed,
  completedStepNumber,
  completedStepLabel,
  completedStepConnector,
  pending,
  content,
  wizardActions,
} from "./Wizard.css";

interface WizardProps {
  currentStep: number;
  steps: string[];
  children: ReactNode;
}

export function Wizard({ currentStep, steps, children }: WizardProps) {
  return (
    <div className={wizard}>
      <div className={stepsClass}>
        {steps.map((stepName, index) => (
          <div
            key={index}
            className={`${step} ${
              index < currentStep
                ? completed
                : index === currentStep
                ? active
                : pending
            }`}
          >
            <div className={
              index < currentStep
                ? completedStepNumber
                : index === currentStep
                ? activeStepNumber
                : stepNumber
            }>
              {index < currentStep ? "✓" : index + 1}
            </div>
            <div
              className={
                index < currentStep
                  ? completedStepLabel
                  : index === currentStep
                  ? activeStepLabel
                  : stepLabel
              }
              data-testid="wizard-step-label"
            >{stepName}</div>
            {index < steps.length - 1 && (
              <div className={index < currentStep ? completedStepConnector : stepConnector} />
            )}
          </div>
        ))}
      </div>
      <div className={content}>{children}</div>
    </div>
  );
}

interface WizardActionsProps {
  children: ReactNode;
}

export function WizardActions({ children }: WizardActionsProps) {
  return <div className={wizardActions}>{children}</div>;
}
