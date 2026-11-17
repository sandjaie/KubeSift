/*
Purpose: Own Helm navigation and cancellable reads for cluster tabs.
Used by: The workspace and Helm detail pane.
Responsibilities: Keep metadata and selection per tab and discard section data when it leaves view.
Important: Hidden workspaces cannot initiate a Helm scan.
*/
import { useLayoutEffect, useState } from "react";
import type { ConnectionRef } from "../bridge";
import type {
  CurrentResource,
  HistoryPage,
  ManifestResource,
  RecordRef,
  ReleasePage,
  ReleaseSection,
  Scope,
} from "../../bindings/internal/helmreader/models";
import { message } from "../connections/api";
import { newestRead } from "../discovery/model";
import { workloadAPI } from "../workloads/api";
import { listHelm, openCurrent, readHistory, readSection } from "./api";
import { matchesRecord, readKey, type Section } from "./state";

type Driver = "secret" | "configmap";
type View = {
  identity: string;
  scope: Scope;
  driver: Driver;
  filter: string;
  name: string;
  revision?: RecordRef;
  section: Section;
  computed: boolean;
  historyPage: number;
  list?: ReleasePage;
  request: { snapshotID: string; page: number; refresh: number };
};

function closeScope(scope: Scope) {
  void workloadAPI.CloseHelmScope(scope.connection, scope.namespace, scope.driver).catch(() => {});
}

export function useHelm(tab: string, connection: ConnectionRef | undefined, namespace: string, visible: boolean) {
  const [tabs, setTabs] = useState<Record<string, View>>({});
  const [detail, setDetail] = useState<{ key: string; section?: ReleaseSection; history?: HistoryPage; error?: string }>();
  const [inventoryError, setInventoryError] = useState<{ key: string; message: string }>();
  const [link, setLink] = useState<{ key: string; pending: boolean; error?: string }>();
  const [linkOwner] = useState(newestRead);
  const old = tabs[tab];
  const identity = connection ? JSON.stringify([connection.id, connection.generation, namespace]) : "";
  const current = old?.identity === identity ? old : undefined;
  const driver = current?.driver ?? old?.driver ?? "secret";
  const scope = connection ? { connection, namespace, driver, releaseName: current?.name ?? "" } : undefined;
  const revision = current?.revision;
  const section = current?.section ?? "Overview";
  const computed = section === "Values" && !!current?.computed;
  const historyPage = section === "History" ? (current?.historyPage ?? 0) : 0;

  useLayoutEffect(() => {
    if (!scope || !tab || !namespace || current) return;
    if (old) closeScope(old.scope);
    setTabs((all) => ({
      ...all,
      [tab]: {
        identity,
        scope,
        driver,
        filter: "",
        name: "",
        section: "Overview",
        computed: false,
        historyPage: 0,
        request: { snapshotID: "", page: 0, refresh: 0 },
      },
    }));
  }, [tab, identity]);

  const request = current?.request;
  const listKey = visible && scope && request && !current?.list ? JSON.stringify([tab, identity, driver, scope.releaseName, request]) : "";
  useLayoutEffect(() => {
    if (!listKey || !scope || !current || !request) return;
    const read = listHelm({ scope, snapshotID: request.snapshotID, page: request.page });
    let active = true;
    void read.promise.then(
      (list) => {
        if (!active) return;
        setTabs((all) => {
          const view = all[tab];
          return view?.identity === identity && view.driver === driver && view.request === request
            ? { ...all, [tab]: { ...view, list } }
            : all;
        });
      },
      (error: unknown) => {
        if (active) setInventoryError({ key: listKey, message: message(error) });
      },
    );
    return () => {
      active = false;
      read.cancel();
    };
  }, [listKey]);

  const detailKey = visible && revision ? readKey(tab, revision, section, computed, historyPage, request?.refresh ?? 0) : "";
  useLayoutEffect(() => {
    setDetail(undefined);
    setLink(undefined);
    if (!detailKey || !revision) return;
    const read =
      section === "History"
        ? readHistory({ record: revision, page: historyPage })
        : readSection({ record: revision, section, computedValues: computed });
    let active = true;
    void read.promise.then(
      (out) => {
        if (!active) return;
        if ("section" in out) {
          if (out.section === section && matchesRecord(out.record, revision)) setDetail({ key: detailKey, section: out });
        } else if (out.page === historyPage) {
          setDetail({ key: detailKey, history: out });
        }
      },
      (error: unknown) => {
        if (active) setDetail({ key: detailKey, error: message(error) });
      },
    );
    return () => {
      active = false;
      read.cancel();
      linkOwner.cancel();
    };
  }, [detailKey, linkOwner]);

  function update(change: Partial<View>) {
    setTabs((all) => {
      const view = all[tab];
      return view?.identity === identity ? { ...all, [tab]: { ...view, ...change } } : all;
    });
  }

  function reload(name = current?.name ?? "", nextDriver = driver) {
    if (!current || !scope) return;
    if (nextDriver !== driver) closeScope(current.scope);
    update({
      scope: { ...scope, driver: nextDriver, releaseName: name },
      name,
      driver: nextDriver,
      list: undefined,
      revision: undefined,
      section: "Overview",
      request: { snapshotID: "", page: 0, refresh: current.request.refresh + 1 },
    });
  }

  return {
    driver,
    filter: current?.filter ?? "",
    name: current?.name ?? "",
    list: current?.list,
    listError: inventoryError?.key === listKey ? inventoryError?.message : undefined,
    loadingList: !!listKey && inventoryError?.key !== listKey,
    revision,
    section,
    computed,
    historyPage,
    detail: detail?.key === detailKey ? detail : undefined,
    link: link?.key === detailKey ? link : undefined,
    setDriver: (next: Driver) => reload(current?.name ?? "", next),
    setFilter: (filter: string) => update({ filter }),
    applyFilter: () => reload(current?.filter.trim() ?? ""),
    refresh: () => reload(),
    changePage: (page: number) =>
      current?.list &&
      update({
        list: undefined,
        request: { ...current.request, snapshotID: current.list.snapshotID, page },
      }),
    choose: (record: RecordRef) => update({ revision: record, section: "Overview", computed: false, historyPage: 0 }),
    setSection: (section: Section) => update({ section, historyPage: 0 }),
    setComputed: (computed: boolean) => update({ computed }),
    setHistoryPage: (historyPage: number) => update({ historyPage }),
    openResource: (resource: ManifestResource, onOpen: (value: CurrentResource) => void) => {
      if (!revision || !detailKey) return;
      setLink({ key: detailKey, pending: true });
      linkOwner.start(
        openCurrent({ record: revision, resource }),
        (value) => {
          setLink(undefined);
          onOpen(value);
        },
        (error) => setLink({ key: detailKey, pending: false, error: message(error) }),
      );
    },
    closeTab: (key: string) => {
      if (tabs[key]) closeScope(tabs[key].scope);
      setTabs((all) => {
        const next = { ...all };
        delete next[key];
        return next;
      });
    },
  };
}

export type HelmController = ReturnType<typeof useHelm>;
