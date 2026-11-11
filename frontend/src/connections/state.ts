/*
Purpose: Apply connection responses only to their originating live tab.
Used by: useConnections and reducer tests.
Responsibilities: Reject superseded attempts/read epochs and age connection evidence.
Important: Active-tab selection never routes an incoming resource response.
*/
import type { CatalogEntry, ConnectionView, SourceResult } from "./api";
import type { ConnectionsState } from "./model";
export type ConnectionsAction =
  | { type: "sources"; sources: SourceResult[] }
  | { type: "error"; error: string }
  | { type: "open"; entry: CatalogEntry; request: number }
  | { type: "focus"; key: string | null }
  | { type: "close"; key: string }
  | { type: "attempt"; key: string; request: number; id: string }
  | { type: "snapshot"; key: string; epoch: number; view: ConnectionView }
  | { type: "failure"; key: string; request: number; error: string }
  | { type: "pollFailed"; key: string; attemptId: string }
  | { type: "namespace"; key: string; namespace: string }
  | { type: "filter"; key: string; filter: string }
  | { type: "select"; key: string; uid: string | null };
export const tabKey = (entry: Pick<CatalogEntry, "sourceId" | "context">) => JSON.stringify([entry.sourceId, entry.context]);
export const initialConnections = (): ConnectionsState => ({ sources: [], tabs: {}, active: null, error: "", loading: true });
export function visibleState(view: ConnectionView | null, now = Date.now()): string {
  if (!view) return "not_connected";
  if (view.state !== "connected") return view.state;
  const stamp = Date.parse(view.lastSuccessAt);
  return !Number.isFinite(stamp) || now - stamp > 90000 ? "stale" : "connected";
}
export function reduceConnections(s: ConnectionsState, a: ConnectionsAction): ConnectionsState {
  if (a.type === "sources") return { ...s, sources: a.sources, error: "", loading: false };
  if (a.type === "error") return { ...s, error: a.error, loading: false };
  if (a.type === "focus") return { ...s, active: a.key };
  if (a.type === "open") {
    const previous = s.tabs[tabKey(a.entry)];
    return {
      ...s,
      active: tabKey(a.entry),
      tabs: {
        ...s.tabs,
        [tabKey(a.entry)]: {
          entry: a.entry,
          request: a.request,
          attemptId: "",
          epoch: 0,
          namespace: previous?.namespace ?? a.entry.namespace,
          filter: previous?.filter ?? "",
          selected: null,
          view: null,
          error: "",
          pollFailed: false,
        },
      },
    };
  }
  const t = s.tabs[a.key];
  if (!t) return s;
  if (a.type === "close") {
    const tabs = { ...s.tabs };
    delete tabs[a.key];
    return { ...s, tabs, active: s.active === a.key ? (Object.keys(tabs)[0] ?? null) : s.active };
  }
  let next = t;
  switch (a.type) {
    case "attempt":
      if (t.request !== a.request) return s;
      next = { ...t, attemptId: a.id };
      break;
    case "snapshot":
      if (
        t.attemptId !== a.view.attemptId ||
        t.epoch !== a.epoch ||
        t.namespace !== a.view.namespace ||
        a.view.sourceId !== t.entry.sourceId ||
        a.view.context !== t.entry.context
      )
        return s;
      if (t.view?.ref && a.view.ref && (t.view.ref.id !== a.view.ref.id || t.view.ref.generation !== a.view.ref.generation)) return s;
      next = {
        ...t,
        view: a.view,
        selected: a.view.podLoading ? t.selected : a.view.pods?.some((p) => p.uid === t.selected) ? t.selected : null,
        error: "",
        pollFailed: false,
      };
      break;
    case "failure":
      if (t.request !== a.request) return s;
      next = { ...t, error: a.error };
      break;
    case "pollFailed":
      if (t.attemptId !== a.attemptId) return s;
      next = { ...t, pollFailed: true };
      break;
    case "namespace":
      next = {
        ...t,
        epoch: t.epoch + 1,
        namespace: a.namespace,
        selected: null,
        view: t.view ? { ...t.view, namespace: a.namespace, pods: [], podLoading: true, podProblem: null } : null,
      };
      break;
    case "filter":
      next = { ...t, filter: a.filter };
      break;
    case "select":
      next = { ...t, selected: a.uid };
      break;
  }
  return { ...s, tabs: { ...s.tabs, [a.key]: next } };
}
