/*
Purpose: Retain native Helm operation status across workspace navigation.
Used by: App and the existing operation-results dialog.
Responsibilities: Poll active native workers, show fixed targets and offer cancellation or read-only refresh.
Important: Polling and navigation never resubmit an action.
*/
import { useCallback, useEffect, useRef, useState } from "react";
import type { Result, Target } from "../../bindings/internal/helmmanager/models";
import { message } from "../connections/api";
import { helmManagementAPI as api } from "./managementApi";

export function useHelmOperationResults() {
  const [results, setResults] = useState<Result[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [readError, setReadError] = useState("");
  const epoch = useRef(0);
  const refresh = useCallback(() => {
    const current = ++epoch.current;
    void api.ListHelmResults().then(
      (rows) => {
        if (current === epoch.current) {
          setResults(rows ?? []);
          setReadError("");
        }
      },
      () => {
        if (current === epoch.current) setReadError("Helm results could not be read. Refresh to check native operation status.");
      },
    );
  }, []);
  const tracking = pending || results.some((result) => result.state === "running");
  useEffect(() => {
    refresh();
    if (!tracking) return;
    const timer = window.setInterval(refresh, 1000);
    return () => window.clearInterval(timer);
  }, [tracking, refresh]);
  useEffect(
    () => () => {
      epoch.current++;
    },
    [],
  );
  async function submit(token: string) {
    setPending(true);
    setError("");
    try {
      const result = await api.ExecuteHelmAction(token);
      setResults((rows) => [result, ...rows.filter((row) => row.id !== result.id)]);
    } catch (reason) {
      setError(`${message(reason)} Check native results before preparing any recovery.`);
    } finally {
      void api.DiscardHelmDraft(token).catch(() => {});
      setPending(false);
      refresh();
    }
  }
  async function cancel(id: string) {
    try {
      await api.CancelHelmAction(id);
      refresh();
    } catch (reason) {
      setError(message(reason));
    }
  }
  return { results, pending, error: error || readError, refresh, submit, cancel };
}

export function HelmOperationResults({
  results,
  pending,
  error,
  onRefresh,
  onCancel,
  onTarget,
  canOpen,
}: {
  results: Result[];
  pending: boolean;
  error: string;
  onRefresh: () => void;
  onCancel: (id: string) => void;
  onTarget: (target: Target) => void;
  canOpen: (target: Target) => boolean;
}) {
  return (
    <section aria-label="Helm operation results">
      <h3>Helm operations</h3>
      <button onClick={onRefresh}>Refresh Helm results</button>
      {pending && <p role="status">Submitting the reviewed Helm action…</p>}
      {error && <p role="alert">{error}</p>}
      {!results.length && !pending && <p>No Helm operations in this app session.</p>}
      {results.map((result) => (
        <section className="scale-result" key={result.id}>
          <h4>
            {result.action} · {result.state === "unknown" ? "Outcome unknown" : result.state}
          </h4>
          <p className="frozen-target">
            {result.contextName} / {result.target.namespace} / {result.target.releaseName} · {result.target.driver} storage
          </p>
          <p role="status">{result.stage}</p>
          {result.problem && <p role="alert">{result.problem.message}</p>}
          {result.observed && (
            <p>
              Latest observed revision: {result.observed.record.revision} · {result.observed.status}
            </p>
          )}
          <p className="muted">Helm actions may leave partial changes. No automatic rollback, cleanup or retry is performed.</p>
          <div className="button-row">
            {result.state === "running" && <button onClick={() => onCancel(result.id)}>Cancel Helm operation</button>}
            <button disabled={!canOpen(result.target)} onClick={() => onTarget(result.target)}>
              Refresh release/history
            </button>
          </div>
        </section>
      ))}
    </section>
  );
}
