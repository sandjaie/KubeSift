/*
Purpose: Bind scaling UI calls to the typed native service.
Used by: The scale dialog and session results.
Responsibilities: Supply preparation cancellation without owning execution.
Important: Confirm passes only a token and acknowledgement.
*/
import { readRequest, workloadAPI } from "../workloads/api";
import type { ScaleRequest } from "../../bindings/github.com/sandjaie/KubeSift/internal/operation/models";
export type { ScaleRequest, ScalePreview, ScaleResult } from "../../bindings/github.com/sandjaie/KubeSift/internal/operation/models";
export const prepareScale = (request: ScaleRequest) => readRequest((id) => workloadAPI.PrepareScale(id, request));
export const executeScale = workloadAPI.ExecuteScale;
export const listScaleResults = workloadAPI.ListScaleResults;
export const revokeScale = (token: string) => {
  if (token) void workloadAPI.RevokeScale(token).catch(() => {});
};
