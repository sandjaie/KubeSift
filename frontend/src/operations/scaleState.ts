/*
Purpose: Validate the scaling draft and its accompanying review.
Used by: The scale dialog and tests.
Responsibilities: Gate exact decimal input, expiry and zero acknowledgement.
Important: Native code independently enforces the same rules.
*/
import type { ScalePreview } from "./api";
type Review = Pick<ScalePreview, "token" | "expiresAt" | "after" | "noChange"> & { resource: { name: string } };
export function validReplicaText(value: string) {
  return value.length > 0 && value.length <= 128 && !/[^0-9]/.test(value) && Number(value) <= 2147483647;
}
export function canConfirmScale(review: Review, count: string, acknowledgement: string, now: number) {
  return (
    !!review.token &&
    !review.noChange &&
    validReplicaText(count) &&
    Number(count) === review.after &&
    now < Date.parse(review.expiresAt) &&
    (review.after !== 0 || acknowledgement === review.resource.name)
  );
}
