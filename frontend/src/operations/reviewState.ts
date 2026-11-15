/*
Purpose: Own one cancellable review before a confirmed operation.
Used by: Scale and restart dialogs.
Responsibilities: Revoke abandoned and late reviews and claim each token once.
Important: A claimed token belongs to the executing operation.
*/
export function reviewOwner(revoke: (token: string) => void) {
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
