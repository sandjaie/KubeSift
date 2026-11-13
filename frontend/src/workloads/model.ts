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
  RelatedJobPage,
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
  RelatedJobPage,
  EventPage,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
import { isNetworkKind, networkKinds, type NetworkKind, type NetworkPage, type ServiceSlicePage } from "../networking/model";
import { configurationKinds, isConfigurationKind, type ConfigurationKind, type ConfigurationPage } from "../configuration/model";
import { isStorageKind, storageKinds, type StorageKind, type StoragePage, type ResolvedStorageReference } from "../storage/model";
export const workloadKinds = ["Pod", "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet", "Job", "CronJob"] as const;
export type Kind = (typeof workloadKinds)[number];
export function isWorkloadKind(kind: string): kind is Kind {
  return workloadKinds.some((item) => item === kind);
}
export type ResourceKind = Kind | NetworkKind | ConfigurationKind | StorageKind;
export const resourceKinds = [...workloadKinds, ...networkKinds, ...configurationKinds, ...storageKinds];
export const isResourceKind = (kind: string): kind is ResourceKind =>
  isWorkloadKind(kind) || isNetworkKind(kind) || isConfigurationKind(kind) || isStorageKind(kind);
export const isClusterScoped = (kind: string) =>
  kind === "IngressClass" || kind === "Namespace" || (isStorageKind(kind) && kind !== "PersistentVolumeClaim");
export const inventoryNamespace = (kind: ResourceKind, namespace: string) => (isClusterScoped(kind) ? "" : namespace);
export type Section = "Overview" | "YAML" | "Events" | "Pods" | "Logs" | "ReplicaSets" | "Jobs" | "EndpointSlices" | "Data";
export function resourceSections(kind: string): Section[] {
  if (kind === "Secret") return ["Overview"];
  const sections: Section[] = ["Overview"];
  if (kind === "ConfigMap") sections.push("Data");
  sections.push("YAML");
  if (!isClusterScoped(kind)) sections.push("Events");
  if (kind === "Pod") sections.push("Logs");
  else if (kind === "Service") sections.push("EndpointSlices");
  else if (isWorkloadKind(kind)) {
    sections.push(kind === "CronJob" ? "Jobs" : "Pods");
    if (kind === "Deployment") sections.push("ReplicaSets");
  }
  return sections;
}
export type Evidence = ResourceDetail | YAMLDocument | RelatedPage | RelatedReplicaSetPage | RelatedJobPage | EventPage | ServiceSlicePage;
export interface SectionState {
  loading: boolean;
  error: string;
  data: Evidence | null;
}
export type StorageLinkEvidence = Pick<
  ResolvedStorageReference,
  "bindingEvidence" | "sourceVersion" | "targetVersion" | "identityEvidence"
>;
export interface KindState {
  filter: string;
  sort: "asc" | "desc";
  scroll: number;
  history: string[];
  page: WorkloadPage | NetworkPage | ConfigurationPage | StoragePage | null;
  selected: { resource: ResourceRef } | null;
  related: boolean;
  identity: string;
  storageLink?: StorageLinkEvidence;
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
  identity?: string;
  namespace?: string;
  storageLink?: StorageLinkEvidence;
  position?: Pick<KindState, "filter" | "sort" | "scroll" | "history">;
}
export interface TabState {
  scope: string;
  clusterScope: string;
  kind: ResourceKind;
  kinds: Record<ResourceKind, KindState>;
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
