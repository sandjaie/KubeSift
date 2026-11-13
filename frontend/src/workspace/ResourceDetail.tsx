/*
Purpose: Present live workload investigation in the approved detail pane.
Used by: Workspace after explicit resource selection.
Responsibilities: Keep section failures independent and render cluster fields as inert text.
Important: Overview configuration is not measured usage and references never fetch Secret values.
*/
import type { ReactNode } from "react";
import type { WorkloadController } from "../workloads/useWorkloads";
import type { ResourceDetail as Detail, YAMLDocument, EventPage, RelatedPage, Section } from "../workloads/model";
import { ResourceYaml } from "./ResourceYaml";
import { ResourceEvents } from "./ResourceEvents";
import { DeploymentPods } from "./DeploymentPods";
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
  const tabs: Section[] = row.resource.kind === "Pod" ? ["Overview", "YAML", "Events", "Logs"] : ["Overview", "YAML", "Events", "Pods"];
  return (
    <div className="compact-pod-detail">
      {c.back && row.resource.kind === "Pod" && <button onClick={c.backToDeployment}>← Back to Deployment {c.back.resource.name}</button>}
      <div className="pane-heading">
        <h2>{row.resource.name}</h2>
        {section !== "Logs" && (
          <button onClick={c.refreshSection} disabled={state?.loading}>
            Refresh section
          </button>
        )}
      </div>
      <p className="detail-identity">
        {context} / {row.resource.namespace} / {row.resource.kind}
      </p>
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
      {section === "Overview" && data && <ResourceOverview data={data as Detail} />}
      {section === "YAML" && data && (
        <ResourceYaml key={JSON.stringify([row.resource.uid, (data as YAMLDocument).fetchedAt])} data={data as YAMLDocument} />
      )}
      {section === "Events" && data && <ResourceEvents key={row.resource.uid} data={data as EventPage} onNext={c.nextSection} />}
      {section === "Pods" && data && <DeploymentPods data={data as RelatedPage} onNext={c.nextSection} onSelect={c.openPod} />}
      {section === "Logs" && logs}
    </div>
  );
}
