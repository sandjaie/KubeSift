/*
Purpose: Display one bounded page of Deployment-owned ReplicaSets.
Used by: The Deployment ReplicaSets tab.
Responsibilities: Preserve unknown and zero counts and provide explicit relationship links.
Important: Rows carry captured UIDs and do not imply rollout history.
*/
import type { RelatedReplicaSetPage, ResourceRef } from "../workloads/model";
import { age } from "./ResourceTable";
export function ReplicaSetList({
  data,
  onNext,
  onSelect,
}: {
  data: RelatedReplicaSetPage;
  onNext: (cursor: string) => void;
  onSelect: (resource: ResourceRef) => void;
}) {
  const rows = data.items ?? [];
  return (
    <div className="evidence-section">
      {data.problem && <p role="alert">{data.problem.message}</p>}
      {!data.problem && !rows.length && (
        <p>{data.complete ? "No owned ReplicaSets found." : "No ReplicaSets in this batch. More results may be available."}</p>
      )}
      {!!rows.length && (
        <>
          <p className="muted">{rows.length} shown</p>
          <div className="evidence-table-wrap">
            <table className="evidence-table">
              <thead>
                <tr>
                  <th>ReplicaSet</th>
                  <th>Desired</th>
                  <th>Current</th>
                  <th>Ready</th>
                  <th>Available</th>
                  <th>Age</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.resource.uid}>
                    <td>
                      <button onClick={() => onSelect(row.resource)}>{row.resource.name}</button>
                    </td>
                    <td>{row.desired ?? "Unknown"}</td>
                    <td>{row.current ?? "Unknown"}</td>
                    <td>{row.ready ?? "Unknown"}</td>
                    <td>{row.available ?? "Unknown"}</td>
                    <td>{age(row.createdAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
      {data.page.nextCursor && <button onClick={() => onNext(data.page.nextCursor)}>Load more ReplicaSets</button>}
    </div>
  );
}
