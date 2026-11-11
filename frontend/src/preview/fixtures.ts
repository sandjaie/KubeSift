/*
Purpose: Supply explicit synthetic UI examples.
Used by: Preview entry and workspace state tests.
Responsibilities: Keep Pod status, events, logs and YAML consistent.
Important: Never import live adapters or substitute fixtures for live failures.
*/
import type { ResourceView, TabView } from "../workspace/model";
export const capturedAt = "2026-11-06T10:42:03Z";
export function createPreview(): { tabs: TabView[]; resources: ResourceView[] } {
  const tabs: TabView[] = [
    ["a", "dev-local", "payments"],
    ["b", "staging-local", "checkout"],
  ].map(([id, label, namespace]) => ({
    id: `preview:${id}`,
    label,
    environment: id === "a" ? "Local" : "Staging",
    scope: { kind: "namespace", name: namespace },
    resourceKind: "Pod",
    filter: "",
    selected: null,
    detail: "overview",
    scroll: { list: 0, detail: 0 },
    status: "ready",
  }));
  const resources: ResourceView[] = tabs.flatMap((tab, index) => {
    const namespace = index === 0 ? "payments" : "checkout";
    return [true, false].map((failing) => {
      const name = failing ? "checkout-api-7d9b8c-x2k4p" : "payments-worker-5f6d7c-h8n2v";
      const uid = `preview:${index}:${failing ? "api" : "worker"}`;
      return {
        key: { clusterId: tab.id, apiVersion: "v1", kind: "Pod", namespace, name, uid },
        status: failing ? "CrashLoopBackOff" : "Running",
        ready: failing ? "0/1" : "1/1",
        containers: ["app"],
        summary: {
          Container: "app",
          "Exit code": failing ? "1" : "—",
          Restarts: failing ? "6" : "0",
          "Explicit environment": failing ? "DATABASE_HOST, DATABASE_PORT" : "DATABASE_URL",
          Configuration: failing ? "DATABASE_URL missing; envFrom not inspected" : "Synthetic healthy configuration",
          "Captured at": capturedAt,
        },
        yaml: `apiVersion: v1\nkind: Pod\nmetadata:\n  name: ${name}\n  namespace: ${namespace}\n  uid: ${uid}\n  annotations:\n    example: '<script>alert("inert fixture")</script>'\nspec:\n  containers:\n    - name: app\n      image: example.invalid/checkout:preview\n      env:\n        - name: DATABASE_HOST\n          value: database\n        - name: DATABASE_PORT\n          value: "5432"\n${failing ? "" : "        - name: DATABASE_URL\n          value: synthetic-only\n"}`,
        events: [
          {
            id: `${uid}:event`,
            at: capturedAt,
            reason: failing ? "BackOff" : "Started",
            message: failing ? "Back-off restarting failed container app; last exit code 1." : "Started container app.",
          },
        ],
        logs: [
          {
            id: `${uid}:start`,
            at: "2026-11-06T10:42:01Z",
            severity: "info" as const,
            text: "Starting checkout-api",
            container: "app",
            previous: false,
          },
          {
            id: `${uid}:log`,
            at: "2026-11-06T10:42:02Z",
            severity: failing ? ("error" as const) : ("info" as const),
            text: failing ? "Required setting DATABASE_URL is missing" : "Worker ready",
            container: "app",
            previous: false,
          },
          {
            id: `${uid}:previous`,
            at: "2026-11-06T10:41:00Z",
            severity: "info" as const,
            text: "Previous synthetic attempt ended; no live stream.",
            container: "app",
            previous: true,
          },
        ],
      };
    });
  });
  resources.push({
    key: {
      clusterId: "preview:a",
      apiVersion: "example.io/v1",
      kind: "Widget",
      namespace: "payments",
      name: "checkout-policy",
      uid: "preview:widget",
    },
    status: "Ready",
    ready: "—",
    containers: [],
    summary: { "Captured at": capturedAt, Source: "Synthetic custom resource" },
    yaml: "apiVersion: example.io/v1\nkind: Widget\nmetadata:\n  name: checkout-policy\n  namespace: payments\nspec:\n  enabled: true",
    events: [],
    logs: [],
  });
  return { tabs, resources };
}
