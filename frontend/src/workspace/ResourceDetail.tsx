/*
Purpose: Present live workload investigation in the approved detail pane.
Used by: Workspace after explicit resource selection.
Responsibilities: Keep section failures independent and render cluster fields as inert text.
Important: Overview configuration is not measured usage and references never fetch Secret values.
*/
import type { ReactNode } from "react";
import type { WorkloadController } from "../workloads/useWorkloads";
import type {
  ResourceDetail as Detail,
  YAMLDocument,
  EventPage,
  RelatedPage,
  RelatedReplicaSetPage,
  RelatedJobPage,
  Section,
} from "../workloads/model";
import { isNetworkKind, type ServiceSlicePage } from "../networking/model";
import { NetworkOverview } from "../networking/NetworkOverview";
import { ServiceEndpointSlices } from "../networking/ServiceEndpointSlices";
import { ResourceYaml } from "./ResourceYaml";
import { ResourceEvents } from "./ResourceEvents";
import { DeploymentPods } from "./DeploymentPods";
import { RelatedJobs } from "./RelatedJobs";
import { ReplicaSetList } from "./ReplicaSetList";
import { ResourceOverview } from "./ResourceOverview";
export function ResourceDetail({
  controller: c,
  context,
  stale,
  logs,
}: {
  controller: WorkloadController;
  context: string;
  stale: boolean;
  logs?: ReactNode;
}) {
  const row = c.selection;
  if (!row) return null;
  const section = c.activeSection,
    state = c.sections[section],
    data = state?.data;
  const kind = row.resource.kind;
  const tabs: Section[] = ["Overview", "YAML"];
  if (kind !== "IngressClass") tabs.push("Events");
  if (kind === "Pod") tabs.push("Logs");
  else if (kind === "Service") tabs.push("EndpointSlices");
  else if (!isNetworkKind(kind)) {
    tabs.push(kind === "CronJob" ? "Jobs" : "Pods");
    if (kind === "Deployment") tabs.push("ReplicaSets");
  }
  return (
    <div className={section === "YAML" ? "compact-pod-detail yaml-detail" : "compact-pod-detail"}>
      {c.back && (
        <button onClick={c.backToResource}>
          ← Back to {c.back.resource.kind} {c.back.resource.name}
        </button>
      )}
      {c.navigationError && <p role="alert">{c.navigationError}</p>}
      <div className="pane-heading">
        <h2>{row.resource.name}</h2>
        {section !== "Logs" && (
          <button onClick={c.refreshSection} disabled={state?.loading}>
            Refresh section
          </button>
        )}
      </div>
      <p className="detail-identity">
        {context} / {kind === "IngressClass" ? "Cluster-scoped" : row.resource.namespace} / {row.resource.kind}
      </p>
      {c.inventory.identity && <p className="muted">{c.inventory.identity}</p>}
      {stale && <p className="warning">Retained evidence — connection status is stale.</p>}
      <div className="detail-tabs" role="tablist">
        {tabs.map((tab) => (
          <button key={tab} role="tab" aria-selected={tab === section} onClick={() => c.selectSection(tab)}>
            {tab}
          </button>
        ))}
      </div>
      {state?.loading && <p>Loading {section}…</p>}
      {state?.error && (
        <p role="alert">
          {state.error} {data ? "Retained section evidence may be stale." : ""}
        </p>
      )}
      {section === "Overview" &&
        data &&
        (isNetworkKind(kind) ? (
          <NetworkOverview data={data as Detail} onOpenReference={(target) => c.openReference(data as Detail, target)} />
        ) : (
          <ResourceOverview data={data as Detail} onOpenOwner={c.openRelated} />
        ))}
      {section === "EndpointSlices" && data && (
        <ServiceEndpointSlices
          data={data as ServiceSlicePage}
          onNext={c.nextSection}
          onSelect={(target) => c.openServiceSlice((data as ServiceSlicePage).sourceVersion, target)}
        />
      )}
      {section === "YAML" && data && (
        <ResourceYaml key={JSON.stringify([row.resource.uid, (data as YAMLDocument).fetchedAt])} data={data as YAMLDocument} />
      )}
      {section === "Events" && data && <ResourceEvents key={row.resource.uid} data={data as EventPage} onNext={c.nextSection} />}
      {section === "Pods" && data && (
        <DeploymentPods
          showOwner={row.resource.kind === "Deployment"}
          data={data as RelatedPage}
          onNext={c.nextSection}
          onSelect={c.openRelated}
        />
      )}
      {section === "ReplicaSets" && data && (
        <ReplicaSetList data={data as RelatedReplicaSetPage} onNext={c.nextSection} onSelect={c.openRelated} />
      )}
      {section === "Jobs" && data && <RelatedJobs data={data as RelatedJobPage} onNext={c.nextSection} onSelect={c.openRelated} />}
      {section === "Logs" && logs}
    </div>
  );
}
