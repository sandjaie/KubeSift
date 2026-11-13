/*
Purpose: Isolate workload pages and investigations across tabs and generations.
Used by: The workload controller and focused reducer tests.
Responsibilities: Reject stale responses and cap page history without a growing payload cache.
Important: Connection health snapshots never enter this reducer.
*/
import type { Kind, KindState, WorkloadsState, WorkloadRow, Section, NavigationEntry } from "./model";
export const initialWorkloads = (): WorkloadsState => ({});
export const emptyKind = (): KindState => ({
  filter: "",
  sort: "asc",
  scroll: 0,
  history: [""],
  page: null,
  selected: null,
  related: false,
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
  | { type: "select"; tab: string; row: WorkloadRow; epoch: number }
  | { type: "section"; tab: string; section: Section; epoch: number }
  | { type: "scope"; tab: string; scope: string }
  | { type: "close"; tab: string }
  | { type: "kind"; tab: string; kind: Kind }
  | { type: "navigate"; tab: string; scope: string; kind: Kind; entry: NavigationEntry; history: NavigationEntry[]; epoch: number }
  | { type: "next"; tab: string; cursor: string }
  | { type: "patch"; tab: string; scope: string; kind: Kind; patch: Partial<KindState> }
  | { type: "response"; tab: string; scope: string; kind: Kind; epoch: number; section?: boolean; patch: Partial<KindState> };
export function reduceWorkloads(state: WorkloadsState, a: Action): WorkloadsState {
  if (a.type === "close") {
    const next = { ...state };
    delete next[a.tab];
    return next;
  }
  const t = state[a.tab];
  if (a.type === "scope")
    return t?.scope === a.scope
      ? state
      : {
          ...state,
          [a.tab]: {
            scope: a.scope,
            kind: t?.kind ?? "Pod",
            kinds: {
              Pod: { ...emptyKind(), auto: t?.kinds.Pod.auto ?? false },
              Deployment: { ...emptyKind(), auto: t?.kinds.Deployment.auto ?? false },
              ReplicaSet: { ...emptyKind(), auto: t?.kinds.ReplicaSet.auto ?? false },
            },
            navigation: [],
          },
        };
  if (!t) return state;
  if (a.type === "kind") {
    const target = t.kinds[a.kind];
    return {
      ...state,
      [a.tab]: {
        ...t,
        kind: a.kind,
        navigation: [],
        kinds: { ...t.kinds, [a.kind]: target.related ? { ...target, selected: null, related: false, sections: {} } : target },
      },
    };
  }
  if (a.type === "navigate") {
    if (t.scope !== a.scope) return state;
    return {
      ...state,
      [a.tab]: {
        ...t,
        kind: a.kind,
        navigation: a.history,
        kinds: {
          ...t.kinds,
          [a.kind]: {
            ...t.kinds[a.kind],
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
    patch = { related: false, selected: a.row, section: "Overview", sections: {}, sectionCursor: "", sectionEpoch: a.epoch };
  } else if (a.type === "section") {
    if (k.section === a.section) return state;
    patch = { section: a.section, sectionCursor: "", sectionEpoch: a.epoch };
  } else if (a.type === "next") {
    if (!a.cursor || k.history.length >= 20) return state;
    patch = { history: [...k.history, a.cursor] };
  } else {
    if (a.scope !== t.scope || (a.type === "response" && a.epoch !== (a.section ? k.sectionEpoch : k.epoch))) return state;
    patch = a.patch;
  }
  return {
    ...state,
    [a.tab]: { ...t, ...(a.type === "select" ? { navigation: [] } : {}), kinds: { ...t.kinds, [kind]: { ...k, ...patch } } },
  };
}
