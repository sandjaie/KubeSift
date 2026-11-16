/*
Purpose: Keep a rollback review tied to its original workload and revision.
Used by: Revisions, the rollback dialog and operation results.
Responsibilities: Compare captured identities and check review expiry.
Important: Navigation cannot substitute another source or target.
*/
import type { RollbackPreview, RollbackRequest } from "./api";
import type { ResourceRef } from "../workloads/model";

const identity = ({ connection, apiVersion, kind, namespace, name, uid }: ResourceRef) => [
  connection.id,
  connection.generation,
  apiVersion,
  kind,
  namespace,
  name,
  uid,
];

export const rollbackSelectionKey = (request: RollbackRequest) =>
  JSON.stringify([identity(request.resource), request.resourceVersion, identity(request.source), request.sourceVersion, request.revision]);

export function canConfirmRollback(
  preview: Pick<RollbackPreview, "request" | "token" | "expiresAt"> | undefined,
  request: RollbackRequest,
  now: number,
) {
  return !!preview?.token && Date.parse(preview.expiresAt) > now && rollbackSelectionKey(preview.request) === rollbackSelectionKey(request);
}

export const rollbackAcceptedText = "Kubernetes accepted the Pod template patch. Pod replacement and readiness are separate.";
