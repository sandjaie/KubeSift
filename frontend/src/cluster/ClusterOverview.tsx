/*
Purpose: Present captured infrastructure observations and explicit navigation intents.
Used by: Cluster and Extensions details.
Responsibilities: Show optional evidence without inferring usage, leadership or read access.
Important: Large evidence is disclosed as inert text with bounded rendering.
*/
import { useState } from "react";
import { Disclosure, Field } from "../access-control/PolicyRules";
import { EvidencePage } from "../storage/StorageOverview";
import { evidenceTime, overviewObject } from "../workspace/ResourceOverview";
import type { ResourceDetail } from "../workloads/model";
import type { CatalogIntent } from "./model";

type RecordValue = Record<string, unknown>;
const obj = (v: unknown): RecordValue => (v && typeof v === "object" && !Array.isArray(v) ? (v as RecordValue) : {});
const list = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
const value = (v: unknown): string =>
  v == null || v === "" ? "Not reported" : typeof v === "object" ? JSON.stringify(v, null, 2) : String(v);
const text = (v: unknown): string => (typeof v === "string" ? v : "");
const condition = (status: RecordValue, type: string) =>
  text(obj(list(status.conditions).find((r) => obj(r).type === type)).status) || "Unknown";
function Entries({ values }: { values: RecordValue }) {
  return (
    <dl className="pod-facts">
      {Object.entries(values).map(([label, item]) => (
        <Field key={label} label={label}>
          <pre className="fact-text">{value(item)}</pre>
        </Field>
      ))}
    </dl>
  );
}
function Evidence({ label, data }: { label: string; data: unknown }) {
  return (
    <Disclosure label={label}>
      <textarea className="yaml-view" aria-label={label} readOnly spellCheck={false} value={value(data)} />
    </Disclosure>
  );
}
export function ClusterOverview({
  data,
  onTarget,
  onBrowse,
}: {
  data: ResourceDetail;
  onTarget: () => void;
  onBrowse: (intent: CatalogIntent) => void;
}) {
  const body = overviewObject(data),
    spec = obj(body.spec),
    status = obj(body.status);
  const [version, setVersion] = useState("");
  const kind = data.resource.kind;
  const names = obj(spec.names),
    regarding = obj(body.regarding ?? body.involvedObject),
    series = obj(body.series);
  const versions = list(spec.versions).map(obj);
  const availableVersions = versions.filter((v) => v.served === true && text(v.name));
  const selectedVersion = availableVersions.some((v) => v.name === version)
    ? version
    : availableVersions.length === 1
      ? text(availableVersions[0].name)
      : "";
  const canBrowse = !!(
    text(spec.group) &&
    selectedVersion &&
    text(names.plural) &&
    text(names.kind) &&
    (spec.scope === "Namespaced" || spec.scope === "Cluster")
  );
  const counts = series.count ?? body.deprecatedCount ?? body.count;
  const canOpen = !!data.eventTarget;
  const details =
    kind === "Node" ? (
      <>
        <Entries
          values={{
            Ready: condition(status, "Ready"),
            "Scheduling disabled": spec.unschedulable,
            "Kubelet version": obj(status.nodeInfo).kubeletVersion,
          }}
        />
        <p className="muted">Capacity and allocatable are reported quantities, not current usage.</p>
        <Evidence label="Capacity and allocatable" data={{ capacity: status.capacity, allocatable: status.allocatable }} />
        <Evidence label="Taints" data={spec.taints} />
        <Evidence label="Addresses" data={status.addresses} />
        <Evidence label="Node information" data={status.nodeInfo} />
      </>
    ) : kind === "PriorityClass" ? (
      <Entries
        values={{
          Value: body.value,
          Description: body.description,
          "Global default": body.globalDefault,
          "Preemption policy": body.preemptionPolicy,
        }}
      />
    ) : kind === "RuntimeClass" ? (
      <>
        <Entries values={{ Handler: body.handler }} />
        <p className="muted">This configuration does not establish handler availability on a node.</p>
        <Evidence label="Overhead" data={body.overhead} />
        <Evidence label="Scheduling selectors and tolerations" data={body.scheduling} />
      </>
    ) : kind === "Lease" ? (
      <>
        <Entries
          values={{
            Holder: spec.holderIdentity,
            "Acquire time": spec.acquireTime,
            "Renew time": spec.renewTime,
            "Duration (s)": spec.leaseDurationSeconds,
            Transitions: spec.leaseTransitions,
          }}
        />
        <p className="muted">Lease observations do not establish current leadership, expiry or availability.</p>
        <Evidence label="Returned coordination fields" data={spec} />
      </>
    ) : kind === "CustomResourceDefinition" ? (
      <>
        <Entries
          values={{
            Group: spec.group,
            Scope: spec.scope,
            Established: condition(status, "Established"),
            "Stored versions": status.storedVersions,
          }}
        />
        <Evidence label="Specified names" data={names} />
        <Evidence label="Accepted names" data={status.acceptedNames} />
        <EvidencePage
          items={versions}
          render={(v, i) => <Entries key={i} values={{ Version: v.name, Served: v.served, Storage: v.storage }} />}
        />
        <Evidence
          label="Schema and conversion"
          data={{ versions: versions.map((v) => ({ name: v.name, schema: v.schema })), conversion: spec.conversion }}
        />
        <label>
          Served version{" "}
          <select value={selectedVersion} onChange={(e) => setVersion(e.target.value)}>
            <option value="">Select version</option>
            {availableVersions.map((v, i) => (
              <option key={i} value={text(v.name)}>
                {text(v.name)}
              </option>
            ))}
          </select>
        </label>{" "}
        <button
          disabled={!canBrowse}
          onClick={() =>
            onBrowse({
              group: text(spec.group),
              version: selectedVersion,
              match: { resource: text(names.plural), kind: text(names.kind), namespaced: spec.scope === "Namespaced" },
            })
          }
        >
          Browse instances
        </button>
        <p className="muted">Requires a matching live API. A served version does not establish read access.</p>
      </>
    ) : kind === "APIService" ? (
      <>
        <Entries
          values={{
            Group: spec.group || "core",
            Version: spec.version,
            Registration: spec.service ? "Service-backed" : "Local",
            Available: condition(status, "Available"),
            "Group priority": spec.groupPriorityMinimum,
            "Version priority": spec.versionPriority,
          }}
        />
        <Evidence
          label="Service and TLS configuration"
          data={{ service: spec.service, caBundle: spec.caBundle, insecureSkipTLSVerify: spec.insecureSkipTLSVerify }}
        />
        <button disabled={!text(spec.version)} onClick={() => onBrowse({ group: text(spec.group), version: text(spec.version) })}>
          View resources
        </button>
      </>
    ) : (
      <>
        <Entries
          values={{
            Type: body.type,
            Reason: body.reason,
            Message: body.note ?? body.message,
            Regarding: `${text(regarding.kind)} ${text(regarding.name)}`.trim(),
            Occurrences: counts,
            "First observed":
              body.deprecatedFirstTimestamp ?? body.firstTimestamp ?? body.eventTime ?? obj(body.metadata).creationTimestamp,
            "Last observed": series.lastObservedTime ?? body.deprecatedLastTimestamp ?? body.lastTimestamp ?? body.eventTime,
            Reporter: body.reportingController || obj(body.source ?? body.deprecatedSource).component,
            "Reporting instance": body.reportingInstance,
            API: data.resource.apiVersion,
          }}
        />
        {canOpen ? (
          <button onClick={onTarget}>
            Open {text(regarding.kind)} {text(regarding.name)}
          </button>
        ) : (
          <p className="muted">No supported target with a complete identity.</p>
        )}
      </>
    );
  return (
    <div className="evidence-section workload-overview">
      {details}
      {kind !== "Event" && list(status.conditions).length > 0 && (
        <Disclosure label="Conditions">
          <EvidencePage items={list(status.conditions)} render={(c, i) => <Entries key={i} values={obj(c)} />} />
        </Disclosure>
      )}
      <p className="muted">
        Captured {evidenceTime(data.fetchedAt)} · resourceVersion {data.resourceVersion || "Unknown"}
      </p>
    </div>
  );
}
