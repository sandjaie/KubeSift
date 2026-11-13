/*
Purpose: Check truthful, inert networking evidence.
Used by: Vitest.
Responsibilities: Exercise configuration semantics and bounded endpoint markup.
Important: All evidence is synthetic.
*/
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { NetworkOverview } from "./NetworkOverview";
import type { ResourceDetail } from "../workloads/model";

function overview(kind: string, object: unknown) {
  const data: ResourceDetail = {
    resource: { kind, name: "example", uid: "u", namespace: "ns", connection: { id: "c", generation: 1 }, apiVersion: "v1" },
    fetchedAt: "2026-11-13T00:00:00Z",
    resourceVersion: "1",
    job: null,
    objectJSON: JSON.stringify(object),
  };
  return renderToStaticMarkup(<NetworkOverview data={data} onOpenReference={() => {}} />);
}

test("service configuration preserves named ports, families and valid special types", () => {
  const headless = overview("Service", {
    spec: { clusterIP: "None", ipFamilies: ["IPv4", "IPv6"], ports: [{ port: 80, targetPort: "http" }] },
  });
  for (const value of ["Headless", "IPv6", "http", "Selectorless"]) expect(headless).toContain(value);
  expect(overview("Service", { spec: { type: "ExternalName", externalName: "example.invalid" } })).toContain("DNS alias");
});

test("slices render at most 100 endpoint entries and report omitted conditions", () => {
  const html = overview("EndpointSlice", { endpoints: Array.from({ length: 101 }, (_, i) => ({ addresses: [`endpoint-${i}-address`] })) });
  expect(html).toContain("100 / 101 captured");
  expect(html).toContain("Unknown");
  expect(html).not.toContain("endpoint-100-address");
});

test("Ingress separates explicit class and legacy annotation and leaves TLS references inert", () => {
  const html = overview("Ingress", {
    metadata: { annotations: { "kubernetes.io/ingress.class": "legacy" } },
    spec: {
      defaultBackend: { resource: { apiGroup: "example", kind: "Resource", name: "item" } },
      tls: [{ secretName: "<script>secret</script>" }],
      rules: [
        {
          host: "*.example",
          http: { paths: [{ pathType: "ImplementationSpecific", backend: { service: { name: "web", port: { name: "http" } } } }] },
        },
      ],
    },
  });
  for (const value of ["Not specified", "legacy", "ImplementationSpecific", "*.example", "http", "&lt;script&gt;secret&lt;/script&gt;"])
    expect(html).toContain(value);
  expect(html).not.toContain("<script>");
});

test("policy rules retain direction, grouping, empty rules and literal ports", () => {
  const html = overview("NetworkPolicy", {
    spec: {
      podSelector: {},
      ingress: [
        {},
        {
          from: [
            { namespaceSelector: {}, podSelector: { matchExpressions: [{ key: "team", operator: "In", values: ["dev"] }] } },
            { ipBlock: { cidr: "10.0.0.0/8", except: ["10.1.0.0/16"] } },
          ],
          ports: [{ port: "http", endPort: 8082 }],
        },
      ],
    },
  });
  for (const value of [
    "API-defaulted interpretation",
    "All Pods",
    "OR",
    "AND",
    "All peers and ports",
    "Not selected by this policy",
    "10.1.0.0/16",
    "http",
    "8082",
  ])
    expect(html).toContain(value);
  expect(overview("NetworkPolicy", { spec: { policyTypes: ["Egress"] } })).toContain("No allow rules in this policy");
});
