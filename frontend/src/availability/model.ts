/*
Purpose: Describe availability resources and their supported references.
Used by: The explorer and availability evidence views.
Responsibilities: Preserve API group, scope and captured owner identity.
Important: Unsupported references stay readable without inviting a lookup.
*/
import type { ResourceRef } from "../workloads/model";
export type {
  AvailabilityRow,
  AvailabilityPage,
  SelectedPodPage,
  ControllerRevisionPage,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";

export const availabilityLabels = {
  HorizontalPodAutoscaler: "HorizontalPodAutoscalers",
  PodDisruptionBudget: "PodDisruptionBudgets",
  ControllerRevision: "ControllerRevisions",
} as const;
export type AvailabilityKind = keyof typeof availabilityLabels;
export const availabilityKinds = Object.keys(availabilityLabels) as AvailabilityKind[];
export const isAvailabilityKind = (kind: string): kind is AvailabilityKind => availabilityKinds.some((value) => value === kind);

export type Reference = { apiVersion?: string; kind?: string; name?: string; uid?: string; controller?: boolean };
export const observationLabel = (status?: string) =>
  status === "current"
    ? "Observed current generation"
    : status === "stale"
      ? "Observation differs from current generation"
      : "Observation not reported";

export function availabilityTarget(source: ResourceRef, ref?: Reference): ResourceRef | undefined {
  if (!ref?.name || !ref.apiVersion || !ref.kind) return;
  const { name, apiVersion, kind, uid } = ref;
  if (
    !/^[a-z0-9](?:[-a-z0-9.]*[a-z0-9])?$/.test(name) ||
    name.length > 253 ||
    !name.split(".").every((part) => /^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$/.test(part))
  )
    return;
  if (source.kind === "HorizontalPodAutoscaler") {
    if (
      !(apiVersion === "apps/v1" && ["Deployment", "StatefulSet", "ReplicaSet"].includes(kind)) &&
      !(apiVersion === "v1" && kind === "ReplicationController")
    )
      return;
  } else if (
    source.kind !== "ControllerRevision" ||
    !ref.controller ||
    !uid ||
    apiVersion !== "apps/v1" ||
    !["StatefulSet", "DaemonSet"].includes(kind)
  )
    return;
  return { ...source, name, apiVersion, kind, uid: source.kind === "ControllerRevision" ? uid! : "" };
}

export const selectorLabel = (selector: unknown) =>
  selector == null
    ? "Selects no Pods"
    : Object.keys(selector).length === 0
      ? "Selects all Pods in this namespace"
      : "Selects matching Pods in this namespace";
