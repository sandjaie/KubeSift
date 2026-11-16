/*
Purpose: Mount one product interactive session owner for the application lifetime.
Used by: App and the shared Sessions dock.
Responsibilities: Compose native bindings and subscribe to retained metadata.
Important: Terminal data acknowledgements remain with the mounted renderer.
*/
import { useEffect, useState, useSyncExternalStore } from "react";
import { ExplorerService } from "../../bindings/github.com/sandjaie/KubeSift/internal/desktop";
import { activateSession, onSessionEvent } from "../bridge";
import { readRequest } from "../workloads/api";
import { createInteractiveStore } from "./interactiveState";
import type { ForwardRequest, TerminalRequest } from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";

export function useInteractiveSessions() {
  const [store] = useState(() =>
    createInteractiveStore({
      subscribe: onSessionEvent,
      activate: activateSession,
      stop: ExplorerService.StopInteractive,
      close: ExplorerService.CloseInteractive,
    }),
  );
  useEffect(() => {
    store.attach();
    return () => {
      void store.dispose();
    };
  }, [store]);
  const entries = useSyncExternalStore(store.subscribe, store.snapshot);
  return { store, entries };
}

export const terminalStart = (request: TerminalRequest) => () => readRequest((id) => ExplorerService.OpenTerminal(id, request));
export const forwardStart = (request: ForwardRequest) => () => readRequest((id) => ExplorerService.OpenForward(id, request));
export type InteractiveSessionsController = ReturnType<typeof useInteractiveSessions>;
