/*
Purpose: Present structured workload overview evidence.
Used by: Workload investigation details.
Responsibilities: Show observed facts and expandable configuration.
Important: Cluster strings are inert and configuration does not measure usage.
*/
import type { ResourceDetail as Detail } from "../workloads/model";
import { deploymentCounts } from "../workloads/model";
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
export function evidenceTime(value: unknown) {
  const date = new Date(typeof value === "string" ? value : "");
  return Number.isNaN(date.getTime()) ? "Unknown" : date.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "long" });
}

export function ResourceOverview({ data }: { data: Detail }) {
  const root = overviewObject(data),
    meta = object(root.metadata),
    spec = object(root.spec),
    status = object(root.status);
  const pod = data.resource.kind === "Pod";
  const template = object(object(spec.template).spec);
  const regular = list(status.containerStatuses).map(object);
  const counts = deploymentCounts(status);
  const conditions = list(status.conditions).map(object);
  const facts = pod
    ? {
        Node: spec.nodeName,
        "Pod IPs":
          list(status.podIPs)
            .map((ip) => object(ip).ip)
            .join(", ") || status.podIP,
        Created: evidenceTime(meta.creationTimestamp),
        Started: evidenceTime(status.startTime),
        ...(meta.deletionTimestamp ? { Deletion: evidenceTime(meta.deletionTimestamp) } : {}),
      }
    : { "Desired generation": meta.generation, "Observed generation": status.observedGeneration };
  return (
    <div className="evidence-section workload-overview">
      {pod && (
        <p>
          <PodPhase phase={String(status.phase ?? "Unknown")} />
          {meta.deletionTimestamp ? " · Terminating" : ""}
        </p>
      )}
      <div className="evidence-stats">
        <Facts
          values={
            pod
              ? {
                  "Ready containers": regular.length
                    ? `${regular.filter((c) => c.ready === true).length} / ${list(spec.containers).length}`
                    : undefined,
                  Restarts: regular.length ? regular.reduce((total, c) => total + Number(c.restartCount ?? 0), 0) : undefined,
                  QoS: status.qosClass,
                }
              : {
                  Desired: spec.replicas,
                  Updated: counts.updated,
                  Ready: counts.ready,
                  Available: counts.available,
                  Unavailable: counts.unavailable,
                }
          }
        />
      </div>
      <Facts values={facts} />
      <h3>Conditions</h3>
      {conditions.length ? (
        <div className="evidence-table-wrap">
          <table className="evidence-table">
            <thead>
              <tr>
                <th>Type</th>
                <th>Status</th>
                <th>Reason</th>
                <th>Message</th>
              </tr>
            </thead>
            <tbody>
              {conditions.map((condition, i) => (
                <tr key={i}>
                  <td>{text(condition.type)}</td>
                  <td>{text(condition.status)}</td>
                  <td>{text(condition.reason)}</td>
                  <td>
                    <details>
                      <summary>{text(condition.message).slice(0, 120)}</summary>
                      <Facts
                        values={{ Message: condition.message, Transition: condition.lastTransitionTime, Update: condition.lastUpdateTime }}
                      />
                    </details>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="muted">No conditions reported.</p>
      )}
      {[
        ["Regular containers", "containers", "containerStatuses"],
        ["Init containers", "initContainers", "initContainerStatuses"],
        ["Ephemeral containers", "ephemeralContainers", "ephemeralContainerStatuses"],
      ].map(([label, field, statusField]) => {
        const containers = list((pod ? spec : template)[field]).map(object);
        return (
          <section key={field}>
            <h3>
              {label}
              {pod ? "" : " · template"}
            </h3>
            {containers.length ? (
              <div className="evidence-table-wrap">
                <table className="evidence-table">
                  <thead>
                    <tr>
                      <th>Container</th>
                      {pod && (
                        <>
                          <th>State</th>
                          <th>Ready</th>
                          <th>Restarts</th>
                        </>
                      )}
                    </tr>
                  </thead>
                  <tbody>
                    {containers.map((config, i) => {
                      const observation = object(list(status[statusField]).find((value) => object(value).name === config.name));
                      const state = object(observation.state);
                      const stateName = Object.keys(state)[0];
                      return (
                        <tr key={i}>
                          <td>
                            <details>
                              <summary>
                                {text(config.name)} <span className="muted">{text(config.image)}</span>
                              </summary>
                              <Facts
                                values={{
                                  Image: config.image,
                                  ...(pod
                                    ? {
                                        Started: observation.started,
                                        "Current state": observation.state,
                                        "Last termination": observation.lastState,
                                      }
                                    : {}),
                                  "Requests / limits (configuration)": config.resources ?? "Not configured",
                                  "Readiness probe": config.readinessProbe ?? "Not configured",
                                  "Liveness probe": config.livenessProbe ?? "Not configured",
                                  "Startup probe": config.startupProbe ?? "Not configured",
                                }}
                              />
                            </details>
                          </td>
                          {pod && (
                            <>
                              <td>
                                {text(stateName)} {String(object(state[stateName]).reason ?? "")}
                              </td>
                              <td>{text(observation.ready)}</td>
                              <td>{text(observation.restartCount)}</td>
                            </>
                          )}
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="muted">None configured.</p>
            )}
          </section>
        );
      })}
      {!pod && (
        <details>
          <summary>Selector and strategy</summary>
          <Facts values={{ Selector: spec.selector, Strategy: spec.strategy }} />
        </details>
      )}
      <details>
        <summary>Volumes and owners</summary>
        <Facts values={{ Volumes: (pod ? spec : template).volumes, Owners: meta.ownerReferences }} />
      </details>
      <details>
        <summary>Labels</summary>
        <pre>{text(meta.labels)}</pre>
      </details>
      <details>
        <summary>Annotations</summary>
        <pre>{text(meta.annotations)}</pre>
      </details>
      <details className="muted">
        <summary>
          Captured {evidenceTime(data.fetchedAt)} · resourceVersion {data.resourceVersion || "Unknown"}
        </summary>
        <Facts
          values={{
            Captured: data.fetchedAt,
            Created: meta.creationTimestamp,
            Started: status.startTime,
            Deletion: meta.deletionTimestamp,
          }}
        />
      </details>
    </div>
  );
}
