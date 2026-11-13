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
import { canAutoRefresh, emptyKind, initialWorkloads, reduceWorkloads, kindScope, relatedHistory, type Action } from "./state";
import { inventoryNamespace, isWorkloadKind, isResourceKind, isClusterScoped, resourceSections } from "./model";
import { isConfigurationKind, type ConfigurationPage, type NamespaceDestination } from "../configuration/model";
import type { NetworkPage } from "../networking/model";
import type { Evidence, ResourceKind, KindState, Section, WorkloadPage, NavigationEntry, ResourceRef, ResourceDetail } from "./model";
export function useWorkloads(
  tabID: string,
  connection: ConnectionRef | null,
  namespace: string,
  visible: boolean,
  onNamespace: (tab: string, namespace: string) => void,
) {
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
  const namespaceScope = connection ? JSON.stringify([connection.id, connection.generation, namespace]) : "";
  const clusterScope = connection ? JSON.stringify([connection.id, connection.generation]) : "";
  const kind = state[tabID]?.kind ?? "Pod";
  const targetNamespace = inventoryNamespace(kind, namespace);
  const scope = isClusterScoped(kind) ? clusterScope : namespaceScope;
  const tab = state[tabID];
  const t = tab && kindScope(tab, kind) === scope ? tab : null;
  const inventory = t?.kinds[kind] ?? emptyKind();
  const cursor = inventory.history[inventory.history.length - 1] ?? "";
  const selected = inventory.selected;
  const section = inventory.section;
  const patch = (value: Partial<KindState>) => commit({ type: "patch", tab: tabID, scope, kind, patch: value });
  useLayoutEffect(() => {
    setNavigationError("");
    return cancelNavigation;
  }, [tabID, namespaceScope, visible]);
  useLayoutEffect(() => {
    if (!visible || !tabID) return;
    return () => {
      const currentTab = current.current[tabID];
      if (currentTab)
        commit({
          type: "patch",
          tab: tabID,
          scope: kindScope(currentTab, "ConfigMap"),
          kind: "ConfigMap",
          patch: { sections: { ...currentTab.kinds.ConfigMap.sections, Data: undefined }, sectionEpoch: ++serial.current },
        });
    };
  }, [tabID, visible, commit]);
  useEffect(() => {
    if (tabID) commit({ type: "scope", tab: tabID, scope: namespaceScope, clusterScope });
  }, [tabID, namespaceScope, clusterScope, commit]);
  useEffect(() => {
    if (!visible || !connection || !t) return;
    const epoch = ++serial.current;
    commit({ type: "patch", tab: tabID, scope, kind, patch: { epoch, loading: true, error: "" } });
    const request = readRequest<WorkloadPage | NetworkPage | ConfigurationPage>((id) => {
      const req = { connection, namespace: targetNamespace, kind, cursor };
      if (isWorkloadKind(kind)) return api.ReadWorkloads(id, req);
      return isConfigurationKind(kind) ? api.ReadConfiguration(id, req) : api.ReadNetworking(id, req);
    });
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
          patch: {
            page,
            selected,
            loading: false,
            error: "",
            ...(selected?.resource.uid !== old?.resource.uid ? { sections: {}, sectionEpoch: ++serial.current } : {}),
          },
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
    if (!visible || !selected || section === "Logs" || !resourceSections(kind).includes(section)) return;
    const epoch = ++serial.current;
    const existing = current.current[tabID]?.kinds[kind].sections ?? {};
    commit({
      type: "patch",
      tab: tabID,
      scope,
      kind,
      patch: {
        sectionEpoch: epoch,
        sections: {
          ...existing,
          [section]: { loading: true, error: "", data: section === "Data" ? null : (existing[section]?.data ?? null) },
        },
      },
    });
    const resource = selected.resource;
    const request = readRequest<Evidence>((id) => {
      if (section === "YAML") return api.ReadResourceYAML(id, { resource });
      if (section === "EndpointSlices") return api.ReadServiceEndpointSlices(id, { resource, cursor: inventory.sectionCursor });
      if (section === "Events") return api.ReadResourceEvents(id, { resource, cursor: inventory.sectionCursor });
      if (section === "ReplicaSets") return api.ReadRelatedReplicaSets(id, { resource, cursor: inventory.sectionCursor });
      if (section === "Jobs") return api.ReadRelatedJobs(id, { resource, cursor: inventory.sectionCursor });
      if (section === "Pods") return api.ReadRelatedPods(id, { resource, cursor: inventory.sectionCursor });
      return api.ReadResource(id, { resource, includeConfigMapData: section === "Data" });
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
    void request.promise
      .then((data) => finish(data, ""))
      .catch((e) => finish(section === "Data" ? null : (existing[section]?.data ?? null), message(e)));
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
  const selectResource = (row: { resource: ResourceRef }) => {
    cancelNavigation();
    setNavigationError("");
    commit({ type: "select", tab: tabID, row, epoch: ++serial.current });
  };
  const canOpen = (resource: ResourceRef): resource is ResourceRef & { kind: ResourceKind } =>
    !!visible &&
    !!connection &&
    isResourceKind(resource.kind) &&
    resource.namespace === inventoryNamespace(resource.kind, namespace) &&
    resource.connection.id === connection.id &&
    resource.connection.generation === connection.generation;
  function navigationRequest<T>(read: (id: string) => Promise<T>, open: (data: T) => void) {
    cancelNavigation();
    setNavigationError("");
    const request = readRequest(read);
    pendingNavigation.current = request.cancel;
    const stillCurrent = () => {
      const active = current.current[tabID];
      const source = active?.kinds[kind];
      return (
        !!active &&
        kindScope(active, kind) === scope &&
        active.kind === kind &&
        source?.selected?.resource.uid === selected?.resource.uid &&
        source?.section === section &&
        source?.sectionCursor === inventory.sectionCursor
      );
    };
    void request.promise
      .then((data) => {
        if (pendingNavigation.current !== request.cancel) return;
        pendingNavigation.current = null;
        if (stillCurrent()) open(data);
      })
      .catch((error) => {
        if (pendingNavigation.current !== request.cancel) return;
        pendingNavigation.current = null;
        if (stillCurrent()) setNavigationError(message(error));
      });
  }
  function show(entry: NavigationEntry, history: NavigationEntry[]) {
    if (!canOpen(entry.resource)) return;
    commit({ type: "navigate", tab: tabID, scope, kind: entry.resource.kind, entry, history, epoch: ++serial.current });
  }
  function navigate(entry: NavigationEntry, history: NavigationEntry[]) {
    if (canOpen(entry.resource))
      navigationRequest(
        (id) => api.ReadResource(id, { resource: entry.resource }),
        () => show(entry, history),
      );
  }
  function openFromSelection(resource: ResourceRef, identity?: string, resolved = false) {
    if (!t || !selected || !canOpen(resource) || resource.uid === selected.resource.uid) return;
    const { filter, sort, scroll, history } = inventory;
    const entries = relatedHistory(
      t.navigation,
      {
        resource: selected.resource,
        section,
        cursor: inventory.sectionCursor,
        identity: inventory.identity,
        position: { filter, sort, scroll, history },
      },
      resource,
    );
    const entry: NavigationEntry = { resource, section: "Overview", cursor: "", identity };
    if (resolved) show(entry, entries);
    else navigate(entry, entries);
  }
  return {
    kind,
    inventory,
    selection: selected,
    activeSection: section,
    sections: inventory.sections,
    back: t?.navigation[t.navigation.length - 1],
    clusterScoped: isClusterScoped(kind),
    navigationError,
    patch,
    selectResource,
    selectKind: (kind: ResourceKind) => {
      cancelNavigation();
      setNavigationError("");
      commit({ type: "kind", tab: tabID, kind, epoch: ++serial.current });
    },
    selectSection: (section: Section) => {
      cancelNavigation();
      setNavigationError("");
      commit({ type: "section", tab: tabID, section, epoch: ++serial.current });
    },
    refresh: () => {
      cancelNavigation();
      setNavigationError("");
      patch({
        history: [""],
        refresh: inventory.refresh + 1,
        ...(section === "Data"
          ? {
              sections: { ...inventory.sections, Data: undefined },
              sectionEpoch: ++serial.current,
              sectionRefresh: inventory.sectionRefresh + 1,
            }
          : {}),
      });
    },
    refreshSection: () => {
      cancelNavigation();
      setNavigationError("");
      patch({
        sectionCursor: "",
        sectionRefresh: inventory.sectionRefresh + 1,
        ...(section === "Data" ? { sections: { ...inventory.sections, Data: undefined }, sectionEpoch: ++serial.current } : {}),
      });
    },
    next: () => commit({ type: "next", tab: tabID, cursor: inventory.page?.page.nextCursor ?? "" }),
    previous: () => patch({ history: inventory.history.length > 1 ? inventory.history.slice(0, -1) : inventory.history }),
    nextSection: (cursor: string) => patch({ sectionCursor: cursor }),
    openRelated: (resource: ResourceRef) => openFromSelection(resource),
    openReference: (source: ResourceDetail, target: ResourceRef) => {
      if (!selected || section !== "Overview" || selected.resource.uid !== source.resource.uid || !canOpen(target)) return;
      navigationRequest(
        (id) =>
          api.ResolveNetworkReference(id, {
            source: source.resource,
            sourceVersion: source.resourceVersion,
            target,
          }),
        (result) =>
          openFromSelection(
            result.resource,
            result.identityEvidence === "uid-verified" ? "Referenced UID verified" : "Resolved by name at open time",
            true,
          ),
      );
    },
    openConfigurationReference: (source: ResourceDetail, target: ResourceRef) => {
      if (
        !selected ||
        section !== "Overview" ||
        selected.resource.uid !== source.resource.uid ||
        source.resource.kind !== "Pod" ||
        !canOpen(target)
      )
        return;
      navigationRequest(
        (id) => api.ResolveConfigurationReference(id, { source: source.resource, sourceVersion: source.resourceVersion, target }),
        (result) => openFromSelection(result.resource, "Resolved by name at open time", true),
      );
    },
    browseNamespace: (source: ResourceDetail, destination: NamespaceDestination) => {
      if (
        !selected ||
        section !== "Overview" ||
        source.resource.kind !== "Namespace" ||
        selected.resource.uid !== source.resource.uid ||
        !canOpen(source.resource)
      )
        return;
      navigationRequest(
        (id) => api.ReadResource(id, { resource: source.resource }),
        () => {
          const nextNamespace = source.resource.name;
          const nextScope = JSON.stringify([source.resource.connection.id, source.resource.connection.generation, nextNamespace]);
          commit({ type: "scope", tab: tabID, scope: nextScope, clusterScope });
          commit({ type: "kind", tab: tabID, kind: destination });
          onNamespace(tabID, nextNamespace);
        },
      );
    },
    openServiceSlice: (sourceVersion: string, target: ResourceRef) => {
      if (!selected || section !== "EndpointSlices" || selected.resource.kind !== "Service" || !canOpen(target)) return;
      navigationRequest(
        (id) => api.ResolveNetworkReference(id, { source: selected.resource, sourceVersion, target }),
        (result) => {
          openFromSelection(
            result.resource,
            result.association === "owner-confirmed"
              ? "Label-associated; Service owner UID matches"
              : "Label-associated; ownership unverified",
            true,
          );
        },
      );
    },
    backToResource: () => {
      const entry = t?.navigation[t.navigation.length - 1];
      if (entry && t) navigate(entry, t.navigation.slice(0, -1));
    },
    close: (tab: string) => commit({ type: "close", tab }),
  };
}
export type WorkloadController = ReturnType<typeof useWorkloads>;
