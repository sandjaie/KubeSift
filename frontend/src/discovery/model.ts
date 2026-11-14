/*
Purpose: Keep installed-API selection separate from fixed-kind identity.
Used by: Catalog navigation and generic views.
Responsibilities: Match live descriptors and reset scoped state when namespace changes.
Important: Only native-issued handles are used for generic I/O.
*/
import type { CatalogIntent } from "../cluster/model";
import type { NavigationEntry } from "../workloads/model";
import type {
  APIResourceCatalog,
  APIResourceDescriptor,
  GenericResourceDetail,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
export type {
  APIGroupCatalog,
  APIResourceCatalog,
  APIResourceDescriptor,
  GenericResourceDetail,
  GenericResourcePage,
  GenericResourceRow,
  GenericResourceRef,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
export type CatalogSeed = {
  group: string;
  version: string;
  descriptor?: APIResourceDescriptor;
  origin?: NavigationEntry;
  history?: string[];
  namespace?: string;
  capture?: GenericResourceDetail;
  section?: "Overview" | "YAML";
};
export function selectCatalogIntent(intent: CatalogIntent, catalog: APIResourceCatalog): CatalogSeed {
  const match = intent.match;
  const descriptor =
    match &&
    catalog.resources?.find(
      (r) =>
        r.group === intent.group &&
        r.version === intent.version &&
        r.resource === match.resource &&
        r.kind === match.kind &&
        r.namespaced === match.namespaced &&
        r.mode === "generic",
    );
  if (match && !descriptor) throw new Error("No live API matches this CRD’s plural, kind and scope.");
  return { group: intent.group, version: intent.version, descriptor };
}
export function visibleGenericRoute(route: CatalogSeed, namespace: string): CatalogSeed {
  if (!route.descriptor?.namespaced || route.namespace === namespace) return route;
  return { ...route, namespace, history: [""], capture: undefined, section: "Overview" };
}
