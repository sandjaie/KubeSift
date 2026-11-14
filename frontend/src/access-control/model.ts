/*
Purpose: Define access control destinations and captured reference fields.
Used by: Inventory, Overview and workload navigation.
Responsibilities: Keep external subjects inert and project only explicit resource links.
Important: The backend revalidates every opened reference.
*/
import type { ResourceRef } from "../workloads/model";
export type { AccessControlRow, AccessControlPage } from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";

export const accessControlLabels = {
  ServiceAccount: "ServiceAccounts",
  Role: "Roles",
  RoleBinding: "RoleBindings",
  ClusterRole: "ClusterRoles",
  ClusterRoleBinding: "ClusterRoleBindings",
} as const;
export type AccessControlKind = keyof typeof accessControlLabels;
export const accessControlKinds = Object.keys(accessControlLabels) as AccessControlKind[];
export const isAccessControlKind = (kind: string): kind is AccessControlKind => accessControlKinds.some((item) => item === kind);
const group = "rbac.authorization.k8s.io";
export type Subject = { kind?: string; name?: string; apiGroup?: string; namespace?: string };
export type RoleRef = Omit<Subject, "namespace">;
export type Rule = { verbs?: string[]; apiGroups?: string[]; resources?: string[]; resourceNames?: string[]; nonResourceURLs?: string[] };
export type Selector = {
  matchLabels?: Record<string, string>;
  matchExpressions?: { key?: string; operator?: string; values?: string[] }[];
};
export type Aggregation = { clusterRoleSelectors?: Selector[] };
const dnsLabel = /^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$/;
export const automount = (value?: boolean) => (value == null ? "Not specified" : String(value));

export function accountTarget(source: ResourceRef, name?: string, namespace = source.namespace): ResourceRef | undefined {
  if (
    !name ||
    name.length > 253 ||
    !name.split(".").every((part) => dnsLabel.test(part)) ||
    namespace.length > 63 ||
    !dnsLabel.test(namespace)
  )
    return;
  return { ...source, kind: "ServiceAccount", apiVersion: "v1", uid: "", name, namespace };
}

export function subjectTarget(source: ResourceRef, subject: Subject) {
  if (subject.kind !== "ServiceAccount" || subject.apiGroup) return;
  return accountTarget(source, subject.name, subject.namespace || (source.kind === "RoleBinding" ? source.namespace : ""));
}

export function roleTarget(source: ResourceRef, ref?: RoleRef): ResourceRef | undefined {
  const name = ref?.name;
  if (!ref || ref.apiGroup !== group || !name || name === "." || name === ".." || /[/%]/.test(name)) return;
  if (ref.kind !== "ClusterRole" && !(ref.kind === "Role" && source.kind === "RoleBinding")) return;
  return { ...source, kind: ref.kind, apiVersion: group + "/v1", name, uid: "", namespace: ref.kind === "Role" ? source.namespace : "" };
}

export function recognizedRule(rule: Rule) {
  if (!rule.verbs?.length) return false;
  if (rule.nonResourceURLs?.length) return !rule.apiGroups?.length && !rule.resources?.length && !rule.resourceNames?.length;
  return Boolean(rule.apiGroups?.length && rule.resources?.length);
}
