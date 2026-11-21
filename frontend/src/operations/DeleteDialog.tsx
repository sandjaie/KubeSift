/*
Purpose: Review irreversible workload deletion with exact typed-name confirmation.
Used by: The workload Overview toolbar.
Responsibilities: Present the frozen target, native impact and mandatory preview.
Important: Acceptance leaves removal to Kubernetes and background garbage collection.
*/
import { useEffect, useLayoutEffect, useState } from "react";
import { message } from "../connections/api";
import { Modal } from "../ui/ReviewDialog";
import type { ResourceDetail } from "../workloads/model";
import { prepareDelete, revokeDelete, type DeletePreview } from "./api";
import { canConfirmDelete } from "./deleteState";
import { reviewOwner } from "./reviewState";

export function DeleteEvidence({ review }: { review: DeletePreview }) {
  const { impact, resource } = review;
  const finalizers = impact.finalizers ?? [];
  const pod = resource.kind === "Pod";
  const scheduled = resource.kind === "DaemonSet";
  const job = resource.kind === "Job";
  const cron = resource.kind === "CronJob";
  return (
    <>
      <dl className="pod-facts">
        <div>
          <dt>Propagation</dt>
          <dd>{impact.propagation}</dd>
        </div>
        <div>
          <dt>Grace</dt>
          <dd>{impact.gracePolicy}</dd>
        </div>
        {pod && (
          <div>
            <dt>Observed Pod grace</dt>
            <dd>{impact.podGrace == null ? "Unknown" : `${impact.podGrace}s${impact.podGraceDefault ? " (API default)" : ""}`}</dd>
          </div>
        )}
        {!pod && !job && !cron && (
          <>
            <div>
              <dt>{scheduled ? "Desired scheduled" : "Desired replicas"}</dt>
              <dd>{impact.desired ?? "Unknown"}</dd>
            </div>
            <div>
              <dt>{scheduled ? "Current scheduled" : "Current replicas"}</dt>
              <dd>{impact.current ?? "Unknown"}</dd>
            </div>
            <div>
              <dt>{scheduled ? "Ready scheduled" : "Ready replicas"}</dt>
              <dd>{impact.ready ?? "Unknown"}</dd>
            </div>
          </>
        )}
        {(job || cron) && (
          <div>
            <dt>{cron ? "Active Jobs" : "Active Pods"}</dt>
            <dd>{impact.active ?? "Unknown"}</dd>
          </div>
        )}
        {job && (
          <div>
            <dt>Ready Pods</dt>
            <dd>{impact.ready ?? "Unknown"}</dd>
          </div>
        )}
        {impact.whenDeleted && (
          <div>
            <dt>StatefulSet claim retention on deletion</dt>
            <dd>
              {impact.whenDeleted}
              {impact.retentionDefault && " (API default)"}
            </dd>
          </div>
        )}
      </dl>
      {impact.ownershipUnknown ? (
        <p className="warning">Controller ownership is unknown. The owner evidence was incomplete or inconsistent.</p>
      ) : impact.owner ? (
        <p className="network-value">
          Observed controlling owner: {impact.owner.apiVersion} {impact.owner.kind} {impact.owner.name} · UID {impact.owner.uid}. The owner
          may recreate this workload.
        </p>
      ) : (
        <p>
          {pod
            ? "No controlling owner was observed. Deleting a standalone Pod does not trigger controller recreation."
            : "No controlling owner was observed."}
        </p>
      )}
      {!pod && (
        <p className="warning">
          {cron
            ? "Deleting this CronJob stops future schedules. Background garbage collection can remove retained Jobs and their Pods."
            : "Background garbage collection can remove this controller’s dependents, including managed Pods."}{" "}
          Captured counts cover the workload’s current status.
        </p>
      )}
      {impact.whenDeleted && <p>Existing ownership and other controllers can still affect claims and application data.</p>}
      <p className="warning">
        {pod
          ? "Direct Pod deletion bypasses the eviction API and its PodDisruptionBudget admission check."
          : "Deleting a controller can cause downtime. PodDisruptionBudgets do not guarantee protection."}
      </p>
      <p>Finalizers and grace can delay removal. External automation may recreate resources. Deletion has no undo or automatic retry.</p>
      <p>
        Finalizers: {impact.finalizerCount}
        {impact.finalizersPartial && ` (showing ${finalizers.length})`}
      </p>
      {!!finalizers.length && (
        <ul className="network-value">
          {finalizers.map((name, index) => (
            <li key={`${name}-${index}`}>{name}</li>
          ))}
        </ul>
      )}
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

export function DeleteDialog({
  source,
  cluster,
  onDismiss,
  onExecute,
}: {
  source: ResourceDetail;
  cluster: string;
  onDismiss: () => void;
  onExecute: (review: DeletePreview, acknowledgement: string) => void;
}) {
  const [target] = useState(() => ({
    resource: { ...source.resource, connection: { ...source.resource.connection } },
    sourceVersion: source.resourceVersion,
    cluster,
  }));
  const [owner] = useState(() => reviewOwner(revokeDelete));
  const { abandon } = owner;
  const [review, setReview] = useState<DeletePreview>();
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
      prepareDelete({ resource: target.resource, sourceVersion: target.sourceVersion }),
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
    if (!review || !canConfirmDelete(review, ack, Date.now()) || !owner.claim(review.token)) return;
    onExecute(review, ack);
    onDismiss();
  }
  return (
    <Modal
      title="Delete workload"
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
      <p className="warning">Deleting this workload is irreversible.</p>
      {loading && <p role="status">Checking current identity and running server dry-run…</p>}
      {error && <p role="alert">{error}</p>}
      {review && <DeleteEvidence review={review} />}
      {review && !expired && (
        <label className="scale-field">
          Type {target.resource.name} to confirm deletion
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
      {expired && <p role="alert">This review expired. Review deletion again.</p>}
      <div className="scale-actions">
        {(!review || expired) && (
          <button className="primary" disabled={loading} onClick={prepare}>
            Review deletion
          </button>
        )}
        {review && !expired && (
          <button className="danger" disabled={!canConfirmDelete(review, ack, Date.now())} onClick={confirm}>
            Confirm deletion
          </button>
        )}
      </div>
    </Modal>
  );
}
