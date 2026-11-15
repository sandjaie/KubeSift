/*
Purpose: Review and explicitly confirm one controller restart.
Used by: The workload Overview header.
Responsibilities: Show the frozen target, exact annotation change and controller impact.
Important: A successful submission only proves that Kubernetes accepted the patch.
*/
import { useEffect, useLayoutEffect, useState } from "react";
import { message } from "../connections/api";
import { Modal } from "../ui/ReviewDialog";
import type { ResourceDetail } from "../workloads/model";
import { prepareRestart, revokeRestart, type RestartPreview } from "./api";
import { canConfirmRestart } from "./restartState";
import { reviewOwner } from "./reviewState";

function observed(value: number | string | null, fallback = false) {
  return value == null || value === "" ? "Unknown" : `${value}${fallback ? " (default)" : ""}`;
}

export function RestartEvidence({ review }: { review: RestartPreview }) {
  const { impact, resource } = review;
  const daemon = resource.kind === "DaemonSet";
  return (
    <>
      <dl className="pod-facts">
        <div>
          <dt>Strategy</dt>
          <dd>{observed(impact.strategy, impact.strategyDefault)}</dd>
        </div>
        <div>
          <dt>{daemon ? "Desired scheduled" : "Desired replicas"}</dt>
          <dd>{observed(impact.desired, impact.desiredDefault)}</dd>
        </div>
        <div>
          <dt>{daemon ? "Current scheduled" : "Current replicas"}</dt>
          <dd>{observed(impact.current)}</dd>
        </div>
        <div>
          <dt>{daemon ? "Ready scheduled" : "Ready replicas"}</dt>
          <dd>{observed(impact.ready)}</dd>
        </div>
        {impact.maxSurge && (
          <div>
            <dt>Max surge</dt>
            <dd>{observed(impact.maxSurge, impact.maxSurgeDefault)}</dd>
          </div>
        )}
        {impact.maxUnavailable && (
          <div>
            <dt>Max unavailable</dt>
            <dd>{observed(impact.maxUnavailable, impact.maxUnavailableDefault)}</dd>
          </div>
        )}
        {impact.partition != null && (
          <div>
            <dt>Partition</dt>
            <dd>{observed(impact.partition, impact.partitionDefault)}</dd>
          </div>
        )}
      </dl>
      {impact.zeroPods && <p>No current Pods are expected to restart. Kubernetes can still accept the template change.</p>}
      {resource.kind === "StatefulSet" && <p>Updates proceed in reverse ordinal order. An unavailable Pod can stall progress.</p>}
      {daemon && <p>The controller manages Pod placement across eligible nodes.</p>}
      {impact.recreate && <p className="warning">Recreate can terminate all existing Pods before replacements start.</p>}
      <p className="warning">Controller updates can cause downtime. PodDisruptionBudgets do not guarantee protection during a rollout.</p>
      <p>External automation may overwrite the template change.</p>
      <p className="network-value">
        Pod template annotation <code>kubectl.kubernetes.io/restartedAt</code>
      </p>
      <dl className="pod-facts">
        <div>
          <dt>Previous</dt>
          <dd className="network-value">
            {review.previousSet ? <code>{review.previousValue || '""'}</code> : "Not set"}
            {review.previousTruncated && " (truncated)"}
          </dd>
        </div>
        <div>
          <dt>Proposed</dt>
          <dd className="network-value">
            <code>{review.restartAt}</code>
          </dd>
        </div>
      </dl>
      <p>
        Server dry-run passed. {review.permission === "allowed" ? "Permission hint: allowed." : "Permission hint: unknown."} Kubernetes
        checks authorization again on submission.
      </p>
      <p className="muted">Review expires {new Date(review.expiresAt).toLocaleTimeString()}.</p>
      {!!review.warnings?.length && (
        <ul>
          {review.warnings.map((warning, index) => (
            <li key={index}>{warning}</li>
          ))}
        </ul>
      )}
    </>
  );
}

export function RestartDialog({
  source,
  cluster,
  onDismiss,
  onExecute,
}: {
  source: ResourceDetail;
  cluster: string;
  onDismiss: () => void;
  onExecute: (review: RestartPreview, acknowledgement: string) => void;
}) {
  const [target] = useState(() => ({
    resource: { ...source.resource, connection: { ...source.resource.connection } },
    sourceVersion: source.resourceVersion,
    cluster,
  }));
  const [owner] = useState(() => reviewOwner(revokeRestart));
  const { abandon } = owner;
  const [review, setReview] = useState<RestartPreview>();
  const [error, setError] = useState("");
  const [ack, setAck] = useState("");
  const [loading, setLoading] = useState(false);
  const [expired, setExpired] = useState(false);
  useLayoutEffect(() => abandon, [abandon]);
  useEffect(() => {
    if (!review?.token) return;
    const timer = window.setTimeout(
      () => {
        abandon();
        setExpired(true);
      },
      Math.max(0, Date.parse(review.expiresAt) - Date.now()),
    );
    return () => window.clearTimeout(timer);
  }, [abandon, review]);
  function prepare() {
    if (!owner.canPrepare()) return;
    setLoading(true);
    setExpired(false);
    setReview(undefined);
    setError("");
    setAck("");
    owner.prepare(
      prepareRestart({ resource: target.resource, sourceVersion: target.sourceVersion }),
      (value) => {
        setReview(value);
        setLoading(false);
      },
      (failure) => {
        setError(message(failure));
        setLoading(false);
      },
    );
  }
  function confirm() {
    if (!review || !canConfirmRestart(review, ack, Date.now()) || !owner.claim(review.token)) return;
    onExecute(review, ack);
    onDismiss();
  }
  return (
    <Modal
      title="Restart workload"
      onCancel={() => {
        abandon();
        onDismiss();
      }}
    >
      <p className="frozen-target">
        {target.cluster} / {target.resource.namespace} / {target.resource.kind} / {target.resource.name}
      </p>
      <p className="muted">
        {target.resource.apiVersion} · UID {target.resource.uid} · version {target.sourceVersion}
      </p>
      {loading && <p role="status">Checking current identity and running server dry-run…</p>}
      {error && <p role="alert">{error}</p>}
      {review && <RestartEvidence review={review} />}
      {review?.impact.recreate && !expired && (
        <label className="scale-field">
          Type {target.resource.name} to confirm Recreate
          <input
            value={ack}
            onChange={(event) => setAck(event.target.value)}
            spellCheck={false}
            autoCorrect="off"
            autoCapitalize="none"
            autoComplete="off"
          />
        </label>
      )}
      {expired && <p role="alert">This review expired. Review the restart again.</p>}
      <div className="scale-actions">
        {(!review || expired) && (
          <button className="primary" disabled={loading} onClick={prepare}>
            Review restart
          </button>
        )}
        {review && !expired && (
          <button className="primary" disabled={!canConfirmRestart(review, ack, Date.now())} onClick={confirm}>
            Confirm restart
          </button>
        )}
      </div>
    </Modal>
  );
}
