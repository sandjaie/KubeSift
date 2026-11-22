/*
Purpose: Coordinate product RPCs and poll safe native status snapshots.
Used by: App and its Settings/workspace children.
Responsibilities: Bind every response to its source, attempt and namespace epoch; refresh on foreground return.
Important: Project-bound tabs revalidate their captured UID. Closing during Start cancels the returned attempt.
*/
import { useCallback, useEffect, useRef, useState } from "react";
import { createAttemptQueue } from "./attempts";
import { api, message, type CatalogEntry } from "./api";
import { initialConnections, reduceConnections, tabKey, type ConnectionsAction } from "./state";
export function useConnections() {
  const [state, setState] = useState(initialConnections);
  const current = useRef(state);
  const selection = useRef(0);
  const serial = useRef(0);
  const transitions = useRef(createAttemptQueue());
  const attempts = useRef(createAttemptQueue());
  const sourceEpoch = useRef(0);
  const mounted = useRef(true);
  const commit = useCallback((a: ConnectionsAction) => {
    const previous = current.current;
    current.current = reduceConnections(previous, a);
    if (
      ["focus", "open", "close"].includes(a.type) ||
      (a.type === "projectLost" && a.key === previous.active && previous !== current.current)
    )
      selection.current++;
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
          if (t.projectUID && !t.projectUnavailable && view.ref && key === current.current.active) {
            try {
              const p = await api.ReadProject(crypto.randomUUID(), view.ref, t.namespace, t.projectUID);
              if (p.phase !== "Active" || p.deletingAt) throw new Error("Project is no longer active.");
            } catch (e) {
              commit({ type: "projectLost", key, epoch: t.epoch, uid: t.projectUID, error: message(e) });
            }
          }
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
    return attempts.current(key, async () => {
      if (!mounted.current || current.current.tabs[key]?.request !== request) return;
      try {
        let selected = entry;
        if (retry) {
          const old = await api.ReloadSource(entry.sourceId);
          const fresh = old.contexts?.find((c) => c.context === entry.context);
          if (!fresh) throw new Error(old.problem?.message ?? "Context is no longer in this source. Check Settings.");
          selected = fresh;
        }
        if (current.current.tabs[key]?.request !== request) return;
        commit({ type: "entry", key, request, entry: selected });
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
    });
  }
  async function cancel(key: string) {
    const t = current.current.tabs[key];
    commit({ type: "close", key });
    if (t?.attemptId) {
      try {
        await attempts.current(key, async () => {
          await api.CancelAttempt(t.attemptId);
        });
      } catch (e) {
        commit({ type: "error", error: message(e) });
      }
    }
  }
  async function selectNamespace(
    key: string,
    namespace: string,
    project?: { uid: string; valid: () => boolean; onLost?: (message: string) => void },
  ) {
    const t = current.current.tabs[key];
    if (!t?.view?.ref) return false;
    const n = ++selection.current;
    if (project) {
      const ref = t.view.ref;
      const valid = () =>
        project.valid() && current.current.active === key && current.current.tabs[key]?.request === t.request && selection.current === n;
      let verifiedActive = false;
      try {
        const verified = await api.ReadProject(crypto.randomUUID(), ref, namespace, project.uid);
        if (!valid()) return false;
        if (verified.phase !== "Active" || verified.deletingAt) throw new Error("Choose an active Project.");
        verifiedActive = true;
        let navigate = false;
        await transitions.current(key, async () => {
          if (!valid()) return;
          const before = current.current.tabs[key].epoch;
          const view = await api.SetWorkloadNamespace(ref, namespace);
          const origin = current.current.tabs[key];
          if (
            origin?.request !== t.request ||
            (origin.epoch !== before &&
              (!origin.projectUnavailable || (origin.namespace === namespace && origin.projectUID === verified.resource.uid))) ||
            view.ref?.id !== ref.id ||
            view.ref.generation !== ref.generation ||
            view.namespace !== namespace
          )
            return;
          // Native scope already changed. Publish to the origin even after focus moved.
          navigate = valid();
          commit({ type: "namespace", key, namespace, projectUID: verified.resource.uid });
          commit({ type: "snapshot", key, epoch: current.current.tabs[key].epoch, view });
        });
        return navigate;
      } catch (e) {
        if (valid()) {
          if (!verifiedActive) {
            project.onLost?.(message(e));
            if (project.uid && t.projectUID === project.uid && t.namespace === namespace)
              commit({ type: "projectLost", key, uid: project.uid, epoch: t.epoch, error: message(e) });
          }
          commit({ type: "failure", key, request: t.request, error: message(e) });
        }
        return false;
      }
    }
    if (t.namespace === namespace && !t.projectUID) return false;
    commit({ type: "namespace", key, namespace });
    const epoch = current.current.tabs[key].epoch;
    try {
      const ref = t.view.ref;
      await transitions.current(key, async () => {
        if (current.current.tabs[key]?.request !== t.request || current.current.tabs[key].epoch !== epoch) return;
        const view = await api.SetWorkloadNamespace(ref, namespace);
        commit({ type: "snapshot", key, epoch, view });
      });
      await poll();
    } catch (e) {
      if (current.current.tabs[key]?.epoch === epoch) commit({ type: "failure", key, request: t.request, error: message(e) });
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
    selectNamespace,
    poll,
  };
}
export type ConnectionsController = ReturnType<typeof useConnections>;
