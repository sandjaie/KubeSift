/*
Purpose: Verify truthful and inert workload evidence markup.
Used by: Vitest.
Responsibilities: Cover sparse Pod observations and untrusted event messages.
Important: Synthetic captures require no cluster.
*/
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { ResourceOverview } from "./ResourceOverview";
import { ResourceEvents } from "./ResourceEvents";

test("partially observed regular containers do not imply complete readiness or restart totals", () => {
  const html = renderToStaticMarkup(
    <ResourceOverview
      data={{
        resource: { connection: { id: "c", generation: 1 }, kind: "Pod", apiVersion: "v1", namespace: "ns", name: "pod", uid: "p" },
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
