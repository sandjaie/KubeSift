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
      <p>
        Captured {data.page.fetchedAt} · {data.complete ? "Traversal complete" : "Partial relationship coverage"}
      </p>
      {data.problem && <p role="alert">{data.problem.message}</p>}
      {!data.items?.length && (
        <p>{data.complete ? "No owned Pods on this page." : "No owned Pods on this page; more evidence may remain."}</p>
      )}
      {data.items?.map((row) => (
        <p key={row.pod.resource.uid}>
          <button onClick={() => onSelect(row.pod)}>{row.pod.resource.name}</button> · ReplicaSet {row.replicaSet.name} ·{" "}
          <PodPhase phase={row.pod.phase} /> · {row.pod.ready ?? "?"}/{row.pod.desired ?? "?"}
        </p>
      ))}
      {data.page.nextCursor && <button onClick={() => onNext(data.page.nextCursor)}>Load next relationship page</button>}
    </div>
  );
}
