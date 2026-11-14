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

export function scaleReviewOwner(revoke: (token: string) => void) {
  let active: { cancel: () => void } | undefined;
  let token = "";
  let confirmed = false;
  function abandon() {
    const request = active;
    active = undefined;
    request?.cancel();
    if (token) revoke(token);
    token = "";
  }
  return {
    abandon,
    canPrepare: () => !active && !confirmed,
    prepare<T extends { token: string }>(
      request: { promise: Promise<T>; cancel: () => void },
      show: (value: T) => void,
      fail: (error: unknown) => void,
    ) {
      abandon();
      active = request;
      void request.promise.then(
        (value) => {
          if (active !== request) {
            if (value.token) revoke(value.token);
            return;
          }
          active = undefined;
          token = value.token;
          show(value);
        },
        (error) => {
          if (active !== request) return;
          active = undefined;
          fail(error);
        },
      );
    },
    claim(value: string) {
      if (confirmed || !token || token !== value) return false;
      confirmed = true;
      token = "";
      return true;
    },
  };
}
