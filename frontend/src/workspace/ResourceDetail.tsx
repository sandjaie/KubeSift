/*
Purpose: Present live workload investigation in the approved detail pane.
Used by: Workspace after explicit resource selection.
Responsibilities: Keep section failures independent and render cluster fields as inert text.
Important: Overview configuration is not measured usage and references never fetch Secret values.
*/
import type { ReactNode } from "react";
import type { WorkloadController } from "../workloads/useWorkloads";
import type { ResourceDetail as Detail, YAMLDocument, EventPage, RelatedPage, Section } from "../workloads/model";
import { deploymentCounts } from "../workloads/model";
import { ResourceYaml } from "./ResourceYaml";
import { ResourceEvents } from "./ResourceEvents";
import { DeploymentPods } from "./DeploymentPods";
import { PodPhase } from "./PodPhase";
const object = (v: unknown): Record<string, unknown> =>
  v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
const list = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
const text = (v: unknown) => (v === undefined || v === null ? "Unknown" : typeof v === "object" ? JSON.stringify(v, null, 2) : String(v));
function Facts({ values }: { values: Record<string, unknown> }) {
  return (
    <dl className="pod-facts">
      {Object.entries(values).map(([k, v]) => (
        <div key={k}>
          <dt>{k}</dt>
          <dd>
            <pre className="fact-text">{text(v)}</pre>
          </dd>
        </div>
      ))}
    </dl>
  );
}
export function overviewObject(data: Detail | undefined) {
  try {
    return object(JSON.parse(data?.objectJSON ?? "{}"));
  } catch {
    return {};
  }
}
function Overview({ data }: { data: Detail }) {
  const root = overviewObject(data),
    meta = object(root.metadata),
    spec = object(root.spec),
    status = object(root.status);
  const pod = data.resource.kind === "Pod";
  const counts = deploymentCounts(status);
  const template = object(object(spec.template).spec);
  return (
    <div className="evidence-section">
      <p>
        Captured {data.fetchedAt} · resourceVersion {data.resourceVersion || "Unknown"}
      </p>
      {pod ? (
        <>
          <PodPhase phase={String(status.phase ?? "Unknown")} />
          <Facts
            values={{
              Lifecycle: meta.deletionTimestamp ? "Terminating" : "No deletion requested",
              Node: spec.nodeName,
              "Pod IPs": status.podIPs,
              QoS: status.qosClass,
              "Start time": status.startTime,
              Created: meta.creationTimestamp,
              "Deletion time": meta.deletionTimestamp,
              Conditions: status.conditions,
            }}
          />
          {[
            ["Regular containers", "containers", "containerStatuses"],
            ["Init containers", "initContainers", "initContainerStatuses"],
            ["Ephemeral containers", "ephemeralContainers", "ephemeralContainerStatuses"],
          ].map(([label, field, statusField]) => (
            <section key={field}>
              <h3>{label}</h3>
              {list(spec[field]).length ? (
                list(spec[field]).map((c, index) => {
                  const config = object(c),
                    s = object(list(status[statusField]).find((v) => object(v).name === config.name));
                  return (
                    <details key={index} open>
                      <summary>
                        {text(config.name)} · {text(config.image)}
                      </summary>
                      <Facts
                        values={{
                          Ready: s.ready,
                          Started: s.started,
                          Restarts: s.restartCount,
                          "Current state": s.state,
                          "Last termination": s.lastState,
                          "Requests / limits": config.resources,
                          "Readiness probe": config.readinessProbe,
                          "Liveness probe": config.livenessProbe,
                          "Startup probe": config.startupProbe,
                        }}
                      />
                    </details>
                  );
                })
              ) : (
                <p className="muted">None configured.</p>
              )}
            </section>
          ))}
        </>
      ) : (
        <Facts
          values={{
            Desired: spec.replicas,
            Updated: counts.updated,
            Ready: counts.ready,
            Available: counts.available,
            Unavailable: counts.unavailable,
            "Desired generation": meta.generation,
            "Observed generation": status.observedGeneration,
            Conditions: status.conditions,
            Selector: spec.selector,
            Strategy: spec.strategy,
            "Template containers (configuration)": template.containers,
          }}
        />
      )}
      <details>
        <summary>Volumes and owner references (configuration)</summary>
        <Facts values={{ Volumes: pod ? spec.volumes : template.volumes, Owners: meta.ownerReferences }} />
      </details>
      <details>
        <summary>Labels</summary>
        <pre>{text(meta.labels)}</pre>
      </details>
      <details>
        <summary>Annotations</summary>
        <pre>{text(meta.annotations)}</pre>
      </details>
    </div>
  );
}
export function ResourceDetail({
  controller: c,
  context,
  source,
  stale,
  logs,
}: {
  controller: WorkloadController;
  context: string;
  source: string;
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
      {section === "Overview" && data && <Overview data={data as Detail} />}
      {section === "YAML" && data && <ResourceYaml key={row.resource.uid} data={data as YAMLDocument} />}
      {section === "Events" && data && <ResourceEvents data={data as EventPage} onNext={c.nextSection} />}
      {section === "Pods" && data && <DeploymentPods data={data as RelatedPage} onNext={c.nextSection} onSelect={c.openPod} />}
      {section === "Logs" && logs}
      <details className="pod-identity-disclosure">
        <summary>Identity &amp; source</summary>
        <Facts values={{ UID: row.resource.uid, Source: source, "Connection generation": row.resource.connection.generation }} />
      </details>
    </div>
  );
}
