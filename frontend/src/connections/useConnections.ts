/*
Purpose: Coordinate product RPCs and poll safe native status snapshots.
Used by: App and its Settings/workspace children.
Responsibilities: Bind every response to its source, attempt and namespace epoch; refresh on foreground return.
Important: Polls do not trigger Kubernetes reads, and closing during Start cancels the returned attempt.
*/
import { useCallback, useEffect, useRef, useState } from "react";
import { api, message, type CatalogEntry } from "./api";
import { initialConnections, reduceConnections, tabKey, type ConnectionsAction } from "./state";
export function useConnections() {
  const [state, setState] = useState(initialConnections);
  const current = useRef(state);
  const serial = useRef(0);
  const sourceEpoch = useRef(0);
  const mounted = useRef(true);
  const commit = useCallback((a: ConnectionsAction) => {
    current.current = reduceConnections(current.current, a);
    if (mounted.current) setState(current.current);
  }, []);
  const refreshSources = useCallback(async () => {
    const epoch = ++sourceEpoch.current;
    try {
      const sources = await api.ListSources();
      if (epoch === sourceEpoch.current) commit({ type: "sources", sources: sources ?? [] });
    } catch (e) {
      if (epoch === sourceEpoch.current) commit({ type: "error", error: message(e) });
    }
  }, [commit]);
  const poll = useCallback(async () => {
    const tabs = Object.entries(current.current.tabs);
    await Promise.all(
      tabs.map(async ([key, t]) => {
        if (!t.attemptId) return;
        try {
          const view = await api.GetAttempt(t.attemptId);
          commit({ type: "snapshot", key, epoch: t.epoch, view });
        } catch {
          commit({ type: "pollFailed", key, attemptId: t.attemptId });
        }
      }),
    );
  }, [commit]);
  useEffect(() => {
    mounted.current = true;
    void refreshSources();
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    let busy = false;
    const tick = async () => {
      if (busy || stopped) return;
      busy = true;
      await poll();
      busy = false;
      if (!stopped)
        timer = setTimeout(tick, Object.values(current.current.tabs).some((t) => !t.view || t.view.state === "connecting") ? 500 : 5000);
    };
    const foreground = () => {
      if (document.visibilityState === "visible") {
        clearTimeout(timer);
        void tick();
      }
    };
    void tick();
    document.addEventListener("visibilitychange", foreground);
    return () => {
      stopped = true;
      mounted.current = false;
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", foreground);
      for (const t of Object.values(current.current.tabs)) if (t.attemptId) void api.CancelAttempt(t.attemptId);
    };
  }, [refreshSources, poll]);
  async function start(entry: CatalogEntry, retry = false) {
    const key = tabKey(entry);
    if (current.current.tabs[key] && !retry) {
      commit({ type: "focus", key });
      return;
    }
    const request = ++serial.current;
    const ns = current.current.tabs[key]?.namespace ?? entry.namespace;
    commit({ type: "open", entry, request });
    try {
      let selected = entry;
      if (retry) {
        const old = await api.ReloadSource(entry.sourceId);
        const fresh = old.contexts?.find((c) => c.context === entry.context);
        if (!fresh) throw new Error(old.problem?.message ?? "Context is no longer in this source. Check Settings.");
        selected = fresh;
      }
      if (current.current.tabs[key]?.request !== request) return;
      const attempt = await api.StartConnection({
        sourceId: selected.sourceId,
        snapshotId: selected.snapshotId,
        context: selected.context,
        namespace: ns,
      });
      if (!mounted.current || current.current.tabs[key]?.request !== request) {
        await api.CancelAttempt(attempt.id);
        return;
      }
      commit({ type: "attempt", key, request, id: attempt.id });
      await poll();
    } catch (e) {
      commit({ type: "failure", key, request, error: message(e) });
    } finally {
      void refreshSources();
    }
  }
  async function cancel(key: string) {
    const t = current.current.tabs[key];
    commit({ type: "close", key });
    if (t?.attemptId) {
      try {
        await api.CancelAttempt(t.attemptId);
      } catch (e) {
        commit({ type: "error", error: message(e) });
      }
    }
  }
  async function readPods(key: string, namespace: string) {
    const t = current.current.tabs[key];
    if (!t?.view?.ref) return;
    commit({ type: "namespace", key, namespace });
    const epoch = current.current.tabs[key].epoch;
    try {
      const view = await api.ReadPods(t.view.ref, namespace);
      commit({ type: "snapshot", key, epoch, view });
      await poll();
    } catch (e) {
      commit({ type: "failure", key, request: t.request, error: message(e) });
    }
  }
  return {
    state,
    commit,
    refreshSources,
    connect: (entry: CatalogEntry) => start(entry),
    retry: (key: string) => {
      const t = current.current.tabs[key];
      return t ? start(t.entry, true) : Promise.resolve();
    },
    cancel,
    disconnect: cancel,
    readPods,
    poll,
  };
}
export type ConnectionsController = ReturnType<typeof useConnections>;
