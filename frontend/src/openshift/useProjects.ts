/*
Purpose: Load the active connection's OpenShift capability and visible Project suggestions.
Used by: Workspace navigation and the identity bar.
Responsibilities: Bound inventory and discard old generations and refreshes.
Important: Suggestions come exclusively from the current principal's Projects response.
*/
import { useEffect, useState } from "react";
import { api, message, type ConnectionRef } from "../connections/api";
import { readRequest } from "../workloads/api";
import type { Project, ProjectCapabilities } from "../../bindings/internal/kube/models";

interface Inventory {
  key: string;
  caps?: ProjectCapabilities;
  items: Project[];
  partial: boolean;
  error: string;
}
export function useProjects(ref?: ConnectionRef | null) {
  const id = ref?.id,
    generation = ref?.generation,
    key = JSON.stringify([id, generation]);
  const [revision, refresh] = useState(0);
  const [state, set] = useState<Inventory>();
  useEffect(() => {
    if (!id || generation === undefined) return;
    const connection = { id, generation };
    let current = true;
    const base = { key, items: [], partial: true, error: "" };
    set(base);
    let cancel = () => {};
    async function run() {
      const probe = readRequest((i) => api.ProjectCapabilities(i, connection, revision > 0));
      cancel = probe.cancel;
      const caps = await probe.promise;
      if (!current) return;
      set({ ...base, caps });
      if (!caps.list) return;
      let cursor = "";
      let items: Project[] = [];
      const seen = new Set<string>();
      do {
        seen.add(cursor);
        const req = readRequest((i) => api.ListProjects(i, connection, cursor));
        cancel = req.cancel;
        const page = await req.promise;
        if (!current) return;
        const rows = page.items ?? [];
        const clipped = items.length + rows.length > 500;
        items = items.concat(rows.slice(0, 500 - items.length));
        cursor = page.next;
        set({ key, caps, items, partial: clipped || page.partial, error: "" });
        if (seen.has(cursor) || items.length >= 500 || !rows.length) break;
      } while (cursor);
    }
    void run().catch((e) => {
      if (current) set((s) => ({ ...(s ?? base), key, partial: true, error: message(e) }));
    });
    return () => {
      current = false;
      cancel();
    };
  }, [id, generation, key, revision]);
  return { inventory: state?.key === key ? state : undefined, refresh: () => refresh((n) => n + 1) };
}
