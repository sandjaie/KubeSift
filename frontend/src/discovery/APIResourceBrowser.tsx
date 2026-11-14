/*
Purpose: Browse one advertised group-version and one object page at a time.
Used by: The Extensions workspace.
Responsibilities: Retain connection-owned routes, cancel departed reads and expose recoverable failures.
Important: Selectors never issue native reads using arbitrary URLs.
*/
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ConnectionRef } from "../bridge";
import { message } from "../connections/api";
import { readRequest, workloadAPI as api } from "../workloads/api";
import { isResourceKind } from "../workloads/model";
import { ResourceSplit } from "../workspace/ResourceSplit";
import { GenericResourceTable } from "./GenericResourceTable";
import { GenericResourceDetail } from "./GenericResourceDetail";
import {
  visibleGenericRoute,
  type CatalogSeed,
  type APIGroupCatalog,
  type APIResourceCatalog,
  type APIResourceDescriptor,
  type GenericResourcePage,
  type GenericResourceRef,
} from "./model";

type Load<T> = { data?: T; error?: string; loading?: boolean };
function useRead<T>(key: string, read: (id: string) => Promise<T>): Load<T> {
  const [state, set] = useState<Load<T> & { key: string }>({ key: "" });
  useEffect(() => {
    if (!key) return;
    let active = true;
    const current = readRequest(read);
    set({ key, loading: true });
    void current.promise.then(
      (data) => {
        if (active) set({ key, data });
      },
      (error) => {
        if (active) set({ key, error: message(error) });
      },
    );
    return () => {
      active = false;
      current.cancel();
    };
    // The key contains every input used by the request.
  }, [key]);
  return state.key === key ? state : { loading: !!key };
}

