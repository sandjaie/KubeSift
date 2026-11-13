/*
Purpose: Present captured namespace policy and configuration facts.
Used by: The shared resource detail pane.
Responsibilities: Show exact reported quantities, independent limit items and safe Secret identity.
Important: Policy configuration does not predict admission or measure consumption.
*/
import type { ReactNode } from "react";
import type { ResourceDetail } from "../workloads/model";
import { evidenceTime, overviewObject } from "../workspace/ResourceOverview";
import { scopeSummary, type ConfigurationCapture, type NamespaceDestination, type Quantities } from "./model";

function Facts({ items }: { items: [string, ReactNode][] }) {
  return (
    <dl className="pod-facts">
      {items.map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd className="network-value">{value ?? "Not reported"}</dd>
        </div>
      ))}
    </dl>
  );
}
function Entries({ title, values }: { title: string; values?: Record<string, string> }) {
  return (
    <details>
      <summary>{title}</summary>
      {Object.keys(values ?? {}).length ? (
        <Facts items={Object.entries(values ?? {}).sort(([a], [b]) => a.localeCompare(b))} />
      ) : (
        <p>None reported.</p>
      )}
    </details>
  );
}
function QuantitiesTable({ columns, maps }: { columns: string[]; maps: (Quantities | undefined)[] }) {
  const keys = [...new Set(maps.flatMap((map) => Object.keys(map ?? {})))].sort();
  if (!keys.length) return <p>No resource quantities reported.</p>;
  return (
    <div className="evidence-table-wrap">
      <table className="evidence-table">
        <thead>
          <tr>
            <th>Resource</th>
            {columns.map((name) => (
              <th key={name}>{name}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {keys.map((key) => (
            <tr key={key}>
              <td className="network-value">{key}</td>
              {maps.map((map, index) => (
                <td className="network-value" key={columns[index]}>
                  {map && Object.prototype.hasOwnProperty.call(map, key) ? map[key] : "Not reported"}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
export function ConfigurationOverview({
  data,
  onBrowseNamespace,
}: {
  data: ResourceDetail;
  onBrowseNamespace: (kind: NamespaceDestination) => void;
}) {
  const { resource } = data;
  const o = overviewObject(data) as ConfigurationCapture;
  if (resource.kind === "Secret")
    return (
      <div className="evidence-section workload-overview">
        <p>Only identity metadata is requested. Secret contents are not available in this phase.</p>
        <Facts
          items={[
            ["Name", resource.name],
            ["Namespace", resource.namespace],
            ["UID", resource.uid],
            ["Resource version", data.resourceVersion || undefined],
            ["Created", evidenceTime(o.creationTimestamp)],
          ]}
        />
      </div>
    );
  return (
    <div className="evidence-section workload-overview">
      <p className="muted">
        Captured {evidenceTime(data.fetchedAt)} · Resource version {data.resourceVersion || "Unknown"}
      </p>
      {resource.kind === "Namespace" && (
        <>
          <Facts
            items={[
              ["Phase", o.status?.phase || "Unknown"],
              ["Created", evidenceTime(o.metadata?.creationTimestamp)],
              ["Finalizers", o.spec?.finalizers?.join(", ") || "None reported"],
            ]}
          />
          <div className="inventory-pagination">
            <button onClick={() => onBrowseNamespace("ConfigMap")}>Browse namespace</button>
            <button onClick={() => onBrowseNamespace("ResourceQuota")}>ResourceQuotas</button>
            <button onClick={() => onBrowseNamespace("LimitRange")}>LimitRanges</button>
          </div>
          <h3>Conditions</h3>
          {o.status?.conditions?.length ? (
            o.status.conditions.map((c, index) => (
              <section key={index}>
                <h4>{c.type}</h4>
                <Facts
                  items={[
                    ["Status", c.status],
                    ["Reason", c.reason],
                    ["Message", c.message],
                    ["Last transition", evidenceTime(c.lastTransitionTime)],
                  ]}
                />
              </section>
            ))
          ) : (
            <p>No conditions reported.</p>
          )}
        </>
      )}
      {resource.kind === "ResourceQuota" && (
        <>
          <h3>Resource quota</h3>
          <p className="network-value">Scopes: {scopeSummary(o.spec?.scopes, o.spec?.scopeSelector)}</p>
          <QuantitiesTable
            columns={["Configured hard", "Reported hard", "Reported used"]}
            maps={[o.spec?.hard, o.status?.hard, o.status?.used]}
          />
          <p className="muted">Reported use is quota accounting. It does not measure CPU or memory consumption.</p>
        </>
      )}
      {resource.kind === "LimitRange" && (
        <>
          <h3>Limit range</h3>
          {o.spec?.limits?.length ? (
            o.spec.limits.map((item, index) => (
              <section key={index}>
                <h4>{item.type || "Unknown item type"}</h4>
                <QuantitiesTable
                  columns={["Min", "Max", "Default limit", "Default request", "Max limit/request ratio"]}
                  maps={[item.min, item.max, item.default, item.defaultRequest, item.maxLimitRequestRatio]}
                />
              </section>
            ))
          ) : (
            <p>No limit items reported.</p>
          )}
          <p className="muted">These constraints and defaults describe configuration for admission. Existing Pods may differ.</p>
        </>
      )}
      {resource.kind === "ConfigMap" && (
        <Facts
          items={[
            ["UID", resource.uid],
            ["Created", evidenceTime(o.metadata?.creationTimestamp)],
            ["Immutable", o.immutable == null ? undefined : String(o.immutable)],
            ["Text keys", o.textKeys],
            ["Binary keys", o.binaryKeys],
          ]}
        />
      )}
      <Entries title="Labels" values={o.metadata?.labels} />
      <Entries title="Annotations" values={o.metadata?.annotations} />
    </div>
  );
}
