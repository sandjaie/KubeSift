/*
Purpose: Own one bounded log history and subscription shared by detail and dock.
Used by: The application log hook and focused lifecycle tests.
Responsibilities: Decode split UTF-8, acknowledge offscreen frames and distinguish stop from close.
Important: Eight retained sessions, 1 MiB text each and explicit loss; no automatic reconnect or persistence.
*/
import type { ConnectionRef, SessionEvent, SessionRef } from "../bridge";
import type { LogRequest } from "../workloads/model";
export interface LogBridge {
  subscribe: (receive: (event: SessionEvent) => void) => () => void;
  open: (req: LogRequest) => Promise<SessionRef>;
  activate: (ref: SessionRef) => Promise<void>;
  ack: (ref: SessionRef, sequence: number) => Promise<void>;
  stop: (ref: SessionRef) => Promise<void>;
  close: (ref: SessionRef) => Promise<void>;
}
export interface LogSession {
  id: string;
  tab: string;
  label: string;
  request: LogRequest;
  ref: SessionRef | null;
  text: string;
  status: string;
  error: string;
  dropped: number;
  loss: string;
  segments: number;
  active: boolean;
}
const encoder = new TextEncoder();
export function scrollLogViewport(viewport: { scrollTop: number; scrollHeight: number } | null, enabled: boolean) {
  if (viewport && enabled) viewport.scrollTop = viewport.scrollHeight;
}
function boundedText(text: string, limit: number) {
  const bytes = encoder.encode(text);
  if (bytes.length <= limit) return { text, dropped: 0 };
  let start = bytes.length - limit;
  while (start < bytes.length && (bytes[start] & 0xc0) === 0x80) start++;
  return { text: new TextDecoder().decode(bytes.subarray(start)), dropped: start };
}
export function displayWindow(text: string, offset: number) {
  const bytes = encoder.encode(text);
  let start = Math.max(0, Math.min(bytes.length, offset));
  while (start < bytes.length && (bytes[start] & 0xc0) === 0x80) start++;
  let end = Math.min(bytes.length, start + 65536);
  while (end > start && end < bytes.length && (bytes[end] & 0xc0) === 0x80) end--;
  return new TextDecoder().decode(bytes.subarray(start, end));
}
export function createLogStore(bridge: LogBridge) {
  let sessions: LogSession[] = [];
  const listeners = new Set<() => void>();
  const decoders = new Map<string, TextDecoder>();
  const sequences = new Map<string, number>();
  const operations = new Map<string, symbol>();
  let off: (() => void) | null = null;
  const notify = () => listeners.forEach((fn) => fn());
  const update = (id: string, patch: Partial<LogSession>) => {
    sessions = sessions.map((s) => (s.id === id ? { ...s, ...patch } : s));
    notify();
  };
  const find = (id: string) => sessions.find((s) => s.id === id);
  const message = (e: unknown) => (e instanceof Error ? e.message : "Log request failed");
  function receive(event: SessionEvent) {
    const s = sessions.find(
      (s) =>
        s.ref?.id === event.session.id &&
        s.ref.connection.id === event.session.connection.id &&
        s.ref.connection.generation === event.session.connection.generation,
    );
    if (!s) return;

    if (!s.active || event.sequence <= (sequences.get(s.id) ?? 0)) {
      if (event.kind === "data") void bridge.ack(event.session, event.sequence).catch(() => {});
      return;
    }
    sequences.set(s.id, event.sequence);
    const bytes = Uint8Array.from(atob(event.data ?? ""), (c) => c.charCodeAt(0));
    if (event.kind === "data") {
      const decoder = decoders.get(s.id)!;
      const retained = boundedText(s.text + decoder.decode(bytes, { stream: true }), 1048576);
      update(s.id, { text: retained.text, dropped: s.dropped + retained.dropped });
      void bridge.ack(event.session, event.sequence).catch(() => {});
    } else {
      const value = new TextDecoder().decode(bytes);
      if (event.kind === "truncated") {
        decoders.set(s.id, new TextDecoder());
        update(s.id, { dropped: s.dropped + (Number(value) || 0), loss: "Native delivery dropped bytes; text contains a gap." });
      } else if (event.kind === "error") update(s.id, { status: "error", error: value, active: false });
      else if (event.kind === "state") {
        if (value.startsWith("loss:")) update(s.id, { loss: value });
        else if (value.startsWith("gap:")) update(s.id, { status: "gap", loss: value, active: false });
        else if (value.startsWith("complete:")) update(s.id, { status: "complete", active: false });
        else if (value === "running") update(s.id, { status: "running" });
        else if (value === "ended") update(s.id, { status: "ended", active: false });
      }
    }
  }
  async function start(id: string, req: LogRequest, operation: symbol) {
    const ref = await bridge.open(req);
    const s = find(id);
    if (!s || !s.active || operations.get(id) !== operation) {
      await bridge.close(ref);
      return;
    }
    update(id, { ref, status: "running" });
    await bridge.activate(ref);
  }
  async function open(request: LogRequest, tab: string, label: string) {
    if (sessions.length >= 8) throw new Error("Close a session first; maximum eight retained log sessions.");
    const id = crypto.randomUUID();
    sessions = [
      ...sessions,
      { id, tab, label, request, ref: null, text: "", status: "opening", error: "", dropped: 0, loss: "", segments: 1, active: true },
    ];
    decoders.set(id, new TextDecoder());
    notify();
    const operation = Symbol();
    operations.set(id, operation);
    try {
      await start(id, request, operation);
    } catch (e) {
      if (operations.get(id) === operation) update(id, { status: "error", error: message(e), active: false });
    }
    return id;
  }
  async function stop(id: string) {
    const s = find(id);
    if (!s) return;
    operations.delete(id);
    update(id, { active: false, status: "stopped" });
    if (s.ref) await bridge.stop(s.ref);
  }
  async function close(id: string) {
    const s = find(id);
    if (!s) return;
    operations.delete(id);
    update(id, { active: false, status: "closing" });
    if (s.ref) await bridge.close(s.ref);
    sessions = sessions.filter((s) => s.id !== id);
    decoders.delete(id);
    sequences.delete(id);
    notify();
  }
  return {
    snapshot: () => sessions,
    subscribe: (fn: () => void) => {
      listeners.add(fn);
      return () => {
        listeners.delete(fn);
      };
    },
    attach: () => {
      if (!off) off = bridge.subscribe(receive);
    },
    open,
    stop,
    close,
    clear: (id: string) => {
      decoders.set(id, new TextDecoder());
      update(id, { text: "" });
    },
    resume: async (id: string, confirmedConnection?: ConnectionRef) => {
      const s = find(id);
      if (!s || s.active || s.status === "closing") return;
      const operation = Symbol();
      operations.set(id, operation);
      update(id, { active: true, status: "opening" });
      try {
        if (s.ref) await bridge.close(s.ref);
      } catch (e) {
        if (operations.get(id) === operation) update(id, { active: false, status: "error", error: message(e) });
        return;
      }
      if (operations.get(id) !== operation || !find(id)?.active) return;
      const request = {
        ...s.request,
        resource: { ...s.request.resource, connection: confirmedConnection ?? s.request.resource.connection },
      };
      decoders.set(id, new TextDecoder());
      sequences.delete(id);
      update(id, {
        request,
        ref: null,
        active: true,
        status: "opening",
        error: "",
        segments: s.segments + 1,
        loss: "Resumed segment; overlap or gaps are possible.",
      });
      try {
        await start(id, request, operation);
      } catch (e) {
        if (operations.get(id) === operation) update(id, { status: "error", error: message(e), active: false });
      }
    },
    disconnected: (tab: string) => {
      for (const s of sessions)
        if (s.tab === tab && s.active) {
          operations.delete(s.id);
          update(s.id, { active: false, status: "disconnected", loss: "Connection ended; explicit Resume required." });
          if (s.ref) void bridge.stop(s.ref).catch(() => {});
        }
    },
    closeTab: async (tab: string) => {
      for (const s of [...sessions]) if (s.tab === tab) await close(s.id);
    },
    dispose: async () => {
      off?.();
      off = null;
      for (const s of [...sessions]) await close(s.id);
    },
  };
}
export type LogStore = ReturnType<typeof createLogStore>;
