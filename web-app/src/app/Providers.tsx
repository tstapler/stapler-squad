"use client";

import { Provider } from "react-redux";
import { store } from "@/lib/store/store";
import { NotificationProvider } from "@/lib/contexts/NotificationContext";
import { OmnibarProvider } from "@/lib/contexts/OmnibarContext";
import { ReviewQueueProvider } from "@/lib/contexts/ReviewQueueContext";
import { ApprovalsProvider } from "@/lib/contexts/ApprovalsContext";
import { GlobalSessionServiceProvider } from "@/lib/contexts/SessionServiceContext";

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <Provider store={store}>
      <NotificationProvider>
        <GlobalSessionServiceProvider>
          <OmnibarProvider>
            <ReviewQueueProvider>
              <ApprovalsProvider>
                {children}
              </ApprovalsProvider>
            </ReviewQueueProvider>
          </OmnibarProvider>
        </GlobalSessionServiceProvider>
      </NotificationProvider>
    </Provider>
  );
}
