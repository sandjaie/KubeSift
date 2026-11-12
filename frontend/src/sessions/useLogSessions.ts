/*
Purpose: Mount the product log store once for the application lifetime.
Used by: App and shared Logs/Sessions presentation.
Responsibilities: Register event delivery before activation and release native sessions on unmount.
Important: Theme, Settings and detail remounts do not own or recreate this subscription.
*/
import { useEffect, useState, useSyncExternalStore } from "react";
import { ExplorerService } from "../../bindings/github.com/sandjaie/KubeSift/internal/desktop";
import { acknowledge, activateSession, onSessionEvent, stopSession } from "../bridge";
import { createLogStore } from "./logStore";
export function useLogSessions() {
  const [store] = useState(() =>
    createLogStore({
      subscribe: onSessionEvent,
      open: ExplorerService.OpenPodLogs,
      close: ExplorerService.ClosePodLogs,
      activate: activateSession,
      ack: acknowledge,
      stop: stopSession,
    }),
  );
  useEffect(() => {
    store.attach();
    return () => {
      void store.dispose();
    };
  }, [store]);
  const sessions = useSyncExternalStore(store.subscribe, store.snapshot);
  return { store, sessions };
}
export type LogSessionsController = ReturnType<typeof useLogSessions>;
