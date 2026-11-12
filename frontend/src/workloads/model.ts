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
  EventPage,
  LogRequest,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
import type {
  WorkloadRow,
  WorkloadPage,
  ResourceDetail,
  YAMLDocument,
  RelatedPage,
  EventPage,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
export type Kind = "Pod" | "Deployment";
export type Section = "Overview" | "YAML" | "Events" | "Pods" | "Logs";
export type Evidence = ResourceDetail | YAMLDocument | RelatedPage | EventPage;
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
  selected: WorkloadRow | null;
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
export interface TabState {
  scope: string;
  kind: Kind;
  kinds: Record<Kind, KindState>;
  back: WorkloadRow | null;
}
export type WorkloadsState = Record<string, TabState>;
