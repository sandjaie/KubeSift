/*
Purpose: Review and explicitly confirm one captured workload scale.
Used by: The dedicated workload detail header.
Responsibilities: Freeze target and count, revoke departed reviews and explain scale effects.
Important: The dialog delegates execution, which outlives this view.
*/
import { useEffect, useLayoutEffect, useState } from "react";
import { message } from "../connections/api";
import { Modal } from "../ui/ReviewDialog";
import type { ResourceDetail } from "../workloads/model";
import { overviewObject } from "../workspace/ResourceOverview";
import { prepareScale, revokeScale, type ScalePreview } from "./api";
import { canConfirmScale, scaleReviewOwner, validReplicaText } from "./scaleState";

export function ScaleImpact({ review }: { review: ScalePreview }) {
  return (
    <>
      <p>Autoscalers, GitOps and other controllers may overwrite the desired count.</p>
      {review.after < review.before && (
        <p className="warning">Pods may terminate. PodDisruptionBudgets do not establish protection during replica reduction.</p>
      )}
      {review.impact.whenScaled && review.after < review.before && (
        <>
          <p className="warning">Scale-down can delete claims under the StatefulSet’s retention policy.</p>
          <p>
            Claim retention when scaled: {review.impact.whenScaled}. When deleted: {review.impact.whenDeleted}.
            {review.impact.policyDefault && " Includes the default policy."}
          </p>
        </>
      )}
    </>
  );
}

export function ScaleDialog({
  source,
  cluster,
  onDismiss,
  onExecute,
}: {
  source: ResourceDetail;
  cluster: string;
  onDismiss: () => void;
  onExecute: (review: ScalePreview, acknowledgement: string) => void;
}) {
  const [target] = useState(() => ({
    resource: { ...source.resource, connection: { ...source.resource.connection } },
    sourceVersion: source.resourceVersion,
    cluster,
  }));
  const [initial] = useState(() => {
    const spec = overviewObject(source).spec as { replicas?: unknown } | undefined;
    return typeof spec?.replicas === "number" ? String(spec.replicas) : "";
  });
  const [count, setCount] = useState(initial);
  const [ack, setAck] = useState("");
  const [review, setReview] = useState<ScalePreview>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [expired, setExpired] = useState(false);
  const [owner] = useState(() => scaleReviewOwner(revokeScale));
  const { abandon } = owner;
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
  }, [review]);
  function change(value: string) {
    abandon();
    setCount(value);
    setAck("");
    setReview(undefined);
    setError("");
    setLoading(false);
    setExpired(false);
  }
  function prepare() {
    if (!validReplicaText(count) || !owner.canPrepare()) return;
    abandon();
    setLoading(true);
    setError("");
    setReview(undefined);
    setExpired(false);
    owner.prepare(
      prepareScale({ resource: target.resource, sourceVersion: target.sourceVersion, replicasText: count }),
      (result) => {
        setReview(result);
        setLoading(false);
      },
      (failure) => {
        setError(message(failure));
        setLoading(false);
      },
    );
  }
  function confirm() {
    if (!review || !canConfirmScale(review, count, ack, Date.now()) || !owner.claim(review.token)) return;
    onExecute(review, ack);
    onDismiss();
  }
  return (
    <Modal
      title="Scale workload"
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
      <p>Current desired replicas: {review?.before ?? (initial || "Not reported")}</p>
      <label className="scale-field">
        Desired replicas
        <input
          value={count}
          onChange={(e) => change(e.target.value)}
          inputMode="numeric"
          maxLength={128}
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="none"
          autoComplete="off"
        />
      </label>
      {count && !validReplicaText(count) && <p role="alert">Enter a whole replica count from 0 through 2147483647.</p>}
      {loading && <p role="status">Checking current identity and running server dry-run…</p>}
      {error && <p role="alert">{error}</p>}
      {review?.noChange && <p role="status">The workload already has this desired count.</p>}
      {review?.token && (
        <>
          <p>
            <strong>
              {review.before} → {review.after} desired replicas
            </strong>
          </p>
          <p>
            Server dry-run passed. {review.permission === "allowed" ? "Permission hint: allowed." : "Permission hint: unknown."} Kubernetes
            checks authorization again on submission.
          </p>
          <p className="muted">Review expires {new Date(review.expiresAt).toLocaleTimeString()}.</p>
          <ScaleImpact review={review} />
          {review.warnings?.length ? (
            <ul>
              {review.warnings.map((warning, i) => (
                <li key={i}>{warning}</li>
              ))}
            </ul>
          ) : null}
          {review.after === 0 && (
            <label className="scale-field">
              Type {target.resource.name} to confirm zero replicas
              <input
                value={ack}
                onChange={(e) => setAck(e.target.value)}
                spellCheck={false}
                autoCorrect="off"
                autoCapitalize="none"
                autoComplete="off"
              />
            </label>
          )}
        </>
      )}
      {expired && <p role="alert">This review expired. Review the change again.</p>}
      <div className="scale-actions">
        {(!review?.token || expired) && (
          <button className="primary" disabled={loading || !validReplicaText(count) || review?.noChange} onClick={prepare}>
            Review change
          </button>
        )}
        {review?.token && !expired && (
          <button className="primary" disabled={!canConfirmScale(review, count, ack, Date.now())} onClick={confirm}>
            Confirm scale
          </button>
        )}
      </div>
    </Modal>
  );
}
