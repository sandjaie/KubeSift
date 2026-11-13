/*
Purpose: Describe the supported networking kinds and captured projections.
Used by: Inventory, networking details and resource navigation.
Responsibilities: Preserve optional values from typed Kubernetes JSON.
Important: Missing values must not become positive health evidence.
*/
export type {
  NetworkPage,
  NetworkRow,
  ServiceSlicePage,
  NetworkReferenceRequest,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";

export const networkKinds = ["Service", "EndpointSlice", "Ingress", "IngressClass", "NetworkPolicy"] as const;
export type NetworkKind = (typeof networkKinds)[number];
export const isNetworkKind = (kind: string): kind is NetworkKind => networkKinds.some((candidate) => candidate === kind);
export const networkLabels: Record<NetworkKind, string> = {
  Service: "Services",
  EndpointSlice: "EndpointSlices",
  Ingress: "Ingresses",
  IngressClass: "IngressClasses",
  NetworkPolicy: "NetworkPolicies",
};
export const serviceLabel = "kubernetes.io/service-name";
export const managerLabel = "endpointslice.kubernetes.io/managed-by";
export const defaultClassAnnotation = "ingressclass.kubernetes.io/is-default-class";

type Labels = Record<string, string>;
export interface Selector {
  matchLabels?: Labels;
  matchExpressions?: { key: string; operator: string; values?: string[] }[];
}
export interface Reference {
  apiVersion?: string;
  apiGroup?: string;
  kind?: string;
  name?: string;
  namespace?: string;
  uid?: string;
  scope?: string;
}
export interface Port {
  name?: string;
  protocol?: string;
  port?: number | string;
  targetPort?: number | string;
  nodePort?: number;
  appProtocol?: string;
  endPort?: number;
}
export interface Address {
  ip?: string;
  hostname?: string;
  ipMode?: string;
  ports?: { port: number; protocol: string; error?: string }[];
}
export interface ServiceSpec {
  type?: string;
  clusterIP?: string;
  clusterIPs?: string[];
  ipFamilies?: string[];
  ipFamilyPolicy?: string;
  externalName?: string;
  externalIPs?: string[];
  ports?: Port[];
  selector?: Labels;
  sessionAffinity?: string;
  sessionAffinityConfig?: { clientIP?: { timeoutSeconds?: number } };
  externalTrafficPolicy?: string;
  internalTrafficPolicy?: string;
  publishNotReadyAddresses?: boolean;
  healthCheckNodePort?: number;
  trafficDistribution?: string;
  allocateLoadBalancerNodePorts?: boolean;
  loadBalancerClass?: string;
  loadBalancerIP?: string;
  loadBalancerSourceRanges?: string[];
}
export interface Endpoint {
  addresses?: string[];
  hostname?: string;
  conditions?: { ready?: boolean; serving?: boolean; terminating?: boolean };
  nodeName?: string;
  zone?: string;
  targetRef?: Reference;
  hints?: { forZones?: { name: string }[]; forNodes?: { name: string }[] };
  deprecatedTopology?: Labels;
}
export interface Backend {
  service?: { name?: string; port?: { name?: string; number?: number } };
  resource?: Reference;
}
export interface IngressSpec {
  ingressClassName?: string;
  defaultBackend?: Backend;
  rules?: { host?: string; http?: { paths?: { path?: string; pathType?: string; backend?: Backend }[] } }[];
  tls?: { hosts?: string[]; secretName?: string }[];
}
export interface Peer {
  podSelector?: Selector;
  namespaceSelector?: Selector;
  ipBlock?: { cidr?: string; except?: string[] };
}
export interface PolicySpec {
  podSelector?: Selector;
  policyTypes?: string[];
  ingress?: { from?: Peer[]; ports?: Port[] }[];
  egress?: { to?: Peer[]; ports?: Port[] }[];
}
export interface ClassSpec {
  controller?: string;
  parameters?: Reference;
}
type Status = { status?: { loadBalancer?: { ingress?: Address[] } } };
export interface InventoryProjection {
  Service: { spec?: Pick<ServiceSpec, "type" | "clusterIP" | "clusterIPs" | "externalName" | "externalIPs" | "ports"> } & Status;
  EndpointSlice: { metadata?: { labels?: Labels }; addressType?: string; endpointCount?: number };
  Ingress: { spec?: { ingressClassName?: string; hosts?: string[] } } & Status;
  IngressClass: { spec?: ClassSpec; defaultClass?: string };
  NetworkPolicy: { spec?: Pick<PolicySpec, "podSelector" | "policyTypes">; ingressRuleCount?: number; egressRuleCount?: number };
}
export interface Capture<T> extends Status {
  spec?: T;
  metadata?: { labels?: Labels; annotations?: Labels; ownerReferences?: Reference[] };
  addressType?: string;
  ports?: Port[];
  endpoints?: Endpoint[];
}
export function selectorText(selector: Selector | undefined, empty: string) {
  if (!selector) return "Unknown";
  return (
    [
      ...Object.entries(selector.matchLabels ?? {}).map(([key, value]) => `${key} = ${value}`),
      ...(selector.matchExpressions ?? []).map(({ key, operator, values }) =>
        [key, operator, values?.length ? `(${values.join(", ")})` : ""].filter(Boolean).join(" "),
      ),
    ].join(" AND ") || empty
  );
}
export function addressText(addresses: Address[] | undefined) {
  return (addresses ?? [])
    .map(
      ({ ip, hostname, ipMode, ports }) =>
        [ip, hostname, ipMode, ...(ports ?? []).map(({ port, protocol, error }) => `${port}/${protocol}${error ? `: ${error}` : ""}`)]
          .filter(Boolean)
          .join(" · ") || "Unknown address",
    )
    .join(", ");
}
export function refText(ref: Reference | undefined) {
  if (!ref) return "Not specified";
  return (
    [
      ref.apiVersion ?? ref.apiGroup,
      ref.kind,
      ref.namespace ? `${ref.namespace}/${ref.name ?? "?"}` : ref.name,
      ref.scope,
      ref.uid ? `UID ${ref.uid}` : "",
    ]
      .filter(Boolean)
      .join(" · ") || "Unknown reference"
  );
}
