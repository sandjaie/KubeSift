/*
Purpose: Present only fields returned by the basic live Pod reader.
Used by: Workspace after explicit Pod selection.
Responsibilities: Show context, namespace, UID, phase, readiness and container names.
Important: YAML, logs, events and operational actions remain unavailable.
*/
import type { PodSummary } from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
import { PodPhase } from "./PodPhase";
import { ResourceIcon } from "../ui/ResourceIcon";
export function ResourceDetail({ pod, context, source, stale }: { pod: PodSummary; context: string; source: string; stale: boolean }) {
  return (
    <div className="compact-pod-detail">
      <div className="pane-heading">
        <h2>
          <ResourceIcon kind="Pod" /> {pod.name}
        </h2>
      </div>
      <p className="detail-identity">
        {context} / {pod.namespace}
      </p>
      {stale && <p className="warning detail-identity">Retained data — connection evidence is stale or unavailable.</p>}
      <dl className="pod-facts">
        <div>
          <dt>Phase</dt>
          <dd>
            <PodPhase phase={pod.phase} />
          </dd>
        </div>
        {Object.entries({
          Readiness: pod.ready ? "Ready" : "Not ready",
          Containers: pod.containers?.join(", ") || "None",
          Marker: pod.marker || "None",
        }).map(([name, value]) => (
          <div key={name}>
            <dt>{name}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <details key={pod.uid} className="pod-identity-disclosure">
        <summary>Identity &amp; source</summary>
        <dl>
          <div>
            <dt>UID</dt>
            <dd>{pod.uid}</dd>
          </div>
          <div>
            <dt>Source</dt>
            <dd>{source}</dd>
          </div>
        </dl>
      </details>
      <p className="detail-identity">More resource details are not available yet.</p>
    </div>
  );
}
