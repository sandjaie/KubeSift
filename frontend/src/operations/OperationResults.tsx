/*
Purpose: Present reviewed operation feedback across workload and cluster navigation.
Used by: The app header and result dialog.
Responsibilities: Recover native results and distinguish acceptance, rejection, staleness and uncertainty.
Important: Refreshing results or targets never calls Execute.
*/
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { message } from "../connections/api";
import { Modal } from "../ui/ReviewDialog";
import type { ResourceRef } from "../workloads/model";
import {
  checkDeleteTarget,
  executeDelete,
  executeRestart,
  executeScale,
  listOperationResults,
  type DeleteObservation,
  type OperationAction,
  type OperationResult,
} from "./api";

const submissions = { delete: executeDelete, restart: executeRestart, scale: executeScale };
const actionLabel: Record<string, string> = { delete: "Delete", restart: "Restart", scale: "Scale" };

export function useOperationResults(onResult: (result: Pick<OperationResult, "resource" | "state" | "action">) => void) {
  const [results, setResults] = useState<OperationResult[]>([]);
  const [pending, setPending] = useState(0);
  const [error, setError] = useState("");
  const query = useRef(0);
  const acceptedDeletions = useRef(new Set<string>());
  const onFinish = useRef(onResult);
  onFinish.current = onResult;
  const reportDeletion = useCallback((result: OperationResult) => {
    const seen = acceptedDeletions.current;
    if (result.state !== "accepted" || seen.has(result.operationID)) return;
    seen.add(result.operationID);
    if (seen.size > 128) {
      const oldest = seen.values().next().value;
      if (oldest !== undefined) seen.delete(oldest);
    }
    onFinish.current(result);
  }, []);
  const refresh = useCallback(() => {
    const request = ++query.current;
    void listOperationResults().then(
      (rows) => {
        if (request !== query.current) return;
        const next = rows ?? [];
        for (const row of next) {
          if (row.action === "delete") reportDeletion(row);
        }
        setResults(next);
      },
      () => {
        if (request === query.current) setError("Operation results could not be read. Refresh results to check again.");
      },
    );
  }, [reportDeletion]);
  const tracking = pending > 0 || results.some((row) => row.state === "executing");
  useEffect(() => {
    refresh();
    if (!tracking) return;
    const timer = window.setInterval(refresh, 1000);
    return () => window.clearInterval(timer);
  }, [refresh, tracking]);
  const submit = (action: OperationAction, token: string, acknowledgement: string, releaseDeletion?: (recoverable: boolean) => void) => {
    setError("");
    setPending((count) => count + 1);
    const execute = submissions[action];
    void execute(token, acknowledgement)
      .then(
        (result) => {
          if (action === "delete" && "action" in result) reportDeletion(result);
          else onFinish.current({ ...result, action });
          releaseDeletion?.(false);
        },
        (failure) => {
          releaseDeletion?.(true);
          setError(message(failure) + " Check Operation results for a recorded outcome.");
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
  results: OperationResult[];
  pending: number;
  error: string;
  onClose: () => void;
  onRefresh: () => void;
  onTarget: (ref: ResourceRef) => void;
  canOpen: (ref: ResourceRef) => boolean;
}) {
  return (
    <Modal title="Operation results" onCancel={onClose}>
      <button onClick={onRefresh}>Refresh results</button>
      {pending > 0 && (
        <p role="status">
          {pending} operation submission{pending === 1 ? "" : "s"} pending.
        </p>
      )}
      {error && <p role="alert">{error}</p>}
      {!results.length && !pending && <p>No operation results in this app session.</p>}
      {results.map((row) => (
        <section key={row.operationID} className="scale-result">
          <h3>
            {actionLabel[row.action] ?? row.action} · {outcomes[row.state] ?? row.state}
          </h3>
          <p className="frozen-target">
            {row.cluster} / {row.resource.namespace} / {row.resource.kind} / {row.resource.name}
          </p>
          {row.scale && (
            <>
              <p>
                Desired replicas before review: {row.scale.before}. Requested: {row.scale.requested}.
                {row.scale.reported != null && ` API reported: ${row.scale.reported}.`}
              </p>
              {row.state === "accepted" && <p>Pod readiness and whether the count persists are separate from API acceptance.</p>}
            </>
          )}
          {row.restart && (
            <>
              <p>
                Requested annotation <code>{row.restart.requestedAt}</code>
              </p>
              {row.restart.responseDiffers && <p className="warning">The server response differs from the reviewed spec.</p>}
              {row.state === "accepted" && <p>Kubernetes accepted the template patch. Pod replacement and readiness are separate.</p>}
            </>
          )}
          {row.delete && (
            <>
              <p>
                {row.delete.propagation} propagation · {row.delete.gracePolicy} grace
              </p>
              {row.state === "accepted" && <p>Kubernetes accepted deletion. The workload and its dependents may still be present.</p>}
            </>
          )}
          {row.problem?.message && <p>{row.problem.message}</p>}
          <time dateTime={row.finishedAt ?? row.startedAt}>{new Date(row.finishedAt ?? row.startedAt).toLocaleTimeString()}</time>{" "}
          {row.delete ? (
            row.finishedAt && <DeleteTargetCheck operationID={row.operationID} canCheck={canOpen(row.resource)} />
          ) : (
            <button
              disabled={!canOpen(row.resource)}
              onClick={() => {
                onTarget(row.resource);
                onClose();
              }}
            >
              Open target / refresh
            </button>
          )}
        </section>
      ))}
    </Modal>
  );
}

const observationText: Record<string, string> = {
  present: "The original workload is present.",
  terminating: "The original workload is terminating.",
  absent: "The original workload is absent. Dependents may still be present.",
  replaced: "An object with a different UID now uses this name.",
  unavailable: "Target status is unavailable.",
};

function DeleteTargetCheck({ operationID, canCheck }: { operationID: string; canCheck: boolean }) {
  const [observation, setObservation] = useState<DeleteObservation>();
  const [error, setError] = useState("");
  const [checking, setChecking] = useState(false);
  const active = useRef<ReturnType<typeof checkDeleteTarget> | null>(null);
  useLayoutEffect(
    () => () => {
      const request = active.current;
      active.current = null;
      request?.cancel();
    },
    [],
  );
  function check() {
    if (active.current || !canCheck) return;
    const request = checkDeleteTarget(operationID);
    active.current = request;
    setChecking(true);
    setError("");
    void request.promise.then(
      (value) => {
        if (active.current !== request) return;
        active.current = null;
        setChecking(false);
        setObservation(value);
      },
      (failure) => {
        if (active.current !== request) return;
        active.current = null;
        setChecking(false);
        setError(message(failure));
      },
    );
  }
  return (
    <>
      <button onClick={check} disabled={checking || !canCheck}>
        Check target
      </button>
      {checking && <p role="status">Checking the original target…</p>}
      {error && <p role="alert">{error}</p>}
      {observation && (
        <p role="status">
          {observationText[observation.state]}{" "}
          <time dateTime={observation.checkedAt}>{new Date(observation.checkedAt).toLocaleTimeString()}</time>
        </p>
      )}
    </>
  );
}
