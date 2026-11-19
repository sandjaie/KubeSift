/*
Purpose: Adapt current metrics reads and native visibility to the workspace.
Used by: The shared visible-page polling controller.
Responsibilities: Preserve explicit targets and expose cancellation.
Important: No provider provisioning or alternative credentials are used.
*/
import { Events } from "@wailsio/runtime";
import { readRequest, workloadAPI } from "../workloads/api";
import type { MetricsRequest } from "../../bindings/internal/kube/models";
export const readMetrics = (request: MetricsRequest) => readRequest((id) => workloadAPI.ReadCurrentMetrics(id, request));
// One application-lifetime listener preserves visibility while workspace panes are unmounted.
let visible = true;
let listening = false;
const listeners = new Set<(visible: boolean) => void>();
export function onVisibility(listener: (visible: boolean) => void) {
  if (!listening) {
    Events.On("metrics-visibility", (event) => {
      visible = event.data;
      for (const notify of listeners) notify(visible);
    });
    listening = true;
  }
  listeners.add(listener);
  listener(visible);
  return () => {
    listeners.delete(listener);
  };
}
