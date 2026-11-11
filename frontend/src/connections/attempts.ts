/*
Purpose: Serialize connection ownership changes for each source and context.
Used by: useConnections and deferred RPC regression tests.
Responsibilities: Finish superseded Start cancellation before a replacement Start.
Important: Different contexts remain independent and failed operations do not block the queue.
*/
export function createAttemptQueue() {
  const pending = new Map<string, Promise<void>>();
  return (key: string, operation: () => Promise<void>): Promise<void> => {
    const previous = pending.get(key) ?? Promise.resolve();
    const next = previous.catch(() => {}).then(operation);
    pending.set(key, next);
    void next
      .finally(() => {
        if (pending.get(key) === next) pending.delete(key);
      })
      .catch(() => {});
    return next;
  };
}