export function APIResourceBrowser({
  connection,
  namespace,
  route,
  onRoute,
  onBack,
  onDedicated,
  widthPercent,
  heightPercent,
  onResize,
  onCommit,
}: {
  connection: ConnectionRef;
  namespace: string;
  route?: CatalogSeed;
  onRoute: (route: CatalogSeed | null) => void;
  onBack: () => void;
  onDedicated: (kind: import("../workloads/model").ResourceKind) => void;
  widthPercent: number;
  heightPercent: number;
  onResize: (axis: "x" | "y", value: number) => void;
  onCommit: () => void;
}) {
  const [refresh, setRefresh] = useState(0);
  const [groupFilter, setGroupFilter] = useState("");
  const [filter, setFilter] = useState("");
  const [detail, setDetail] = useState<{ key: string; error?: string; loading?: boolean }>({ key: "" });
  const pending = useRef<null | (() => void)>(null);
  const latestRoute = useRef(route);
  latestRoute.current = route;
  const active = route && visibleGenericRoute(route, namespace);
  useEffect(() => {
    if (route && active !== route && active) onRoute(active);
  }, [route, active, onRoute]);
  const descriptor = active?.descriptor;
  const scope = descriptor?.namespaced ? namespace : "";
  const history = active?.history ?? [""];
  const cursor = history[history.length - 1] ?? "";
  const key = descriptor?.mode === "generic" ? JSON.stringify([descriptor.handle, scope]) : "";
  useLayoutEffect(
    () => () => {
      pending.current?.();
      pending.current = null;
    },
    [key, cursor],
  );
  const root = useRead<APIGroupCatalog>(JSON.stringify(["root", refresh]), (id) =>
    api.ReadAPIGroups(id, { connection, refresh: refresh > 0 }),
  );
  const [resourceFilter, setResourceFilter] = useState("");
  const resources = useRead<APIResourceCatalog>(
    active?.version && root.data ? JSON.stringify([root.data.epoch, active.group, active.version]) : "",
    (id) => api.ReadAPIResources(id, { connection, group: active!.group, version: active!.version, refresh: false }),
  );
  const inventory = useRead<GenericResourcePage>(key && JSON.stringify([key, cursor]), (id) =>
    api.ReadGenericResources(id, { connection, handle: descriptor!.handle, namespace: scope, cursor }),
  );
  const groups = root.data?.groups ?? [];
  const group = groups.find((g) => g.group === active?.group);
  const options = (resources.data?.resources ?? []).filter(
    (r) =>
      (r.mode !== "generic" || r.handle) &&
      (r.resource === descriptor?.resource || `${r.kind} ${r.resource}`.toLowerCase().includes(resourceFilter.trim().toLowerCase())),
  );
  const groupOptions = groups.filter(
    (g) => g.group === active?.group || (g.group || "core").toLowerCase().includes(groupFilter.trim().toLowerCase()),
  );
  const rows = (inventory.data?.items ?? []).filter((r) => r.resource.name.toLowerCase().includes(filter.trim().toLowerCase()));
  const capture = active?.capture;
  const patch = (next: Partial<CatalogSeed>) => {
    if (active) onRoute({ ...active, ...next });
  };
  const clearDetail = () => {
    pending.current?.();
    pending.current = null;
    setDetail({ key: "" });
  };
  const selecting = (candidate?: APIResourceDescriptor) => {
    clearDetail();
    setFilter("");
    patch({ descriptor: candidate, history: [""], namespace, capture: undefined, section: "Overview" });
  };
  function open(resource: GenericResourceRef) {
    if (!descriptor || !key) return;
    clearDetail();
    const req = readRequest((id) =>
      api.ReadGenericDetail(id, { connection, handle: descriptor.handle, namespace: scope, name: resource.name, uid: resource.uid }),
    );
    pending.current = req.cancel;
    setDetail({ key, loading: true });
    void req.promise.then(
      (data) => {
        if (pending.current !== req.cancel) return;
        pending.current = null;
        setDetail({ key });
        const route = latestRoute.current;
        if (route?.descriptor?.handle === descriptor.handle)
          onRoute({ ...route, namespace, capture: data, section: capture?.resource.uid === resource.uid ? active?.section : "Overview" });
      },
      (error) => {
        if (pending.current !== req.cancel) return;
        pending.current = null;
        setDetail({ key, error: message(error) });
      },
    );
  }
  return (
    <ResourceSplit widthPercent={widthPercent} heightPercent={heightPercent} onResize={onResize} onCommit={onCommit}>
      <section className="list-pane api-browser">
        <div className="pane-heading">
          <h2>API resources</h2>
          <button
            onClick={() => {
              clearDetail();
              patch({ descriptor: undefined, capture: undefined, history: [""] });
              setRefresh((n) => n + 1);
            }}
          >
            ↻ Refresh resources
          </button>
        </div>
        {active?.origin && (
          <button
            className="catalog-back"
            onClick={() => {
              clearDetail();
              onBack();
            }}
          >
            ← Back to {active.origin.resource.kind} {active.origin.resource.name}
          </button>
        )}
        <div className="filter-bar">
          <input
            aria-label="Filter API groups"
            placeholder="Filter loaded groups…"
            spellCheck={false}
            autoCorrect="off"
            autoCapitalize="none"
            autoComplete="off"
            value={groupFilter}
            onChange={(e) => setGroupFilter(e.target.value)}
          />
          <select
            aria-label="API group"
            value={active ? "group:" + active.group : ""}
            onChange={(e) => {
              clearDetail();
              setResourceFilter("");
              const selected = groups.find((g) => "group:" + g.group === e.target.value);
              if (selected) onRoute({ group: selected.group, version: selected.preferred, origin: active?.origin });
              else if (active?.origin) onRoute({ group: "", version: "", origin: active.origin });
              else onRoute(null);
            }}
          >
            <option value="">Select API group</option>
            {groupOptions.map((g) => (
              <option key={g.group} value={"group:" + g.group}>
                {g.group || "core"}
              </option>
            ))}
          </select>
          {group && (
            <select
              aria-label="API version"
              value={active?.version ?? ""}
              onChange={(e) => {
                clearDetail();
                setResourceFilter("");
                patch({ version: e.target.value, descriptor: undefined, capture: undefined, history: [""] });
              }}
            >
              <option value="">Select version</option>
              {group.versions?.map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          )}
          {resources.data && (
            <input
              aria-label="Filter loaded resources"
              placeholder="Filter loaded resources…"
              spellCheck={false}
              autoCorrect="off"
              autoCapitalize="none"
              autoComplete="off"
              value={resourceFilter}
              onChange={(e) => setResourceFilter(e.target.value)}
            />
          )}
          {resources.data && (
            <select
              aria-label="API resource"
              value={descriptor?.resource ?? ""}
              onChange={(e) => selecting(options.find((r) => r.resource === e.target.value))}
            >
              <option value="">Select resource</option>
              {options.map((r) => (
                <option key={r.resource} value={r.resource}>
                  {r.kind} · {r.resource} · {r.namespaced ? "Namespaced" : "Cluster-scoped"}
                </option>
              ))}
            </select>
          )}
        </div>
        {(root.loading || resources.loading) && <p>Loading API discovery…</p>}
        {[
          root.error,
          root.data?.coreProblem && `Core discovery: ${root.data.coreProblem.message}`,
          root.data?.apisProblem && `Named API discovery: ${root.data.apisProblem.message}`,
          resources.error,
          inventory.error,
        ]
          .filter(Boolean)
          .map((e, i) => (
            <p role="alert" key={i}>
              {e}
            </p>
          ))}
        {root.data && !groups.length && <p className="state-message">No API groups advertised.</p>}
        {resources.data && !options.length && (
          <p className="state-message">{resourceFilter ? "No matches among loaded resources." : "No top-level resources advertised."}</p>
        )}
        {resources.data && descriptor?.mode === "generic" && !resources.data.resources?.some((r) => r.handle === descriptor.handle) && (
          <p role="alert">The resource selection expired or changed. Reselect it or refresh resources.</p>
        )}

        {descriptor && (
          <p className="table-hint">
            {descriptor.group || "core"}/{descriptor.version} · {descriptor.kind} ·{" "}
            {descriptor.namespaced ? `Namespace: ${namespace}` : "Cluster-scoped"}
          </p>
        )}
        {descriptor?.mode === "dedicated" && (
          <p>
            {descriptor.reason}.{" "}
            {isResourceKind(descriptor.kind) && descriptor.kind !== "APIResources" && (
              <button
                onClick={() => {
                  if (isResourceKind(descriptor.kind)) onDedicated(descriptor.kind);
                }}
              >
                Open {descriptor.kind}
              </button>
            )}
          </p>
        )}
        {descriptor?.mode === "unavailable" && <p>{descriptor.reason}</p>}
        {key && descriptor && (
          <>
            <div className="filter-bar">
              <input
                aria-label="Filter this page"
                placeholder="Filter this page…"
                spellCheck={false}
                autoCorrect="off"
                autoCapitalize="none"
                autoComplete="off"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
              />
            </div>
            <nav className="inventory-pagination catalog-pages" aria-label="Resource pages">
              <button
                disabled={history.length <= 1 || inventory.loading}
                onClick={() => {
                  clearDetail();
                  patch({ history: history.slice(0, -1), capture: undefined });
                }}
              >
                Previous
              </button>
              <span>Page {history.length}</span>
              <button
                disabled={!inventory.data?.page.nextCursor || history.length >= 20 || inventory.loading}
                onClick={() => {
                  clearDetail();
                  patch({ history: [...history, inventory.data!.page.nextCursor], capture: undefined });
                }}
              >
                Next
              </button>
            </nav>
            {history.length >= 20 && <p className="warning">History limit reached. Refresh resources to restart.</p>}
            {inventory.loading && <p>Loading inventory…</p>}
            {!!rows.length && (
              <GenericResourceTable
                rows={rows}
                descriptor={descriptor}
                selected={capture?.resource.uid ?? null}
                onSelect={(row) => open(row.resource)}
              />
            )}
            {!rows.length && inventory.data && (
              <p className="state-message">{filter ? "No matches on this page." : "No resources returned on this page."}</p>
            )}
          </>
        )}
      </section>
      <section className="detail-pane">
        {detail.key === key && detail.loading && <p>Loading captured object…</p>}
        {detail.key === key && detail.error && <p role="alert">{detail.error}</p>}
        {capture ? (
          <GenericResourceDetail
            key={JSON.stringify([capture.resource.uid, capture.fetchedAt])}
            capture={capture}
            section={active?.section ?? "Overview"}
            loading={detail.key === key && !!detail.loading}
            onSection={(section) => patch({ section })}
            onRefresh={() => open(capture.resource)}
            onBack={() => {
              clearDetail();
              patch({ capture: undefined, section: "Overview" });
            }}
          />
        ) : (
          <p className="state-message">Select an object to inspect captured evidence.</p>
        )}
      </section>
    </ResourceSplit>
  );
}
