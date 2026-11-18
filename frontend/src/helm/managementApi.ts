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
