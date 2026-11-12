/*
Purpose: Own live workload navigation once at application scope.
Used by: App, with its controller shared by the active workspace.
Responsibilities: Cancel hidden reads, isolate epochs and fetch only the visible investigation section.
Important: Payloads remain bounded, in memory, and independent of connection-health polling.
*/
import { useCallback, useEffect, useRef, useState } from "react";
import type { ConnectionRef } from "../bridge";
import { message } from "../connections/api";
import { readRequest, workloadAPI as api } from "./api";
import { canAutoRefresh, emptyKind, initialWorkloads, reduceWorkloads, type Action } from "./state";
import type { Evidence, Kind, KindState, Section, WorkloadRow } from "./model";
export function useWorkloads(tabID: string, connection: ConnectionRef | null, namespace: string, visible: boolean) {
  const [state, setState] = useState(initialWorkloads);
  const current = useRef(state);
  const serial = useRef(0);
  const commit = useCallback((a: Action) => {
    current.current = reduceWorkloads(current.current, a);
    setState(current.current);
  }, []);
  const scope = connection ? JSON.stringify([connection.id, connection.generation, namespace]) : "";
  const t = state[tabID]?.scope === scope ? state[tabID] : null;
  const kind = t?.kind ?? "Pod";
  const inventory = t?.kinds[kind] ?? emptyKind();
  const cursor = inventory.history[inventory.history.length - 1] ?? "";
  const selected = inventory.selected;
  const section = inventory.section;
  const patch = (value: Partial<KindState>) => commit({ type: "patch", tab: tabID, scope, kind, patch: value });
  useEffect(() => {
    if (tabID) commit({ type: "scope", tab: tabID, scope });
  }, [tabID, scope, commit]);
  useEffect(() => {
    if (!visible || !connection || !t) return;
    const epoch = ++serial.current;
    commit({ type: "patch", tab: tabID, scope, kind, patch: { epoch, loading: true, error: "" } });
    const request = readRequest((id) => api.ReadWorkloads(id, { connection, namespace, kind, cursor }));
    let active = true;
    void request.promise
      .then((page) => {
        if (!active) return;
        const old = current.current[tabID]?.kinds[kind].selected;
        const selected =
          current.current[tabID]?.back && kind === "Pod"
            ? (old ?? null)
            : (page.items?.find((row) => row.resource.uid === old?.resource.uid) ?? null);
        commit({
          type: "response",
          tab: tabID,
          scope,
          kind,
          epoch,
          patch: { page, selected, loading: false, error: "", ...(selected?.resource.uid !== old?.resource.uid ? { sections: {} } : {}) },
        });
      })
      .catch((e) => {
        if (active) commit({ type: "response", tab: tabID, scope, kind, epoch, patch: { loading: false, error: message(e) } });
      });
    return () => {
      active = false;
      request.cancel();
    };
    // Scope is the stable immutable connection/namespace identity.
  }, [tabID, scope, kind, cursor, inventory.refresh, visible, !!t, commit]);
  useEffect(() => {
    if (!visible || !selected || section === "Logs") return;
    const epoch = ++serial.current;
    const existing = current.current[tabID]?.kinds[kind].sections ?? {};
    commit({
      type: "patch",
      tab: tabID,
      scope,
      kind,
      patch: {
        sectionEpoch: epoch,
        sections: { ...existing, [section]: { loading: true, error: "", data: existing[section]?.data ?? null } },
      },
    });
    const resource = selected.resource;
    const request = readRequest<Evidence>((id) => {
      if (section === "YAML") return api.ReadResourceYAML(id, { resource });
      if (section === "Events") return api.ReadResourceEvents(id, { resource, cursor: inventory.sectionCursor });
      if (section === "Pods") return api.ReadRelatedPods(id, { resource, cursor: inventory.sectionCursor });
      return api.ReadResource(id, { resource });
    });
    let active = true;
    const finish = (data: Evidence | null, error: string) => {
      if (!active) return;
      const sections = current.current[tabID]?.kinds[kind].sections ?? {};
      commit({
        type: "response",
        tab: tabID,
        scope,
        kind,
        epoch,
        section: true,
        patch: { sections: { ...sections, [section]: { data, error, loading: false } } },
      });
    };
    void request.promise.then((data) => finish(data, "")).catch((e) => finish(existing[section]?.data ?? null, message(e)));
    return () => {
      active = false;
      request.cancel();
    };
  }, [tabID, scope, kind, selected?.resource.uid, section, inventory.sectionCursor, inventory.sectionRefresh, visible, commit]);
  useEffect(() => {
    if (!canAutoRefresh(inventory, visible)) return;
    const timer = setTimeout(() => patch({ refresh: inventory.refresh + 1 }), 15000);
    return () => clearTimeout(timer);
  }, [tabID, scope, kind, inventory.auto, inventory.loading, inventory.history.length, inventory.refresh, visible]);
  const selectResource = (row: WorkloadRow) =>
    patch({ selected: row, section: "Overview", sections: {}, sectionCursor: "", sectionEpoch: ++serial.current });
  return {
    kind,
    inventory,
    selection: selected,
    activeSection: section,
    sections: inventory.sections,
    back: t?.back,
    patch,
    selectResource,
    selectKind: (kind: Kind) => commit({ type: "kind", tab: tabID, kind }),
    selectSection: (section: Section) => patch({ section, sectionCursor: "", sectionEpoch: ++serial.current }),
    refresh: () => patch({ history: [""], refresh: inventory.refresh + 1 }),
    refreshSection: () => patch({ sectionCursor: "", sectionRefresh: inventory.sectionRefresh + 1 }),
    next: () => commit({ type: "next", tab: tabID, cursor: inventory.page?.page.nextCursor ?? "" }),
    previous: () => patch({ history: inventory.history.length > 1 ? inventory.history.slice(0, -1) : inventory.history }),
    nextSection: (cursor: string) => patch({ sectionCursor: cursor }),
    openPod: (row: WorkloadRow) => {
      commit({ type: "back", tab: tabID, row: selected });
      commit({
        type: "patch",
        tab: tabID,
        scope,
        kind: "Pod",
        patch: { selected: row, section: "Overview", sections: {}, sectionCursor: "" },
      });
      commit({ type: "kind", tab: tabID, kind: "Pod" });
    },
    backToDeployment: () => {
      if (!t?.back) return;
      commit({ type: "kind", tab: tabID, kind: "Deployment" });
      commit({
        type: "patch",
        tab: tabID,
        scope,
        kind: "Deployment",
        patch: { selected: t.back, section: "Overview", sections: {}, sectionRefresh: t.kinds.Deployment.sectionRefresh + 1 },
      });
      commit({ type: "back", tab: tabID, row: null });
    },
    close: (tab: string) => commit({ type: "close", tab }),
  };
}
export type WorkloadController = ReturnType<typeof useWorkloads>;
