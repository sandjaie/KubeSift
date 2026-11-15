/*
Purpose: Gate restart controls against the captured Overview.
Used by: The workload header and restart dialog.
Responsibilities: Explain unsupported strategies and require explicit confirmation.
Important: The native service independently rechecks the complete target.
*/
import type { ResourceDetail } from "../workloads/model";
import { overviewObject } from "../workspace/ResourceOverview";

const record = (value: unknown): Record<string, unknown> | undefined =>
  value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : undefined;

export function restartUnavailable(source: ResourceDetail) {
  const { resource, resourceVersion } = source;
  const deployment = resource.kind === "Deployment";
  if (!deployment && resource.kind !== "StatefulSet" && resource.kind !== "DaemonSet")
    return "This workload kind does not support restart.";
  if (!resourceVersion || !resource.uid || resource.apiVersion !== "apps/v1")
    return "Refresh Overview to capture the workload identity and version.";
  const { metadata, spec: capturedSpec } = overviewObject(source);
  if (record(metadata)?.deletionTimestamp != null) return "This workload is terminating.";
  const spec = record(capturedSpec);
  const template = record(spec?.template);
  const malformed = "The captured restart evidence is malformed. Refresh Overview.";
  if (!spec || !record(template?.metadata) || !record(template?.spec)) return malformed;
  const annotations = record(template?.metadata)?.annotations;
  if (annotations != null && (!record(annotations) || Object.values(annotations).some((value) => typeof value !== "string")))
    return malformed;
  if (deployment && "paused" in spec && typeof spec.paused !== "boolean") return malformed;
  if (deployment && spec.paused === true) return "This Deployment is paused.";
  const replicaCount = spec.replicas;
  if (
    resource.kind !== "DaemonSet" &&
    "replicas" in spec &&
    (typeof replicaCount !== "number" || !Number.isInteger(replicaCount) || replicaCount < 0 || replicaCount > 2147483647)
  )
    return malformed;
  const key = deployment ? "strategy" : "updateStrategy";
  const strategy = record(spec[key]);
  if (key in spec && !strategy) return malformed;
  const type = strategy && "type" in strategy ? strategy.type : "RollingUpdate";
  if (typeof type !== "string" || !type) return malformed;
  if (type !== "RollingUpdate" && !(deployment && type === "Recreate"))
    return `Restart is unavailable with the ${type === "OnDelete" ? "OnDelete" : "captured"} strategy.`;
  const rolling = record(strategy?.rollingUpdate);
  if (strategy?.rollingUpdate != null && !rolling) return malformed;
  if (type === "RollingUpdate" && rolling) {
    for (const field of resource.kind === "StatefulSet" ? ["maxUnavailable"] : ["maxSurge", "maxUnavailable"]) {
      if (!(field in rolling)) continue;
      const value = rolling[field];
      const count = typeof value === "string" && /^[0-9]+%$/.test(value) ? Number(value.slice(0, -1)) : value;
      if (typeof count !== "number" || !Number.isInteger(count) || count < 0 || count > 2147483647) return malformed;
    }
  }
  if (resource.kind === "StatefulSet" && rolling && "partition" in rolling) {
    return rolling.partition === 0 ? "" : "Restart is unavailable with a nonzero or malformed StatefulSet partition.";
  }
  return "";
}

export function canConfirmRestart(
  review: { token: string; expiresAt: string; impact: { recreate: boolean }; resource: { name: string } },
  acknowledgement: string,
  now: number,
) {
  return !!review.token && now < Date.parse(review.expiresAt) && (!review.impact.recreate || acknowledgement === review.resource.name);
}
