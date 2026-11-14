/*
Purpose: Present scale feedback across workload and cluster navigation.
Used by: The app header and result dialog.
Responsibilities: Recover native results and distinguish acceptance, rejection, staleness and uncertainty.
Important: Refreshing results or targets never calls Execute.
*/
import { useCallback, useEffect, useRef, useState } from "react";
import { message } from "../connections/api";
import { Modal } from "../ui/ReviewDialog";
import type { ResourceRef } from "../workloads/model";
import { executeScale, listScaleResults, type ScalePreview, type ScaleResult } from "./api";

export function useScaleResults(onResult: (result: ScaleResult) => void) {
  const [results, setResults] = useState<ScaleResult[]>([]);
  const [pending, setPending] = useState(0);
  const [error, setError] = useState("");
  const query = useRef(0);
  const onFinish = useRef(onResult);
  onFinish.current = onResult;
  const refresh = useCallback(() => {
    const request = ++query.current;
    void listScaleResults().then(
      (rows) => {
        if (request === query.current) setResults(rows ?? []);
      },
      () => {
        if (request === query.current) setError("Scale results could not be read. Refresh results to check again.");
      },
    );
  }, []);
  const tracking = pending > 0 || results.some((row) => row.state === "executing");
  useEffect(() => {
    refresh();
    if (!tracking) return;
    const timer = window.setInterval(refresh, 1000);
    return () => window.clearInterval(timer);
  }, [refresh, tracking]);
  const submit = (review: ScalePreview, acknowledgement: string) => {
    setError("");
    setPending((count) => count + 1);
    void executeScale(review.token, acknowledgement)
      .then(
        (result) => onFinish.current(result),
        (failure) => {
          setError(message(failure) + " Check Scale results for a recorded outcome.");
        },
      )
      .finally(() => {
        refresh();
        setPending((count) => count - 1);
      });
  };
  return { results, pending, error, refresh, submit };
}

const outcomes: Record<string, string> = {
  executing: "Submitting",
  accepted: "Accepted by Kubernetes",
  stale: "Target changed",
  rejected: "Rejected",
  "cancelled-before-send": "Cancelled before submission",
  unknown: "Outcome unknown",
};
export function OperationResults({
  results,
  pending,
  error,
  onClose,
  onRefresh,
  onTarget,
  canOpen,
}: {
  results: ScaleResult[];
  pending: number;
  error: string;
  onClose: () => void;
  onRefresh: () => void;
  onTarget: (ref: ResourceRef) => void;
  canOpen: (ref: ResourceRef) => boolean;
}) {
  return (
    <Modal title="Scale results" onCancel={onClose}>
      <button onClick={onRefresh}>Refresh results</button>
      {pending > 0 && (
        <p role="status">
          {pending} scale submission{pending === 1 ? "" : "s"} pending.
        </p>
      )}
      {error && <p role="alert">{error}</p>}
      {!results.length && !pending && <p>No scale results in this app session.</p>}
      {results.map((row) => (
        <section key={row.operationID} className="scale-result">
          <h3>{outcomes[row.state] ?? row.state}</h3>
          <p className="frozen-target">
            {row.cluster} / {row.resource.namespace} / {row.resource.kind} / {row.resource.name}
          </p>
          <p>
            Desired replicas before review: {row.before}. Requested: {row.requested}.
            {row.reported != null && ` API reported: ${row.reported}.`}
          </p>
          {row.state === "accepted" && <p>Pod readiness and whether the count persists are separate from API acceptance.</p>}
          {row.problem?.message && <p>{row.problem.message}</p>}
          <time dateTime={row.finishedAt ?? row.startedAt}>{new Date(row.finishedAt ?? row.startedAt).toLocaleTimeString()}</time>{" "}
          <button
            disabled={!canOpen(row.resource)}
            onClick={() => {
              onTarget(row.resource);
              onClose();
            }}
          >
            Open target / refresh
          </button>
        </section>
      ))}
    </Modal>
  );
}
