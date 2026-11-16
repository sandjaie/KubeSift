/*
Purpose: Bind reviewed workload UI calls to the typed native service.
Used by: The operation dialogs and session results.
Responsibilities: Supply preparation cancellation without owning execution.
Important: Confirm passes only a token and acknowledgement.
*/
import { readRequest, workloadAPI } from "../workloads/api";
import type {
  DeleteRequest,
  EditRequest,
  RestartRequest,
  ScaleRequest,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/operation/models";
import type { EditTarget } from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
export type { EditTarget };
export type {
  EditDraft,
  EditPreview,
  DeleteObservation,
  DeletePreview,
  OperationResult,
  RestartRequest,
  RestartPreview,
  ScaleRequest,
  ScalePreview,
  ScaleResult,
} from "../../bindings/github.com/sandjaie/KubeSift/internal/operation/models";
export type OperationAction = "scale" | "restart" | "delete" | "edit";
export const prepareScale = (request: ScaleRequest) => readRequest((id) => workloadAPI.PrepareScale(id, request));
export const executeScale = workloadAPI.ExecuteScale;
export const listScaleResults = workloadAPI.ListScaleResults;
export const revokeScale = (token: string) => {
  if (token) void workloadAPI.RevokeScale(token).catch(() => {});
};

export const prepareRestart = (request: RestartRequest) => readRequest((id) => workloadAPI.PrepareRestart(id, request));
export const executeRestart = workloadAPI.ExecuteRestart;
export const revokeRestart = (token: string) => {
  if (token) void workloadAPI.RevokeRestart(token).catch(() => {});
};
export const listOperationResults = workloadAPI.ListOperationResults;
export const getOperationResult = workloadAPI.GetOperationResult;

export const prepareDelete = (request: DeleteRequest) => readRequest((id) => workloadAPI.PrepareDelete(id, request));
export const executeDelete = workloadAPI.ExecuteDelete;
export const revokeDelete = (token: string) => {
  if (token) void workloadAPI.RevokeDelete(token).catch(() => {});
};
export const checkDeleteTarget = (operationID: string) => readRequest((id) => workloadAPI.CheckDeleteTarget(id, operationID));

export const openEdit = (target: EditTarget) => readRequest((id) => workloadAPI.OpenEdit(id, target));
export const prepareEdit = (request: EditRequest) => readRequest((id) => workloadAPI.PrepareEdit(id, request));
export const executeEdit = workloadAPI.ExecuteEdit;
export const revokeEdit = (token: string) => {
  if (token) void workloadAPI.RevokeEdit(token).catch(() => {});
};
export const closeEdit = (id: string) => {
  if (id) void workloadAPI.CloseEdit(id).catch(() => {});
};
