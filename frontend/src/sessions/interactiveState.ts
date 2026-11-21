/*
Purpose: Own interactive session identity independently of the visible workspace.
Used by: The app-level Sessions hook and the dock.
Responsibilities: Register before activation, track state frames and release cancelled or closed sessions.
Important: View changes never replay commands or retarget a stream.
*/
import { message } from "../connections/api";
import type { ConnectionRef, SessionEvent, SessionRef } from "../bridge";
import type { InteractiveEntry, ResourceRef } from "../../bindings/internal/kube/models";

export type OwnedInteractive = InteractiveEntry & { tab: string; cluster: string };
export type InteractiveStart = () => { promise: Promise<InteractiveEntry>; cancel: () => void };
interface InteractiveBridge {
  subscribe: (receive: (event: SessionEvent) => void) => () => void;
  activate: (ref: SessionRef) => Promise<void>;
  stop: (ref: SessionRef) => Promise<void>;
  close: (ref: SessionRef) => Promise<void>;
}
export const sameConnection = (a: ConnectionRef, b?: ConnectionRef | null) => a.id === b?.id && a.generation === b.generation;
export const interactiveActive = (state: string) => state === "connecting" || state === "running" || state === "listening";

export function createInteractiveStore(bridge: InteractiveBridge) {
  let entries: OwnedInteractive[] = [];
  const subscribers = new Set<() => void>();
  const pending = new Set<{ tab: string; connection: ConnectionRef; abort: () => void }>();
  const activated = new Set<string>();
  const sequences = new Map<string, number>();
  const closing = new Map<string, Promise<void>>();
  let off: (() => void) | undefined;
  const publish = (next: OwnedInteractive[]) => {
    entries = next;
    subscribers.forEach((fn) => fn());
  };
  const find = (id: string) => entries.find((entry) => entry.session.id === id);
  const update = (id: string, patch: Partial<OwnedInteractive>) => {
    if (find(id)) publish(entries.map((entry) => (entry.session.id === id ? { ...entry, ...patch } : entry)));
  };
  function receive(event: SessionEvent) {
    const current = find(event.session.id);
    if (
      !current ||
      !interactiveActive(current.state) ||
      event.kind !== "state" ||
      !sameConnection(current.session.connection, event.session.connection) ||
      event.sequence <= (sequences.get(event.session.id) ?? 0)
    )
      return;
    if (!event.data) return;
    let next: Pick<InteractiveEntry, "state" | "localPort" | "problem">;
    try {
      next = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(event.data), (value) => value.charCodeAt(0))));
    } catch {
      return; // Generic session-manager state text is shared with legacy callers.
    }
    if (!next || !["running", "listening", "ended", "failed"].includes(next.state)) return;
    if (next.state === "listening" && (!Number.isInteger(next.localPort) || next.localPort < 1 || next.localPort > 65535)) return;
    sequences.set(current.session.id, event.sequence);
    update(current.session.id, {
      state: next.state,
      localPort: next.state === "listening" ? next.localPort : 0,
      problem: next.problem ?? null,
    });
  }

  async function open(tab: string, cluster: string, target: ResourceRef, start: InteractiveStart, signal?: AbortSignal) {
    if (signal?.aborted || !off) throw new Error("Session start was cancelled.");
    if (entries.length + pending.size >= 8) throw new Error("Close a session before starting another (maximum eight).");
    const request = start();
    let cancelled = false;
    const abort = () => {
      if (cancelled) return;
      cancelled = true;
      request.cancel();
    };
    const reservation = { tab, connection: target.connection, abort };
    pending.add(reservation);
    signal?.addEventListener("abort", abort, { once: true });
    try {
      const entry = await request.promise;
      const actual = entry.target.resource;
      if (
        cancelled ||
        signal?.aborted ||
        !off ||
        !sameConnection(entry.session.connection, target.connection) ||
        !sameConnection(actual.connection, target.connection) ||
        actual.uid !== target.uid ||
        actual.name !== target.name ||
        actual.namespace !== target.namespace ||
        actual.kind !== target.kind
      ) {
        await bridge.close(entry.session);
        throw new Error("Session start was cancelled or its target changed.");
      }
      publish([...entries, { ...entry, tab, cluster }]);
      return entry;
    } finally {
      pending.delete(reservation);
      signal?.removeEventListener("abort", abort);
    }
  }

  function activate(ref: SessionRef) {
    const current = find(ref.id);
    if (!current || current.state !== "connecting" || !sameConnection(ref.connection, current.session.connection) || activated.has(ref.id))
      return;
    activated.add(ref.id);
    void bridge.activate(ref).catch((error) => {
      if (find(ref.id)?.state !== "connecting") return;
      update(ref.id, { state: "failed", problem: { code: "activation", message: message(error) } });
      void bridge.stop(ref).catch(() => {});
    });
  }

  async function stop(id: string) {
    const current = find(id);
    if (!current || (!interactiveActive(current.state) && current.state !== "cleanup-error")) return;
    update(id, { state: "stopping", localPort: 0 });
    try {
      await bridge.stop(current.session);
      if (find(id)?.state === "stopping") update(id, { state: "stopped", problem: null });
    } catch (error) {
      if (find(id)?.state === "stopping") update(id, { state: "cleanup-error", problem: { code: "cleanup", message: message(error) } });
    }
  }

  function close(id: string) {
    const previous = closing.get(id);
    if (previous) return previous;
    const current = find(id);
    if (!current) return Promise.resolve();
    update(id, { state: "closing", localPort: 0 });
    const job = bridge
      .close(current.session)
      .then(() => {
        activated.delete(id);
        sequences.delete(id);
        publish(entries.filter((entry) => entry.session.id !== id));
      })
      .catch((error: unknown) => {
        update(id, { state: "cleanup-error", problem: { code: "cleanup", message: message(error) } });
        throw error;
      })
      .finally(() => closing.delete(id));
    closing.set(id, job);
    return job;
  }

  return {
    attach: () => {
      off ??= bridge.subscribe(receive);
    },
    subscribe: (callback: () => void) => {
      subscribers.add(callback);
      return () => {
        subscribers.delete(callback);
      };
    },
    snapshot: () => entries,
    open,
    activate,
    stop,
    close,
    syncConnections: (valid: (tab: string, ref: ConnectionRef) => boolean) => {
      for (const request of pending) if (!valid(request.tab, request.connection)) request.abort();
      for (const entry of entries) {
        if ((interactiveActive(entry.state) || entry.state === "stopping") && !valid(entry.tab, entry.session.connection)) {
          update(entry.session.id, { state: "disconnected", localPort: 0 });
          void bridge.stop(entry.session).catch(() => {});
        }
      }
    },
    countTab: (tab: string) =>
      entries.filter((entry) => entry.tab === tab).length + Array.from(pending).filter((entry) => entry.tab === tab).length,
    closeTab: async (tab: string) => {
      for (const request of pending) if (request.tab === tab) request.abort();
      await Promise.all(entries.filter((entry) => entry.tab === tab).map((entry) => close(entry.session.id)));
    },
    dispose: async () => {
      off?.();
      off = undefined;
      for (const request of pending) request.abort();
      await Promise.allSettled(entries.map((entry) => close(entry.session.id)));
    },
  };
}
