/*
Purpose: Adapt generated explorer inventories, relationships and reference reads to independently cancellable invocations.
Used by: The explorer inventory and detail controller.
Responsibilities: Issue unique request IDs and expose native cancellation.
Important: Cancellation is paired with reducer epochs; it is not a guarantee against late replies.
*/
import { ExplorerService } from "../../bindings/github.com/sandjaie/KubeSift/internal/desktop";
export const workloadAPI = ExplorerService;
export function readRequest<T>(read: (id: string) => Promise<T>) {
  const id = crypto.randomUUID();
  return {
    promise: read(id),
    cancel: () => {
      void ExplorerService.CancelWorkloadRead(id).catch(() => {});
    },
  };
}
