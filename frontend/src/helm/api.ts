/*
Purpose: Bind Helm inspection to cancellable native reads.
Used by: The Helm workspace state and resource links.
Responsibilities: Pass frozen scope and revision identities to the desktop service.
Important: Each request receives a separate native cancellation ID.
*/
import { readRequest, workloadAPI } from "../workloads/api";
import type { DetailRequest, HistoryRequest, ListRequest, OpenResourceRequest } from "../../bindings/internal/helmreader/models";

export const listHelm = (req: ListRequest) => readRequest((id) => workloadAPI.ListHelmReleases(id, req));
export const readHistory = (req: HistoryRequest) => readRequest((id) => workloadAPI.ReadHelmHistory(id, req));
export const readSection = (req: DetailRequest) => readRequest((id) => workloadAPI.ReadHelmSection(id, req));
export const openCurrent = (req: OpenResourceRequest) => readRequest((id) => workloadAPI.OpenHelmResource(id, req));
