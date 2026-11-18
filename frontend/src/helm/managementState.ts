/*
Purpose: Keep Helm action labels and confirmation eligibility explicit.
Used by: The Helm workspace and action dialog.
Responsibilities: Share the four supported actions and expire reviews without executing them.
Important: Native tokens and target revalidation remain authoritative.
*/
import type { Review } from "../../bindings/internal/helmmanager/models";
export type HelmAction = "install" | "upgrade" | "rollback" | "uninstall";
export const helmActionLabels: Record<HelmAction, string> = {
  install: "Install",
  upgrade: "Upgrade",
  rollback: "Roll back",
  uninstall: "Uninstall",
};
export function canConfirmHelm(review: Review | undefined, valid: boolean, submitting: boolean) {
  return !!review && valid && !submitting && Date.parse(review.expiresAt) > Date.now();
}
