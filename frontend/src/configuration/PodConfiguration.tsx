/*
Purpose: List explicit configuration references from a captured Pod.
Used by: Pod Overview.
Responsibilities: Preserve location, optionality and key evidence for click-only resolution.
Important: This projection does not read targets or infer key presence.
*/
import type { ResourceDetail, ResourceRef } from "../workloads/model";
import { overviewObject } from "../workspace/ResourceOverview";

type Local = { name?: string; key?: string; optional?: boolean; items?: { key: string; path: string }[] };
type Container = {
  name?: string;
  env?: { name: string; valueFrom?: { configMapKeyRef?: Local; secretKeyRef?: Local } }[];
  envFrom?: { configMapRef?: Local; secretRef?: Local; prefix?: string }[];
};
type Spec = {
  containers?: Container[];
  initContainers?: Container[];
  ephemeralContainers?: Container[];
  volumes?: {
    name: string;
    configMap?: Local;
    secret?: Local & { secretName?: string };
    projected?: { sources?: { configMap?: Local; secret?: Local }[] };
  }[];
  imagePullSecrets?: Local[];
};

export function podConfigurationReferences(data: ResourceDetail) {
  const { spec } = overviewObject(data) as { spec?: Spec };
  const rows: { resource: ResourceRef; location: string; keys?: string; optional?: boolean }[] = [];
  const add = (kind: "ConfigMap" | "Secret", ref: Local | undefined, location: string) => {
    if (!ref?.name) return;
    rows.push({
      resource: { ...data.resource, kind, apiVersion: "v1", name: ref.name, uid: "" },
      location,
      keys: ref.key ?? ref.items?.map(({ key, path }) => `${key} → ${path}`).join(", "),
      optional: ref.optional,
    });
  };
  for (const [label, containers] of [
    ["Container", spec?.containers],
    ["Init container", spec?.initContainers],
    ["Ephemeral container", spec?.ephemeralContainers],
  ] as const) {
    for (const c of containers ?? []) {
      const location = `${label} ${c.name ?? "Unknown"}`;
      for (const { name, valueFrom } of c.env ?? []) {
        add("ConfigMap", valueFrom?.configMapKeyRef, `${location} · env ${name}`);
        add("Secret", valueFrom?.secretKeyRef, `${location} · env ${name}`);
      }
      for (const [index, from] of (c.envFrom ?? []).entries()) {
        const source = `${location} · envFrom ${index + 1}${from.prefix ? ` · Prefix ${from.prefix}` : ""}`;
        add("ConfigMap", from.configMapRef, source);
        add("Secret", from.secretRef, source);
      }
    }
  }
  for (const volume of spec?.volumes ?? []) {
    const location = `Volume ${volume.name}`;
    add("ConfigMap", volume.configMap, location);
    if (volume.secret) add("Secret", { ...volume.secret, name: volume.secret.secretName }, location);
    for (const [index, source] of (volume.projected?.sources ?? []).entries()) {
      add("ConfigMap", source.configMap, `${location} · Projected source ${index + 1}`);
      add("Secret", source.secret, `${location} · Projected source ${index + 1}`);
    }
  }
  for (const ref of spec?.imagePullSecrets ?? []) add("Secret", ref, "Image pull Secret");
  return rows;
}

export function PodConfiguration({ data, onOpen }: { data: ResourceDetail; onOpen: (target: ResourceRef) => void }) {
  const rows = podConfigurationReferences(data);
  if (!rows.length) return null;
  return (
    <div className="evidence-section">
      <h3>Configuration references</h3>
      <p className="muted">References describe Pod configuration. They do not establish current container values or key presence.</p>
      <div className="evidence-table-wrap">
        <table className="evidence-table">
          <thead>
            <tr>
              <th>Reference</th>
              <th>Location</th>
              <th>Key</th>
              <th>Optional</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({ resource, location, keys, optional }, i) => (
              <tr key={i}>
                <td>
                  <button className="network-value" onClick={() => onOpen(resource)}>
                    {resource.kind} {resource.name}
                  </button>
                </td>
                <td className="network-value">{location}</td>
                <td className="network-value">{keys || "—"}</td>
                <td>{optional == null ? "Not specified" : optional ? "Yes" : "No"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
