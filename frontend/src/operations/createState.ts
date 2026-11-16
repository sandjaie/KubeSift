/*
Purpose: Keep create drafts and confirmations tied to their original connection and scope.
Used by: The named-resource editor and its tests.
Responsibilities: Describe unsaved draft state and check the current preview.
Important: The native service independently binds the canonical target and payload.
*/
import type { ConnectionRef } from "../bridge";
import type { CreatePreview } from "./api";
import { sameEditConnection } from "./editState";

export type CreateDestination = { mode: "create"; id: string; tab: string; connection: ConnectionRef; namespace: string };
export const createSkeleton = 'apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: ""\n';

export function canConfirmCreate(review: CreatePreview, connection: ConnectionRef, namespace: string, resource: string, now: number) {
  return (
    !!review.token &&
    now < Date.parse(review.expiresAt) &&
    sameEditConnection(review.target.connection, connection) &&
    review.target.namespace === namespace &&
    (!resource || review.target.resource === resource)
  );
}
