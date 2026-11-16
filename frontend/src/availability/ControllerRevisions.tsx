/*
Purpose: Show one bounded page of owned workload history.
Used by: Deployment, StatefulSet and DaemonSet Revisions tabs.
Responsibilities: Select an exact source, keep inspection links and open a review for that row.
Important: Retention is not complete rollout or audit history.
*/
import { useId, useState } from "react";
import type { ResourceRef } from "../workloads/model";
import { evidenceTime } from "../workspace/ResourceOverview";
import { RollbackDialog } from "../operations/RollbackDialog";
import { rollbackSelectionKey } from "../operations/rollbackState";
import type { RollbackRequest } from "../operations/api";
import type { RolloutHistoryPage } from "./model";

export function ControllerRevisions({
  data,
  resource,
  context,
  canReview,
  onNext,
  onInspect,
  onConfirm,
}: {
  data: RolloutHistoryPage;
  resource: ResourceRef;
  context: string;
  canReview: boolean;
  onNext: (cursor: string) => void;
  onInspect: (target: ResourceRef) => void;
  onConfirm: (token: string) => void;
}) {
  const name = useId();
  const [selected, setSelected] = useState("");
  const [review, setReview] = useState<RollbackRequest>();
  const rows = data.items ?? [];
  const selection = rows.find((row) => row.resource.uid === selected);
  const request = selection && {
    resource,
    resourceVersion: data.sourceVersion,
    source: selection.resource,
    sourceVersion: selection.resourceVersion,
    revision: selection.revision,
  };
  const activeReview =
    canReview && review && request && rollbackSelectionKey(review) === rollbackSelectionKey(request) ? review : undefined;
  const showMarkers = resource.kind === "StatefulSet";
  return (
    <div className="evidence-section">
      <p className="muted">
        Retained owned revisions · workload resourceVersion {data.sourceVersion}.
        {!data.complete && !data.problem && " More results may be available."}
      </p>
      {data.problem ? (
        <p role="alert">{data.problem.message}</p>
      ) : (
        <>
          <p>
            {rows.length
              ? `${rows.length} revisions on this page`
              : data.complete
                ? "No retained owned revisions found."
                : "No revisions on this page."}
          </p>
          {!!rows.length && (
            <div className="evidence-table-wrap">
              <table className="evidence-table" aria-label="Owned revisions">
                <thead>
                  <tr>
                    <th>Revision</th>
                    <th>Source</th>
                    {showMarkers && <th>Captured markers</th>}
                    <th>Created</th>
                    <th>Images / change cause</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <tr key={row.resource.uid}>
                      <td>
                        <label className="revision-choice">
                          <input
                            type="radio"
                            name={name}
                            aria-label={`Select revision ${row.revision}: ${row.resource.name}`}
                            checked={selected === row.resource.uid}
                            onChange={() => {
                              setReview(undefined);
                              setSelected(row.resource.uid);
                            }}
                          />
                          {row.revision}
                        </label>
                      </td>
                      <td>
                        <button className="network-value" onClick={() => onInspect(row.resource)}>
                          {row.resource.name}
                        </button>
                      </td>
                      {showMarkers && (
                        <td>
                          {[
                            row.resource.name === data.currentRevision ? "Referenced by captured currentRevision" : "",
                            row.resource.name === data.updateRevision ? "Referenced by captured updateRevision" : "",
                          ]
                            .filter(Boolean)
                            .join(" · ") || "—"}
                        </td>
                      )}
                      <td>{evidenceTime(row.createdAt)}</td>
                      <td>{[...(row.images ?? []), row.changeCause].filter(Boolean).join(" · ") || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
      <div className="evidence-toolbar">
        {data.page.nextCursor && <button onClick={() => onNext(data.page.nextCursor)}>Load more revisions</button>}
        {!!rows.length && (
          <button
            disabled={!canReview || !request}
            onClick={(event) => {
              event.currentTarget.focus();
              setReview(request);
            }}
          >
            Roll back…
          </button>
        )}
      </div>
      {activeReview && (
        <RollbackDialog
          key={rollbackSelectionKey(activeReview)}
          request={activeReview}
          context={context}
          onClose={() => setReview(undefined)}
          onConfirm={(token) => {
            setReview(undefined);
            onConfirm(token);
          }}
        />
      )}
    </div>
  );
}
