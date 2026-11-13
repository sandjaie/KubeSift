/*
Purpose: Render captured Service, EndpointSlice, Ingress and IngressClass configuration.
Used by: The shared resource detail pane.
Responsibilities: Keep routing fields literal and offer only supported explicit references.
Important: Displayed configuration and reported status do not establish reachability.
*/
import { useState, type ReactNode } from "react";
import type { ResourceDetail, ResourceRef } from "../workloads/model";
import { evidenceTime, overviewObject } from "../workspace/ResourceOverview";
import { addressText, defaultClassAnnotation, managerLabel, refText, serviceLabel } from "./model";
import type { Backend, Capture, ClassSpec, Endpoint, IngressSpec, PolicySpec, Port, ServiceSpec } from "./model";
import { PolicyOverview } from "./PolicyOverview";

function Facts({ items }: { items: [string, ReactNode][] }) {
  return (
    <dl className="pod-facts">
      {items.map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd className="network-value">{value ?? "Not specified"}</dd>
        </div>
      ))}
    </dl>
  );
}

function Table({ columns, rows }: { columns: string[]; rows: ReactNode[][] }) {
  return (
    <div className="evidence-table-wrap">
      <table className="evidence-table">
        <thead>
          <tr>
            {columns.map((label) => (
              <th key={label}>{label}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i}>
              {row.map((value, j) => (
                <td className="network-value" key={j}>
                  {value ?? "—"}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Ports({ ports, slice = false }: { ports?: Port[]; slice?: boolean }) {
  if (!ports?.length) return <p>None specified.</p>;
  return (
    <Table
      columns={["Name", "Protocol", "Port", ...(slice ? [] : ["Target port", "Node port"]), "App protocol"]}
      rows={ports.map((p) => [p.name, p.protocol, p.port, ...(slice ? [] : [p.targetPort, p.nodePort || undefined]), p.appProtocol])}
    />
  );
}
const tri = (value: boolean | null | undefined) => (value == null ? "Unknown" : value ? "Yes" : "No");
const optionalBool = (value: boolean | undefined) => (value === undefined ? undefined : tri(value));
const entries = (values?: Record<string, string>) =>
  Object.entries(values ?? {})
    .map(([key, value]) => `${key} = ${value}`)
    .join(", ");

function ServiceOverview({ o }: { o: Capture<ServiceSpec> }) {
  const s = o.spec ?? {};
  return (
    <>
      <h3>Service configuration</h3>
      {s.type === "ExternalName" && <p className="muted">ExternalName is a DNS alias reference.</p>}
      <Facts
        items={[
          ["Type", s.type],
          ["Cluster IPs", s.clusterIPs?.join(", ") || s.clusterIP || undefined],
          ["Address mode", s.clusterIP === "None" || s.clusterIPs?.includes("None") ? "Headless" : undefined],
          ["IP families", s.ipFamilies?.join(", ")],
          ["IP family policy", s.ipFamilyPolicy],
          ["External name", s.externalName],
          ["Declared external IPs", s.externalIPs?.join(", ")],
          ["Reported load balancer", addressText(o.status?.loadBalancer?.ingress) || "None reported"],
          ["Selector", entries(s.selector) || "Selectorless"],
          ["Session affinity", s.sessionAffinity],
          ["Affinity timeout (seconds)", s.sessionAffinityConfig?.clientIP?.timeoutSeconds],
          ["External traffic policy", s.externalTrafficPolicy],
          ["Internal traffic policy", s.internalTrafficPolicy],
          ["Publish not-ready addresses", optionalBool(s.publishNotReadyAddresses)],
          ["Health check node port", s.healthCheckNodePort || undefined],
          ["Traffic distribution", s.trafficDistribution],
          ["Allocate load balancer node ports", optionalBool(s.allocateLoadBalancerNodePorts)],
          ["Load balancer class", s.loadBalancerClass],
          ["Requested load balancer IP", s.loadBalancerIP],
          ["Load balancer source ranges", s.loadBalancerSourceRanges?.join(", ")],
        ]}
      />
      <h3>Ports</h3>
      <Ports ports={s.ports} />
    </>
  );
}

function Endpoints({ endpoints, open, resource }: { endpoints: Endpoint[]; open: (target: ResourceRef) => void; resource: ResourceRef }) {
  const [page, setPage] = useState(0);
  const first = page * 100;
  const rows = endpoints.slice(first, first + 100);
  return (
    <>
      <h3>Endpoints</h3>
      <p>
        {rows.length} / {endpoints.length} captured{rows.length > 0 && ` · Entries ${first + 1}–${first + rows.length}`}
      </p>
      <p className="muted">
        Consumers can interpret omitted ready as ready. Unknown means readiness was not reported; conditions do not establish traffic
        health.
      </p>
      {rows.length > 0 && (
        <Table
          columns={["Addresses", "Hostname", "Ready", "Serving", "Terminating", "Node / zone", "Target reference", "Hints / topology"]}
          rows={rows.map((e) => {
            const ref = e.targetRef;
            const pod =
              ref?.kind === "Pod" &&
              (!ref.apiVersion || ref.apiVersion === "v1") &&
              ref.name &&
              (!ref.namespace || ref.namespace === resource.namespace);
            return [
              e.addresses?.join(", "),
              e.hostname,
              tri(e.conditions?.ready),
              tri(e.conditions?.serving),
              tri(e.conditions?.terminating),
              [e.nodeName, e.zone].filter(Boolean).join(" / ") || undefined,
              ref ? (
                <>
                  {refText(ref)}{" "}
                  {pod && (
                    <button onClick={() => open({ ...resource, kind: "Pod", apiVersion: "v1", name: ref.name!, uid: ref.uid ?? "" })}>
                      Open Pod {ref.name}
                    </button>
                  )}
                </>
              ) : (
                "Not specified"
              ),
              (e.hints || e.deprecatedTopology) && (
                <details>
                  <summary>Show details</summary>
                  <Facts
                    items={[
                      ["Preferred zones", e.hints?.forZones?.map((zone) => zone.name).join(", ")],
                      ["Preferred nodes", e.hints?.forNodes?.map((node) => node.name).join(", ")],
                      ["Deprecated topology", entries(e.deprecatedTopology) || undefined],
                    ]}
                  />
                </details>
              ),
            ];
          })}
        />
      )}
      {endpoints.length > 100 && (
        <nav className="inventory-pagination" aria-label="Captured endpoint pages">
          <button disabled={page === 0} onClick={() => setPage(page - 1)}>
            Previous
          </button>
          <button disabled={first + 100 >= endpoints.length} onClick={() => setPage(page + 1)}>
            Next
          </button>
        </nav>
      )}
    </>
  );
}

function SliceOverview({ o, resource, open }: { o: Capture<never>; resource: ResourceRef; open: (target: ResourceRef) => void }) {
  const service = o.metadata?.labels?.[serviceLabel];
  const owners = o.metadata?.ownerReferences ?? [];
  const serviceOwners = owners.filter((r) => r.kind === "Service" && r.apiVersion === "v1" && r.uid);
  const uid = serviceOwners[0]?.uid;
  const conflict = serviceOwners.some((r) => r.name !== service || r.uid !== uid);
  return (
    <>
      <h3>Slice configuration</h3>
      <Facts
        items={[
          [
            "Associated Service label",
            service ? (
              <>
                {service} · Label association{" "}
                {!conflict && (
                  <button onClick={() => open({ ...resource, kind: "Service", apiVersion: "v1", name: service, uid: uid ?? "" })}>
                    Open Service {service}
                  </button>
                )}
              </>
            ) : undefined,
          ],
          [
            "Ownership evidence",
            conflict
              ? "Relationship conflict: Service owner differs"
              : uid
                ? "Service owner UID recorded; verified on open"
                : "Ownership unverified",
          ],
          ["Managed by", o.metadata?.labels?.[managerLabel]],
          ["Address type", o.addressType],
          ["Owners", owners.map(refText).join("; ") || "None recorded"],
        ]}
      />
      <h3>Slice ports</h3>
      <Ports ports={o.ports} slice />
      <Endpoints endpoints={o.endpoints ?? []} open={open} resource={resource} />
    </>
  );
}

function IngressOverview({ o, resource, open }: { o: Capture<IngressSpec>; resource: ResourceRef; open: (target: ResourceRef) => void }) {
  const s = o.spec ?? {};
  const backend = (b?: Backend) => {
    if (!b) return "Not specified";
    if (b.resource && !b.service) return `Resource backend: ${refText(b.resource)}`;
    const service = b.service;
    const port = service?.port;
    if (
      b.resource ||
      !service?.name ||
      Boolean(port?.name) === Boolean(port?.number) ||
      (port?.number && (port.number < 1 || port.number > 65535))
    )
      return "Invalid or unknown backend";
    return (
      <>
        <button onClick={() => open({ ...resource, apiVersion: "v1", kind: "Service", name: service.name!, uid: "" })}>
          {service.name}
        </button>{" "}
        · {port?.name || port?.number}
      </>
    );
  };
  const routes = (s.rules ?? []).flatMap((rule) =>
    (rule.http?.paths ?? []).map((route) => [
      rule.host || "Any host",
      route.path ?? "Not specified",
      route.pathType ?? "Unknown",
      backend(route.backend),
    ]),
  );
  return (
    <>
      <h3>Ingress configuration</h3>
      <Facts
        items={[
          [
            "Ingress class",
            s.ingressClassName ? (
              <button
                onClick={() =>
                  open({
                    ...resource,
                    apiVersion: "networking.k8s.io/v1",
                    kind: "IngressClass",
                    namespace: "",
                    name: s.ingressClassName!,
                    uid: "",
                  })
                }
              >
                {s.ingressClassName}
              </button>
            ) : undefined,
          ],
          ["Legacy class annotation", o.metadata?.annotations?.["kubernetes.io/ingress.class"]],
          ["Reported addresses", addressText(o.status?.loadBalancer?.ingress) || "None reported"],
          ["Default backend", backend(s.defaultBackend)],
        ]}
      />
      <h3>Routes</h3>
      {routes.length ? <Table columns={["Host", "Path", "Path type", "Backend / port"]} rows={routes} /> : <p>No HTTP paths specified.</p>}
      <h3>TLS references</h3>
      {s.tls?.length ? (
        <Table columns={["Configured hosts", "Secret name"]} rows={s.tls.map((entry) => [entry.hosts?.join(", "), entry.secretName])} />
      ) : (
        <p>None specified.</p>
      )}
      <p className="muted">TLS names are configuration references. Certificate contents and status are not inspected.</p>
      {Object.keys(o.metadata?.annotations ?? {}).length > 0 && (
        <details>
          <summary>Annotations</summary>
          <p className="network-value">{entries(o.metadata?.annotations)}</p>
        </details>
      )}
    </>
  );
}

export function NetworkOverview({ data, onOpenReference }: { data: ResourceDetail; onOpenReference: (target: ResourceRef) => void }) {
  const o = overviewObject(data);
  const props = { resource: data.resource, open: onOpenReference };
  return (
    <div className="evidence-section workload-overview">
      <p className="muted">
        Captured {evidenceTime(data.fetchedAt)} · Resource version {data.resourceVersion || "Unknown"}
      </p>
      {data.resource.kind === "Service" && <ServiceOverview o={o as Capture<ServiceSpec>} />}
      {data.resource.kind === "EndpointSlice" && <SliceOverview key={data.fetchedAt} o={o as Capture<never>} {...props} />}
      {data.resource.kind === "Ingress" && <IngressOverview o={o as Capture<IngressSpec>} {...props} />}
      {data.resource.kind === "IngressClass" && (
        <Facts
          items={[
            ["Scope", "Cluster-scoped"],
            ["Controller", (o as Capture<ClassSpec>).spec?.controller],
            ["Default class annotation", (o as Capture<ClassSpec>).metadata?.annotations?.[defaultClassAnnotation]],
            ["Parameters reference", refText((o as Capture<ClassSpec>).spec?.parameters)],
          ]}
        />
      )}
      {data.resource.kind === "NetworkPolicy" && <PolicyOverview spec={(o as Capture<PolicySpec>).spec ?? {}} />}
    </div>
  );
}
