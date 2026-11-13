/*
Purpose: Isolate workload pages and investigations across tabs and generations.
Used by: The workload controller and focused reducer tests.
Responsibilities: Reject stale responses and cap page history without a growing payload cache.
Important: Connection health snapshots never enter this reducer.
*/
import { isClusterScoped, resourceKinds, resourceSections } from "./model";
import type { ResourceKind, KindState, WorkloadsState, ResourceRef, Section, NavigationEntry, TabState } from "./model";
export const initialWorkloads = (): WorkloadsState => ({});
export const emptyKind = (): KindState => ({
  filter: "",
  sort: "asc",
  scroll: 0,
  history: [""],
  page: null,
  selected: null,
  related: false,
  identity: "",
  section: "Overview",
  sections: {},
  loading: false,
  error: "",
  epoch: 0,
  sectionEpoch: 0,
  refresh: 0,
  sectionRefresh: 0,
  sectionCursor: "",
  auto: false,
});
export function canAutoRefresh(s: KindState, visible: boolean) {
  return visible && s.auto && s.history.length === 1 && !s.loading;
}
export type Action =
  | { type: "select"; tab: string; row: { resource: ResourceRef }; epoch: number }
  | { type: "section"; tab: string; section: Section; epoch: number }
  | { type: "scope"; tab: string; scope: string; clusterScope?: string }
  | { type: "close"; tab: string }
  | { type: "kind"; tab: string; kind: ResourceKind; epoch?: number }
  | { type: "navigate"; tab: string; scope: string; kind: ResourceKind; entry: NavigationEntry; history: NavigationEntry[]; epoch: number }
  | { type: "next"; tab: string; cursor: string }
  | { type: "patch"; tab: string; scope: string; kind: ResourceKind; patch: Partial<KindState> }
  | { type: "response"; tab: string; scope: string; kind: ResourceKind; epoch: number; section?: boolean; patch: Partial<KindState> };
export const kindScope = (tab: TabState, kind: ResourceKind) => (isClusterScoped(kind) ? tab.clusterScope : tab.scope);

export function relatedHistory(entries: NavigationEntry[], origin: NavigationEntry, target: ResourceRef) {
  return [...entries.filter(({ resource }) => resource.uid !== origin.resource.uid && resource.uid !== target.uid), origin].slice(-8);
}
function clearOutgoingData(t: TabState, destination: ResourceKind, epoch?: number) {
  if (t.kind !== "ConfigMap" || destination === t.kind) return {};
  const source = t.kinds.ConfigMap;
  return { ConfigMap: { ...source, sections: { ...source.sections, Data: undefined }, sectionEpoch: epoch ?? source.sectionEpoch + 1 } };
}

export function reduceWorkloads(state: WorkloadsState, a: Action): WorkloadsState {
  if (a.type === "close") {
    const next = { ...state };
    delete next[a.tab];
    return next;
  }
  const t = state[a.tab];
  if (a.type === "scope") {
    const clusterScope = a.clusterScope ?? a.scope;
    if (t?.scope === a.scope && t.clusterScope === clusterScope) return state;
    return {
      ...state,
      [a.tab]: {
        scope: a.scope,
        clusterScope,
        kind: t?.kind ?? "Pod",
        kinds: Object.fromEntries(
          resourceKinds.map((kind) => [
            kind,
            isClusterScoped(kind) && t?.clusterScope === clusterScope
              ? t.kinds[kind]
              : { ...emptyKind(), auto: t?.kinds[kind].auto ?? false },
          ]),
        ) as Record<ResourceKind, KindState>,
        navigation: [],
      },
    };
  }
  if (!t) return state;
  if (a.type === "kind") {
    if (t.kind === a.kind) return state;
    const target = t.kinds[a.kind];
    return {
      ...state,
      [a.tab]: {
        ...t,
        kind: a.kind,
        navigation: [],
        kinds: {
          ...t.kinds,
          ...clearOutgoingData(t, a.kind, a.epoch),
          [a.kind]: target.related ? { ...target, selected: null, related: false, identity: "", sections: {} } : target,
        },
      },
    };
  }
  if (a.type === "navigate") {
    if (kindScope(t, t.kind) !== a.scope) return state;
    return {
      ...state,
      [a.tab]: {
        ...t,
        kind: a.kind,
        navigation: a.history,
        kinds: {
          ...t.kinds,
          ...clearOutgoingData(t, a.kind, a.epoch),
          [a.kind]: {
            ...t.kinds[a.kind],
            ...a.entry.position,
            identity: a.entry.identity ?? "",
            selected: { resource: a.entry.resource },
            related: true,
            section: a.entry.section,
            sectionCursor: a.entry.cursor,
            sectionEpoch: a.epoch,
            sections: {},
          },
        },
      },
    };
  }
  const kind = a.type === "next" || a.type === "select" || a.type === "section" ? t.kind : a.kind;
  const k = t.kinds[kind];
  let patch: Partial<KindState>;
  if (a.type === "select") {
    if (k.selected?.resource.uid === a.row.resource.uid && !k.related && !t.navigation.length) return state;
    patch = { related: false, identity: "", selected: a.row };
    if (k.selected?.resource.uid !== a.row.resource.uid || k.section !== "Overview") {
      Object.assign(patch, { section: "Overview", sections: {}, sectionCursor: "", sectionEpoch: a.epoch });
    }
  } else if (a.type === "section") {
    if (k.section === a.section || !resourceSections(kind).includes(a.section)) return state;
    patch = {
      section: a.section,
      sectionCursor: "",
      sectionEpoch: a.epoch,
      ...(k.section === "Data" ? { sections: { ...k.sections, Data: undefined } } : {}),
    };
  } else if (a.type === "next") {
    if (!a.cursor || k.history.length >= 20) return state;
    patch = { history: [...k.history, a.cursor] };
  } else {
    if (a.scope !== kindScope(t, kind) || (a.type === "response" && a.epoch !== (a.section ? k.sectionEpoch : k.epoch))) return state;
    patch = a.patch;
  }
  return {
    ...state,
    [a.tab]: { ...t, ...(a.type === "select" ? { navigation: [] } : {}), kinds: { ...t.kinds, [kind]: { ...k, ...patch } } },
  };
}
