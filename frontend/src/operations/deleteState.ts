/*
Purpose: Gate deletion controls using captured Overview evidence.
Used by: The workload toolbar and deletion dialog.
Responsibilities: Explain local ineligibility and enforce exact typed-name confirmation.
Important: Native preparation independently verifies the live target and impact.
*/
import type { ResourceDetail } from "../workloads/model";
import { overviewObject } from "../workspace/ResourceOverview";

export function deleteUnavailable(source: ResourceDetail) {
  const { resource, resourceVersion } = source;
  if (!resource.uid || !resourceVersion || !resource.namespace) return "Refresh Overview to capture the workload identity and version.";
  if (source.deleteBlocked) return source.deleteBlocked;
  const { metadata, spec } = overviewObject(source);
  if (metadata && typeof metadata === "object" && "deletionTimestamp" in metadata && metadata.deletionTimestamp != null)
    return "This workload is terminating.";
  if (resource.kind === "StatefulSet") {
    if (!spec || typeof spec !== "object") return "StatefulSet claim retention could not be verified.";
    if ("persistentVolumeClaimRetentionPolicy" in spec) {
      const policy = spec.persistentVolumeClaimRetentionPolicy;
      if (
        !policy ||
        typeof policy !== "object" ||
        Array.isArray(policy) ||
        ("whenDeleted" in policy && policy.whenDeleted !== "Retain") ||
        ("whenScaled" in policy && policy.whenScaled !== "Retain" && policy.whenScaled !== "Delete")
      )
        return "StatefulSet deletion requires a verified Retain policy for claims.";
    }
  }
  return "";
}

export function canConfirmDelete(
  review: { token: string; expiresAt: string; resource: { name: string } },
  acknowledgement: string,
  now: number,
) {
  return !!review.token && now < Date.parse(review.expiresAt) && acknowledgement === review.resource.name;
}
