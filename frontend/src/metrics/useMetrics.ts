/*
Purpose: Share current metrics between one visible page and its selected detail.
Used by: Workspace, independently of resource inventory and connection health.
Responsibilities: Bound sequential batches, cancel obsolete reads and age latest-only evidence.
Important: Timers start after completion; hidden work never queues a polling backlog.
*/
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ConnectionRef } from "../bridge";
import type { ResourceRef, ResourceUsage } from "../../bindings/internal/kube/models";
import { onVisibility, readMetrics } from "./api";
export type SampleView = { sample: ResourceUsage; last?: string; now: number };
export const metricsIdentity = (r: ResourceRef) =>
  JSON.stringify([r.connection.id, r.connection.generation, r.apiVersion, r.kind, r.namespace, r.name, r.uid]);
export function useMetrics({
  connection,
  targets,
  detail,
  visible,
  refreshKey = "",
}: {
  connection: ConnectionRef | null;
  targets: ResourceRef[];
  detail: ResourceRef | null;
  visible: boolean;
  refreshKey?: string;
}) {
  const [nativeVisible, setNativeVisible] = useState(true);
  const [documentVisible, setDocumentVisible] = useState(document.visibilityState !== "hidden");
  const [refreshCount, setRefreshCount] = useState(0);
  const refreshToken = `${refreshKey}|${refreshCount}`;
  const previousRefresh = useRef(refreshToken);
  const [now, setNow] = useState(Date.now());
  const [loading, setLoading] = useState(false);
  const [stored, setStored] = useState<{ scope: string; values: Record<string, Omit<SampleView, "now">> }>({ scope: "", values: {} });
  const unique = [...new Map([...(detail ? [detail] : []), ...targets].map((r) => [metricsIdentity(r), r])).values()];
  const scope = JSON.stringify([connection, unique.map(metricsIdentity), detail && metricsIdentity(detail)]);
  const active = visible && nativeVisible && documentVisible && !!connection && unique.length > 0;
  useEffect(() => {
    const change = () => setDocumentVisible(document.visibilityState !== "hidden");
    document.addEventListener("visibilitychange", change);
    const off = onVisibility((v) => setNativeVisible(v));
    return () => {
      document.removeEventListener("visibilitychange", change);
      off();
    };
  }, []);
  useEffect(() => {
    if (!visible || !documentVisible) return;
    const age = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(age);
  }, [visible, documentVisible]);
  useLayoutEffect(() => {
    if (!active || !connection) {
      setLoading(false);
      if (!visible || !connection) setStored({ scope: "", values: {} });
      return;
    }
    let alive = true,
      timer: ReturnType<typeof setTimeout> | undefined,
      cancel: (() => void) | undefined;
    const keep = new Set(unique.map(metricsIdentity));
    setStored((old) => ({ scope, values: Object.fromEntries(Object.entries(old.values).filter(([key]) => keep.has(key))) }));
    const store = (sample: ResourceUsage) => {
      if (!alive) return;
      const key = metricsIdentity(sample.resource);
      if (!keep.has(key)) return;
      setStored((old) => {
        const prior = old.values[key];
        const clear = ["forbidden", "auth_required", "resource_replaced", "stale_target"].includes(sample.problem?.code ?? "");
        const retained = sample.state === "unavailable" && !clear && prior?.sample.sampledAt;
        return {
          scope,
          values: {
            ...old.values,
            [key]: retained ? { sample: prior.sample, last: sample.problem?.message ?? "Refresh failed" } : { sample },
          },
        };
      });
    };
    async function poll(manual: boolean) {
      setLoading(true);
      let sharedFailure = false;
      try {
        for (let offset = 0; offset < unique.length && alive; offset += 100) {
          const batch = unique.slice(offset, offset + 100);
          const req = readMetrics({
            connection: connection!,
            targets: batch,
            detail: detail && batch.some((r) => metricsIdentity(r) === metricsIdentity(detail)) ? detail : null,
            refreshDiscovery: manual && offset === 0,
          });
          cancel = req.cancel;
          const response = await req.promise;
          if (!alive) return;
          for (const sample of response.items ?? []) store(sample);
          if (response.problem) {
            const unavailable = response.items?.find((sample) => sample.state === "unavailable");
            if (unavailable) for (const resource of unique.slice(offset + 100)) store({ ...unavailable, resource });
            sharedFailure = true;
            break;
          }
        }
      } catch {
        if (alive) {
          sharedFailure = true;
          setStored((old) => ({
            scope,
            values: Object.fromEntries(Object.entries(old.values).map(([key, v]) => [key, { ...v, last: "Metrics refresh failed" }])),
          }));
        }
      } finally {
        if (alive) {
          setLoading(false);
          setNow(Date.now());
          timer = setTimeout(() => void poll(false), sharedFailure ? 60000 : 15000);
        }
      }
    }
    const manual = previousRefresh.current !== refreshToken;
    previousRefresh.current = refreshToken;
    void poll(manual);
    return () => {
      alive = false;
      cancel?.();
      clearTimeout(timer);
    };
    // Captured scope owns each sequential polling cycle; object identity must not restart it.
  }, [scope, active, refreshCount, refreshKey]);
  const byIdentity: Record<string, SampleView> = {};
  if (stored.scope === scope)
    for (const [key, v] of Object.entries(stored.values)) byIdentity[key] = { ...v, now: Math.max(now, Date.now()) };
  return { byIdentity, loading, refresh: () => setRefreshCount((n) => n + 1) };
}
