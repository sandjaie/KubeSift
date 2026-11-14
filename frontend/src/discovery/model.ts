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

export function currentGenericDescriptor(descriptor: APIResourceDescriptor | undefined, catalog: APIResourceCatalog | undefined) {
  return descriptor?.mode === "generic" &&
    catalog?.resources?.some(
      (candidate) =>
        candidate.handle === descriptor.handle &&
        candidate.group === descriptor.group &&
        candidate.version === descriptor.version &&
        candidate.resource === descriptor.resource &&
        candidate.kind === descriptor.kind &&
        candidate.namespaced === descriptor.namespaced &&
        candidate.mode === "generic",
    )
    ? descriptor
    : undefined;
}

// Cancellation releases native work; ownership rejects replies that already raced it.
export function newestRead() {
  let active: (() => void) | undefined;
  const cancel = () => {
    const stop = active;
    active = undefined;
    stop?.();
  };
  return {
    cancel,
    start<T>(request: { promise: Promise<T>; cancel: () => void }, success: (data: T) => void, failure: (error: unknown) => void) {
      cancel();
      const owner = () => request.cancel();
      active = owner;
      const settle = (callback: () => void) => {
        if (active !== owner) return;
        active = undefined;
        callback();
      };
      void request.promise.then(
        (data) => settle(() => success(data)),
        (error) => settle(() => failure(error)),
      );
    },
  };
}
