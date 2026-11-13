/*
Purpose: Show bounded Deployment-owned Pod evidence.
Used by: The Deployment Pods section.
Responsibilities: Disclose partial coverage and open actual Pod identities.
Important: Empty intermediate pages do not imply the Deployment has no Pods.
*/
import type { RelatedPage, WorkloadRow } from "../workloads/model";
import { PodPhase } from "./PodPhase";
export function DeploymentPods({
  data,
  onNext,
  onSelect,
}: {
  data: RelatedPage;
  onNext: (cursor: string) => void;
  onSelect: (row: WorkloadRow) => void;
}) {
  return (
    <div className="evidence-section">
      {data.problem && <p role="alert">{data.problem.message}</p>}
      {!data.problem && !data.items?.length && (
        <p>{data.complete ? "No owned Pods found." : "No Pods found in this batch. More results may be available."}</p>
      )}
      {!!data.items?.length && (
        <>
          <p className="muted">{data.items.length} shown</p>
          <div className="evidence-table-wrap">
            <table className="evidence-table">
              <thead>
                <tr>
                  <th>Pod</th>
                  <th>Phase</th>
                  <th>Ready</th>
                  <th>Restarts</th>
                  <th>ReplicaSet</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map(({ pod, replicaSet }) => (
                  <tr key={pod.resource.uid}>
                    <td>
                      <button onClick={() => onSelect(pod)}>{pod.resource.name}</button>
                    </td>
                    <td>
                      <PodPhase phase={pod.phase} />
                      {pod.reason && <div>{pod.reason}</div>}
                    </td>
                    <td>
                      {pod.ready ?? "?"}/{pod.desired ?? "?"}
                    </td>
                    <td>{pod.restarts ?? "Unknown"}</td>
                    <td>{replicaSet.name}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
      {data.page.nextCursor && <button onClick={() => onNext(data.page.nextCursor)}>Load more Pods</button>}
    </div>
  );
}
