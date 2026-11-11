/*
Purpose: Provide the product's narrow typed desktop API seam.
Used by: Connection hooks and Settings.
Responsibilities: Re-export generated contracts and normalize safe bridge errors.
Important: No kubeconfig bodies or credentials cross this adapter.
*/
export { ExplorerService as api } from "../../bindings/github.com/sandjaie/KubeSift/internal/desktop";
export type { CatalogEntry, SourceResult } from "../../bindings/github.com/sandjaie/KubeSift/internal/sources/models";
export type { ConnectionView, ConnectRequest } from "../../bindings/github.com/sandjaie/KubeSift/internal/explorer/models";
export type { ConnectionRef, Problem } from "../../bindings/github.com/sandjaie/KubeSift/internal/model/models";
export type { PreviewRecord, Entry as TrustedHelper } from "../../bindings/github.com/sandjaie/KubeSift/internal/trust/models";
export function message(error: unknown): string {
  return error instanceof Error
    ? error.message
    : typeof error === "string"
      ? error
      : "The desktop request failed. Retry or check Settings.";
}
