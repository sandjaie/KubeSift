/*
Purpose: Own live workload navigation once at application scope.
Used by: App, with its controller shared by the active workspace.
Responsibilities: Cancel hidden reads, isolate epochs and fetch only the visible investigation section.
Important: Payloads remain bounded, in memory, and independent of connection-health polling.
*/
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ConnectionRef } from "../bridge";
import { message } from "../connections/api";
import { readRequest, workloadAPI as api } from "./api";
import { canAutoRefresh, emptyKind, initialWorkloads, reduceWorkloads, type Action } from "./state";
import { isWorkloadKind } from "./model";
import type { Evidence, Kind, KindState, Section, WorkloadRow, NavigationEntry, ResourceRef } from "./model";
export function useWorkloads(tabID: string, connection: ConnectionRef | null, namespace: string, visible: boolean) {
  const [state, setState] = useState(initialWorkloads);
  const current = useRef(state);
  const serial = useRef(0);
  const pendingNavigation = useRef<(() => void) | null>(null);
  const [navigationError, setNavigationError] = useState("");
  const cancelNavigation = () => {
    pendingNavigation.current?.();
    pendingNavigation.current = null;
  };
  const commit = useCallback((a: Action) => {
    current.current = reduceWorkloads(current.current, a);
    setState(current.current);
  }, []);
  const scope = connection ? JSON.stringify([connection.id, connection.generation, namespace]) : "";
  const t = state[tabID]?.scope === scope ? state[tabID] : null;
  const kind = state[tabID]?.kind ?? "Pod";
  const inventory = t?.kinds[kind] ?? emptyKind();
  const cursor = inventory.history[inventory.history.length - 1] ?? "";
  const selected = inventory.selected;
  const section = inventory.section;
  const patch = (value: Partial<KindState>) => commit({ type: "patch", tab: tabID, scope, kind, patch: value });
  useLayoutEffect(() => {
    setNavigationError("");
    return cancelNavigation;
  }, [tabID, scope, visible]);
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
        const selected = current.current[tabID]?.kinds[kind].related
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
      if (section === "ReplicaSets") return api.ReadRelatedReplicaSets(id, { resource, cursor: inventory.sectionCursor });
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
  const selectResource = (row: WorkloadRow) => {
    cancelNavigation();
    setNavigationError("");
    commit({ type: "select", tab: tabID, row, epoch: ++serial.current });
  };
  function navigate(entry: NavigationEntry, history: NavigationEntry[]) {
    const targetKind = entry.resource.kind;
    if (
      !visible ||
      !connection ||
      !isWorkloadKind(targetKind) ||
      entry.resource.namespace !== namespace ||
      entry.resource.connection.id !== connection.id ||
      entry.resource.connection.generation !== connection.generation
    )
      return;
    cancelNavigation();
    setNavigationError("");
    const request = readRequest((id) => api.ReadResource(id, { resource: entry.resource }));
    pendingNavigation.current = request.cancel;
    void request.promise
      .then(() => {
        if (pendingNavigation.current !== request.cancel || current.current[tabID]?.scope !== scope) return;
        pendingNavigation.current = null;
        commit({ type: "navigate", tab: tabID, scope, kind: targetKind, entry, history, epoch: ++serial.current });
      })
      .catch((error) => {
        if (pendingNavigation.current !== request.cancel) return;
        pendingNavigation.current = null;
        setNavigationError(`${message(error)}. Select the resource again from its inventory.`);
      });
  }
  return {
    kind,
    inventory,
    selection: selected,
    activeSection: section,
    sections: inventory.sections,
    back: t?.navigation[t.navigation.length - 1],
    navigationError,
    patch,
    selectResource,
    selectKind: (kind: Kind) => {
      cancelNavigation();
      setNavigationError("");
      commit({ type: "kind", tab: tabID, kind });
    },
    selectSection: (section: Section) => {
      cancelNavigation();
      setNavigationError("");
      commit({ type: "section", tab: tabID, section, epoch: ++serial.current });
    },
    refresh: () => patch({ history: [""], refresh: inventory.refresh + 1 }),
    refreshSection: () => patch({ sectionCursor: "", sectionRefresh: inventory.sectionRefresh + 1 }),
    next: () => commit({ type: "next", tab: tabID, cursor: inventory.page?.page.nextCursor ?? "" }),
    previous: () => patch({ history: inventory.history.length > 1 ? inventory.history.slice(0, -1) : inventory.history }),
    nextSection: (cursor: string) => patch({ sectionCursor: cursor }),
    openRelated: (resource: ResourceRef) => {
      if (!t || !selected || resource.uid === selected.resource.uid) return;
      const history = t.navigation.filter(({ resource: prior }) => prior.uid !== resource.uid && prior.uid !== selected.resource.uid);
      history.push({ resource: selected.resource, section, cursor: inventory.sectionCursor });
      navigate({ resource, section: "Overview", cursor: "" }, history.slice(-8));
    },
    backToResource: () => {
      const entry = t?.navigation[t.navigation.length - 1];
      if (entry && t) navigate(entry, t.navigation.slice(0, -1));
    },
    close: (tab: string) => commit({ type: "close", tab }),
  };
}
export type WorkloadController = ReturnType<typeof useWorkloads>;
