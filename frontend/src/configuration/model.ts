/*
Purpose: Describe configuration inventory and captured view contracts.
Used by: Configuration views and shared resource navigation.
Responsibilities: Keep namespace scope and supported browse targets explicit.
Important: Secret data is absent from these contracts.
*/
export type { ConfigurationPage, ConfigurationRow } from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";

export const configurationKinds = ["Namespace", "ResourceQuota", "LimitRange", "ConfigMap", "Secret"] as const;
export type ConfigurationKind = (typeof configurationKinds)[number];
export type NamespaceDestination = "ConfigMap" | "ResourceQuota" | "LimitRange";
export const isConfigurationKind = (kind: string): kind is ConfigurationKind => configurationKinds.some((candidate) => candidate === kind);
export const configurationLabels: Record<ConfigurationKind, string> = {
  Namespace: "Namespaces",
  ResourceQuota: "ResourceQuotas",
  LimitRange: "LimitRanges",
  ConfigMap: "ConfigMaps",
  Secret: "Secrets",
};
export type Quantities = Record<string, string>;
export type ScopeSelector = { matchExpressions?: { scopeName: string; operator: string; values?: string[] }[] };
export type Metadata = { creationTimestamp?: string; labels?: Record<string, string>; annotations?: Record<string, string> };
export type ConfigurationCapture = {
  metadata?: Metadata;
  creationTimestamp?: string;
  immutable?: boolean;
  textKeys?: number;
  binaryKeys?: number;
  data?: Record<string, string>;
  binaryData?: Record<string, string>;
  spec?: {
    hard?: Quantities;
    scopes?: string[];
    scopeSelector?: ScopeSelector;
    finalizers?: string[];
    limits?: {
      type: string;
      min?: Quantities;
      max?: Quantities;
      default?: Quantities;
      defaultRequest?: Quantities;
      maxLimitRequestRatio?: Quantities;
    }[];
  };
  status?: {
    phase?: string;
    hard?: Quantities;
    used?: Quantities;
    conditions?: { type: string; status: string; reason?: string; message?: string; lastTransitionTime?: string }[];
  };
};
export function scopeSummary(scopes?: string[], selector?: ScopeSelector) {
  return (
    [
      ...(scopes ?? []),
      ...(selector?.matchExpressions ?? []).map(({ scopeName, operator, values }) =>
        [scopeName, operator, values?.join(", ")].filter(Boolean).join(" "),
      ),
    ].join(" AND ") || "None specified"
  );
}
