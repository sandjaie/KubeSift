/*
Purpose: Verify truthful and inert workload evidence markup.
Used by: Vitest.
Responsibilities: Cover sparse Pod observations and untrusted event messages.
Important: Synthetic captures require no cluster.
*/
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { ResourceOverview } from "./ResourceOverview";
import { ResourceTable } from "./ResourceTable";
import { ResourceEvents } from "./ResourceEvents";

test("partially observed regular containers do not imply complete readiness or restart totals", () => {
  const html = renderToStaticMarkup(
    <ResourceOverview
      data={{
        resource: { connection: { id: "c", generation: 1 }, kind: "Pod", apiVersion: "v1", namespace: "ns", name: "pod", uid: "p" },
        job: null,
        resourceVersion: "1",
        fetchedAt: "2026-11-13T00:00:00Z",
        objectJSON: JSON.stringify({
          metadata: { deletionTimestamp: "2026-11-13T00:00:00Z" },
          spec: { containers: [{ name: "one" }, { name: "two" }] },
          status: { phase: "Running", containerStatuses: [{ name: "one", ready: true, restartCount: 2 }] },
        }),
      }}
      onOpenOwner={() => {}}
    />,
  );
  expect(html).toContain("Terminating");
  expect(html).toContain("Running");
  expect(html).not.toContain("1 / 2");
  expect(html).toContain('<dt>Restarts</dt><dd><pre class="fact-text">Unknown');
});

test("event record counts, occurrence counts and messages remain separate", () => {
  const html = renderToStaticMarkup(
    <ResourceEvents
      data={{
        items: [
          {
            uid: "e",
            type: "Warning",
            reason: "Failed",
            message: "<script>payload</script>",
            count: 0,
            reporter: "",
            firstAt: null,
            lastAt: null,
          },
        ],
        page: { nextCursor: "next", resourceVersion: "1", fetchedAt: "invalid" },
      }}
      onNext={() => {}}
    />,
  );
  expect(html).toContain("Warnings (1)");
  expect(html).toContain("<td>0</td><td>Unknown</td>");
  expect(html).toContain("&lt;script&gt;payload&lt;/script&gt;");
  expect(html).not.toContain("End of results");
});

test("remaining workload tables label Pod, node and schedule counts correctly", () => {
  const cases = [
    ["StatefulSet", "Total Pods", "Current revision Pods"],
    ["DaemonSet", "Ready / desired nodes", "Misscheduled nodes"],
    ["Job", "Failed attempts", "No terminal condition reported"],
    ["CronJob", "Active Job references", "Controller default (unknown)"],
  ] as const;
  for (const [kind, column, fact] of cases) {
    const resource = {
      connection: { id: "c", generation: 1 },
      kind,
      apiVersion: kind === "Job" || kind === "CronJob" ? "batch/v1" : "apps/v1",
      uid: "id",
      name: "test",
      namespace: "test",
    };
    const job = { status: "No terminal condition reported", active: 0, succeeded: 0, failed: 2, completions: null };
    const row = {
      resource,
      job,
      cronJob: null,
      total: 3,
      desired: 0,
      current: 0,
      available: 0,
      updated: 0,
      ready: 0,
      phase: "",
      reason: "",
      node: "",
      restarts: null,
      createdAt: null,
    };
    expect(renderToStaticMarkup(<ResourceTable kind={kind} rows={[row]} selected={null} onSelect={() => {}} />)).toContain(column);
    const html = renderToStaticMarkup(
      <ResourceOverview
        data={{
          resource,
          job,
          resourceVersion: "1",
          fetchedAt: "",
          objectJSON: JSON.stringify({
            spec: { suspend: true },
            status: { observedGeneration: 1, replicas: 3, currentReplicas: 1, active: [{ name: "child" }] },
          }),
        }}
        onOpenOwner={() => {}}
      />,
    );
    expect(html).toContain(fact);
    if (kind === "CronJob") expect(html).toContain("Existing Jobs can continue");
  }
});
