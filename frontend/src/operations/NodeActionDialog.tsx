/*
Purpose: Review a captured Node scheduling change or graceful drain.
Used by: Node Overview's Actions menu.
Responsibilities: Display cross-namespace impact, wait settings, expiry and typed consent.
Important: Dismissing this review never cancels a confirmed native operation.
*/
import { useLayoutEffect, useEffect, useState } from "react";
import { Modal } from "../ui/ReviewDialog";
import { message } from "../connections/api";
import type { ResourceDetail } from "../workloads/model";
import { prepareNode, revokeNode, type NodePreview } from "./api";
import { reviewOwner } from "./reviewState";
export type NodeAction = "cordon" | "uncordon" | "drain";
export const nodeLabel = { cordon: "Cordon", uncordon: "Uncordon", drain: "Drain" };
export function NodeActionDialog({
  source,
  cluster,
  action,
  onDismiss,
  onExecute,
}: {
  source: ResourceDetail;
  cluster: string;
  action: NodeAction;
  onDismiss: () => void;
  onExecute: (token: string, ack: string) => void;
}) {
  const [owner] = useState(() => reviewOwner(revokeNode)),
    [wait, setWait] = useState(5),
    [ack, setAck] = useState(""),
    [review, setReview] = useState<NodePreview>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  useLayoutEffect(() => owner.abandon, [owner]);
  useEffect(() => {
    if (!review?.token) return;
    const t = setTimeout(
      () => {
        owner.abandon();
        setReview(undefined);
        setError("Review expired. Prepare a fresh review.");
      },
      Math.max(0, Date.parse(review.expiresAt) - Date.now()),
    );
    return () => clearTimeout(t);
  }, [review, owner]);
  function prepare() {
    setBusy(true);
    setError("");
    setAck("");
    setReview(undefined);
    owner.prepare(
      prepareNode({ resource: source.resource, sourceVersion: source.resourceVersion, action, waitPreset: wait }),
      (p) => {
        setReview(p);
        setBusy(false);
      },
      (e) => {
        setError(message(e));
        setBusy(false);
      },
    );
  }
  return (
    <Modal title={nodeLabel[action] + " Node"} onCancel={onDismiss}>
      <p className="frozen-target">
        {cluster} · Node {source.resource.name}
        {review?.environment && " · " + review.environment}
      </p>
      {action === "drain" ? (
        <>
          <p>
            Review Pods across all namespaces. The Node stays cordoned after drain, including after cancellation or partial progress.
            Returning it to service requires a separate Uncordon.
          </p>
          <p>
            Pod writable layers are temporary. Persistent volumes may restrict rescheduling. Eviction does not guarantee replacement
            readiness.
          </p>
          <label>
            Total wait{" "}
            <select
              value={wait}
              onChange={(e) => {
                owner.abandon();
                setWait(Number(e.target.value));
                setReview(undefined);
                setBusy(false);
                setAck("");
                setError("");
              }}
            >
              {[2, 5, 10].map((v) => (
                <option key={v} value={v}>
                  {v} minutes
                </option>
              ))}
            </select>
          </label>
        </>
      ) : (
        <p>
          {action === "cordon"
            ? "Prevent ordinary new scheduling. Existing Pods can continue running."
            : "Allow ordinary scheduling subject to the Node’s other constraints."}
        </p>
      )}
      {error && <p role="alert">{error}</p>}
      {review && (
        <>
          <p>
            Observed scheduling: {review.cordoned ? "cordoned" : "uncordoned"}.{review.noChange && " Already in the requested state."}
          </p>
          {action === "drain" && (
            <>
              <p>
                {review.rows?.length ?? 0} assigned Pods. A PodDisruptionBudget protects application availability and can delay eviction.
              </p>
              {Object.entries(review.counts ?? {}).map(([state, count]) => (
                <details key={state} open={state === "blocked"}>
                  <summary>
                    {state} ({count})
                  </summary>
                  <ul>
                    {review.rows
                      ?.filter((r) => r.state === state)
                      .map((r) => (
                        <li key={r.uid}>
                          {r.namespace}/{r.name} · {r.reason}
                        </li>
                      ))}
                  </ul>
                </details>
              ))}
            </>
          )}
          {review.token && review.typed && (
            <label>
              Type the Node name to confirm <input autoComplete="off" value={ack} onChange={(e) => setAck(e.target.value)} />
            </label>
          )}
        </>
      )}
      <div className="dialog-actions">
        <button onClick={onDismiss}>Cancel</button>
        {review?.token ? (
          <button
            disabled={review.typed && ack !== source.resource.name}
            onClick={() => {
              if (owner.claim(review.token)) onExecute(review.token, ack);
            }}
          >
            Confirm {nodeLabel[action]}
          </button>
        ) : (
          <button disabled={busy} onClick={prepare}>
            {busy ? "Preparing…" : "Prepare review"}
          </button>
        )}
      </div>
    </Modal>
  );
}
