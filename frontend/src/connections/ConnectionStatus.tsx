/*
Purpose: Render textual connection state backed by recent native evidence.
Used by: Cluster list and workspace header.
Responsibilities: Keep stale or unknown access amber and failures descriptive.
Important: Resource health is never inferred from a connection dot.
*/
import type { ConnectionView } from "./api";
import { visibleState } from "./state";
const labels: Record<string, string> = {
  not_connected: "Not connected",
  connecting: "Connecting",
  connected: "Connected",
  restricted: "Access restricted",
  review_required: "Review required",
  auth_required: "Authentication required",
  external_login_required: "External login required",
  helper_missing: "Helper missing",
  unreachable: "Unreachable",
  certificate: "Certificate error",
  timeout: "Timed out",
  stale: "Stale / checking",
  failed: "Connection failed",
};
export function ConnectionStatus({
  view,
  now = Date.now(),
  unknown = false,
}: {
  view: ConnectionView | null;
  now?: number;
  unknown?: boolean;
}) {
  const state = unknown ? "stale" : visibleState(view, now);
  const tone =
    state === "connected"
      ? "good"
      : state === "not_connected"
        ? "neutral"
        : ["connecting", "restricted", "review_required", "stale"].includes(state)
          ? "warning"
          : "danger";
  return (
    <span className={`connection-status ${tone}`} title={view?.lastSuccessAt ? `Last successful read: ${view.lastSuccessAt}` : undefined}>
      <span aria-hidden="true" className="connection-dot" />
      {labels[state] ?? "Connection failed"}
    </span>
  );
}
