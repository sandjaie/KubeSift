/*
Purpose: Describe memory-only workload navigation using generated native contracts.
Used by: The workload reducer, controller and investigation views.
Responsibilities: Keep page and section ownership separate from connection health.
Important: No workload evidence belongs in persisted preferences.
*/
export type {
  ResourceRef,
  WorkloadRow,
  WorkloadPage,
  ResourceDetail,
  YAMLDocument,
  RelatedPage,
  RelatedReplicaSetPage,
  EventPage,
  LogRequest,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
import type {
  ResourceRef,
  WorkloadPage,
  ResourceDetail,
  YAMLDocument,
  RelatedPage,
  RelatedReplicaSetPage,
  EventPage,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
export const workloadKinds = ["Pod", "Deployment", "ReplicaSet"] as const;
export type Kind = (typeof workloadKinds)[number];
export function isWorkloadKind(kind: string): kind is Kind {
  return workloadKinds.some((item) => item === kind);
}
export type Section = "Overview" | "YAML" | "Events" | "Pods" | "Logs" | "ReplicaSets";
export type Evidence = ResourceDetail | YAMLDocument | RelatedPage | RelatedReplicaSetPage | EventPage;
export interface SectionState {
  loading: boolean;
  error: string;
  data: Evidence | null;
}
export interface KindState {
  filter: string;
  sort: "asc" | "desc";
  scroll: number;
  history: string[];
  page: WorkloadPage | null;
  selected: { resource: ResourceRef } | null;
  related: boolean;
  section: Section;
  sections: Partial<Record<Section, SectionState>>;
  loading: boolean;
  error: string;
  epoch: number;
  sectionEpoch: number;
  refresh: number;
  sectionRefresh: number;
  sectionCursor: string;
  auto: boolean;
}
export interface NavigationEntry {
  resource: ResourceRef;
  section: Section;
  cursor: string;
}
export interface TabState {
  scope: string;
  kind: Kind;
  kinds: Record<Kind, KindState>;
  navigation: NavigationEntry[];
}
export type WorkloadsState = Record<string, TabState>;

// Observed typed Deployment status omits zero counters during JSON serialization.
export function deploymentCounts(status: Record<string, unknown>) {
  const observed = Number(status.observedGeneration ?? 0) > 0 || (Array.isArray(status.conditions) && status.conditions.length > 0);
  const count = (name: string) => (typeof status[name] === "number" ? status[name] : observed ? 0 : undefined);
  return {
    updated: count("updatedReplicas"),
    ready: count("readyReplicas"),
    available: count("availableReplicas"),
    unavailable: count("unavailableReplicas"),
  };
}
