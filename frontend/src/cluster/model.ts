/*
Purpose: Identify fixed infrastructure destinations and their display labels.
Used by: Navigation, inventory and detail.
Responsibilities: Keep namespace scope and catalog jump expectations explicit.
Important: Catalog intents are matched against live descriptors before use.
*/
export type {
  ClusterResourcePage,
  ClusterResourceRow,
  EventInventoryPage,
  EventInventoryRow,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
export const clusterLabels = {
  Node: "Nodes",
  Event: "Events",
  PriorityClass: "PriorityClasses",
  RuntimeClass: "RuntimeClasses",
  Lease: "Leases",
  CustomResourceDefinition: "CustomResourceDefinitions",
  APIService: "APIServices",
} as const;
export type ClusterKind = keyof typeof clusterLabels;
export const clusterKinds = Object.keys(clusterLabels) as ClusterKind[];
export const isClusterKind = (kind: string): kind is ClusterKind => clusterKinds.some((k) => k === kind);
export const isClusterInfrastructure = (kind: string) => isClusterKind(kind) && kind !== "Lease" && kind !== "Event";
export type CatalogIntent = {
  group: string;
  version: string;
  match?: { resource: string; kind: string; namespaced: boolean };
};
