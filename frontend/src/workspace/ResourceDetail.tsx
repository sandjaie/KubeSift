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
} from "../workloads/model";
import { isClusterScoped, resourceSections } from "../workloads/model";
import { isConfigurationKind } from "../configuration/model";
import { ConfigurationOverview } from "../configuration/ConfigurationOverview";
import { ConfigMapData } from "../configuration/ConfigMapData";
import { PodConfiguration } from "../configuration/PodConfiguration";
import { isNetworkKind, type ServiceSlicePage } from "../networking/model";
import { isStorageKind } from "../storage/model";
import { StorageOverview } from "../storage/StorageOverview";
import { PodStorage } from "../storage/PodStorage";
import { isAccessControlKind } from "../access-control/model";
import { AccessControlOverview } from "../access-control/AccessControlOverview";
import { PodIdentity } from "../access-control/PodIdentity";
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
  const tabs = resourceSections(kind);
  const link = c.inventory.referenceEvidence;
  const capturedLink =
    link &&
    section === "Overview" &&
    !state?.loading &&
    !state?.error &&
    (data as Detail | undefined)?.resourceVersion === link.targetVersion;
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
        {context} / {isClusterScoped(kind) ? "Cluster-scoped" : row.resource.namespace} / {row.resource.kind}
      </p>
      {c.inventory.identity && <p className="muted">{c.inventory.identity}</p>}
      {link && (
        <div className="muted" title={`Source version ${link.sourceVersion}; target version ${link.targetVersion}`}>
          <p>{link.identityEvidence === "uid-verified" ? "Referenced UID verified" : "Resolved by name at open time"}</p>
          {capturedLink && link.bindingEvidence && link.bindingEvidence !== "not-applicable" && (
            <p>
              {link.bindingEvidence === "references-agree" ? "Binding references agree" : "Reference only; binding unconfirmed"} · Observed
              in separate reads.
            </p>
          )}
        </div>
      )}
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
        (isAccessControlKind(kind) ? (
          <AccessControlOverview data={data as Detail} onOpenReference={(target) => c.openAccessControlReference(data as Detail, target)} />
        ) : isStorageKind(kind) ? (
          <StorageOverview data={data as Detail} onOpenReference={(target) => c.openStorageReference(data as Detail, target)} />
        ) : isNetworkKind(kind) ? (
          <NetworkOverview data={data as Detail} onOpenReference={(target) => c.openReference(data as Detail, target)} />
        ) : isConfigurationKind(kind) ? (
          <ConfigurationOverview
            data={data as Detail}
            onBrowseNamespace={(destination) => c.browseNamespace(data as Detail, destination)}
          />
        ) : (
          <>
            <ResourceOverview data={data as Detail} onOpenOwner={c.openRelated} />
            {kind === "Pod" && (
              <>
                <PodIdentity data={data as Detail} onOpenReference={(target) => c.openAccessControlReference(data as Detail, target)} />
                <PodStorage data={data as Detail} onOpenReference={(target) => c.openStorageReference(data as Detail, target)} />
                <PodConfiguration data={data as Detail} onOpen={(target) => c.openConfigurationReference(data as Detail, target)} />
              </>
            )}
          </>
        ))}
      {section === "Data" && kind === "ConfigMap" && data && (
        <ConfigMapData key={JSON.stringify([row.resource.uid, (data as Detail).resourceVersion])} data={data as Detail} />
      )}
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
