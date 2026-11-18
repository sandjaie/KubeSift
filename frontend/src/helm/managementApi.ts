/*
Purpose: Adapt native Helm management to cancellable frontend preparation.
Used by: Helm source settings and the action dialog.
Responsibilities: Forward explicit inputs and read native values handles without retaining payloads.
Important: Execution accepts only a native review token; credentials are never read back.
*/
import { ExplorerService } from "../../bindings/internal/desktop";
import type { ChartRequest, PrepareRequest } from "../../bindings/internal/helmmanager/models";
import { readRequest } from "../workloads/api";

export const helmManagementAPI = ExplorerService;
export const resolveHelmChart = (request: ChartRequest) => readRequest((id) => ExplorerService.ResolveHelmChart(id, request));
export const prepareHelmAction = (request: PrepareRequest) => readRequest((id) => ExplorerService.PrepareHelmAction(id, request));
export const listHelmCharts = (source: string, filter: string, cursor = "") =>
  readRequest((id) => ExplorerService.ListHelmCharts(id, source, filter, cursor));
export const readHelmReview = (token: string, section: string, offset: number) =>
  readRequest((id) => ExplorerService.ReadHelmReview(id, token, section, offset));

export async function importHelmValues(): Promise<string> {
  const handle = await ExplorerService.PickHelmValues();
  try {
    let text = "";
    let offset = 0;
    for (;;) {
      const part = await ExplorerService.ReadHelmValues(handle, offset);
      text += part.text;
      if (part.done) return text;
      offset = part.nextOffset;
    }
  } finally {
    await ExplorerService.DiscardHelmDraft(handle);
  }
}

export function prepareHelmReview(request: PrepareRequest) {
  const active = prepareHelmAction(request);
  let cancelRead = active.cancel;
  let cancelled = false;
  let token = "";
  const promise = (async () => {
    try {
      const review = await active.promise;
      token = review.token;
      const content: Record<string, string> = {};
      for (const section of review.sections ?? []) {
        let offset = 0;
        content[section] = "";
        for (;;) {
          if (cancelled) throw new Error("Helm review cancelled.");
          const read = readHelmReview(token, section, offset);
          cancelRead = read.cancel;
          const part = await read.promise;
          content[section] += part.text;
          if (part.done) break;
          offset = part.nextOffset;
        }
      }
      if (cancelled) throw new Error("Helm review cancelled.");
      return { ...review, content };
    } catch (error) {
      if (token) void ExplorerService.DiscardHelmDraft(token).catch(() => {});
      throw error;
    }
  })();
  return {
    promise,
    cancel: () => {
      cancelled = true;
      cancelRead();
      if (token) void ExplorerService.DiscardHelmDraft(token).catch(() => {});
    },
  };
}

export function loadUpgradeValues(target: import("../../bindings/internal/helmmanager/models").Target) {
  const list = readRequest((id) => ExplorerService.ListHelmReleases(id, { scope: target, snapshotID: "", page: 0 }));
  let cancelRead = list.cancel;
  let cancelled = false;
  return {
    promise: (async () => {
      const page = await list.promise;
      if (cancelled) throw new Error("Values read cancelled.");
      const current = page.items?.find((item) => item.record.releaseName === target.releaseName);
      if (!current || current.problem) throw new Error(current?.problem?.message ?? "Current release is unavailable.");
      const read = readRequest((id) =>
        ExplorerService.ReadHelmSection(id, { record: current.record, section: "Values", computedValues: false }),
      );
      cancelRead = read.cancel;
      const result = await read.promise;
      return result.text || "{}\n";
    })(),
    cancel: () => {
      cancelled = true;
      cancelRead();
    },
  };
}
