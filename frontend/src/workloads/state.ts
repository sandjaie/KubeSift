/*
Purpose: Isolate workload pages and investigations across tabs and generations.
Used by: The workload controller and focused reducer tests.
Responsibilities: Reject stale responses and cap page history without a growing payload cache.
Important: Connection health snapshots never enter this reducer.
*/
import type { Kind, KindState, WorkloadsState, WorkloadRow, Section } from "./model";
export const initialWorkloads = (): WorkloadsState => ({});
export const emptyKind = (): KindState => ({
  filter: "",
  sort: "asc",
  scroll: 0,
  history: [""],
  page: null,
  selected: null,
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
  | { type: "back"; tab: string; row: WorkloadRow | null }
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
            },
            back: null,
          },
        };
  if (!t) return state;
  if (a.type === "kind") return { ...state, [a.tab]: { ...t, kind: a.kind } };
  if (a.type === "back") return { ...state, [a.tab]: { ...t, back: a.row } };
  const kind = a.type === "next" || a.type === "select" || a.type === "section" ? t.kind : a.kind;
  const k = t.kinds[kind];
  let patch: Partial<KindState>;
  if (a.type === "select") {
    if (k.selected?.resource.uid === a.row.resource.uid) return state;
    patch = { selected: a.row, section: "Overview", sections: {}, sectionCursor: "", sectionEpoch: a.epoch };
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
  return { ...state, [a.tab]: { ...t, kinds: { ...t.kinds, [kind]: { ...k, ...patch } } } };
}
