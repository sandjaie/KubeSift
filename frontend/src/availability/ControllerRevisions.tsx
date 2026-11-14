/*
Purpose: Show one bounded page of controller-owned revisions.
Used by: StatefulSet and DaemonSet Revisions tabs.
Responsibilities: Preserve exact revision text and captured StatefulSet markers.
Important: Retention is not complete rollout or audit history.
*/
import type { ResourceRef } from "../workloads/model";
import { evidenceTime } from "../workspace/ResourceOverview";
import type { ControllerRevisionPage } from "./model";

export function ControllerRevisions({
  data,
  onNext,
  onSelect,
}: {
  data: ControllerRevisionPage;
  onNext: (cursor: string) => void;
  onSelect: (target: ResourceRef) => void;
}) {
  const rows = data.items ?? [];
  return (
    <div className="evidence-section">
      <p className="muted">
        Retained owned revisions · source resourceVersion {data.sourceVersion}. Markers reflect captured StatefulSet status.
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
                : "No revisions on this page. More results may be available."}
          </p>
          {!!rows.length && (
            <div className="evidence-table-wrap">
              <table className="evidence-table" aria-label="Owned revisions">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Revision</th>
                    <th>Captured markers</th>
                    <th>Created</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <tr key={row.resource.uid}>
                      <td>
                        <button className="network-value" onClick={() => onSelect(row.resource)}>
                          {row.resource.name}
                        </button>
                      </td>
                      <td>{row.summary.revision || "Not reported"}</td>
                      <td>
                        {[
                          row.resource.name === data.currentRevision ? "Referenced by captured currentRevision" : "",
                          row.resource.name === data.updateRevision ? "Referenced by captured updateRevision" : "",
                        ]
                          .filter(Boolean)
                          .join(" · ") || "—"}
                      </td>
                      <td>{evidenceTime(row.createdAt)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
      {data.page.nextCursor && <button onClick={() => onNext(data.page.nextCursor)}>Load more revisions</button>}
    </div>
  );
}
