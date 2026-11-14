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
import { accessControlKinds, isAccessControlKind, type AccessControlKind, type AccessControlPage } from "../access-control/model";
import {
  availabilityKinds,
  isAvailabilityKind,
  type AvailabilityKind,
  type AvailabilityPage,
  type SelectedPodPage,
  type ControllerRevisionPage,
} from "../availability/model";
import {
  clusterKinds,
  isClusterKind,
  isClusterInfrastructure,
  type ClusterKind,
  type ClusterResourcePage,
  type EventInventoryPage,
} from "../cluster/model";
export const workloadKinds = [
  "Pod",
  "Deployment",
  "ReplicaSet",
  "ReplicationController",
  "StatefulSet",
  "DaemonSet",
  "Job",
  "CronJob",
] as const;
export type Kind = (typeof workloadKinds)[number];
export function isWorkloadKind(kind: string): kind is Kind {
  return workloadKinds.some((item) => item === kind);
}
export type ResourceKind =
  | Kind
  | NetworkKind
  | ConfigurationKind
  | StorageKind
  | AccessControlKind
  | AvailabilityKind
  | ClusterKind
  | "APIResources";
export const resourceKinds: ResourceKind[] = [
  ...workloadKinds,
  ...networkKinds,
  ...configurationKinds,
  ...storageKinds,
  ...accessControlKinds,
  ...availabilityKinds,
  ...clusterKinds,
  "APIResources",
];
export const isResourceKind = (kind: string): kind is ResourceKind =>
  isWorkloadKind(kind) ||
  isNetworkKind(kind) ||
  isConfigurationKind(kind) ||
  isStorageKind(kind) ||
  isAccessControlKind(kind) ||
  isAvailabilityKind(kind) ||
  isClusterKind(kind) ||
  kind === "APIResources";
export const isClusterScoped = (kind: string) =>
  isClusterInfrastructure(kind) ||
  kind === "IngressClass" ||
  kind === "Namespace" ||
  kind === "ClusterRole" ||
  kind === "ClusterRoleBinding" ||
  (isStorageKind(kind) && kind !== "PersistentVolumeClaim");
export const inventoryNamespace = (kind: ResourceKind, namespace: string) => (isClusterScoped(kind) ? "" : namespace);
export type Section =
  | "Overview"
  | "YAML"
  | "Events"
  | "Pods"
  | "Logs"
  | "ReplicaSets"
  | "Jobs"
  | "EndpointSlices"
  | "Data"
  | "Selected Pods"
  | "Revisions";
export function resourceSections(kind: string): Section[] {
  if (kind === "Secret") return ["Overview"];
  const sections: Section[] = ["Overview"];
  if (kind === "ConfigMap") sections.push("Data");
  sections.push("YAML");
  if (!isClusterScoped(kind) && kind !== "Event" && kind !== "APIResources") sections.push("Events");
  if (kind === "Pod") sections.push("Logs");
  else if (kind === "Service") sections.push("EndpointSlices");
  else if (kind === "PodDisruptionBudget") sections.push("Selected Pods");
  else if (isWorkloadKind(kind)) {
    sections.push(kind === "CronJob" ? "Jobs" : "Pods");
    if (kind === "Deployment") sections.push("ReplicaSets");
    if (kind === "StatefulSet" || kind === "DaemonSet") sections.push("Revisions");
  }
  return sections;
}
export type Evidence =
  | ResourceDetail
  | YAMLDocument
  | RelatedPage
  | RelatedReplicaSetPage
  | RelatedJobPage
  | EventPage
  | ServiceSlicePage
  | SelectedPodPage
  | ControllerRevisionPage;
export interface SectionState {
  loading: boolean;
  error: string;
  data: Evidence | null;
}
export type ReferenceEvidence = Pick<ResolvedStorageReference, "sourceVersion" | "targetVersion" | "identityEvidence"> & {
  bindingEvidence?: string;
  association?: string;
};
export interface KindState {
  filter: string;
  sort: "asc" | "desc";
  scroll: number;
  history: string[];
  page:
    | WorkloadPage
    | NetworkPage
    | ConfigurationPage
    | StoragePage
    | AccessControlPage
    | AvailabilityPage
    | ClusterResourcePage
    | EventInventoryPage
    | null;
  selected: { resource: ResourceRef } | null;
  related: boolean;
  identity: string;
  referenceEvidence?: ReferenceEvidence;
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
  detail?: ResourceDetail;
  resource: ResourceRef;
  section: Section;
  cursor: string;
  identity?: string;
  namespace?: string;
  referenceEvidence?: ReferenceEvidence;
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
