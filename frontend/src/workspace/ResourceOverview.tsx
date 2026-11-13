/*
Purpose: Present structured workload overview evidence.
Used by: Workload investigation details.
Responsibilities: Show observed facts and expandable configuration.
Important: Cluster strings are inert and configuration does not measure usage.
*/
import type { ResourceDetail as Detail, ResourceRef } from "../workloads/model";
import { deploymentCounts } from "../workloads/model";
import { controllerOverview } from "./controllerOverview";
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

export function ResourceOverview({ data, onOpenOwner }: { data: Detail; onOpenOwner: (target: ResourceRef) => void }) {
  const root = overviewObject(data),
    meta = object(root.metadata),
    spec = object(root.spec),
    status = object(root.status);
  const pod = data.resource.kind === "Pod";
  const jobTemplate = object(object(spec.jobTemplate).spec);
  const templateObject = object(data.resource.kind === "CronJob" ? jobTemplate.template : spec.template);
  const template = object(templateObject.spec);
  const controller = controllerOverview(data, spec, status);
  const regular = list(spec.containers).map((container) =>
    object(list(status.containerStatuses).find((item) => object(item).name === object(container).name)),
  );
  const readinessKnown = regular.length > 0 && regular.every((item) => typeof item.ready === "boolean");
  const restartsKnown = regular.length > 0 && regular.every((item) => typeof item.restartCount === "number");
  const counts = deploymentCounts(status);
  const replicaSet = data.resource.kind === "ReplicaSet";
  const observed =
    Number(status.observedGeneration) > 0 ||
    list(status.conditions).length > 0 ||
    ["replicas", "readyReplicas", "availableReplicas", "fullyLabeledReplicas"].some((key) => Number(status[key]) > 0);
  const replicaCount = (key: string) => (observed ? (status[key] ?? 0) : undefined);
  const owners = list(meta.ownerReferences).map(object);
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
    : {
        "Desired generation": meta.generation,
        "Observed generation": status.observedGeneration,
        Created: evidenceTime(meta.creationTimestamp),
        ...(meta.deletionTimestamp ? { Deletion: evidenceTime(meta.deletionTimestamp) } : {}),
        ...controller?.facts,
      };
  return (
    <div className="evidence-section workload-overview">
      {pod && (
        <p>
          <PodPhase phase={String(status.phase ?? "Unknown")} />
          {meta.deletionTimestamp ? " · Terminating" : ""}
        </p>
      )}
      {!pod && meta.deletionTimestamp != null && <p>Terminating</p>}
      <div className="evidence-stats">
        <Facts
          values={
            controller?.summary ??
            (pod
              ? {
                  "Ready containers": readinessKnown
                    ? `${regular.filter((c) => c.ready === true).length} / ${list(spec.containers).length}`
                    : undefined,
                  Restarts: restartsKnown ? regular.reduce((total, c) => total + Number(c.restartCount), 0) : undefined,
                  QoS: status.qosClass,
                }
              : replicaSet
                ? {
                    Desired: spec.replicas,
                    Current: replicaCount("replicas"),
                    Ready: replicaCount("readyReplicas"),
                    Available: replicaCount("availableReplicas"),
                    ...(status.fullyLabeledReplicas !== undefined ? { "Fully labeled": status.fullyLabeledReplicas } : {}),
                  }
                : {
                    Desired: spec.replicas,
                    Updated: counts.updated,
                    Ready: counts.ready,
                    Available: counts.available,
                    Unavailable: counts.unavailable,
                  })
          }
        />
      </div>
      <Facts values={facts} />
      {data.resource.kind === "CronJob" && spec.suspend === true && (
        <p>Suspension prevents future scheduling. Existing Jobs can continue.</p>
      )}
      {controller && (
        <details>
          <summary>Controller configuration</summary>
          <Facts values={controller.configuration} />
        </details>
      )}
      {!pod && (
        <details>
          <summary>Pod template configuration</summary>
          <Facts
            values={{
              Metadata: templateObject.metadata,
              "Node selector": template.nodeSelector,
              Affinity: template.affinity,
              Tolerations: template.tolerations,
            }}
          />
        </details>
      )}
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
      {!pod && data.resource.kind !== "CronJob" && (
        <details>
          <summary>{data.resource.kind === "Deployment" ? "Selector and strategy" : "Selector"}</summary>
          <Facts values={{ Selector: spec.selector, ...(data.resource.kind === "Deployment" ? { Strategy: spec.strategy } : {}) }} />
        </details>
      )}
      <details>
        <summary>Volumes and owners</summary>
        <Facts values={{ Volumes: (pod ? spec : template).volumes }} />
        {owners.length ? (
          <ul>
            {owners.map((owner, i) => {
              const { kind, name, uid, apiVersion } = owner;
              const supported =
                (((kind === "ReplicaSet" || kind === "Deployment" || kind === "StatefulSet" || kind === "DaemonSet") &&
                  apiVersion === "apps/v1") ||
                  ((kind === "Job" || kind === "CronJob") && apiVersion === "batch/v1")) &&
                typeof name === "string" &&
                typeof uid === "string" &&
                !!uid;
              return (
                <li key={i}>
                  {supported ? (
                    <button onClick={() => onOpenOwner({ ...data.resource, kind, name, uid, apiVersion })}>
                      {kind} {name}
                    </button>
                  ) : (
                    `${text(kind)} ${text(name)}`
                  )}
                  <details>
                    <summary>Reference</summary>
                    <Facts values={owner} />
                  </details>
                </li>
              );
            })}
          </ul>
        ) : (
          <p>No owner references.</p>
        )}
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
