/*
Purpose: Bind reviewed workload UI calls to the typed native service.
Used by: The operation dialogs and session results.
Responsibilities: Supply preparation cancellation without owning execution.
Important: Confirm forwards only the token and any acknowledgement required by that action.
*/
import { readRequest, workloadAPI } from "../workloads/api";
import type {
  CreateRequest,
  DeleteRequest,
  EditRequest,
  RollbackRequest,
  RestartRequest,
  ScaleRequest,
} from "../../bindings/internal/operation/models";
import type { ConnectionRef } from "../bridge";
import type { EditTarget, ManifestAddress } from "../../bindings/internal/kube/models";
export type { EditTarget, ManifestAddress } from "../../bindings/internal/kube/models";
export type {
  CreateDraft,
  CreateObservation,
  CreatePreview,
  EditDraft,
  EditPreview,
  DeleteObservation,
  DeletePreview,
  OperationResult,
  RollbackPreview,
  RollbackRequest,
  RestartRequest,
  RestartPreview,
  ScaleRequest,
  ScalePreview,
  ScaleResult,
} from "../../bindings/internal/operation/models";
export type CreatedResourceOpening = { target: ManifestAddress; operationID: string };

export type OperationAction = "scale" | "restart" | "delete" | "edit" | "rollback" | "create";
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

export const prepareRollback = (request: RollbackRequest) => readRequest((id) => workloadAPI.PrepareRollback(id, request));
export const executeRollback = workloadAPI.ExecuteRollback;
export const revokeRollback = (token: string) => {
  if (token) void workloadAPI.RevokeRollback(token).catch(() => {});
};

export const openCreate = (connection: ConnectionRef, namespace: string) =>
  readRequest((id) => workloadAPI.OpenCreate(id, connection, namespace));
export const prepareCreate = (request: CreateRequest) => readRequest((id) => workloadAPI.PrepareCreate(id, request));
export const executeCreate = workloadAPI.ExecuteCreate;
export const revokeCreate = (token: string) => {
  if (token) void workloadAPI.RevokeCreate(token).catch(() => {});
};
export const closeCreate = (id: string) => {
  if (id) void workloadAPI.CloseCreate(id).catch(() => {});
};
export const inspectCreateName = (operationID: string) => readRequest((id) => workloadAPI.InspectCreateName(id, operationID));
